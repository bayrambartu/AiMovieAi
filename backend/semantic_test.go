package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAchievementIsStoryNotGenre(t *testing.T) {
	for _, prompt := range []string{"başarı anlatan film öner", "azim ve başarı hakkında bir film", "hayallerinin peşinden giden birini anlatan film"} {
		in := localIntent(prompt)
		if !contains(in.Themes, "başarı ve azim") || len(in.Genres) != 0 || in.StoryArc == "" || needsClarification(RecRequest{Prompt: prompt}, in) {
			t.Fatalf("%s: %+v", prompt, in)
		}
		if !contains(retrievalKeywords(in), "underdog") {
			t.Fatal("missing theme retrieval")
		}
	}
	for _, prompt := range []string{"başka bir film", "asdkj qwerty", "iyi bir şey öner"} {
		if !needsClarification(RecRequest{Prompt: prompt}, localIntent(prompt)) {
			t.Fatal("vague request treated as recognized", prompt)
		}
	}
	in := localIntent("başarı anlatan film öner, korku olmasın")
	if contains(in.Genres, "Horror") || !contains(in.ExcludedGenres, "Horror") {
		t.Fatal(in)
	}
}
func TestUnknownRequestClarifiesWithoutDBOrTMDB(t *testing.T) {
	a := testApp(t)
	a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return modelReply(Intent{NeedsClarification: true, Confidence: .1}), nil
	})
	var calls atomic.Int32
	a.tmdb.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { calls.Add(1); return jsonResponse(500, nil), nil })
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("POST", "/api/recommendations", strings.NewReader(`{"prompt":"asdkj qwerty"}`)))
	if w.Code != 200 || calls.Load() != 0 {
		t.Fatal(w.Code, calls.Load(), w.Body.String())
	}
	var res recommendationResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != "needs_clarification" || len(res.Movies) != 0 || res.Question == "" {
		t.Fatal(res)
	}
}
func achievementPool() rankingSet {
	movies := []Movie{
		{ID: 155, Title: "The Dark Knight", Genres: []string{"Action", "Crime"}, Rating: 9.9, Votes: 100000, Overview: "Batman confronts a criminal who brings chaos to Gotham City.", Keywords: []string{"superhero", "crime"}},
		{ID: 1, Title: "A goal worth pursuing", Genres: []string{"Drama"}, Rating: 7.2, Votes: 100, Overview: "An aspiring runner overcomes adversity and achieves her dream of competing professionally.", Keywords: []string{"underdog", "perseverance"}},
		{ID: 2, Title: "A detective", Genres: []string{"Crime"}, Rating: 9, Votes: 20000, Overview: "A successful detective investigates a mysterious disappearance.", Keywords: []string{"detective"}},
		{ID: 3, Title: "Excluded horror", Genres: []string{"Horror"}, Rating: 8, Overview: "An underdog achieves his dream.", Keywords: []string{"underdog"}},
		{ID: 4, Title: "Already watched", Genres: []string{"Drama"}, Rating: 8, Overview: "An underdog achieves his dream.", Keywords: []string{"underdog"}},
	}
	byID := map[int]Movie{}
	for _, m := range movies {
		byID[m.ID] = m
	}
	return rankingSet{movies: movies, byID: byID}
}
func semanticTMDB(t *testing.T) *tmdbClient {
	t.Helper()
	c := mockTMDB(t)
	pool := achievementPool()
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		path := strings.TrimPrefix(r.URL.Path, "/3")
		raw := func(m Movie) map[string]any {
			ids := []int{}
			for _, g := range m.Genres {
				ids = append(ids, genreID(g))
			}
			keys := []map[string]string{}
			for _, k := range m.Keywords {
				keys = append(keys, map[string]string{"name": k})
			}
			return map[string]any{"id": m.ID, "title": m.Title, "genre_ids": ids, "vote_average": m.Rating, "vote_count": m.Votes, "overview": m.Overview, "keywords": map[string]any{"keywords": keys}}
		}
		if path == "/search/keyword" {
			return jsonResponse(200, map[string]any{"results": []map[string]any{{"id": 10, "name": r.URL.Query().Get("query")}}}), nil
		}
		if path == "/discover/movie" {
			if r.URL.Query().Get("with_keywords") == "" {
				t.Error("unconstrained popular discovery for semantic request")
			}
			results := []map[string]any{}
			for _, m := range pool.movies {
				results = append(results, raw(m))
			}
			return jsonResponse(200, map[string]any{"results": results, "total_pages": 1, "page": 1}), nil
		}
		if strings.HasPrefix(path, "/movie/") {
			id, _ := strconv.Atoi(strings.TrimPrefix(path, "/movie/"))
			if m, ok := pool.byID[id]; ok {
				return jsonResponse(200, raw(m)), nil
			}
		}
		t.Errorf("unexpected request: %s", path)
		return jsonResponse(404, nil), nil
	})
	return c
}
func TestAchievementPipelineRejectsPopularUnrelatedFilms(t *testing.T) {
	a := testApp(t)
	a.tmdb = semanticTMDB(t)
	req := RecRequest{Prompt: "başarı anlatan film öner, korku olmasın", ExcludeWatched: true}
	intent := localIntent(req.Prompt)
	result, err := a.recommendFor(httptest.NewRequest("POST", "/", nil), req, map[int]Activity{4: {MovieID: 4, Watched: true}}, intent, "content")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.Evaluation != "evidence_rules" || len(result.Movies) != 1 || result.Movies[0].ID != 1 {
		t.Fatalf("unrelated popular movie leaked: %+v", result)
	}
	if !strings.Contains(result.Movies[0].Reason, "underdog") && !strings.Contains(result.Movies[0].Reason, "overcomes adversity") {
		t.Fatal("ungrounded reason", result.Movies[0].Reason)
	}
}
func TestUnresolvedKeywordBroadensRetrievalWithoutAcceptingUnverifiedMovies(t *testing.T) {
	a := testApp(t)
	var discover atomic.Int32
	a.tmdb.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "discover") {
			discover.Add(1)
		}
		return jsonResponse(200, map[string]any{"results": []map[string]any{{"id": 9, "name": "unrelated near-match"}}}), nil
	})
	intent := localIntent("başarı anlatan film öner")
	pool, _, err := a.candidates(context.Background(), RecRequest{Prompt: "başarı anlatan film öner"}, nil, intent)
	if err != nil || len(pool.movies) != 1 || discover.Load() != 1 {
		t.Fatal(err, len(pool.movies), discover.Load())
	}
	if out := evidenceRank(RecRequest{}, intent, pool); len(out) != 0 {
		t.Fatal("broad retrieval accepted unrelated movie", out)
	}
}
func modelReply(value any) *http.Response {
	content, _ := json.Marshal(value)
	return jsonResponse(200, map[string]any{"message": map[string]any{"content": string(content)}})
}
func TestSemanticModelEvidenceAndRejection(t *testing.T) {
	pool := achievementPool()
	pool.movies = pool.movies[:3]
	intent := localIntent("başarı anlatan film öner")
	request := httptest.NewRequest("POST", "/", nil)
	for _, tc := range []struct {
		name  string
		items []evaluationItem
		want  int
		mode  string
	}{
		{"supported", []evaluationItem{{ID: 1, Relevance: .95, Evidence: "An aspiring runner overcomes adversity"}, {ID: 155, Relevance: .2}}, 1, "model_overview"},
		{"model rejects all", []evaluationItem{}, 0, "model_overview"},
		{"invented quote rejected", []evaluationItem{{ID: 155, Relevance: .99, Evidence: "Batman becomes a successful entrepreneur"}}, 1, "evidence_rules"},
		{"invented id rejected", []evaluationItem{{ID: 999, Relevance: .99, Evidence: "underdog"}}, 1, "evidence_rules"},
		{"valid item survives invalid neighbor", []evaluationItem{{ID: 1, Relevance: .95, Evidence: "underdog"}, {ID: 999, Relevance: .99, Evidence: "fabricated"}}, 1, "model_overview"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			a.ollamaURL = "http://model.test"
			a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				return modelReply(map[string]any{"evaluations": tc.items}), nil
			})
			out, mode := a.evaluate(request, RecRequest{Prompt: "başarı anlatan film öner"}, intent, pool)
			if len(out) != tc.want || mode != tc.mode {
				t.Fatal(out, mode)
			}
			for _, m := range out {
				if m.ID != 1 {
					t.Fatal("unrelated film accepted", m)
				}
			}
		})
	}
}
func TestGroundedEvidenceAndMultipleThemes(t *testing.T) {
	m := achievementPool().byID[1]
	if _, ok := groundedEvidence(m, "A fabricated success story"); ok {
		t.Fatal("fabricated quote accepted")
	}
	if _, ok := groundedEvidence(m, "underdog"); !ok {
		t.Fatal("real keyword rejected")
	}
	in := localIntent("başarı ve dostluk anlatan film")
	pool := achievementPool()
	out := evidenceRank(RecRequest{}, in, pool)
	if len(out) != 0 {
		t.Fatal("one theme ignored", out)
	}
}
func TestIntentConfidenceAndInventedReferences(t *testing.T) {
	a := testApp(t)
	a.ollamaURL = "http://model.test"
	for _, tc := range []struct {
		in      Intent
		clarify bool
	}{
		{Intent{Genres: []string{"Action"}, Confidence: .2}, true},
		{Intent{SeedTitle: "Inception", Genres: []string{"Action"}, Confidence: .9}, true},
	} {
		a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return modelReply(tc.in), nil })
		request := httptest.NewRequest("POST", "/", nil)
		in, _ := a.understand(request, RecRequest{Prompt: "asdkj qwerty"})
		if !needsClarification(RecRequest{Prompt: "asdkj qwerty"}, in) {
			t.Fatal("low confidence/invented title accepted", in)
		}
	}
}
