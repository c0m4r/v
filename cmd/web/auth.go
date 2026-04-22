package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// loadOrGenerateToken loads a persisted token or creates a new one.
// The token is stored at <dataDir>/.token (mode 0600) so it survives restarts.
func loadOrGenerateToken(dataDir string) (string, error) {
	tokenPath := filepath.Join(dataDir, ".token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		if tok := strings.TrimSpace(string(data)); len(tok) == 64 {
			return tok, nil
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	tok := hex.EncodeToString(b)
	return tok, os.WriteFile(tokenPath, []byte(tok), 0600)
}

// authMiddleware enforces:
//   - Host-header validation (DNS rebinding protection, loopback binds only)
//   - Origin validation on /api/* paths (cross-site request / WebSocket hijacking)
//   - Bearer token authentication on /api/* paths
func authMiddleware(token, listenAddr string, next http.Handler) http.Handler {
	loopback := isLoopbackBind(listenAddr)
	allowed := buildAllowedHosts(listenAddr)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DNS rebinding protection: reject unexpected Host headers when bound
		// to a loopback address.
		if loopback && !isAllowedHost(r.Host, allowed) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Cross-site request / WebSocket hijacking protection.
		// If Origin is present it must resolve to an allowed host.
		if origin := r.Header.Get("Origin"); origin != "" {
			if !isAllowedOrigin(origin, allowed) {
				jsonError(w, http.StatusForbidden, "forbidden origin")
				return
			}
		}

		// Token authentication.
		if !hasValidToken(r, token) {
			jsonError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hasValidToken(r *http.Request, token string) bool {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ") == token
	}
	// Query param for WebSocket upgrades (browsers can't set Upgrade request headers).
	return r.URL.Query().Get("token") == token
}

// isLoopbackBind reports whether listenAddr is bound to a loopback interface.
func isLoopbackBind(listenAddr string) bool {
	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// buildAllowedHosts returns the set of Host header values that are acceptable.
func buildAllowedHosts(listenAddr string) []string {
	allowed := []string{"localhost", "127.0.0.1", "[::1]", "::1"}
	host, _, err := net.SplitHostPort(listenAddr)
	if err == nil && host != "" && host != "0.0.0.0" && host != "::" {
		allowed = append(allowed, host)
	}
	return allowed
}

// isAllowedHost checks whether the Host header value (with optional port) is
// in the allowed list.
func isAllowedHost(requestHost string, allowed []string) bool {
	h, _, err := net.SplitHostPort(requestHost)
	if err != nil {
		h = requestHost
	}
	for _, a := range allowed {
		if strings.EqualFold(h, a) {
			return true
		}
	}
	return false
}

// isAllowedOrigin checks whether an Origin header value's host is in the
// allowed list.
func isAllowedOrigin(origin string, allowed []string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isAllowedHost(u.Host, allowed)
}
