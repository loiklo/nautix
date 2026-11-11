// Package auth handles instructor authentication and database-backed sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName   = "nautix_session"
	cookieMaxAge = 10 * 365 * 24 * 3600 // "infinite"; browsers cap it, so it is renewed on use
	maxSessions  = 5
	touchEvery   = time.Minute

	maxFailures = 5
	lockTime    = time.Minute
)

var (
	ErrBadCredentials = errors.New("bad credentials")
	ErrThrottled      = errors.New("too many attempts")
)

// User is the authenticated instructor attached to a request.
type User struct {
	ID            int64
	Name          string
	IsAdmin       bool
	LastSectionID *int64
	SessionID     int64
}

type failure struct {
	count int
	until time.Time
}

type Auth struct {
	DB           *sql.DB
	CookieSecure bool

	mu       sync.Mutex
	failures map[string]*failure
}

func New(db *sql.DB, cookieSecure bool) *Auth {
	return &Auth{DB: db, CookieSecure: cookieSecure, failures: make(map[string]*failure)}
}

type ctxKey struct{}

// UserFrom returns the authenticated user of a request that went through Require.
func UserFrom(r *http.Request) *User {
	u, _ := r.Context().Value(ctxKey{}).(*User)
	return u
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (a *Auth) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// throttled reports whether the key is currently locked out.
func (a *Auth) throttled(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.failures[key]
	return f != nil && time.Now().Before(f.until)
}

func (a *Auth) recordFailure(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.failures) > 10000 {
		a.failures = make(map[string]*failure)
	}
	f := a.failures[key]
	if f == nil {
		f = &failure{}
		a.failures[key] = f
	}
	f.count++
	if f.count >= maxFailures {
		f.count = 0
		f.until = time.Now().Add(lockTime)
	}
}

func (a *Auth) clearFailures(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.failures, key)
}

// Login checks the credentials, opens a session and sets the session cookie.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request, name, password string) error {
	key := clientIP(r) + "|" + strings.ToLower(name)
	if a.throttled(key) {
		return ErrThrottled
	}

	var id int64
	var hash string
	err := a.DB.QueryRow("SELECT id, password_hash FROM instructors WHERE name = ?", name).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Spend the same time as a real check so unknown names are not revealed.
		bcrypt.CompareHashAndPassword([]byte("$2a$10$CwTycUXWue0Thq9StjUM0uJ8rT5Y0jH0xhHqzFKdQF5n0yQ3N3s3e"), []byte(password))
		a.recordFailure(key)
		return ErrBadCredentials
	}
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		a.recordFailure(key)
		return ErrBadCredentials
	}
	a.clearFailures(key)

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	now := time.Now().UTC()
	if _, err := a.DB.Exec(
		"INSERT INTO sessions (token_hash, instructor_id, created_at, last_used_at) VALUES (?, ?, ?, ?)",
		hashToken(token), id, now, now); err != nil {
		return err
	}
	// Keep only the most recent sessions of this instructor.
	if _, err := a.DB.Exec(`DELETE FROM sessions WHERE instructor_id = ? AND id NOT IN (
		SELECT id FROM (SELECT id FROM sessions WHERE instructor_id = ? ORDER BY id DESC LIMIT ?) AS recent)`,
		id, id, maxSessions); err != nil {
		return err
	}
	a.setCookie(w, token, cookieMaxAge)
	return nil
}

// Logout deletes the current session and clears the cookie.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.DB.Exec("DELETE FROM sessions WHERE token_hash = ?", hashToken(c.Value))
	}
	a.setCookie(w, "", -1)
}

// Require rejects requests without a valid session and attaches the user to
// the request context.
func (a *Auth) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || c.Value == "" {
			http.Error(w, "Non connecté.", http.StatusUnauthorized)
			return
		}
		u := &User{}
		var lastUsed time.Time
		var lastSection sql.NullInt64
		err = a.DB.QueryRow(`SELECT s.id, s.last_used_at, i.id, i.name, i.is_admin, i.last_section_id
			FROM sessions s JOIN instructors i ON i.id = s.instructor_id WHERE s.token_hash = ?`,
			hashToken(c.Value)).Scan(&u.SessionID, &lastUsed, &u.ID, &u.Name, &u.IsAdmin, &lastSection)
		if errors.Is(err, sql.ErrNoRows) {
			a.setCookie(w, "", -1)
			http.Error(w, "Session expirée.", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "Erreur interne.", http.StatusInternalServerError)
			return
		}
		if lastSection.Valid {
			u.LastSectionID = &lastSection.Int64
		}
		if now := time.Now().UTC(); now.Sub(lastUsed) > touchEvery {
			a.DB.Exec("UPDATE sessions SET last_used_at = ? WHERE id = ?", now, u.SessionID)
			a.setCookie(w, c.Value, cookieMaxAge) // sliding renewal
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

// RequireAdmin is Require plus an administrator check.
func (a *Auth) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.Require(func(w http.ResponseWriter, r *http.Request) {
		if !UserFrom(r).IsAdmin {
			http.Error(w, "Réservé aux administrateurs.", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// HashPassword returns the bcrypt hash of a password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}
