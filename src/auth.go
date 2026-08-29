package main

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// apiMiddleware wraps the whole management mux (port 9005) and does two things:
//
//  1. Optional API authentication. When config.json sets "api_token", every
//     /api/* request must present it via one of:
//       - header      X-API-Token: <token>
//       - header      Authorization: Bearer <token>
//       - query param ?token=<token>   (handy for TVHeadend playlist URLs)
//     Loopback requests (the UI served on the same host, the EPG janitor fetching
//     its own playlists) are trusted without a token, so nothing on the server
//     itself breaks. /api/health stays open for external monitoring.
//
//  2. CORS preflight. OPTIONS requests get the same-origin-restricted CORS headers
//     that sendJSON emits, so browser UIs on a configured origin keep working.
func apiMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		configLock.RLock()
		token := Config.APIToken
		origin := Config.CORSOrigin
		configLock.RUnlock()
		if origin == "" {
			origin = "*"
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Token")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if token != "" && r.URL.Path != "/api/health" && !isLoopbackRequest(r) && !tokenAccepted(r, token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized: missing or invalid API token"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func tokenAccepted(r *http.Request, expected string) bool {
	got := r.Header.Get("X-API-Token")
	if got == "" {
		if b := r.Header.Get("Authorization"); strings.HasPrefix(b, "Bearer ") {
			got = strings.TrimSpace(strings.TrimPrefix(b, "Bearer "))
		}
	}
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	if got == "" {
		return false
	}
	// Constant-time compare so the token cannot be brute-forced by timing.
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// isLoopbackRequest reports whether the request came from the same machine. The
// loopback exemption only applies to the API port so that the locally served UI,
// TVHeadend running on the same box and the EPG janitor keep working after a
// token is configured. Remote TVHeadend setups should append ?token=... to their
// playlist URLs instead.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
