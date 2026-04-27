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
	httpClient       = &http.Client{Timeout: 10 * time.Second}
)

func checkUrlHealth(targetUrl string) bool {
	resp, err := httpClient.Head(targetUrl)
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
	if clientIP == "" { clientIP = r.Header.Get("X-Real-IP") }
	if clientIP == "" { clientIP = r.RemoteAddr }
	if strings.Contains(clientIP, ",") { clientIP = strings.Split(clientIP, ",")[0] }
	clientIP = strings.TrimSpace(clientIP)
	if colonIdx := strings.LastIndex(clientIP, ":"); colonIdx != -1 { clientIP = clientIP[:colonIdx] }
	clientIP = strings.Trim(clientIP, "[]")

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	sourceID := parts[0]
	channelID := parts[1]
	// Remove ALL extensions for the key lookup
	cleanID := channelID
	if idx := strings.Index(cleanID, "."); idx != -1 {
		cleanID = cleanID[:idx]
	}
	
	key := fmt.Sprintf("%s:%s", sourceID, cleanID)
	log.Printf("Proxy Request from %s | Key: %s | Path: %s", clientIP, key, r.URL.Path)

	allowedIPs, enforcementEnabled := getAllowedIPs()
	if enforcementEnabled && !allowedIPs[clientIP] {
		log.Printf("FORBIDDEN: Access denied for IP %s", clientIP)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	configLock.RLock()
	redirectMode := Config.RedirectMode
	sourceUrls := Config.Sources[sourceID]
	configLock.RUnlock()

	// External Bypass (302 Redirect)
	if redirectMode && !strings.Contains(r.URL.RawQuery, "proxy=1") {
		if len(sourceUrls) > 0 {
			u := strings.Replace(sourceUrls[0], "{channel_id}", cleanID, -1)
			log.Printf("Bypassing %s -> %s", key, u)
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
	}

	streamsLock.Lock()
	s, exists := streams[key]
	if !exists {
		configLock.RLock()
		autoFallback := Config.AutoFallback
		fallbackMode := Config.FallbackMode
		configLock.RUnlock()

		var urls []string
		if fallbackMode {
			urls = []string{FallbackURL}
		} else {
			// 1. Add local sources
			for _, u := range sourceUrls {
				urls = append(urls, strings.Replace(u, "{channel_id}", cleanID, -1))
			}

			// 2. Smart Proxy lookup
			channelNamesLock.RLock()
			name := ChannelNames[key]
			channelNamesLock.RUnlock()
			if name != "" {
				log.Printf("Smart Proxy: Searching alternatives for '%s' (%s)", name, key)
				alts := searchGlobalAlternatives(name)
				urls = append(urls, alts...)
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
	s.Clients[clientChan] = true
	clientsCount := len(s.Clients)
	s.Mu.Unlock()

	if !exists { go startProducer(s) }

	defer func() {
		s.Mu.Lock()
		delete(s.Clients, clientChan)
		clientsCount = len(s.Clients)
		s.Mu.Unlock()
		if clientsCount == 0 { time.AfterFunc(CleanupDelay, func() { cleanupStream(key) }) }
	}()

	// Wait for stream to become active
	waitStart := time.Now()
	for {
		s.Mu.RLock()
		hasData := s.CurrentBytesRead > 65536
		s.Mu.RUnlock()
		if hasData { break }
		if time.Since(waitStart) > 30*time.Second {
			http.Error(w, "Stream Timeout", http.StatusGatewayTimeout)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	hj, ok := w.(http.Hijacker)
	if !ok { return }
	conn, bufrw, err := hj.Hijack()
	if err != nil { return }
	defer conn.Close()

	bufrw.WriteString("HTTP/1.0 200 OK\r\nContent-Type: video/mp2t\r\nConnection: keep-alive\r\n\r\n")
	bufrw.Flush()

	var lastWrite time.Time
	for {
		select {
		case chunk := <-clientChan:
			// Burst pacing: limit speed to ~8-16 MB/s to prevent TVHeadend from choking on initial bursts
			elapsed := time.Since(lastWrite)
			if elapsed < 2 * time.Millisecond {
				time.Sleep((2 * time.Millisecond) - elapsed)
			}
			lastWrite = time.Now()

			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := conn.Write(chunk); err != nil { return }
		case <-time.After(60 * time.Second):
			return
		}
	}
}
