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
	httpClient       = &http.Client{Timeout: 5 * time.Second}
)

func checkUrlHealth(targetUrl string) bool {
	resp, err := httpClient.Head(targetUrl)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode < 400 {
			return true
		}
	}
	resp, err = httpClient.Get(targetUrl)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode < 400 {
			return true
		}
	}
	return false
}

func getAllowedIPs() (map[string]bool, bool) {
	configLock.RLock()
	ips := Config.AllowedIPs
	domains := Config.AllowedDomains
	configLock.RUnlock()
	if len(ips) == 0 && len(domains) == 0 { return nil, false }
	dnsMutex.Lock()
	defer dnsMutex.Unlock()
	now := time.Now()
	if now.Sub(lastDNSCheck) > 5*time.Minute {
		newIPs := map[string]bool{"127.0.0.1": true, "::1": true}
		for _, ip := range ips { newIPs[ip] = true }
		for _, dom := range domains {
			resolvedIPs, err := net.LookupIP(dom)
			if err == nil {
				for _, ip := range resolvedIPs { newIPs[ip.String()] = true }
			}
		}
		allowedIPsCache = newIPs
		lastDNSCheck = now
	}
	return allowedIPsCache, true
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	if strings.Contains(clientIP, ",") {
		clientIP = strings.Split(clientIP, ",")[0]
	}
	clientIP = strings.TrimSpace(clientIP)
	if colonIdx := strings.LastIndex(clientIP, ":"); colonIdx != -1 {
		clientIP = clientIP[:colonIdx]
	}
	clientIP = strings.Trim(clientIP, "[]")
	log.Printf("Proxy Request from %s | Host: %s | UA: %s | Path: %s", clientIP, r.Host, r.UserAgent(), r.URL.Path)

	allowedIPs, enforcementEnabled := getAllowedIPs()
	if enforcementEnabled && !allowedIPs[clientIP] {
		log.Printf("FORBIDDEN: Access denied for IP %s (not in whitelist). UA: %s", clientIP, r.UserAgent())
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	sourceID := parts[0]
	channelID := strings.TrimSuffix(parts[1], ".m3u8")
	channelID = strings.TrimSuffix(channelID, ".ts")
	key := fmt.Sprintf("%s:%s", sourceID, channelID)

	configLock.RLock()
	redirectMode := Config.RedirectMode
	sourceUrls := Config.Sources[sourceID]
	configLock.RUnlock()

	// External Redirect (client -> upstream)
	if redirectMode && !strings.Contains(r.URL.RawQuery, "proxy=1") {
		if len(sourceUrls) > 0 {
			var redirectUrl string
			for _, u := range sourceUrls {
				if strings.HasSuffix(u, ".m3u8") {
					redirectUrl = strings.Replace(u, "{channel_id}", channelID, -1)
					break
				}
			}
			if redirectUrl == "" {
				redirectUrl = strings.Replace(sourceUrls[0], "{channel_id}", channelID, -1)
			}

			log.Printf("Checking health for redirect %s -> %s", key, redirectUrl)
			if checkUrlHealth(redirectUrl) {
				log.Printf("Redirecting %s to %s", key, redirectUrl)
				http.Redirect(w, r, redirectUrl, http.StatusFound)
				return
			}
		}
	}

	// (Removed Internal Redirect .ts -> .m3u8)

	streamsLock.Lock()
	s, exists := streams[key]
	if !exists {
		configLock.RLock()
		sourceUrls := Config.Sources[sourceID]
		fallbackMode := Config.FallbackMode
		autoFallback := Config.AutoFallback
		configLock.RUnlock()

		var urls []string
		if fallbackMode {
			urls = []string{FallbackURL}
		} else {
			// Try variants provider by provider (Interleaved)
			for _, u := range sourceUrls {
				replaced := strings.Replace(u, "{channel_id}", channelID, -1)
				var variants []string
				if strings.HasSuffix(replaced, ".m3u8") {
					variants = append(variants, replaced)
					variants = append(variants, strings.TrimSuffix(replaced, ".m3u8")+".ts")
				} else if strings.HasSuffix(replaced, ".ts") {
					variants = append(variants, strings.TrimSuffix(replaced, ".ts")+".m3u8")
					variants = append(variants, replaced)
				} else {
					variants = append(variants, replaced)
				}
				// Shuffle variants of the same source slightly or keep order? Let's try m3u8 first.
				urls = append(urls, variants...)
			}

			if autoFallback {
				urls = append(urls, FallbackURL)
			}
		}

		s = &Stream{Key: key, Urls: urls, Clients: make(map[chan []byte]bool), Created: time.Now(), LastDataTime: time.Now()}
		streams[key] = s
	}
	streamsLock.Unlock()

	clientChan := make(chan []byte, BufferQueueSize)
	s.Mu.Lock()
	// Send recent chunks to new client for instant start
	for _, chunk := range s.RecentChunks {
		select {
		case clientChan <- chunk:
		default:
		}
	}
	s.Clients[clientChan] = true
	clientsCount := len(s.Clients)
	s.Mu.Unlock()

	if !exists { go startProducer(s) }
	log.Printf("Connect: %s (Total: %d)", key, clientsCount)

	defer func() {
		s.Mu.Lock()
		delete(s.Clients, clientChan)
		clientsCount = len(s.Clients)
		s.Mu.Unlock()
		if clientsCount == 0 { time.AfterFunc(CleanupDelay, func() { cleanupStream(key) }) }
	}()

	waitStart := time.Now()
	for {
		s.Mu.RLock()
		hasData := s.CurrentBytesRead > 0
		s.Mu.RUnlock()
		if hasData {
			break
		}
		if time.Since(waitStart) > 20*time.Second {
			log.Printf("TIMEOUT: Giving up after 20s for %s (Client: %s)", key, clientIP)
			http.Error(w, "Timeout", http.StatusGatewayTimeout)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	hj, _ := w.(http.Hijacker)
	conn, bufrw, _ := hj.Hijack()
	defer conn.Close()

	// Tell the player it's MPEG-TS data regardless of whether the URL ends in .ts or .m3u8
	bufrw.WriteString("HTTP/1.0 200 OK\r\nContent-Type: video/mp2t\r\nConnection: keep-alive\r\n\r\n")
	bufrw.Flush()

	for {
		select {
		case chunk := <-clientChan:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := conn.Write(chunk); err != nil { return }
		case <-time.After(30 * time.Second):
			return
		}
	}
}
