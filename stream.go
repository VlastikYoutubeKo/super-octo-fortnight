package main

import (
	"encoding/base64"
	"io"
	"log"
	"math/rand"
	"net/url"
	"os"
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
	s.Active = true
	urls := make([]string, len(s.Urls))
	copy(urls, s.Urls)
	s.Mu.Unlock()

	log.Printf("Producer started: %s", s.Key)

	for {
		s.Mu.RLock()
		active := s.Active
		clients := len(s.Clients)
		s.Mu.RUnlock()

		if !active || clients <= 0 {
			break
		}

		for idx, srcUrl := range urls {
			isFallback := (srcUrl == FallbackURL)

			retryCount := 0
			maxRetries := 3

			for retryCount < maxRetries {
				s.Mu.Lock()
				s.CurrentUrlIdx = idx
				s.OnFallback = isFallback
				s.Mu.Unlock()

				log.Printf("Starting %s [%d/%d] (Try %d/%d)%s", s.Key, idx+1, len(urls), retryCount+1, maxRetries, func() string {
					if isFallback {
						return " - Fallback"
					}
					return ""
				}())

				args := []string{}
				proxies := getProxies()
				if len(proxies) > 0 && !isFallback {
					proxyStr := proxies[rand.Intn(len(proxies))]
					u, err := url.Parse(proxyStr)
					if err == nil {
						if u.User != nil {
							pass, _ := u.User.Password()
							auth := u.User.Username() + ":" + pass
							encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
							// remove userinfo from url for -http_proxy
							u.User = nil
							args = append(args, "-http_proxy", u.String())
							args = append(args, "-headers", "Proxy-Authorization: Basic "+encodedAuth+"\r\n")
						} else {
							args = append(args, "-http_proxy", proxyStr)
						}
					} else {
						args = append(args, "-http_proxy", proxyStr)
					}
				}

				if isFallback {
					args = append(args, "-stream_loop", "-1", "-re")
				}

				args = append(args,
					"-hide_banner",
					"-loglevel", "error",
					"-user_agent", "VLC/3.0.23 LibVLC/3.0.23",
					"-reconnect", "1",
					"-reconnect_streamed", "1",
					"-reconnect_delay_max", "5",
					"-reconnect_on_http_error", "4xx,5xx",
					"-analyzeduration", "5000000",
					"-probesize", "10000000",
					"-fflags", "+genpts+igndts+discardcorrupt",
					"-i", srcUrl,
					"-map", "0:v:0?", "-map", "0:a:0?", "-map", "0:s?",
					"-c", "copy",
					"-avoid_negative_ts", "make_zero",
					"-mpegts_flags", "initial_discontinuity+resend_headers",
					"-f", "mpegts",
					"-flush_packets", "1",
					"pipe:1",
				)

				cmd := exec.Command("ffmpeg", args...)
				
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					log.Printf("Error creating stdout pipe for %s: %v", s.Key, err)
					retryCount++
					continue
				}
				
				if err := cmd.Start(); err != nil {
					log.Printf("Error starting ffmpeg for %s: %v", s.Key, err)
					retryCount++
					continue
				}

				s.Mu.Lock()
				s.Proc = cmd
				s.LastDataTime = time.Now()
				s.CurrentProcessStart = time.Now()
				s.CurrentBytesRead = 0
				s.Mu.Unlock()

				runStart := time.Now()

				buf := make([]byte, FfmpegBuffer)
				doneReading := make(chan bool)
				
				go func() {
					for {
						n, err := stdout.Read(buf)
						if n > 0 {
							chunk := make([]byte, n)
							copy(chunk, buf[:n])
							
							s.Mu.Lock()
							s.LastDataTime = time.Now()
							s.CurrentBytesRead += int64(n)
							s.RecentChunks = append(s.RecentChunks, chunk)
							if len(s.RecentChunks) > 150 {
								s.RecentChunks = s.RecentChunks[1:]
							}
							
							for ch := range s.Clients {
								select {
								case ch <- chunk:
								default:
									select { case <-ch: default: }
									select { case ch <- chunk: default: }
								}
							}
							s.Mu.Unlock()
						}
						
						if err != nil {
							if err != io.EOF && err != os.ErrClosed {
								log.Printf("FFmpeg read error %s: %v", s.Key, err)
							}
							break
						}
					}
					doneReading <- true
				}()

				monitorLoop := true
				for monitorLoop {
					select {
					case <-doneReading:
						monitorLoop = false
					case <-time.After(1 * time.Second):
						s.Mu.RLock()
						active := s.Active
						clients := len(s.Clients)
						timeSinceData := time.Since(s.LastDataTime)
						s.Mu.RUnlock()

						if !active || clients <= 0 {
							if cmd.Process != nil {
								cmd.Process.Kill()
							}
							monitorLoop = false
						} else if timeSinceData > DataTimeout {
							log.Printf("FFmpeg stuck for %v on %s", timeSinceData, s.Key)
							if cmd.Process != nil {
								cmd.Process.Kill()
							}
							monitorLoop = false
						}
					}
				}

				cmd.Wait()

				runDuration := time.Since(runStart)

				s.Mu.RLock()
				active = s.Active
				clients = len(s.Clients)
				timeSinceData := time.Since(s.LastDataTime)
				s.Mu.RUnlock()

				if !active || clients <= 0 {
					break // break retry loop
				}

				if runDuration > 15 * time.Second && timeSinceData < DataTimeout {
					// It ran successfully and did not hang, reset retries
					retryCount = 0
				} else {
					retryCount++
				}
				if retryCount < maxRetries {
					log.Printf("Source %d for %s ended prematurely (ran for %v), retrying same source...", idx+1, s.Key, runDuration)
					time.Sleep(2 * time.Second)
				} else {
					log.Printf("Source %d for %s failed after %d retries, moving to next source...", idx+1, s.Key, maxRetries)
				}
			}

			s.Mu.RLock()
			active = s.Active
			clients = len(s.Clients)
			s.Mu.RUnlock()
			
			if !active || clients <= 0 {
				break
			}
		}
		
		s.Mu.RLock()
		active = s.Active
		clients = len(s.Clients)
		s.Mu.RUnlock()
		
		if !active || clients <= 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	s.Mu.Lock()
	s.Proc = nil
	s.Active = false
	s.Mu.Unlock()
	log.Printf("Producer ended: %s", s.Key)
}

func cleanupStream(key string) {
	streamsLock.Lock()
	s, exists := streams[key]
	if exists {
		s.Mu.Lock()
		clients := len(s.Clients)
		s.Mu.Unlock()

		if clients == 0 {
			s.Mu.Lock()
			s.Active = false
			if s.Proc != nil && s.Proc.Process != nil {
				s.Proc.Process.Kill()
			}
			s.Mu.Unlock()
			delete(streams, key)
			log.Printf("Cleaned up: %s", key)
		}
	}
	streamsLock.Unlock()
}
