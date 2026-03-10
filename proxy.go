package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	allowedIPsCache  = map[string]bool{}
	lastDNSCheck     time.Time
	dnsMutex         sync.Mutex
)

func getAllowedIPs() (map[string]bool, bool) {
	configLock.RLock()
	ips := Config.AllowedIPs
	domains := Config.AllowedDomains
	configLock.RUnlock()

	// If both lists are empty, whitelist is disabled -> allow all
	if len(ips) == 0 && len(domains) == 0 {
		return nil, false
	}

	dnsMutex.Lock()
	defer dnsMutex.Unlock()

	now := time.Now()
	if now.Sub(lastDNSCheck) > 5*time.Minute {
		newIPs := map[string]bool{"127.0.0.1": true, "::1": true}
		for _, ip := range ips {
			newIPs[ip] = true
		}
		for _, dom := range domains {
			resolvedIPs, err := net.LookupIP(dom)
			if err == nil {
				for _, ip := range resolvedIPs {
					newIPs[ip.String()] = true
				}
			} else {
				log.Printf("Could not resolve whitelist domain %s: %v", dom, err)
			}
		}
		allowedIPsCache = newIPs
		lastDNSCheck = now
		log.Printf("Updated allowed IPs whitelist: %d IPs", len(allowedIPsCache))
	}
	return allowedIPsCache, true
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
    clientIP := r.RemoteAddr
    if colonIdx := strings.LastIndex(clientIP, ":"); colonIdx != -1 {
        clientIP = clientIP[:colonIdx]
    }
    
    // Remove IPv6 brackets if present
    clientIP = strings.Trim(clientIP, "[]")

	allowedIPs, enforcementEnabled := getAllowedIPs()
	if enforcementEnabled && !allowedIPs[clientIP] {
		log.Printf("Blocked unauthorized access from IP: %s", clientIP)
		http.Error(w, "Forbidden: IP not allowed", http.StatusForbidden)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	sourceID := parts[0]
	channelID := strings.TrimSuffix(parts[1], ".ts")
	key := fmt.Sprintf("%s:%s", sourceID, channelID)

    cooldownsLock.RLock()
    cd, hasCd := cooldowns[key]
    cooldownsLock.RUnlock()
    if hasCd {
        remaining := int(time.Until(cd).Seconds())
        if remaining > 0 {
            http.Error(w, fmt.Sprintf("Cooldown: %ds", remaining), http.StatusServiceUnavailable)
            return
        }
    }

	streamsLock.Lock()
	s, exists := streams[key]
	if !exists {
        configLock.RLock()
		sourceUrls, sourceExists := Config.Sources[sourceID]
        fallbackMode := Config.FallbackMode
        autoFallback := Config.AutoFallback
        configLock.RUnlock()
        
		if !sourceExists && !fallbackMode {
			streamsLock.Unlock()
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		var urls []string
		if fallbackMode {
			urls = []string{FallbackURL}
		} else {
			for _, u := range sourceUrls {
				formatted := strings.Replace(u, "{channel_id}", channelID, -1)
				urls = append(urls, formatted)
			}
			shuffle(urls)
			
			if autoFallback {
				hasFallback := false
				for _, u := range urls {
					if u == FallbackURL {
						hasFallback = true
						break
					}
				}
				if !hasFallback {
					urls = append(urls, FallbackURL)
				}
			}
		}

		s = &Stream{
			Key:          key,
			Urls:         urls,
			Clients:      make(map[chan []byte]bool),
			Created:      time.Now(),
			LastDataTime: time.Now(),
		}
		streams[key] = s
		go startProducer(s)
	}
	streamsLock.Unlock()

	clientChan := make(chan []byte, BufferQueueSize)

	s.Mu.Lock()
	for _, chunk := range s.RecentChunks {
		select {
		case clientChan <- chunk:
		default:
		}
	}
	s.Clients[clientChan] = true
	clientsCount := len(s.Clients)
	s.Mu.Unlock()

	log.Printf("Client connected: %s (total: %d)", key, clientsCount)

	defer func() {
		s.Mu.Lock()
		delete(s.Clients, clientChan)
		clientsCount = len(s.Clients)
		s.Mu.Unlock()

		log.Printf("Client disconnected: %s (remaining: %d)", key, clientsCount)

		if clientsCount == 0 {
			time.AfterFunc(CleanupDelay, func() {
				cleanupStream(key)
			})
		}
	}()

	waitStart := time.Now()
	for {
		s.Mu.RLock()
		hasProc := s.Proc != nil
		s.Mu.RUnlock()
		
		if hasProc {
			break
		}
		
		if time.Since(waitStart) > StartupTimeout {
			log.Printf("Timeout waiting for stream to start: %s", key)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("Webserver doesn't support hijacking for %s", key)
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		log.Printf("Hijack failed for %s: %v", key, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	headers := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: video/MP2T\r\n" +
		"Cache-Control: no-cache, no-store, must-revalidate\r\n" +
		"Pragma: no-cache\r\n" +
		"Expires: 0\r\n" +
		"X-Accel-Buffering: no\r\n" +
		"Connection: keep-alive\r\n\r\n"
	
	_, err = bufrw.WriteString(headers)
	if err != nil {
		log.Printf("Failed to write headers to %s: %v", key, err)
		return
	}
	bufrw.Flush()

	emptyReads := 0
	for {
		select {
		case chunk := <-clientChan:
			emptyReads = 0
			_, err := conn.Write(chunk)
			if err != nil {
				log.Printf("Client disconnected or write failed for %s: %v", key, err)
				return
			}
		case <-time.After(10 * time.Second):
			emptyReads++
			if emptyReads > 6 {
				log.Printf("Timeout waiting for data on client: %s", key)
				return
			}
		}
	}
}