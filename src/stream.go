package main

import (
	"bufio"
	"io"
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

			isFallback := (srcUrl == FallbackURL)
			args := []string{"-hide_banner", "-loglevel", "error", "-user_agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"}

			if isFallback {
				args = append(args, "-stream_loop", "-1", "-re")
			} else {
				args = append(args, "-reconnect", "1", "-reconnect_at_eof", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-rw_timeout", "10000000")
			}

			args = append(args, "-err_detect", "ignore_err")
			args = append(args, "-fflags", "+genpts+igndts+flush_packets")
			args = append(args, "-avoid_negative_ts", "make_zero")
			args = append(args, "-probesize", "1000000", "-analyzeduration", "1000000")
			args = append(args, "-i", srcUrl)

			if isFallback {
			        args = append(args, "-c", "copy", "-map", "0:v?", "-map", "0:a?")
			} else {
			        args = append(args, "-c", "copy", "-map", "0?", "-ignore_unknown")
			}

			args = append(args, "-f", "mpegts", "-mpegts_flags", "resend_headers+initial_discontinuity", "-copyts", "pipe:1")
			cmd := exec.Command("ffmpeg", args...)
			stdout, err := cmd.StdoutPipe()
			if err != nil { continue }
			stderr, _ := cmd.StderrPipe()

			if err := cmd.Start(); err != nil {
				log.Printf("FFmpeg failed to start for %s: %v", s.Key, err)
				continue
			}

			// Background thread to log FFmpeg errors and detect stream freezing
			go func(c *exec.Cmd, streamKey string) {
				scanner := bufio.NewScanner(stderr)
				for scanner.Scan() {
					log.Printf("FFmpeg Error [%s]: %s", streamKey, scanner.Text())
				}
			}(cmd, s.Key)

			s.Mu.Lock()
			s.Proc = cmd
			s.LastDataTime = time.Now()
			s.CurrentBytesRead = 0
			s.RecentChunks = nil 
			s.Mu.Unlock()

			go func(c *exec.Cmd, stream *Stream, out io.ReadCloser) {
			        lastBytes := int64(0)
			        lastCheck := time.Now()
			        lowDataTicks := 0

			        for {
			                stream.Mu.RLock()
			                active := stream.Active
			                last := stream.LastDataTime
			                proc := stream.Proc
			                currentBytes := stream.CurrentBytesRead
			                stream.Mu.RUnlock()

			                if !active || proc != c { break }

			                now := time.Now()
			                diffBytes := currentBytes - lastBytes
			                lastBytes = currentBytes

			                // Bitrate check: if less than 30KB/s
			                if diffBytes < 30000 {
			                        lowDataTicks++
			                } else {
			                        lowDataTicks = 0
			                }

			                // 12 seconds of no data OR 12 seconds of very low data (<30KB/s)
			                if time.Since(last) > DataTimeout || lowDataTicks > 6 {
			                        reason := "frozen"
			                        if lowDataTicks > 6 { reason = "low bitrate" }

			                        log.Printf("DataTimeout [%s]: FFmpeg %s (last data %v ago, ticks %d), forcing exit...", stream.Key, reason, time.Since(last), lowDataTicks)
			                        if out != nil {
			                        out.Close()
			                        }
			                        if c.Process != nil {
			                        c.Process.Kill()
			                        }
			                        break
			                        }
			                lastCheck = now
			                _ = lastCheck
			                time.Sleep(2 * time.Second)
			        }
			}(cmd, s, stdout)
			buf := make([]byte, FfmpegBuffer)
			log.Printf("Entering read loop for %s source #%d", s.Key, idx)
			for {
			        n, err := stdout.Read(buf)
			        if n > 0 {
			                chunk := make([]byte, n)
			                copy(chunk, buf[:n])
			                s.Mu.Lock()
			                s.LastDataTime = time.Now()
			                s.CurrentBytesRead += int64(n)

			                // Keep approx 2MB of recent data for instant start (32 chunks * 64KB)
			                s.RecentChunks = append(s.RecentChunks, chunk)
			                if len(s.RecentChunks) > 32 {
			                        s.RecentChunks = s.RecentChunks[1:]
			                }

			                for ch := range s.Clients {
			                        select {
			                        case ch <- chunk:
			                        default:
			                        }
			                }
			                s.Mu.Unlock()
			        }
			        if err != nil { 
			                log.Printf("Read error for %s: %v", s.Key, err)
			                break 
			        }

			        s.Mu.RLock()
			        if !s.Active {
			                s.Mu.RUnlock()
			                break
			        }
			        s.Mu.RUnlock()
			}
			log.Printf("Exited read loop for %s", s.Key)

			if cmd.Process != nil {
			        cmd.Process.Kill()
			}
			cmd.Wait()
			stdout.Close()
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
