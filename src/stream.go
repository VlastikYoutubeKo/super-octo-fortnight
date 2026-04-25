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

	log.Printf("Starting stable producer: %s", s.Key)

	for {
		s.Mu.RLock()
		if !s.Active {
			s.Mu.RUnlock()
			break
		}
		s.Mu.RUnlock()

		// Race mechanism: try sources one by one, but don't wait forever for each
		winnerFound := make(chan bool, 1)
		
		for idx, srcUrl := range urls {
			go func(urlIdx int, url string) {
				isFallback := (url == FallbackURL)
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
				args = append(args, "-i", url)

				if isFallback {
						args = append(args, "-c", "copy", "-map", "0:v?", "-map", "0:a?")
				} else {
						args = append(args, "-c", "copy", "-map", "0?", "-ignore_unknown")
				}

				args = append(args, "-f", "mpegts", "-mpegts_flags", "resend_headers+initial_discontinuity", "-copyts", "pipe:1")
				cmd := exec.Command("ffmpeg", args...)
				stdout, err := cmd.StdoutPipe()
				if err != nil { return }
				stderr, _ := cmd.StderrPipe()

				if err := cmd.Start(); err != nil {
					return
				}

				// Error logging
				go func() {
					scanner := bufio.NewScanner(stderr)
					for scanner.Scan() {
						s.Mu.RLock()
						currentProc := s.Proc
						s.Mu.RUnlock()
						if currentProc == cmd {
							log.Printf("FFmpeg Error [%s]: %s", s.Key, scanner.Text())
						}
					}
				}()

				buf := make([]byte, FfmpegBuffer)
				firstChunk := true
				
				// Monitoring for this specific attempt
				go func() {
					lastBytes := int64(0)
					lastCheck := time.Now()
					lowDataTicks := 0
					
					for {
						s.Mu.RLock()
						active := s.Active
						proc := s.Proc
						currentBytes := s.CurrentBytesRead
						s.Mu.RUnlock()

						if !active || (proc != nil && proc != cmd) { break }
						
						// If we are the winner, we monitor bitrate
						if proc == cmd {
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
							
							if time.Since(s.LastDataTime) > DataTimeout || lowDataTicks > 6 {
								log.Printf("DataTimeout [%s]: FFmpeg frozen/low bitrate, killing attempt.", s.Key)
								stdout.Close()
								cmd.Process.Kill()
								break
							}
							lastCheck = now
						}
						time.Sleep(2 * time.Second)
					}
				}()

				for {
					n, err := stdout.Read(buf)
					if n > 0 {
						s.Mu.Lock()
						// Check if we already have a better winner
						if s.Proc != nil && s.Proc != cmd {
							s.Mu.Unlock()
							stdout.Close()
							cmd.Process.Kill()
							return
						}

						if firstChunk {
							firstChunk = false
							log.Printf("Winner found for %s: Source #%d (%s)", s.Key, urlIdx, url)
							s.Proc = cmd
							s.CurrentUrlIdx = urlIdx
							s.CurrentProcessStart = time.Now()
							s.LastDataTime = time.Now()
							s.CurrentBytesRead = 0
							s.RecentChunks = nil
							select {
							case winnerFound <- true:
							default:
							}
						}

						chunk := make([]byte, n)
						copy(chunk, buf[:n])
						s.LastDataTime = time.Now()
						s.CurrentBytesRead += int64(n)
						
						s.RecentChunks = append(s.RecentChunks, chunk)
						if len(s.RecentChunks) > 32 { s.RecentChunks = s.RecentChunks[1:] }

						for ch := range s.Clients {
							select {
							case ch <- chunk:
							default:
							}
						}
						s.Mu.Unlock()
					}
					if err != nil {
						s.Mu.Lock()
						if s.Proc == cmd {
							s.Proc = nil
							log.Printf("Producer [%s] died: %v", s.Key, err)
							select {
							case winnerFound <- false:
							default:
							}
						}
						s.Mu.Unlock()
						stdout.Close()
						cmd.Process.Kill()
						return
					}
					
					s.Mu.RLock()
					active := s.Active
					s.Mu.RUnlock()
					if !active { 
						stdout.Close()
						cmd.Process.Kill()
						return 
					}
				}
			}(idx, srcUrl)

			// Wait for a winner or timeout to try next source
			select {
			case success := <-winnerFound:
				if success {
					// We have a working stream! Wait for it to end.
					for {
						time.Sleep(1 * time.Second)
						s.Mu.RLock()
						active := s.Active
						proc := s.Proc
						s.Mu.RUnlock()
						if !active || proc == nil { break }
					}
					goto nextIter
				}
			case <-time.After(4 * time.Second):
				// Timeout reached, try next source in next loop iteration
				log.Printf("Source #%d for %s taking too long, trying next...", idx, s.Key)
				continue
			}
		}
		
		nextIter:
		s.Mu.RLock()
		if !s.Active {
			s.Mu.RUnlock()
			break
		}
		s.Mu.RUnlock()
		time.Sleep(1 * time.Second)
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
