package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *app {
	t.Helper()
	return &app{model: "test", tmdb: mockTMDB(t), client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { return nil, io.EOF })}}
}
func fixtureMovies() []Movie {
	return []Movie{
		{ID: 27205, Title: "Inception", OriginalTitle: "Inception", Genres: []string{"Science Fiction", "Action"}, Keywords: []string{"dream"}, Rating: 8.4, Votes: 30000},
		{ID: 157336, Title: "Interstellar", Genres: []string{"Science Fiction", "Adventure"}, Keywords: []string{"space"}, Rating: 8.5, Votes: 30000},
		{ID: 3, Title: "Comedy", Genres: []string{"Comedy"}, Rating: 6, Votes: 1000},
		{ID: 4, Title: "Dream world", Genres: []string{"Science Fiction", "Action"}, Keywords: []string{"dream"}, Rating: 9, Votes: 10000},
	}
}
func rankingFixture() rankingSet {
	ms := fixtureMovies()
	byID := map[int]Movie{}
	for _, m := range ms {
		byID[m.ID] = m
	}
	return rankingSet{movies: ms, byID: byID}
}
func TestCatalogSearchAndPagination(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct {
		path   string
		status int
	}{{"/api/movies?q=Inception", 200}, {"/api/movies?genre=Science+Fiction&sort=rating&page=2", 200}, {"/api/movies?page=501", 400}, {"/api/movies?page=oops", 400}, {"/api/movies?q=Inception&genre=Comedy", 400}, {"/api/movies?sort=oops", 400}, {"/api/movies/27205", 200}, {"/api/genres", 200}} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
func TestWeightedSimilarity(t *testing.T) {
	a := map[string]float64{"genre": 2, "theme": 1}
	b := map[string]float64{"genre": 2, "other": 1}
	if got := similarity(a, b); math.Abs(got-.5) > 1e-9 {
		t.Fatal(got)
	}
	if similarity(a, a) != 1 {
		t.Fatal("identity")
	}
	if similarity(nil, nil) != 0 {
		t.Fatal("empty")
	}
	if similarity(a, map[string]float64{"no": 1}) != 0 {
		t.Fatal("disjoint")
	}
}

func TestRankingRespectsPreferencesAndWatched(t *testing.T) {
	a := rankingFixture()
	acts := map[int]Activity{27205: {MovieID: 27205, Watched: true, Liked: true, Rating: 9}}
	for _, in := range []RecRequest{
		{SeedID: 27205, ExcludeWatched: true, MinRating: 7, Genres: []string{"Science Fiction"}},
		{SeedID: 27205, ExcludeWatched: true, MinRating: 7, Genres: []string{"Science Fiction"}},
	} {
		ranked := a.rank(in, acts, Intent{})
		if len(ranked) != 2 {
			t.Fatalf("expected 2, got %d", len(ranked))
		}
		for i, m := range ranked {
			if m.ID == 27205 || m.Rating < 7 || !contains(m.Genres, "Science Fiction") {
				t.Fatalf("filters ignored: %+v", m)
			}
			if i > 0 && m.Score > ranked[i-1].Score {
				t.Fatal("scores are not ordered")
			}
			if !strings.Contains(m.Reason, "Inception") {
				t.Fatal("missing seed explanation", m.Reason)
			}
		}
	}
	// One liked movie must already influence a cold-start profile.
	personalized := a.rank(RecRequest{ExcludeWatched: true}, acts, Intent{})
	unpersonalized := a.rank(RecRequest{}, map[int]Activity{}, Intent{})
	if personalized[0].ID == 27205 || unpersonalized[0].ID == 0 {
		t.Fatal("one-film profile did not affect recommendations")
	}
	if got := a.rank(RecRequest{MinRating: 10, Genres: []string{"Science Fiction"}}, acts, Intent{}); len(got) != 0 {
		t.Fatal("impossible minimum rating should yield empty results")
	}
}
func TestTurkishIntent(t *testing.T) {
	for _, tc := range []struct{ prompt, genre, keyword string }{{"Uzayda geçen bilim kurgu", "Science Fiction", "space"}, {"İyi hissettiren komedi", "Comedy", ""}, {"Gizemli bir gerilim", "Thriller", ""}, {"Zaman yolculuğu ve bilim kurgu", "Science Fiction", "time travel"}} {
		i := localIntent(tc.prompt)
		if !contains(i.Genres, tc.genre) {
			t.Fatal(tc.prompt, i)
		}
		if tc.keyword != "" && !contains(i.Keywords, tc.keyword) {
			t.Fatal(tc.prompt, i)
		}
	}
}
func TestSessionIsolation(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	id := session(w, r)
	cookies := w.Result().Cookies()
	if len(id) != 64 || len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("invalid cookie")
	}
	r.AddCookie(cookies[0])
	if session(httptest.NewRecorder(), r) != id {
		t.Fatal("session not reused")
	}
	if session(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil)) == id {
		t.Fatal("session collision")
	}
}
func TestInvalidRequests(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{{"POST", "/api/recommendations", `{"prompt":3}`, 400}, {"POST", "/api/recommendations", `{"min_rating":11}`, 400}, {"POST", "/api/recommendations", `{"seed_id":-1}`, 404}, {"POST", "/api/recommendations", `{"genres":["fake"]}`, 400}, {"PUT", "/api/library/27205", `{"rating":11}`, 400}, {"GET", "/api/movies/0", "", 404}, {"GET", "/api/unknown", "", 404}} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/api/recommendations", strings.NewReader(`{}`))
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOllamaIntentValidation(t *testing.T) {
	a := testApp(t)
	a.model = "qwen3:4b"
	a.ollamaURL = "http://ollama.test"
	a.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatal(r.URL)
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["stream"] != false || payload["format"] == nil || payload["think"] != false {
			t.Fatal(payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"message":{"content":"{\"confidence\":0.9,\"genres\":[\"Science Fiction\",\"Invented Genre\"],\"keywords\":[\"space\"]}"}}`)), Header: make(http.Header)}, nil
	})}
	in, err := a.aiIntent(httptest.NewRequest("POST", "/", nil), "uzay")
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Genres) != 1 || in.Genres[0] != "Science Fiction" || !contains(in.Keywords, "space") {
		t.Fatal(in)
	}
}

func TestPostgresFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL yok; gerçek PostgreSQL entegrasyonu atlandı")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	a.db = db
	a.ollamaURL = "http://disabled.test"
	a.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { return nil, io.EOF })}
	db.SetMaxOpenConns(1)
	schemaName := "tmdb_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err = db.ExecContext(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schemaName + " CASCADE")
	if _, err = db.ExecContext(ctx, "SET search_path TO "+schemaName); err != nil {
		t.Fatal(err)
	}
	legacy := `CREATE TABLE movies(id INTEGER PRIMARY KEY,data JSONB NOT NULL);
 CREATE TABLE activities(session_id VARCHAR(64),movie_id INTEGER REFERENCES movies(id) ON DELETE CASCADE,watchlist BOOLEAN DEFAULT FALSE,watched BOOLEAN DEFAULT FALSE,liked BOOLEAN DEFAULT FALSE,rating INTEGER DEFAULT 0,updated_at TIMESTAMPTZ DEFAULT now(),PRIMARY KEY(session_id,movie_id));
 INSERT INTO movies VALUES(27205,'{}');
 INSERT INTO activities(session_id,movie_id,rating) VALUES('legacy',27205,9);`
	if _, err = db.ExecContext(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, schema); err != nil {
		t.Fatal(err)
	}
	var rating int
	if err = db.QueryRowContext(ctx, "SELECT rating FROM activities WHERE session_id='legacy'").Scan(&rating); err != nil || rating != 9 {
		t.Fatal("migration lost preferences", err)
	}
	var table sql.NullString
	if err = db.QueryRowContext(ctx, "SELECT to_regclass('movies')::text").Scan(&table); err != nil || table.Valid {
		t.Fatal("legacy movies table still exists", err)
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/library", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	defer db.Exec("DELETE FROM activities WHERE session_id=$1", cookie.Value)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	request("PUT", "/api/library/27205", `{"watchlist":true,"watched":true,"liked":true,"rating":9}`)
	w = request("GET", "/api/library", "")
	var library map[int]Activity
	_ = json.Unmarshal(w.Body.Bytes(), &library)
	if !library[27205].Watched || library[27205].Rating != 9 {
		t.Fatal(library)
	}
	// The shared fixture has no space evidence; this integration scenario checks genre/profile filtering.
	w = request("POST", "/api/recommendations", `{"genres":["Science Fiction"],"exclude_watched":true,"min_rating":7}`)
	var rec struct {
		Movies []Recommended
		Mode   string
	}
	if err = json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.Movies) == 0 || rec.Mode != "content" {
		t.Fatal(w.Body.String())
	}
	for _, m := range rec.Movies {
		if m.ID == 27205 || m.Rating < 7 {
			t.Fatal("watched/rating filter failed", m)
		}
	}
	w = request("POST", "/api/recommendations", `{"seed_id":27205,"exclude_watched":false,"genres":["Science Fiction"]}`)
	_ = json.Unmarshal(w.Body.Bytes(), &rec)
	for _, m := range rec.Movies {
		if m.ID == 27205 || !contains(m.Genres, "Science Fiction") {
			t.Fatal("seed/genre filter failed", m)
		}
	}
	isolated := httptest.NewRecorder()
	a.routes().ServeHTTP(isolated, httptest.NewRequest("GET", "/api/library", nil))
	if strings.TrimSpace(isolated.Body.String()) != "{}" {
		t.Fatal("another session can see data", isolated.Body.String())
	}
}
