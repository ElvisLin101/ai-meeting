package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-meeting/config"
)

func TestDeepSeekSearchAndFetch(t *testing.T) {
	var gotSearchAuth, gotFetchAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			gotSearchAuth = r.Header.Get("Authorization")
			var body struct {
				Query string `json:"query"`
				Count int    `json:"count"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Query != "Go 面经" || body.Count != 3 {
				t.Errorf("unexpected search body: %+v", body)
			}
			_, _ = io.WriteString(w, `{"results":[
				{"title":"字节面经","url":"https://example.com/a","snippet":"","summary":"高频考点","content":"完整正文"},
				{"title":"腾讯面经","url":"https://example.com/b","snippet":"直接摘要"}
			]}`)
		case "/fetch":
			gotFetchAuth = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `{"title":"面经正文","content":"# 面试题\n- GMP\n- channel"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p, err := NewSearchProvider(config.SearchConfig{
		Provider:      "deepseek",
		APIKey:        "test-key",
		Endpoint:      srv.URL + "/search",
		FetchEndpoint: srv.URL + "/fetch",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	results, err := p.Search(context.Background(), "Go 面经", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotSearchAuth != "Bearer test-key" {
		t.Fatalf("unexpected search auth: %q", gotSearchAuth)
	}
	if len(results) != 2 || results[0].Snippet != "高频考点" || results[1].Snippet != "直接摘要" {
		t.Fatalf("unexpected results: %+v", results)
	}

	fetcher, ok := p.(PageFetcher)
	if !ok {
		t.Fatal("deepseek provider should implement PageFetcher")
	}
	page, err := fetcher.Fetch(context.Background(), "https://example.com/a")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotFetchAuth != "Bearer test-key" || page.Title != "面经正文" || !strings.Contains(page.Content, "GMP") {
		t.Fatalf("unexpected fetch: auth=%q page=%+v", gotFetchAuth, page)
	}
}

func TestDeepSeekSearchErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	p, err := NewSearchProvider(config.SearchConfig{
		Provider: "deepseek", APIKey: "bad", Endpoint: srv.URL,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := p.Search(context.Background(), "q", 1); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestNewSearchProviderRequiresKey(t *testing.T) {
	if _, err := NewSearchProvider(config.SearchConfig{Provider: "deepseek"}); err == nil {
		t.Fatal("missing key should fail")
	}
}

func TestResolveSearchEndpoint(t *testing.T) {
	cases := []struct{ provider, endpoint, want string }{
		{"deepseek", "", deepSeekSearchEndpoint},
		{"deepseek", tavilySearchEndpoint, deepSeekSearchEndpoint},
		{"tavily", "", tavilySearchEndpoint},
		{"tavily", deepSeekSearchEndpoint, tavilySearchEndpoint},
		{"serper", "", serperSearchEndpoint},
		{"serper", tavilySearchEndpoint, serperSearchEndpoint},
		{"deepseek", "http://127.0.0.1:9999/search", "http://127.0.0.1:9999/search"},
	}
	for _, c := range cases {
		if got := resolveSearchEndpoint(c.provider, c.endpoint); got != c.want {
			t.Fatalf("resolveSearchEndpoint(%q,%q)=%q want %q", c.provider, c.endpoint, got, c.want)
		}
	}
}

func TestWebSearchToolMarksUntrusted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"results":[{"title":"t","url":"https://example.com","snippet":"s"}]}`)
	}))
	defer srv.Close()
	p, err := NewSearchProvider(config.SearchConfig{Provider: "deepseek", APIKey: "k", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	tool := NewWebSearchTool(p, 5)
	res, err := tool.Execute(context.Background(), &Context{}, json.RawMessage(`{"query":"q"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Content, "不可信数据") || !strings.Contains(res.Content, "https://example.com") {
		t.Fatalf("unexpected output: %q", res.Content)
	}
}
