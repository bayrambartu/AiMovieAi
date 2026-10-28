package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func jsonResponse(status int, value any) *http.Response {
	b, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}
}
func mockTMDB(t *testing.T) *tmdbClient {
	t.Helper()
	return &tmdbClient{baseURL: "https://tmdb.test/3", token: "test-token-never-expose", cache: map[string]cacheEntry{}, http: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer test-token-never-expose" || r.URL.Query().Get("language") != "tr-TR" {
			t.Error("authentication or language missing")
		}
		raw := []map[string]any{}
		for _, m := range fixtureMovies() {
			ids := []int{}
			for _, g := range m.Genres {
				ids = append(ids, genreID(g))
			}
			raw = append(raw, map[string]any{"id": m.ID, "title": m.Title, "original_title": m.OriginalTitle, "genre_ids": ids, "release_date": "2010-07-16", "vote_average": m.Rating, "vote_count": m.Votes, "poster_path": "/poster.jpg", "original_language": "en"})
		}
		path := strings.TrimPrefix(r.URL.Path, "/3")
		switch {
		case path == "/search/keyword":
			return jsonResponse(200, map[string]any{"results": []map[string]any{{"id": 123, "name": "space"}}}), nil
		case path == "/search/movie":
			return jsonResponse(200, map[string]any{"results": raw[:1], "page": 1, "total_pages": 1, "total_results": 1}), nil
		case path == "/discover/movie" || strings.HasSuffix(path, "/recommendations"):
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page == 0 {
				page = 1
			}
			return jsonResponse(200, map[string]any{"results": raw, "page": page, "total_pages": 2, "total_results": 8}), nil
		case strings.HasPrefix(path, "/movie/"):
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/movie/"))
			for _, m := range raw {
				if m["id"] == id {
					m["genres"] = []map[string]any{{"id": 878, "name": "Bilim-Kurgu"}}
					m["credits"] = map[string]any{"cast": []map[string]string{{"name": "Actor"}}, "crew": []map[string]string{{"name": "Director", "job": "Director"}}}
					m["keywords"] = map[string]any{"keywords": []map[string]string{{"name": "dream"}}}
					m["overview"] = "Gerçek TMDB özet alanı"
					m["runtime"] = 148
					return jsonResponse(200, m), nil
				}
			}
			return jsonResponse(404, map[string]any{}), nil
		default:
			t.Errorf("unexpected TMDB path: %s", path)
			return jsonResponse(500, map[string]any{}), nil
		}
	})}}
}
func TestTMDBDetailsAndBoundedCandidates(t *testing.T) {
	a := testApp(t)
	m, err := a.tmdb.movie(context.Background(), 27205)
	if err != nil {
		t.Fatal(err)
	}
	if m.PosterURL != "https://image.tmdb.org/t/p/w500/poster.jpg" || m.Runtime != 148 || m.Director != "Director" || !contains(m.Keywords, "dream") {
		t.Fatal(m)
	}
	acts := map[int]Activity{27205: {MovieID: 27205, Watched: true, Liked: true, Rating: 9}}
	pool, in, err := a.candidates(context.Background(), RecRequest{Prompt: "Inception gibi", ExcludeWatched: true, MinRating: 7, Genres: []string{"Science Fiction"}}, acts, localIntent("uzay bilim kurgu"))
	if err != nil {
		t.Fatal(err)
	}
	if in.SeedID != 27205 || len(pool.movies) != 3 {
		t.Fatal(in, len(pool.movies))
	}
	ranked := pool.rank(in, acts, Intent{})
	if len(ranked) != 2 {
		t.Fatal(ranked)
	}
	for _, m := range ranked {
		if m.ID == 27205 || m.Rating < 7 || !contains(m.Genres, "Science Fiction") {
			t.Fatal(m)
		}
	}
}
func TestTMDBDiscoverSearchParameters(t *testing.T) {
	a := testApp(t)
	base := a.tmdb.http.Transport
	var seen []*http.Request
	a.tmdb.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { seen = append(seen, r); return base.RoundTrip(r) })
	for _, path := range []string{"/api/movies?genre=Science+Fiction&sort=rating&page=2", "/api/movies?q=Inception&page=1"} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	if len(seen) != 2 {
		t.Fatal(len(seen))
	}
	q := seen[0].URL.Query()
	if q.Get("with_genres") != "878" || q.Get("sort_by") != "vote_average.desc" || q.Get("page") != "2" || q.Get("vote_count.gte") != "100" {
		t.Fatal(q)
	}
	if !strings.HasSuffix(seen[1].URL.Path, "/search/movie") || seen[1].URL.Query().Get("query") != "Inception" {
		t.Fatal(seen[1].URL)
	}
}
func TestTMDBCacheExpiryAndConcurrentAccess(t *testing.T) {
	c := mockTMDB(t)
	base := c.http.Transport
	var calls atomic.Int32
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { calls.Add(1); return base.RoundTrip(r) })
	for i := 0; i < 2; i++ {
		if _, err := c.movie(context.Background(), 27205); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cache missed")
	}
	c.mu.Lock()
	for k, v := range c.cache {
		v.expires = time.Now().Add(-time.Minute)
		c.cache[k] = v
	}
	c.mu.Unlock()
	if _, err := c.movie(context.Background(), 27205); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("expired data reused")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.movie(context.Background(), 27205); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	c.mu.Lock()
	c.cache = map[string]cacheEntry{}
	for i := 0; i < 256; i++ {
		c.cache[strconv.Itoa(i)] = cacheEntry{expires: time.Now().Add(time.Minute)}
	}
	c.mu.Unlock()
	_, _ = c.movie(context.Background(), 27205)
	if len(c.cache) != 256 {
		t.Fatal("cache capacity exceeded")
	}
}
func TestTMDBErrorsNeverExposeToken(t *testing.T) {
	for _, status := range []int{401, 404, 429, 500} {
		a := testApp(t)
		a.tmdb.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return jsonResponse(status, map[string]any{}), nil })
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/movies/27205", nil))
		want := 502
		if status == 404 || status == 429 {
			want = status
		}
		if w.Code != want || strings.Contains(w.Body.String(), a.tmdb.token) {
			t.Fatal(status, w.Code, w.Body.String())
		}
	}
	c := newTMDBClient()
	c.token = ""
	var out any
	if err := c.get(context.Background(), "/test", url.Values{}, &out); err != errTMDBConfig {
		t.Fatal(err)
	}
}
