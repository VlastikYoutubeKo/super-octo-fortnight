package main

import (
	"context"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os/exec"
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

			configLock.RLock()
			engine := Config.Engine
			if engine == "" { engine = "http" }
			configLock.RUnlock()

			var stdoutReader io.Reader
			var cmd *exec.Cmd

			if engine != "http" {
				log.Printf("Starting %s engine for %s", engine, s.Key)
				isFallback := (srcUrl == FallbackURL)
				var args []string
				var cmdName string

				if engine == "ffmpeg" {
					cmdName = "ffmpeg"
					args = []string{"-hide_banner", "-loglevel", "warning", "-user_agent", ua}
					if isFallback {
						args = append(args, "-stream_loop", "-1")
					} else {
						args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-reconnect_on_network_error", "1", "-reconnect_on_http_error", "5xx", "-rw_timeout", "20000000")
					}
					args = append(args, "-analyzeduration", "15000000", "-probesize", "50000000")
					args = append(args, "-fflags", "+genpts+igndts")
					args = append(args, "-i", srcUrl)
					args = append(args, "-map", "0:v:0?", "-map", "0:a:0?", "-map", "0:s?")
					args = append(args, "-c", "copy")
					args = append(args, "-bsf:v", "dump_extra=freq=all")
					args = append(args, "-max_interleave_delta", "0", "-max_muxing_queue_size", "1024")
					args = append(args, "-avoid_negative_ts", "make_zero")
					args = append(args, "-f", "mpegts", "-mpegts_flags", "initial_discontinuity+resend_headers", "-flush_packets", "1", "pipe:1")

				} else if engine == "tsduck" {
					cmdName = "tsp"
					args = []string{"-I", "http", "--user-agent", ua, "--receive-timeout", "20000"}
					if isFallback {
						args = append(args, "--infinite")
					}
					args = append(args, srcUrl)
					
					// Do not use -P regulate, as it chokes the stream to 1.5mbps if PCR is calculated wrongly.
					// We already have a 10MB/s bottleneck in proxy.go, and TVH has its own buffers.
					args = append(args, "-P", "continuity", "--fix")
					args = append(args, "-O", "file")
					
				} else if engine == "gstreamer" {
					cmdName = "gst-launch-1.0"
					args = []string{"-q", "souphttpsrc", "location=" + srcUrl, "user-agent=" + ua, "is-live=true"}
					args = append(args, "!", "tsparse", "set-timestamps=true", "!", "fdsink")
				}
				
				cmd = exec.CommandContext(ctx, cmdName, args...)
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					cancel()
					continue
				}
				stdoutReader = stdout
				
				if err := cmd.Start(); err != nil {
					cancel()
					continue
				}
			} else {
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
				
				// Wrap body so we can close it later
				stdoutReader = resp.Body
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
					
					// 125000 bytes in 2 seconds is 500 kbps
					if diffBytes < 125000 { lowDataTicks++ } else { lowDataTicks = 0 }
					
					if time.Since(lastDataTime) > DataTimeout || lowDataTicks > 10 { // kill after 20 seconds of < 500kbps
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
				n, err := io.ReadFull(stdoutReader, buf)
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

					// Send EVERY chunk to clients without dropping. Blocking send provides natural backpressure.
					for ch := range s.Clients {
						ch <- chunk
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

			if engine != "http" && cmd != nil {
				cancel()
				cmd.Wait()
			} else {
				if closer, ok := stdoutReader.(io.ReadCloser); ok {
					closer.Close()
				}
				cancel()
			}
			
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
