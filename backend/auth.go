package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const authCookie = "aimovieai_auth"
const authLifetime = 30 * 24 * time.Hour

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Surname   string    `json:"surname"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
type credentials struct {
	Name     string `json:"name"`
	Surname  string `json:"surname"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type userContextKey struct{}

// Tokens are random bearer credentials; only their SHA-256 digest is stored.
func newAuthToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(b)
	return raw, tokenHash(raw), nil
}
func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func validToken(raw string) bool {
	if len(raw) != 64 {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}
func setAuthCookie(w http.ResponseWriter, r *http.Request, raw string) {
	http.SetCookie(w, &http.Cookie{Name: authCookie, Value: raw, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: int(authLifetime.Seconds()), Expires: time.Now().Add(authLifetime)})
}
func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func normalizeCredentials(in *credentials, register bool) bool {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) > 254 || len(in.Password) > 72 || len(in.Password) == 0 {
		return false
	}
	if register {
		in.Name = strings.TrimSpace(in.Name)
		in.Surname = strings.TrimSpace(in.Surname)
		if in.Name == "" || in.Surname == "" || utf8.RuneCountInString(in.Name) > 100 || utf8.RuneCountInString(in.Surname) > 100 || utf8.RuneCountInString(in.Password) < 8 {
			return false
		}
	}
	return true
}

// Bound both the attempt rate and the size of the local throttle. Never trust forwarded IP headers.
type authAttempt struct {
	until time.Time
	count int
}
type authThrottle struct {
	mu      sync.Mutex
	entries map[string]authAttempt
}

func (t *authThrottle) allow(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if t.entries == nil {
		t.entries = map[string]authAttempt{}
	}
	for k, v := range t.entries {
		if !now.Before(v.until) {
			delete(t.entries, k)
		}
	}
	v, exists := t.entries[host]
	if !exists {
		if len(t.entries) >= 4096 {
			return false
		}
		v = authAttempt{until: now.Add(15 * time.Minute)}
	}
	v.count++
	t.entries[host] = v
	return v.count <= 20
}
func (a *app) authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/me", a.me)
}
func (a *app) currentUser(r *http.Request) (User, error) {
	var u User
	c, err := r.Cookie(authCookie)
	if err != nil || !validToken(c.Value) {
		return u, sql.ErrNoRows
	}
	err = a.db.QueryRowContext(r.Context(), `SELECT u.id,u.name,u.surname,u.email,u.created_at FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash(c.Value)).Scan(&u.ID, &u.Name, &u.Surname, &u.Email, &u.CreatedAt)
	return u, err
}
func authFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 401, "Oturumun sona erdi. Lütfen tekrar giriş yap.")
	} else {
		fail(w, 503, "Hesap bilgilerine şu anda erişilemiyor.")
	}
}
func (a *app) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.currentUser(r)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			clearCookie(w, r, authCookie)
		}
		authFailure(w, err)
		return
	}
	send(w, 200, u)
}
func (a *app) register(w http.ResponseWriter, r *http.Request) {
	if !a.authLimiter.allow(r.RemoteAddr) {
		w.Header().Set("Retry-After", "900")
		fail(w, 429, "Çok fazla deneme yaptın. Lütfen 15 dakika sonra tekrar dene.")
		return
	}
	var in credentials
	if decode(r, &in) != nil || !normalizeCredentials(&in, true) {
		fail(w, 400, "Ad, soyad, geçerli e-posta ve en az 8 karakter (en fazla 72 bayt) şifre gerekli.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(w, 500, "Hesap oluşturulamadı.")
		return
	}
	raw, digest, err := newAuthToken()
	if err != nil {
		fail(w, 500, "Oturum oluşturulamadı.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "Hesap oluşturulamadı.")
		return
	}
	defer tx.Rollback()
	var u User
	err = tx.QueryRowContext(r.Context(), `INSERT INTO users(name,surname,email,password_hash) VALUES($1,$2,$3,$4) RETURNING id,name,surname,email,created_at`, in.Name, in.Surname, in.Email, string(hash)).Scan(&u.ID, &u.Name, &u.Surname, &u.Email, &u.CreatedAt)
	if err != nil {
		var pg *pq.Error
		if errors.As(err, &pg) && pg.Code == "23505" {
			fail(w, 409, "Bu e-posta adresiyle bir hesap zaten var. Giriş yapabilirsin.")
		} else {
			fail(w, 503, "Hesap oluşturulamadı.")
		}
		return
	}
	// New accounts always start empty; guest activity belongs to its original session.

	if c, e := r.Cookie(authCookie); e == nil && validToken(c.Value) {
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM auth_sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			fail(w, 503, "Oturum yenilenemedi.")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, digest, u.ID, time.Now().Add(authLifetime)); err != nil {
		fail(w, 503, "Oturum oluşturulamadı.")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 503, "Hesap kaydedilemedi.")
		return
	}
	setAuthCookie(w, r, raw)
	clearCookie(w, r, "aimovieai_session")
	send(w, 201, u)
}

// A valid fixed bcrypt digest keeps unknown-user login attempts on the same password-check path.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("invalid-account-placeholder"), bcrypt.DefaultCost)

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if !a.authLimiter.allow(r.RemoteAddr) {
		w.Header().Set("Retry-After", "900")
		fail(w, 429, "Çok fazla deneme yaptın. Lütfen 15 dakika sonra tekrar dene.")
		return
	}
	var in credentials
	if decode(r, &in) != nil || !normalizeCredentials(&in, false) {
		fail(w, 400, "Geçerli e-posta ve şifre gerekli.")
		return
	}
	var u User
	var hash string
	err := a.db.QueryRowContext(r.Context(), `SELECT id,name,surname,email,created_at,password_hash FROM users WHERE email=$1`, in.Email).Scan(&u.ID, &u.Name, &u.Surname, &u.Email, &u.CreatedAt, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(w, 503, "Şu anda giriş yapılamıyor.")
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		hash = string(dummyPasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password))
	if err != nil || passwordErr != nil {
		fail(w, 401, "E-posta veya şifre hatalı.")
		return
	}
	raw, digest, err := newAuthToken()
	if err != nil {
		fail(w, 500, "Oturum oluşturulamadı.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "Oturum oluşturulamadı.")
		return
	}
	defer tx.Rollback()
	// Rotate an existing login on this browser without revoking other devices.
	if c, e := r.Cookie(authCookie); e == nil && validToken(c.Value) {
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM auth_sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			fail(w, 503, "Oturum yenilenemedi.")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM auth_sessions WHERE user_id=$1 AND expires_at<=now()`, u.ID); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, digest, u.ID, time.Now().Add(authLifetime))
	}
	if err != nil {
		fail(w, 503, "Oturum oluşturulamadı.")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 503, "Oturum kaydedilemedi.")
		return
	}
	setAuthCookie(w, r, raw)
	clearCookie(w, r, "aimovieai_session")
	send(w, 200, u)
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(authCookie); err == nil && validToken(c.Value) {
		if _, err = a.db.ExecContext(r.Context(), `DELETE FROM auth_sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			fail(w, 503, "Çıkış yapılamadı. Lütfen tekrar dene.")
			return
		}
	}
	clearCookie(w, r, authCookie)
	clearCookie(w, r, "aimovieai_session")
	send(w, 200, map[string]string{"status": "ok"})
}
func userOwner(id int64) string { return "user:" + strconv.FormatInt(id, 10) }
func activityOwner(w http.ResponseWriter, r *http.Request) string {
	if u, ok := r.Context().Value(userContextKey{}).(User); ok {
		return userOwner(u.ID)
	}
	return session(w, r)
}

// Only a server-validated user may select an account collection; user IDs from the client are ignored.
func (a *app) withIdentity(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if _, err := r.Cookie(authCookie); err == http.ErrNoCookie {
		return r, true
	}
	u, err := a.currentUser(r)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			clearCookie(w, r, authCookie)
		}
		authFailure(w, err)
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)), true
}
