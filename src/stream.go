package main

import (
	"bufio"
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
	Mu            sync.RWMutex
	Proc          *exec.Cmd
	LastDataTime  time.Time
	Created       time.Time
	OnFallback    bool
	CurrentUrlIdx int
	Active        bool
	LastRetry     time.Time
	CurrentBytesRead    int64
	CurrentProcessStart time.Time
	CurrentBitrate      float64 // In Mbps
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

	log.Printf("Starting stable producer (Sequential): %s", s.Key)

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

			isFallback := (srcUrl == FallbackURL)
			args := []string{"-hide_banner", "-loglevel", "warning", "-user_agent", "VLC/3.0.18 LibVLC/3.0.18"}

			if isFallback {
				args = append(args, "-stream_loop", "-1", "-re")
			} else {
				args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-reconnect_on_network_error", "1", "-reconnect_on_http_error", "5xx")
			}

			args = append(args, "-analyzeduration", "15000000", "-probesize", "50000000")
			args = append(args, "-fflags", "+genpts+igndts+discardcorrupt")
			args = append(args, "-i", srcUrl)

			args = append(args, "-map", "0:v:0?", "-map", "0:a:0?", "-map", "0:s?")
			args = append(args, "-c", "copy")
			args = append(args, "-bsf:v", "dump_extra=freq=all")
			args = append(args, "-avoid_negative_ts", "make_zero")

			args = append(args, "-f", "mpegts", "-mpegts_flags", "initial_discontinuity+resend_headers", "-flush_packets", "1", "pipe:1")
			cmd := exec.Command("ffmpeg", args...)
			stdout, err := cmd.StdoutPipe()
			if err != nil { continue }
			stderr, _ := cmd.StderrPipe()

			if err := cmd.Start(); err != nil {
				continue
			}

			// Error logging
			go func(c *exec.Cmd, streamKey string) {
				scanner := bufio.NewScanner(stderr)
				for scanner.Scan() {
					s.Mu.RLock()
					proc := s.Proc
					s.Mu.RUnlock()
					if proc == c {
						log.Printf("FFmpeg Error [%s]: %s", streamKey, scanner.Text())
					}
				}
			}(cmd, s.Key)

			s.Mu.Lock()
			s.Proc = cmd
			s.LastDataTime = time.Now()
			s.CurrentBytesRead = 0
			s.Mu.Unlock()

			// Monitoring goroutine
			go func(c *exec.Cmd) {
				lastBytes := int64(0)
				lastCheck := time.Now()
				lowDataTicks := 0
				
				for {
					s.Mu.RLock()
					active := s.Active
					proc := s.Proc
					currentBytes := s.CurrentBytesRead
					s.Mu.RUnlock()

					if !active || proc != c { break }
					
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
					
					if time.Since(s.LastDataTime) > DataTimeout || lowDataTicks > 15 {
						log.Printf("DataTimeout [%s]: FFmpeg frozen/low bitrate, killing attempt.", s.Key)
						if c.Process != nil {
							c.Process.Kill()
						}
						break
					}
					lastCheck = now
					time.Sleep(2 * time.Second)
				}
			}(cmd)

			buf := make([]byte, FfmpegBuffer)
			firstChunk := true
			var localBytes int64
			var preWinnerChunks [][]byte

			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])
					localBytes += int64(n)

					s.Mu.Lock()
					s.LastDataTime = time.Now()
					
					if firstChunk {
						preWinnerChunks = append(preWinnerChunks, chunk)
						
						// Declare winner after receiving 256KB
						if localBytes > 262144 {
							firstChunk = false
							log.Printf("Source #%d (%s) works! Promoting to active stream for %s", idx, srcUrl, s.Key)
							s.CurrentProcessStart = time.Now()
							s.CurrentBytesRead = localBytes
							
							for _, c := range preWinnerChunks {
								for ch := range s.Clients {
									select {
									case ch <- c:
									default:
									}
								}
							}
							preWinnerChunks = nil // free memory
						}
					} else {
						s.CurrentBytesRead += int64(n)

						for ch := range s.Clients {
							select {
							case ch <- chunk:
							default:
							}
						}
					}
					s.Mu.Unlock()
				}
				
				if err != nil {
					log.Printf("Source #%d [%s] died: %v", idx, s.Key, err)
					break
				}
				
				s.Mu.RLock()
				active := s.Active
				s.Mu.RUnlock()
				if !active { break }
			}

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
			// Slight delay before trying next source to let network settle
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
	if s.Proc != nil && s.Proc.Process != nil {
		s.Proc.Process.Kill()
	}
	delete(streams, key)
	log.Printf("Stream ended and cleaned up: %s", key)
}
