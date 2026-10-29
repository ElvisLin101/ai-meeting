package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeFetcher struct {
	page   FetchedPage
	err    error
	called bool
}

func (f *fakeFetcher) Fetch(_ context.Context, pageURL string) (FetchedPage, error) {
	f.called = true
	if f.page.URL == "" {
		f.page.URL = pageURL
	}
	return f.page, f.err
}

func TestWebFetchToolOutput(t *testing.T) {
	fetcher := &fakeFetcher{page: FetchedPage{Title: "面经", Content: "正文内容"}}
	tool := NewWebFetchTool(fetcher)
	res, err := tool.Execute(context.Background(), &Context{}, json.RawMessage(`{"url":"https://example.com/article"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fetcher.called {
		t.Fatal("fetcher should be called")
	}
	for _, want := range []string{"不可信数据", "面经", "https://example.com/article", "正文内容"} {
		if !strings.Contains(res.Content, want) {
			t.Fatalf("output missing %q: %q", want, res.Content)
		}
	}
}

func TestWebFetchToolTruncates(t *testing.T) {
	fetcher := &fakeFetcher{page: FetchedPage{Title: "t", Content: "123456789"}}
	tool := NewWebFetchTool(fetcher)
	tool.maxRunes = 5
	res, err := tool.Execute(context.Background(), &Context{}, json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(res.Content, "12345…") {
		t.Fatalf("content should be truncated: %q", res.Content)
	}
	if res.Meta["truncated"] != true {
		t.Fatalf("truncated meta missing: %+v", res.Meta)
	}
}

func TestWebFetchToolRejectsPrivateURLsBeforeFetch(t *testing.T) {
	fetcher := &fakeFetcher{page: FetchedPage{Title: "t", Content: "c"}}
	tool := NewWebFetchTool(fetcher)
	bad := []string{
		"http://localhost/a",
		"http://127.0.0.1/a",
		"http://10.0.0.1/a",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/a",
		"http://example.com:8080/a",
		"ftp://example.com/a",
		"file:///etc/passwd",
		"https://user:pass@example.com/a",
		"http://intranet/a",
		"http://2130706433/a",
	}
	for _, raw := range bad {
		args, _ := json.Marshal(map[string]string{"url": raw})
		if _, err := tool.Execute(context.Background(), &Context{}, args); err == nil {
			t.Fatalf("URL %q should be rejected", raw)
		}
	}
	if fetcher.called {
		t.Fatal("fetcher must not be called for rejected URLs")
	}
}

func TestWebFetchToolEmptyContent(t *testing.T) {
	tool := NewWebFetchTool(&fakeFetcher{page: FetchedPage{Title: "t"}})
	_, err := tool.Execute(context.Background(), &Context{}, json.RawMessage(`{"url":"https://example.com"}`))
	if err == nil {
		t.Fatal("empty content should fail")
	}
}

func TestWebFetchToolFetcherError(t *testing.T) {
	tool := NewWebFetchTool(&fakeFetcher{err: errors.New("boom")})
	_, err := tool.Execute(context.Background(), &Context{}, json.RawMessage(`{"url":"https://example.com"}`))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("fetch error should surface: %v", err)
	}
}

func TestValidateFetchURL(t *testing.T) {
	good := []string{
		"https://example.com/article",
		"http://example.com/a?x=1",
		"https://example.com:443/a",
		"https://1.1.1.1/a",
		"https://[2606:4700:4700::1111]/a",
	}
	for _, raw := range good {
		if _, err := validateFetchURL(raw); err != nil {
			t.Fatalf("URL %q should pass: %v", raw, err)
		}
	}
	normalized, err := validateFetchURL("example.com/a")
	if err != nil || normalized != "https://example.com/a" {
		t.Fatalf("scheme-less URL should normalize, got %q err=%v", normalized, err)
	}
}
