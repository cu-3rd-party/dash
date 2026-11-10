package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.cu3rd.ru/cu3rd/dash/modules/setting"
)

// Password sign-in is for a stand without a Forgejo OAuth app: one login from
// DASH_PASSWORD_LOGIN with the password from DASH_PASSWORD. The login still
// has to be a member, so its role comes from the members list as usual.

const (
	maxFailures   = 10
	failureWindow = 15 * time.Minute
)

// ErrTooManyAttempts is returned while a client is locked out after failed passwords.
type ErrTooManyAttempts struct{}

func (ErrTooManyAttempts) Error() string {
	return "слишком много неудачных попыток, попробуйте через 15 минут"
}

type limiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

var attempts = &limiter{failures: map[string][]time.Time{}}

func (l *limiter) recent(ip string, now time.Time) []time.Time {
	kept := l.failures[ip][:0]
	for _, t := range l.failures[ip] {
		if now.Sub(t) < failureWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, ip)
		return nil
	}
	l.failures[ip] = kept
	return kept
}

// ResetAttempts forgets failed attempts (for tests).
func ResetAttempts() {
	attempts.mu.Lock()
	defer attempts.mu.Unlock()
	attempts.failures = map[string][]time.Time{}
}

// clientIP is the client address: the proxy (Caddy, Traefik) appends it to
// X-Forwarded-For, so the last value is the one the client cannot forge.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func equal(a, b string) bool {
	// hashing first makes the comparison independent of the lengths
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// CheckPassword reports whether login and password match the configured pair.
// After maxFailures wrong attempts from one address in failureWindow it refuses
// without checking.
func CheckPassword(r *http.Request, login, password string) (bool, error) {
	ip, now := clientIP(r), time.Now()
	attempts.mu.Lock()
	defer attempts.mu.Unlock()
	if len(attempts.recent(ip, now)) >= maxFailures {
		return false, ErrTooManyAttempts{}
	}
	ok := setting.PasswordLoginEnabled() &&
		equal(strings.ToLower(strings.TrimSpace(login)), setting.PasswordLogin) &&
		equal(password, setting.Password)
	if !ok {
		attempts.failures[ip] = append(attempts.failures[ip], now)
	}
	return ok, nil
}
