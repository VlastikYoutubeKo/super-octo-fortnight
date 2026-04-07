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
    clientIP := r.RemoteAddr
    if colonIdx := strings.LastIndex(clientIP, ":"); colonIdx != -1 { clientIP = clientIP[:colonIdx] }
    clientIP = strings.Trim(clientIP, "[]")

	allowedIPs, enforcementEnabled := getAllowedIPs()
	if enforcementEnabled && !allowedIPs[clientIP] {
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
	isM3U8Request := strings.HasSuffix(parts[1], ".m3u8")
	channelID := strings.TrimSuffix(parts[1], ".m3u8")
	channelID = strings.TrimSuffix(channelID, ".ts")
	key := fmt.Sprintf("%s:%s", sourceID, channelID)

    if isM3U8Request {
        http.Redirect(w, r, "http://"+r.Host+"/"+sourceID+"/"+channelID+".ts", http.StatusFound)
        return
    }

	streamsLock.Lock()
	s, exists := streams[key]
	if !exists {
        configLock.RLock()
		sourceUrls := Config.Sources[sourceID]
        configLock.RUnlock()
		var urls []string
		for _, u := range sourceUrls {
			urls = append(urls, strings.Replace(u, "{channel_id}", channelID, -1))
		}
		shuffle(urls)
		s = &Stream{Key: key, Urls: urls, Clients: make(map[chan []byte]bool), Created: time.Now(), LastDataTime: time.Now()}
		streams[key] = s
	}
	streamsLock.Unlock()

	clientChan := make(chan []byte, BufferQueueSize)
	s.Mu.Lock()
    // History is now disabled for smooth timestamps
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
		if hasData { break }
		if time.Since(waitStart) > 10 * time.Second {
			http.Error(w, "Timeout", http.StatusGatewayTimeout)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	hj, _ := w.(http.Hijacker)
	conn, bufrw, _ := hj.Hijack()
	defer conn.Close()

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
