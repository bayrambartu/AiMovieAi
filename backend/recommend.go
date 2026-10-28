package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalized(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'ı', 'İ':
			return 'i'
		case 'ş', 'Ş':
			return 's'
		case 'ğ', 'Ğ':
			return 'g'
		case 'ü', 'Ü':
			return 'u'
		case 'ö', 'Ö':
			return 'o'
		case 'ç', 'Ç':
			return 'c'
		}
		return unicode.ToLower(r)
	}, s)
}
func contains(values []string, s string) bool {
	for _, v := range values {
		if normalized(v) == normalized(s) {
			return true
		}
	}
	return false
}

type RecRequest struct {
	Prompt         string   `json:"prompt"`
	Genres         []string `json:"genres"`
	SeedID         int      `json:"seed_id"`
	ExcludeWatched bool     `json:"exclude_watched"`
	MinRating      float64  `json:"min_rating"`
}
type Recommended struct {
	Movie
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

func features(m Movie) map[string]float64 {
	out := map[string]float64{}
	for _, g := range m.Genres {
		out["g:"+normalized(g)] = 2
	}
	for _, k := range m.Keywords {
		out["k:"+normalized(k)] = 1
	}
	return out
}
func similarity(a, b map[string]float64) float64 {
	inter, union := 0.0, 0.0
	for k, v := range a {
		inter += math.Min(v, b[k])
		union += math.Max(v, b[k])
	}
	for k, v := range b {
		if _, ok := a[k]; !ok {
			union += v
		}
	}
	if union == 0 {
		return 0
	}
	return inter / union
}
func (a *app) recommend(w http.ResponseWriter, r *http.Request) {
	var in RecRequest
	if decode(r, &in) != nil || utf8.RuneCountInString(in.Prompt) > 1000 || len(in.Genres) > 20 || in.MinRating < 0 || in.MinRating > 10 {
		fail(w, 400, "İstek geçersiz. Metin en fazla 1000 karakter, puan 0–10 olmalı.")
		return
	}
	for _, g := range in.Genres {
		if !contains(knownGenres(), g) {
			fail(w, 400, "Bilinmeyen film türü.")
			return
		}
	}
	if in.SeedID < 0 {
		fail(w, 404, "Referans film bulunamadı.")
		return
	}
	intent, mode := a.understand(r, in)
	if mode == "ai_unavailable" {
		fail(w, 503, "Film isteğini yorumlayan yapay zekâya şu anda erişilemiyor. Lütfen biraz sonra tekrar dene.")
		return
	}
	if needsClarification(in, intent) {
		send(w, 200, clarification(intent, "Ne tür bir hikâye arıyorsun? Örneğin zorlukları aşıp hedefine ulaşan birini, dostluğu veya uzay macerasını anlatabilirsin."))
		return
	}
	acts, err := a.activities(r.Context(), activityOwner(w, r))
	if err != nil {
		fail(w, 503, "Tercihleriniz okunamadı.")
		return
	}
	result, err := a.recommendFor(r, in, acts, intent, mode)
	if err != nil {
		tmdbFailure(w, err)
		return
	}
	if result.Status == "ai_unavailable" {
		fail(w, 503, result.Message)
		return
	}
	send(w, 200, result)
}

type rankingSet struct {
	movies []Movie
	byID   map[int]Movie
}

func (a rankingSet) rank(in RecRequest, acts map[int]Activity, intent Intent) []Recommended {
	target := map[string]float64{}
	for _, g := range append(intent.Genres, in.Genres...) {
		target["g:"+normalized(g)] = 2
	}
	for _, k := range intent.Keywords {
		target["k:"+normalized(k)] = 1
	}
	if in.SeedID != 0 {
		for k, v := range features(a.byID[in.SeedID]) {
			target[k] = math.Max(target[k], v)
		}
	}
	// When a request is explicit, preferences serve only as a tie-breaker.
	personal := map[string]float64{}
	for id, v := range acts {
		if v.Liked || v.Rating >= 7 {
			weight := 0.5 + float64(v.Rating)/20
			for k, f := range features(a.byID[id]) {
				personal[k] = math.Min(3, personal[k]+f*weight)
			}
		}
	}
	if len(target) == 0 {
		for k, v := range personal {
			target[k] = v
		}
	}
	ranked := []Recommended{}
	for _, m := range a.movies {
		if m.ID == in.SeedID || m.Rating < in.MinRating {
			continue
		}
		if in.ExcludeWatched && acts[m.ID].Watched {
			continue
		}
		if len(in.Genres) > 0 {
			match := false
			for _, g := range in.Genres {
				if contains(m.Genres, g) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		if excludedMovie(m, intent) {
			continue
		}
		f := features(m)
		sim := similarity(target, f)
		if len(target) > 0 && sim == 0 {
			continue
		}
		quality := m.Rating / 10 * math.Min(1, math.Log10(float64(m.Votes)+1)/4)
		score := 0.8*sim + 0.15*quality + 0.05*similarity(personal, f)
		matches := []string{}
		for _, g := range m.Genres {
			if target["g:"+normalized(g)] > 0 {
				matches = append(matches, genreTR(g))
			}
		}
		reason := "Yüksek izleyici puanı ve geniş değerlendirme sayısıyla keşfetmeye değer."
		if len(matches) > 0 {
			reason = strings.Join(matches, ", ") + " tercihinle örtüşüyor."
		}
		if in.SeedID != 0 && sim > 0 {
			reason = a.byID[in.SeedID].Title + " ile tür ve tema benzerliği taşıyor."
		}
		ranked = append(ranked, Recommended{Movie: m, Score: math.Round(score*1000) / 1000, Reason: reason})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		if ranked[i].Votes != ranked[j].Votes {
			return ranked[i].Votes > ranked[j].Votes
		}
		return ranked[i].ID < ranked[j].ID
	})
	if len(ranked) > 12 {
		ranked = ranked[:12]
	}
	return ranked
}
func genreTR(g string) string {
	m := map[string]string{"Science Fiction": "Bilim kurgu", "Comedy": "Komedi", "Drama": "Dram", "Action": "Aksiyon", "Adventure": "Macera", "Animation": "Animasyon", "Romance": "Romantik", "Thriller": "Gerilim", "Horror": "Korku", "Mystery": "Gizem", "Crime": "Suç", "Fantasy": "Fantastik", "Family": "Aile"}
	if v := m[g]; v != "" {
		return v
	}
	return g
}

// Work with a bounded set fetched on demand, never download the TMDB catalogue.
func (a *app) candidates(ctx context.Context, in RecRequest, acts map[int]Activity, intent Intent) (rankingSet, RecRequest, error) {
	pool := rankingSet{movies: []Movie{}, byID: map[int]Movie{}}
	add := func(ms []Movie) {
		for _, m := range ms {
			if _, ok := pool.byID[m.ID]; !ok {
				pool.movies = append(pool.movies, m)
				pool.byID[m.ID] = m
			}
		}
	}
	seedTitle := strings.TrimSpace(intent.SeedTitle)
	if seedTitle == "" {
		text := strings.TrimSpace(in.Prompt)
		lower := normalized(text)
		for _, suffix := range []string{" gibi", " benzeri"} {
			if i := strings.Index(lower, suffix); i > 0 {
				seedTitle = strings.Trim(string([]rune(text)[:len([]rune(lower[:i]))]), " \"'")
				break
			}
		}

	}
	if in.SeedID == 0 && seedTitle != "" {
		result, err := a.tmdb.page(ctx, "/search/movie", url.Values{"query": {seedTitle}, "include_adult": {"false"}})
		if err != nil {
			return pool, in, err
		}
		for _, m := range result.Movies {
			if normalized(m.Title) == normalized(seedTitle) || normalized(m.OriginalTitle) == normalized(seedTitle) {
				in.SeedID = m.ID
				break
			}
		}
		if in.SeedID == 0 && !semanticRequest(intent) && len(intent.SearchTitles) == 0 {
			return pool, in, nil
		}
	}
	seeds := []int{}
	if in.SeedID > 0 {
		m, err := a.tmdb.movie(ctx, in.SeedID)
		if err != nil {
			return pool, in, err
		}
		pool.byID[m.ID] = m
		seeds = append(seeds, m.ID)
	}
	// Use at most two positive profile entries per request, in stable priority order.
	profiles := []Activity{}
	for _, v := range acts {
		if (v.Liked || v.Rating >= 7) && v.MovieID != in.SeedID && len(intent.Themes) == 0 && len(intent.Keywords) == 0 {
			profiles = append(profiles, v)
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].Rating != profiles[j].Rating {
			return profiles[i].Rating > profiles[j].Rating
		}
		if profiles[i].Liked != profiles[j].Liked {
			return profiles[i].Liked
		}
		return profiles[i].MovieID < profiles[j].MovieID
	})
	if len(profiles) > 2 {
		profiles = profiles[:2]
	}
	for _, v := range profiles {
		m, err := a.tmdb.movie(ctx, v.MovieID)
		if err != nil {
			var e tmdbError
			if errors.As(err, &e) && e.status == 404 {
				continue
			}
			return pool, in, err
		}
		pool.byID[m.ID] = m
		seeds = append(seeds, m.ID)
	}
	for _, id := range seeds {
		result, err := a.tmdb.page(ctx, fmt.Sprintf("/movie/%d/recommendations", id), url.Values{"page": {"1"}})
		if err != nil {
			return pool, in, err
		}
		add(result.Movies)
	}
	// Model-suggested titles are retrieval hints only. Resolve them against TMDB;
	// only TMDB metadata may reach the final evidence-based evaluation.
	hintIDs := []int{}
	for i, title := range intent.SearchTitles {
		if i >= 3 {
			break
		}
		result, err := a.tmdb.page(ctx, "/search/movie", url.Values{"query": {title}, "include_adult": {"false"}})
		if err != nil {
			return pool, in, err
		}
		for _, m := range result.Movies {
			if normalized(m.Title) == normalized(title) || normalized(m.OriginalTitle) == normalized(title) {
				add([]Movie{m})
				hintIDs = append(hintIDs, m.ID)
				break
			}
		}
	}
	selected := append([]string{}, in.Genres...)
	if len(selected) == 0 {
		selected = append(selected, intent.Genres...)
	}
	if len(selected) == 0 {
		for _, id := range seeds {
			for _, g := range pool.byID[id].Genres {
				if !contains(selected, g) {
					selected = append(selected, g)
				}
			}
		}
	}
	genreIDs := []string{}
	for _, g := range selected {
		if id := genreID(g); id > 0 {
			genreIDs = append(genreIDs, strconv.Itoa(id))
		}
	}
	keywordIDs := []string{}
	for i, k := range retrievalKeywords(intent) {
		if i >= 5 {
			break
		}
		var result struct {
			Results []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"results"`
		}
		if err := a.tmdb.get(ctx, "/search/keyword", url.Values{"query": {k}}, &result); err != nil {
			return pool, in, err
		}
		for _, item := range result.Results {
			if normalized(item.Name) == normalized(k) {
				keywordIDs = append(keywordIDs, strconv.Itoa(item.ID))
				break
			}
		}
	}
	// Missing exact tags must not gate free-form topics. Broader candidates
	// still pass semantic evaluation; discovery popularity is never a final match.
	for page := 1; page <= 2; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "include_adult": {"false"}, "sort_by": {"popularity.desc"}, "vote_average.gte": {strconv.FormatFloat(in.MinRating, 'f', 1, 64)}, "vote_count.gte": {"50"}}
		if len(genreIDs) > 0 {
			q.Set("with_genres", strings.Join(genreIDs, "|"))
		}
		if len(keywordIDs) > 0 {
			q.Set("with_keywords", strings.Join(keywordIDs, "|"))
		}
		excluded := []string{}
		for _, g := range intent.ExcludedGenres {
			if id := genreID(g); id > 0 {
				excluded = append(excluded, strconv.Itoa(id))
			}
		}
		if len(excluded) > 0 {
			q.Set("without_genres", strings.Join(excluded, ","))
		}
		result, err := a.tmdb.page(ctx, "/discover/movie", q)
		if err != nil {
			return pool, in, err
		}
		add(result.Movies)
		if result.Pages <= page {
			break
		}
	}
	for _, id := range hintIDs {
		related, err := a.tmdb.page(ctx, fmt.Sprintf("/movie/%d/recommendations", id), url.Values{"page": {"1"}})
		if err != nil {
			return pool, in, err
		}
		add(related.Movies)
	}
	return pool, in, nil
}
