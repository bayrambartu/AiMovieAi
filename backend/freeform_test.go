package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFreeformTopicsAreNotRestrictedToLocalRules(t *testing.T) {
	for _, topic := range []string{"zombi salgınından kaçan aile", "deniz fenerinde yalnız yaşayan birinin hikâyesi", "hafızasını kaybeden bir müzisyen"} {
		t.Run(topic, func(t *testing.T) {
			a := testApp(t)
			a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				return modelReply(Intent{Themes: []string{topic}, StoryArc: topic, Confidence: .95}), nil
			})
			req := RecRequest{Prompt: topic}
			in, mode := a.understand(httptest.NewRequest("POST", "/", nil), req)
			if mode != "ollama" || needsClarification(req, in) || !contains(in.Themes, topic) {
				t.Fatal(in, mode)
			}
		})
	}
	// A free-form arc or mood alone must also count as understood.
	for _, in := range []Intent{{StoryArc: "bilinmeyen bir gezegende mahsur kalma", Confidence: .9}, {Moods: []string{"tekinsiz bir yalnızlık"}, Confidence: .9}} {
		if needsClarification(RecRequest{Prompt: "özel hikâye"}, in) {
			t.Fatal(in)
		}
	}
}
func TestModelFailureIsNotReportedAsUserAmbiguity(t *testing.T) {
	a := testApp(t)
	a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) { return nil, io.EOF })
	for _, topic := range []string{"zombi filmi öner", "korku filmi öner"} {
		body, _ := json.Marshal(RecRequest{Prompt: topic})
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("POST", "/api/recommendations", strings.NewReader(string(body))))
		if w.Code != 503 || strings.Contains(w.Body.String(), "Ne tür") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestModelTitleHintsResolveRealTMDBMovies(t *testing.T) {
	a := testApp(t)
	a.tmdb.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/search/keyword") {
			return jsonResponse(200, map[string]any{"results": []any{}}), nil
		}
		if strings.HasSuffix(path, "/search/movie") {
			if r.URL.Query().Get("query") != "A Real Candidate" {
				return jsonResponse(200, map[string]any{"results": []any{}}), nil
			}
			return jsonResponse(200, map[string]any{"results": []map[string]any{{"id": 123, "title": "A Real Candidate", "original_title": "A Real Candidate", "overview": "Survivors escape a zombie outbreak."}}, "total_pages": 1}), nil
		}
		return jsonResponse(200, map[string]any{"results": []any{}, "total_pages": 1}), nil
	})
	in := Intent{SeedTitle: "zombi filmi", Themes: []string{"zombi salgını"}, Keywords: []string{"missing-tag"}, SearchTitles: []string{"A Real Candidate"}, Confidence: .9}
	pool, _, err := a.candidates(httptest.NewRequest("POST", "/", nil).Context(), RecRequest{}, nil, in)
	if err != nil || len(pool.movies) != 1 || pool.movies[0].ID != 123 {
		t.Fatal(pool, err)
	}
	// Acceptance is based on the retrieved overview, not the model title hint.
	a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return modelReply(map[string]any{"evaluations": []evaluationItem{{ID: 123, Relevance: .95, Evidence: "Survivors escape a zombie outbreak."}}}), nil
	})
	out, mode := a.evaluate(httptest.NewRequest("POST", "/", nil), RecRequest{Prompt: "zombi filmi"}, in, pool)
	if mode != "model_overview" || len(out) != 1 {
		t.Fatal(out, mode)
	}
}

func TestFreeformEvaluationFailureDoesNotSilentlyUseTopicRules(t *testing.T) {
	a := testApp(t)
	a.tmdb = semanticTMDB(t)
	in := localIntent("başarı anlatan film öner")
	result, err := a.recommendFor(httptest.NewRequest("POST", "/", nil), RecRequest{Prompt: "başarı anlatan film öner"}, nil, in, "ollama")
	if err != nil || result.Status != "ai_unavailable" || len(result.Movies) != 0 || result.CandidateCount == 0 {
		t.Fatal(result, err)
	}
}

func TestContradictoryInferredGenreDoesNotBlockClearTopic(t *testing.T) {
	a := testApp(t)
	a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return modelReply(Intent{Themes: []string{"serbest konu"}, Genres: []string{"Horror"}, ExcludedGenres: []string{"Horror"}, Confidence: .95}), nil
	})
	request := httptest.NewRequest("POST", "/", nil)
	in, _ := a.understand(request, RecRequest{Prompt: "zombi filmi öner"})
	if needsClarification(RecRequest{Prompt: "zombi filmi öner"}, in) || contains(in.ExcludedGenres, "Horror") {
		t.Fatal(in)
	}
	in, _ = a.understand(request, RecRequest{Prompt: "serbest konu, korku olmasın"})
	if contains(in.Genres, "Horror") || !contains(in.ExcludedGenres, "Horror") {
		t.Fatal("explicit exclusion lost", in)
	}
}

func TestEnglishEvidencePreservesTurkishDisplay(t *testing.T) {
	a := testApp(t)
	m := Movie{ID: 9, Title: "Deniz Feneri", Overview: "Bir deniz feneri bekçisinin yalnızlığı.", EvidenceOverview: "A lonely lighthouse keeper lives on an isolated island."}
	pool := rankingSet{movies: []Movie{m}, byID: map[int]Movie{9: m}}
	a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Messages[len(payload.Messages)-1].Content, "A lonely lighthouse keeper") {
			t.Fatal("English evidence missing")
		}
		return modelReply(map[string]any{"evaluations": []evaluationItem{{ID: 9, Relevance: .95, Evidence: "A lonely lighthouse keeper"}}}), nil
	})
	out, mode := a.evaluate(httptest.NewRequest("POST", "/", nil), RecRequest{Prompt: "deniz feneri bekçisi"}, Intent{StoryArc: "A lonely lighthouse keeper"}, pool)
	if mode != "model_overview" || len(out) != 1 || out[0].Title != m.Title || out[0].Overview != m.Overview {
		t.Fatal(out, mode)
	}
}

func TestModelEvidenceIndexesMustReferToSuppliedText(t *testing.T) {
	m := Movie{ID: 9, Overview: "A lighthouse keeper lives alone on an island."}
	pool := rankingSet{movies: []Movie{m}, byID: map[int]Movie{9: m}}
	for _, index := range []int{0, 99} {
		a := testApp(t)
		a.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			return modelReply(map[string]any{"evaluations": []evaluationItem{{ID: 9, Relevance: .95, EvidenceID: &index}}}), nil
		})
		out, _ := a.evaluate(httptest.NewRequest("POST", "/", nil), RecRequest{}, Intent{StoryArc: "A lighthouse keeper"}, pool)
		if index == 0 {
			if len(out) != 1 || !strings.Contains(out[0].Reason, "A lighthouse keeper lives alone") {
				t.Fatal(out)
			}
		} else if len(out) != 0 {
			t.Fatal("fabricated evidence index accepted", out)
		}
	}
}

func TestReasonPrefersRequestedTopicOverIncidentalMetadata(t *testing.T) {
	m := Movie{Overview: "People struggle to survive a zombie pandemic.", Keywords: []string{"philadelphia, pennsylvania", "zombie"}}
	proof := strongestEvidence(m, Intent{Keywords: []string{"zombie"}}, "philadelphia, pennsylvania")
	if !strings.Contains(proof, "zombie") {
		t.Fatal(proof)
	}
	if _, ok := groundedEvidence(m, proof); !ok {
		t.Fatal("reason no longer source backed", proof)
	}
}

func TestPrimarySubjectIsNotDilutedBySecondarySetting(t *testing.T) {
	in := Intent{Keywords: []string{"lighthouse", "island"}}
	out := []Recommended{{Movie: Movie{ID: 1, Overview: "A lone lighthouse keeper lives on an island."}}, {Movie: Movie{ID: 2, Overview: "A robot is stranded on an island."}}}
	if got := preferPrimarySubject(out, in); len(got) != 1 || got[0].ID != 1 {
		t.Fatal(got)
	}
	// A topic with only paraphrased evidence must not become a forbidden topic.
	if got := preferPrimarySubject(out, Intent{Keywords: []string{"isolation"}}); len(got) != 2 {
		t.Fatal("literal keyword became a topic gate", got)
	}
}
