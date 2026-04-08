package main

import (
	"log"
	"math/rand"
	"os/exec"
	"sync"
	"time"
)

type Stream struct {
	Key           string
	Urls          []string
	Clients       map[chan []byte]bool
	RecentChunks  [][]byte
	Mu            sync.RWMutex
	Proc          *exec.Cmd
	LastDataTime  time.Time
	Created       time.Time
	OnFallback    bool
	CurrentUrlIdx       int
	Active              bool
	LastRetry           time.Time
	CurrentBytesRead    int64
	CurrentProcessStart time.Time
}

func shuffle(urls []string) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(urls), func(i, j int) {
		urls[i], urls[j] = urls[j], urls[i]
	})
}

func startProducer(s *Stream) {
	s.Mu.Lock()
	if s.Active {
		s.Mu.Unlock()
		return
	}
	s.Active = true
	urls := make([]string, len(s.Urls))
	copy(urls, s.Urls)
	s.Mu.Unlock()

	log.Printf("Starting stable producer: %s", s.Key)

	for {
		s.Mu.RLock()
		// Stream běží, dokud je Active (což vypíná cleanupStream s prodlevou)
		if !s.Active {
			s.Mu.RUnlock()
			break
		}
		s.Mu.RUnlock()

		for idx, srcUrl := range urls {
			s.Mu.Lock()
			s.CurrentUrlIdx = idx
			s.Mu.Unlock()

			args := []string{
				"-hide_banner", "-loglevel", "quiet",
				"-user_agent", "VLC/3.0.23 LibVLC/3.0.23",
				"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "2",
				"-fflags", "+nobuffer+igndts+discardcorrupt",
				"-probesize", "1000000", "-analyzeduration", "500000",
				"-i", srcUrl,
				"-c", "copy", "-map", "0?", "-ignore_unknown",
				"-f", "mpegts", 
				"-mpegts_flags", "resend_headers+initial_discontinuity",
				"-copyts", 
				"pipe:1",
			}

			cmd := exec.Command("ffmpeg", args...)
			stdout, err := cmd.StdoutPipe()
			if err != nil { continue }

			if err := cmd.Start(); err != nil {
				log.Printf("FFmpeg failed to start: %v", err)
				continue
			}

			s.Mu.Lock()
			s.Proc = cmd
			s.LastDataTime = time.Now()
			s.CurrentBytesRead = 0
			s.RecentChunks = nil 
			s.Mu.Unlock()

			buf := make([]byte, FfmpegBuffer)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])
					s.Mu.Lock()
					s.LastDataTime = time.Now()
					s.CurrentBytesRead += int64(n)
					for ch := range s.Clients {
						select {
						case ch <- chunk:
						default:
						}
					}
					s.Mu.Unlock()
				}
				if err != nil { break }
				
				s.Mu.RLock()
				if !s.Active {
					s.Mu.RUnlock()
					break
				}
				s.Mu.RUnlock()
			}
			
			cmd.Process.Kill()
			cmd.Wait()

			s.Mu.RLock()
			if !s.Active {
				s.Mu.RUnlock()
				break
			}
			s.Mu.RUnlock()
			time.Sleep(1 * time.Second)
		}
		
		s.Mu.RLock()
		if !s.Active {
			s.Mu.RUnlock()
			break
		}
		s.Mu.RUnlock()
		time.Sleep(2 * time.Second)
	}

	s.Mu.Lock()
	s.Proc = nil
	s.Active = false
	s.Mu.Unlock()
}

func cleanupStream(key string) {
	// DŮLEŽITÉ: Tady už nebudeme hned zabíjet. 
	// Tahle funkce se volá z proxy.go s CleanupDelay (10s).
	// My to zkusíme ještě víc "pojistit".
	
	streamsLock.Lock()
	defer streamsLock.Unlock()
	
	s, exists := streams[key]
	if !exists { return }

	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Pokud se mezitím někdo připojil, nic neděláme
	if len(s.Clients) > 0 {
		return
	}

	// Pokud už uplynul čas od posledních dat a nikdo tam není, vypneme to
	s.Active = false
	if s.Proc != nil && s.Proc.Process != nil {
		s.Proc.Process.Kill()
	}
	delete(streams, key)
	log.Printf("Stream ended and cleaned up: %s", key)
}
