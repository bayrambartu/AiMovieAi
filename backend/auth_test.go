package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestAuthInputAndBoundary(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/auth/register", `{"name":"A","surname":"B","email":"a@example.com","password":"short"}`, 400},
		{"POST", "/api/auth/register", `{"name":"A","surname":"B","email":"Name <a@example.com>","password":"password123"}`, 400},
		{"POST", "/api/auth/register", `{"name":"A","surname":"B","email":"a@example.com","password":"password123","id":1}`, 400},
		{"POST", "/api/auth/login", `{"email":"wrong","password":"password123"}`, 400},
		{"POST", "/api/auth/login", `{} {}`, 400},
		{"GET", "/api/auth/me", "", 401},
	} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	// A malformed auth cookie cannot select a guest or account collection.
	for _, path := range []string{"/api/library", "/api/recommendations"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: authCookie, Value: "user:1"})
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, origin := range []string{"https://evil.example", "null", "ftp://example.com"} {
		r := httptest.NewRequest("POST", "http://example.com/api/auth/logout", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(origin, w.Code)
		}
	}
	for i := 0; i < 21; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
		r.RemoteAddr = "192.0.2.2:1234"
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if i == 20 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatal("login rate not limited", w.Code)
		}
	}
	in := credentials{Name: " A ", Surname: " B ", Email: " USER@Example.com ", Password: "strong-password"}
	if !normalizeCredentials(&in, true) || in.Email != "user@example.com" || in.Name != "A" {
		t.Fatal(in)
	}
	in.Password = strings.Repeat("ş", 37)
	if normalizeCredentials(&in, true) {
		t.Fatal("bcrypt byte limit ignored")
	}
}
func TestAuthTokensAndCookies(t *testing.T) {
	raw, digest, err := newAuthToken()
	if err != nil || !validToken(raw) || digest == raw || tokenHash(raw) != digest {
		t.Fatal("invalid token")
	}
	other, _, _ := newAuthToken()
	if other == raw {
		t.Fatal("token reused")
	}
	r := httptest.NewRequest("POST", "https://example.com/api/auth/login", nil)
	r.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	setAuthCookie(w, r, raw)
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 2592000 || c.Path != "/" {
		t.Fatal(c)
	}
}

func TestPostgresAuthFlow(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL yok; gerçek hesap entegrasyonu atlandı")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	namespace := "auth_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err = db.Exec("CREATE SCHEMA " + namespace); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + namespace + " CASCADE")
	if _, err = db.Exec("SET search_path TO " + namespace); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema); err != nil {
		t.Fatal("schema is not repeatable", err)
	}
	a := testApp(t)
	a.db = db
	handler := a.routes()
	request := func(method, path, body string, status int, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: wanted %d got %d %s", path, status, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "$2a$") {
			t.Fatal("password exposed", w.Body.String())
		}
		return w
	}
	auth := func(w *httptest.ResponseRecorder) *http.Cookie {
		t.Helper()
		for _, c := range w.Result().Cookies() {
			if c.Name == authCookie && c.MaxAge > 0 {
				return c
			}
		}
		t.Fatal("missing auth cookie")
		return nil
	}
	guest := request("GET", "/api/library", "", 200).Result().Cookies()[0]
	request("PUT", "/api/library/27205", `{"watchlist":true,"watched":true,"liked":true,"rating":9}`, 200, guest)
	aliceResponse := request("POST", "/api/auth/register", `{"name":"Ada","surname":"Bir","email":"ADA@example.com","password":"strong-password"}`, 201, guest)
	alice := auth(aliceResponse)
	var user User
	if err = json.Unmarshal(aliceResponse.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	var storedHash, storedToken string
	if err = db.QueryRow("SELECT password_hash FROM users WHERE id=$1", user.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("strong-password")) != nil {
		t.Fatal("password not hashed")
	}
	if err = db.QueryRow("SELECT token_hash FROM auth_sessions WHERE user_id=$1", user.ID).Scan(&storedToken); err != nil || storedToken != tokenHash(alice.Value) {
		t.Fatal("token storage", err)
	}
	if strings.TrimSpace(request("GET", "/api/library", "", 200, alice).Body.String()) != "{}" {
		t.Fatal("new account inherited guest preferences")
	}
	if !strings.Contains(request("GET", "/api/library", "", 200, guest).Body.String(), `"rating":9`) {
		t.Fatal("registration changed the guest collection")
	}
	// Account preferences only appear after an explicit save by that account.
	request("PUT", "/api/library/27205", `{"watchlist":true,"watched":true,"liked":true,"rating":9}`, 200, alice)

	request("POST", "/api/auth/register", `{"name":"Ada","surname":"Bir","email":"ada@EXAMPLE.com","password":"strong-password"}`, 409)
	request("POST", "/api/auth/login", `{"email":"ada@example.com","password":"wrong-password"}`, 401)
	request("POST", "/api/auth/login", `{"email":"missing@example.com","password":"wrong-password"}`, 401)
	// Login on a second device sees the same collection and rotates its existing token.
	second := auth(request("POST", "/api/auth/login", `{"email":"ada@example.com","password":"strong-password"}`, 200))
	request("GET", "/api/auth/me", "", 200, second)
	if !strings.Contains(request("GET", "/api/library", "", 200, second).Body.String(), `"rating":9`) {
		t.Fatal("account list did not follow login")
	}
	rotated := auth(request("POST", "/api/auth/login", `{"email":"ada@example.com","password":"strong-password"}`, 200, second))
	request("GET", "/api/auth/me", "", 401, second)
	// A separate account has a separate collection even for the same movie.
	bob := auth(request("POST", "/api/auth/register", `{"name":"Can","surname":"İki","email":"can@example.com","password":"other-password"}`, 201))
	if strings.TrimSpace(request("GET", "/api/library", "", 200, bob).Body.String()) != "{}" {
		t.Fatal("account isolation failed")
	}
	request("PUT", "/api/library/27205", `{"rating":3}`, 200, bob)
	if !strings.Contains(request("GET", "/api/library", "", 200, alice).Body.String(), `"rating":9`) {
		t.Fatal("one user overwrote another")
	}
	// Recommendations read the authenticated user's watched preferences.
	rec := request("POST", "/api/recommendations", `{"seed_id":157336,"exclude_watched":true}`, 200, alice)
	var result recommendationResult
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result.Status != "ok" || len(result.Movies) == 0 {
		t.Fatal("no recommendations to verify", rec.Body.String())
	}
	for _, m := range result.Movies {
		if m.ID == 27205 {
			t.Fatal("account watched filter ignored")
		}
	}
	// Existing-account login must never import a shared browser's guest data.
	guest2 := request("GET", "/api/library", "", 200).Result().Cookies()[0]
	request("PUT", "/api/library/157336", `{"watchlist":true}`, 200, guest2)
	third := auth(request("POST", "/api/auth/login", `{"email":"ada@example.com","password":"strong-password"}`, 200, guest2))
	if strings.Contains(request("GET", "/api/library", "", 200, third).Body.String(), `"157336"`) {
		t.Fatal("login imported guest data")
	}
	request("POST", "/api/auth/logout", `{}`, 200, alice)
	request("GET", "/api/auth/me", "", 401, alice)
	request("GET", "/api/library", "", 401, alice)
	request("GET", "/api/auth/me", "", 200, rotated)
	if _, err = db.Exec("UPDATE auth_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", tokenHash(bob.Value)); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/auth/me", "", 401, bob)
	request("PUT", "/api/library/27205", `{"rating":10}`, 401, bob)
}
