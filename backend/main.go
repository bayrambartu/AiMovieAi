package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schema string

type Movie struct {
	ID               int      `json:"id"`
	Title            string   `json:"title"`
	Year             int      `json:"year"`
	Genres           []string `json:"genres"`
	Language         string   `json:"language"`
	Director         string   `json:"director"`
	Stars            string   `json:"stars"`
	Keywords         []string `json:"keywords"`
	Rating           float64  `json:"rating"`
	Votes            int      `json:"votes"`
	Overview         string   `json:"overview"`
	EvidenceOverview string   `json:"-"`
	PosterURL        string   `json:"poster_url"`
	Runtime          int      `json:"runtime"`
	OriginalTitle    string   `json:"original_title"`
}
type Activity struct {
	MovieID   int  `json:"movie_id"`
	Watchlist bool `json:"watchlist"`
	Watched   bool `json:"watched"`
	Liked     bool `json:"liked"`
	Rating    int  `json:"rating"`
}
type app struct {
	authLimiter      authThrottle
	db               *sql.DB
	tmdb             *tmdbClient
	client           *http.Client
	ollamaURL, model string
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	db, err := sql.Open("postgres", env("DATABASE_URL", "postgres://aimovieai:aimovieai_local@127.0.0.1:5434/aimovieai?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		log.Fatalf("PostgreSQL bağlantısı kurulamadı: %v. README içindeki veritabanı kurulumunu uygulayın.", err)
	}
	if _, err = db.ExecContext(ctx, schema); err != nil {
		log.Fatal(err)
	}
	a := &app{db: db, tmdb: newTMDBClient(), client: &http.Client{Timeout: 65 * time.Second}, ollamaURL: strings.TrimRight(env("OLLAMA_URL", "http://127.0.0.1:11434"), "/"), model: env("OLLAMA_MODEL", "qwen3:4b")}
	if a.tmdb.token == "" {
		log.Print("TMDB_TOKEN eksik; film API istekleri yapılandırma hatası döndürecek.")
	}
	mux := a.routes()
	server := &http.Server{Addr: env("ADDR", "127.0.0.1:8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 155 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	log.Printf("aimovieai: http://%s — TMDB canlı katalog", server.Addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	a.authRoutes(mux)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, c := context.WithTimeout(r.Context(), 2*time.Second)
		defer c()
		if a.db.PingContext(ctx) != nil {
			fail(w, 503, "Veritabanına erişilemiyor.")
			return
		}
		send(w, 200, map[string]any{"status": "ok", "catalog_source": "tmdb", "tmdb_configured": a.tmdb.token != "", "ai_model": a.model})
	})
	mux.HandleFunc("GET /api/movies", a.listMovies)
	mux.HandleFunc("GET /api/movies/{id}", a.movieDetails)
	mux.HandleFunc("GET /api/genres", func(w http.ResponseWriter, r *http.Request) { send(w, 200, knownGenres()) })
	mux.HandleFunc("GET /api/library", a.getLibrary)
	mux.HandleFunc("PUT /api/library/{id}", a.putLibrary)
	mux.HandleFunc("POST /api/recommendations", a.recommend)
	dist := env("FRONTEND_DIST", "../frontend/dist")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fail(w, 404, "Endpoint bulunamadı.")
			return
		}
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		path := filepath.Join(dist, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
			fail(w, 503, "Frontend için npm run build çalıştırın veya localhost:5173 adresini kullanın.")
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == "PUT" || r.Method == "POST" {
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
				fail(w, 403, "Siteler arası istek reddedildi.")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host) {
					fail(w, 403, "Siteler arası istek reddedildi.")
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		}
		ctx, c := context.WithTimeout(r.Context(), 145*time.Second)
		defer c()
		r = r.WithContext(ctx)
		if r.URL.Path == "/api/library" || strings.HasPrefix(r.URL.Path, "/api/library/") || r.URL.Path == "/api/recommendations" {
			var ok bool
			r, ok = a.withIdentity(w, r)
			if !ok {
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	send(w, status, map[string]string{"error": msg})
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("tek JSON nesnesi gerekli")
	}
	return nil
}
func session(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("aimovieai_session"); err == nil && len(c.Value) == 64 {
		if _, err := hex.DecodeString(c.Value); err == nil {
			return c.Value
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	id := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "aimovieai_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 365 * 24 * 60 * 60})
	return id
}
func (a *app) activities(ctx context.Context, sid string) (map[int]Activity, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT movie_id,watchlist,watched,liked,rating FROM activities WHERE session_id=$1", sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]Activity{}
	for rows.Next() {
		var v Activity
		if err = rows.Scan(&v.MovieID, &v.Watchlist, &v.Watched, &v.Liked, &v.Rating); err != nil {
			return nil, err
		}
		out[v.MovieID] = v
	}
	return out, rows.Err()
}
func (a *app) getLibrary(w http.ResponseWriter, r *http.Request) {
	out, err := a.activities(r.Context(), activityOwner(w, r))
	if err != nil {
		fail(w, 503, "Film listeniz yüklenemedi.")
		return
	}
	send(w, 200, out)
}
func (a *app) putLibrary(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if id <= 0 {
		fail(w, 404, "Film bulunamadı.")
		return
	}
	var v Activity
	if decode(r, &v) != nil || v.Rating < 0 || v.Rating > 10 {
		fail(w, 400, "Geçerli bir liste kaydı ve 0–10 arası puan gerekli.")
		return
	}
	if _, err := a.tmdb.movie(r.Context(), id); err != nil {
		tmdbFailure(w, err)
		return
	}
	v.MovieID = id
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO activities(session_id,movie_id,watchlist,watched,liked,rating) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(session_id,movie_id) DO UPDATE SET watchlist=$3,watched=$4,liked=$5,rating=$6,updated_at=now()`, activityOwner(w, r), id, v.Watchlist, v.Watched, v.Liked, v.Rating)
	if err != nil {
		fail(w, 503, "Değişiklik kaydedilemedi.")
		return
	}
	send(w, 200, v)
}
