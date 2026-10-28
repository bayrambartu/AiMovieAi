package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Only TMDB's genre vocabulary is local. No catalogue is persisted.
var tmdbGenres = map[int]string{28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama", 10751: "Family", 14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music", 9648: "Mystery", 10749: "Romance", 878: "Science Fiction", 10770: "TV Movie", 53: "Thriller", 10752: "War", 37: "Western"}

func knownGenres() []string {
	out := make([]string, 0, len(tmdbGenres))
	for _, name := range tmdbGenres {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func genreID(name string) int {
	for id, g := range tmdbGenres {
		if normalized(g) == normalized(name) {
			return id
		}
	}
	return 0
}

type cacheEntry struct {
	data    []byte
	expires time.Time
}
type tmdbClient struct {
	baseURL, token string
	http           *http.Client
	mu             sync.Mutex
	cache          map[string]cacheEntry
}

func newTMDBClient() *tmdbClient {
	return &tmdbClient{baseURL: strings.TrimRight(env("TMDB_BASE_URL", "https://api.themoviedb.org/3"), "/"), token: strings.TrimSpace(env("TMDB_TOKEN", "")), http: &http.Client{Timeout: 10 * time.Second}, cache: map[string]cacheEntry{}}
}

type tmdbError struct{ status int }

func (e tmdbError) Error() string { return fmt.Sprintf("TMDB HTTP %d", e.status) }

var errTMDBConfig = errors.New("TMDB_TOKEN eksik")

func (c *tmdbClient) get(ctx context.Context, path string, q url.Values, out any) error {
	if c.token == "" {
		return errTMDBConfig
	}
	if q == nil {
		q = url.Values{}
	}
	if q.Get("language") == "" {
		q.Set("language", "tr-TR")
	}
	key := path + "?" + q.Encode()
	c.mu.Lock()
	entry, ok := c.cache[key]
	c.mu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return json.Unmarshal(entry.data, out)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+key, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return tmdbError{res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 2<<20 {
		return errors.New("TMDB yanıtı fazla büyük")
	}
	if err = json.Unmarshal(data, out); err != nil {
		return err
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]cacheEntry{}
	}
	if len(c.cache) >= 256 {
		oldestKey := ""
		var oldest time.Time
		for k, v := range c.cache {
			if oldestKey == "" || v.expires.Before(oldest) {
				oldestKey = k
				oldest = v.expires
			}
		}
		delete(c.cache, oldestKey)
	}
	c.cache[key] = cacheEntry{data: data, expires: time.Now().Add(10 * time.Minute)}
	c.mu.Unlock()
	return nil
}

type tmdbMovie struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	GenreIDs      []int  `json:"genre_ids"`
	Genres        []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
	Language string  `json:"original_language"`
	Rating   float64 `json:"vote_average"`
	Votes    int     `json:"vote_count"`
	Overview string  `json:"overview"`
	Poster   string  `json:"poster_path"`
	Runtime  int     `json:"runtime"`
	Adult    bool    `json:"adult"`
	Credits  struct {
		Cast []struct {
			Name string `json:"name"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	Keywords struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"keywords"`
	} `json:"keywords"`
}

func (m tmdbMovie) movie() Movie {
	year := 0
	if len(m.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(m.ReleaseDate[:4])
	}
	title := m.Title
	if title == "" {
		title = m.OriginalTitle
	}
	out := Movie{ID: m.ID, Title: title, OriginalTitle: m.OriginalTitle, Year: year, Language: m.Language, Rating: m.Rating, Votes: m.Votes, Overview: m.Overview, Runtime: m.Runtime, Genres: []string{}, Keywords: []string{}}
	ids := m.GenreIDs
	for _, g := range m.Genres {
		ids = append(ids, g.ID)
	}
	for _, id := range ids {
		if g, ok := tmdbGenres[id]; ok && !contains(out.Genres, g) {
			out.Genres = append(out.Genres, g)
		}
	}
	if strings.HasPrefix(m.Poster, "/") {
		out.PosterURL = "https://image.tmdb.org/t/p/w500" + m.Poster
	}
	directors, stars := []string{}, []string{}
	for _, v := range m.Credits.Crew {
		if v.Job == "Director" {
			directors = append(directors, v.Name)
		}
	}
	for i, v := range m.Credits.Cast {
		if i >= 6 {
			break
		}
		stars = append(stars, v.Name)
	}
	out.Director = strings.Join(directors, ", ")
	out.Stars = strings.Join(stars, ", ")
	for _, k := range m.Keywords.Items {
		out.Keywords = append(out.Keywords, k.Name)
	}
	return out
}

type moviePage struct {
	Movies []Movie `json:"movies"`
	Total  int     `json:"total"`
	Page   int     `json:"page"`
	Pages  int     `json:"pages"`
}

func (c *tmdbClient) page(ctx context.Context, path string, q url.Values) (moviePage, error) {
	var raw struct {
		Results []tmdbMovie `json:"results"`
		Total   int         `json:"total_results"`
		Page    int         `json:"page"`
		Pages   int         `json:"total_pages"`
	}
	err := c.get(ctx, path, q, &raw)
	if err != nil {
		return moviePage{}, err
	}
	p := moviePage{Movies: []Movie{}, Total: raw.Total, Page: raw.Page, Pages: min(raw.Pages, 500)}
	for _, m := range raw.Results {
		if !m.Adult && m.ID > 0 {
			p.Movies = append(p.Movies, m.movie())
		}
	}
	return p, nil
}
func (c *tmdbClient) movie(ctx context.Context, id int) (Movie, error) {
	return c.movieLanguage(ctx, id, "tr-TR")
}
func (c *tmdbClient) movieLanguage(ctx context.Context, id int, language string) (Movie, error) {
	var raw tmdbMovie
	err := c.get(ctx, fmt.Sprintf("/movie/%d", id), url.Values{"append_to_response": {"credits,keywords"}, "language": {language}}, &raw)
	if err != nil {
		return Movie{}, err
	}
	if raw.Adult || raw.ID <= 0 {
		return Movie{}, tmdbError{404}
	}
	return raw.movie(), nil
}

func tmdbFailure(w http.ResponseWriter, err error) {
	var upstream tmdbError
	switch {
	case errors.Is(err, errTMDBConfig):
		fail(w, 503, "TMDB_TOKEN yapılandırılmamış. Sunucunun .env dosyasına TMDB API Read Access Token ekleyin.")
	case errors.As(err, &upstream) && upstream.status == 404:
		fail(w, 404, "Film TMDB üzerinde bulunamadı.")
	case errors.As(err, &upstream) && upstream.status == 429:
		w.Header().Set("Retry-After", "10")
		fail(w, 429, "TMDB istek sınırına ulaşıldı. Kısa bir süre sonra tekrar deneyin.")
	default:
		fail(w, 502, "TMDB'ye erişilemiyor. API token ve bağlantı ayarlarını kontrol edin.")
	}
}
func (a *app) movieDetails(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if id <= 0 {
		fail(w, 404, "Film bulunamadı.")
		return
	}
	m, err := a.tmdb.movie(r.Context(), id)
	if err != nil {
		tmdbFailure(w, err)
		return
	}
	send(w, 200, m)
}
func (a *app) listMovies(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := 1
	if q.Get("page") != "" {
		var err error
		page, err = strconv.Atoi(q.Get("page"))
		if err != nil || page < 1 || page > 500 {
			fail(w, 400, "Sayfa 1–500 arasında olmalı.")
			return
		}
	}
	text := strings.TrimSpace(q.Get("q"))
	genre := q.Get("genre")
	order := q.Get("sort")
	if len(text) > 500 {
		fail(w, 400, "Arama metni çok uzun.")
		return
	}
	if genre != "" && genreID(genre) == 0 {
		fail(w, 400, "Bilinmeyen film türü.")
		return
	}
	sorts := map[string]string{"": "vote_count.desc", "popular": "vote_count.desc", "rating": "vote_average.desc", "year": "primary_release_date.desc"}
	sortBy, ok := sorts[order]
	if !ok {
		fail(w, 400, "Bilinmeyen sıralama.")
		return
	}
	params := url.Values{"page": {strconv.Itoa(page)}, "include_adult": {"false"}}
	path := "/discover/movie"
	if text != "" {
		if genre != "" || (order != "" && order != "popular") {
			fail(w, 400, "TMDB başlık araması tür/sıralama filtresiyle birleştirilemez.")
			return
		}
		path = "/search/movie"
		params.Set("query", text)
	} else {
		params.Set("sort_by", sortBy)
		params.Set("vote_count.gte", "100")
		params.Set("primary_release_date.lte", time.Now().Format("2006-01-02"))
		if genre != "" {
			params.Set("with_genres", strconv.Itoa(genreID(genre)))
		}
	}
	result, err := a.tmdb.page(r.Context(), path, params)
	if err != nil {
		tmdbFailure(w, err)
		return
	}
	send(w, 200, result)
}
