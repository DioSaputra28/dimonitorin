package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const sessionCookie = "dimonitorin_session"
const MinPasswordLength = 8

func HashPassword(password string) (string, error) {
	if len(password) < MinPasswordLength {
		return "", errors.New("password must be at least 8 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + "." + base64.RawStdEncoding.EncodeToString(hash), nil
}

func ComparePassword(encoded string, password string) bool {
	parts := bytesSplit(encoded, '.')
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func bytesSplit(s string, sep byte) []string {
	out := make([]string, 0, 2)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

func NewSessionManager(secret string) *SessionManager {
	return &SessionManager{secret: []byte(secret), ttl: 24 * time.Hour}
}

func (s *SessionManager) Set(w http.ResponseWriter, sessionID string, secure bool) {
	cookie := &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(s.ttl),
		Secure:   secure,
	}
	http.SetCookie(w, cookie)
}

func (s *SessionManager) Clear(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Secure:   secure,
	})
}

func (s *SessionManager) Read(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func RandomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type attempt struct {
	Count       int
	FirstFailed time.Time
	LockedUntil time.Time
}

type LoginProtector struct {
	mu       sync.Mutex
	byIP     map[string]*attempt
	maxFails int
	window   time.Duration
	lockout  time.Duration
}

func NewLoginProtector() *LoginProtector {
	return &LoginProtector{
		byIP:     map[string]*attempt{},
		maxFails: 5,
		window:   15 * time.Minute,
		lockout:  15 * time.Minute,
	}
}

func (p *LoginProtector) Check(ip string, now time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.byIP[ip]
	if !ok {
		return nil
	}
	if now.Before(a.LockedUntil) {
		return errors.New("too many login attempts, please retry later")
	}
	if now.Sub(a.FirstFailed) > p.window {
		delete(p.byIP, ip)
	}
	return nil
}

func (p *LoginProtector) Fail(ip string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.byIP[ip]
	if !ok || now.Sub(a.FirstFailed) > p.window {
		p.byIP[ip] = &attempt{Count: 1, FirstFailed: now}
		return
	}
	a.Count++
	if a.Count >= p.maxFails {
		a.LockedUntil = now.Add(p.lockout)
	}
}

func (p *LoginProtector) Success(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.byIP, ip)
}

func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
