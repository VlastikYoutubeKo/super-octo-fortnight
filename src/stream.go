package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Stream struct {
	Key                 string
	Urls                []string
	Clients             map[chan []byte]bool
	Mu                  sync.RWMutex
	CancelFunc          context.CancelFunc
	LastDataTime        time.Time
	Created             time.Time
	OnFallback          bool
	CurrentUrlIdx       int
	Active              bool
	LastRetry           time.Time
	CurrentBytesRead    int64
	CurrentProcessStart time.Time
	CurrentBitrate      float64
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

	log.Printf("Starting stable producer (Pure HTTP): %s", s.Key)

	for {
		s.Mu.RLock()
		if !s.Active {
			s.Mu.RUnlock()
			break
		}
		s.Mu.RUnlock()

		for idx, srcUrl := range urls {
			s.Mu.Lock()
			s.CurrentUrlIdx = idx
			s.Mu.Unlock()

			userAgents := []string{
				"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36",
				"Mozilla/5.0 (QtEmbedded; U; Linux; C) AppleWebKit/533.3 (KHTML, like Gecko) MAG200 stbapp ver: 2 rev: 234 Safari/533.3",
				"Enigma2 HbbTV/1.1.1 (+PVR+RTSP+DL;openATV;;;)",
				"TiviMate/4.7.0 (Linux; Android 11)",
				"IPTVSmartersPro",
				"VLC/3.0.18 LibVLC/3.0.18",
				"Mozilla/5.0 (SMART-TV; Linux; Tizen 5.0) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/2.2 Chrome/63.0.3239.111 TV Safari/537.36",
			}
			ua := userAgents[rand.Intn(len(userAgents))]

			ctx, cancel := context.WithCancel(context.Background())
			s.Mu.Lock()
			if s.CancelFunc != nil {
				s.CancelFunc()
			}
			s.CancelFunc = cancel
			s.LastDataTime = time.Now()
			s.CurrentBytesRead = 0
			s.Mu.Unlock()

			req, err := http.NewRequestWithContext(ctx, "GET", srcUrl, nil)
			if err != nil {
				cancel()
				continue
			}
			req.Header.Set("User-Agent", ua)

			client := &http.Client{}

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("Failed to fetch source #%d [%s]: %v", idx, s.Key, err)
				cancel()
				continue
			}

			if resp.StatusCode != 200 {
				log.Printf("Source #%d [%s] returned HTTP %d", idx, s.Key, resp.StatusCode)
				resp.Body.Close()
				cancel()
				continue
			}

			// Monitoring goroutine
			go func(checkCtx context.Context) {
				lastBytes := int64(0)
				lastCheck := time.Now()
				lowDataTicks := 0
				
				for {
					select {
					case <-checkCtx.Done():
						return
					case <-time.After(2 * time.Second):
					}

					s.Mu.RLock()
					currentBytes := s.CurrentBytesRead
					lastDataTime := s.LastDataTime
					s.Mu.RUnlock()

					now := time.Now()
					diffBytes := currentBytes - lastBytes
					lastBytes = currentBytes
					
					duration := now.Sub(lastCheck).Seconds()
					if duration > 0 {
						s.Mu.Lock()
						s.CurrentBitrate = (float64(diffBytes) * 8) / (duration * 1000000)
						s.Mu.Unlock()
					}
					
					if diffBytes < 30000 { lowDataTicks++ } else { lowDataTicks = 0 }
					
					if time.Since(lastDataTime) > DataTimeout || lowDataTicks > 15 {
						log.Printf("DataTimeout [%s]: Stream frozen/low bitrate, killing attempt.", s.Key)
						cancel()
						break
					}
					lastCheck = now
				}
			}(ctx)

			buf := make([]byte, StreamBuffer)
			firstChunk := true

			for {
				n, err := io.ReadFull(resp.Body, buf)
				if n > 0 {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])

					s.Mu.Lock()
					s.LastDataTime = time.Now()
					s.CurrentBytesRead += int64(n)
					
					if firstChunk {
						if s.CurrentBytesRead >= 32768 { // 32KB threshold to declare source as working
							firstChunk = false
							log.Printf("Source #%d (%s) works! Promoting to active stream for %s", idx, srcUrl, s.Key)
							s.CurrentProcessStart = time.Now()
						}
					}

					// Send EVERY chunk to clients, don't throw away the first ones!
					for ch := range s.Clients {
						select {
						case ch <- chunk:
						default:
						}
					}
					s.Mu.Unlock()
				}
				
				if err != nil {
					if err != io.EOF && err != io.ErrUnexpectedEOF && err != context.Canceled {
						log.Printf("Source #%d [%s] died: %v", idx, s.Key, err)
					}
					break
				}
				
				s.Mu.RLock()
				active := s.Active
				s.Mu.RUnlock()
				if !active { break }
			}

			resp.Body.Close()
			cancel()
			
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
	if s.CancelFunc != nil {
		s.CancelFunc()
	}
	s.CancelFunc = nil
	s.Active = false
	s.Mu.Unlock()
}

func cleanupStream(key string) {
	streamsLock.Lock()
	defer streamsLock.Unlock()
	
	s, exists := streams[key]
	if !exists { return }

	s.Mu.Lock()
	defer s.Mu.Unlock()

	if len(s.Clients) > 0 {
		return
	}

	s.Active = false
	if s.CancelFunc != nil {
		s.CancelFunc()
	}
	delete(streams, key)
	log.Printf("Stream ended and cleaned up: %s", key)
}
