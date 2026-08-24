package websearch

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTavilySearchMapsNewsRequestAndResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body tavilySearchRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.APIKey != "tvly-secret" || body.Topic != "news" || body.Days != 7 || body.MaxResults != 5 {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"A current story","url":"https://news.example/story","content":"Useful summary","score":0.91,"published_date":"2026-08-24"}]}`))
	}))
	defer server.Close()

	agent, err := New(Config{Provider: "tavily", Endpoint: server.URL, APIKey: "tvly-secret", MaxResults: 12}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	response, err := agent.Search(context.Background(), SearchRequest{Query: "AI", Category: "news", TimeRange: "week", MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if response.ResultCount != 1 || response.Results[0].Source != "news.example" || response.Results[0].PublishedAt != "2026-08-24" {
		t.Fatalf("response = %#v", response)
	}
}

func TestTavilyExtractReturnsBoundedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/extract" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["api_key"] != "tvly-secret" || body["extract_depth"] != "basic" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"url":"https://news.example/story","raw_content":"` + strings.Repeat("新闻", 700) + `"}],"failed_results":[]}`))
	}))
	defer server.Close()

	agent, err := New(Config{Provider: "tavily", Endpoint: server.URL, APIKey: "tvly-secret", MaxContentChars: 1000}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	response, err := agent.openTavily(context.Background(), "https://news.example/story", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !response.Truncated || len([]rune(response.Content)) != 1001 || !strings.HasSuffix(response.Content, "…") {
		t.Fatalf("content length = %d, truncated = %v", len([]rune(response.Content)), response.Truncated)
	}
}

func TestTavilyRequiresAPIKey(t *testing.T) {
	if _, err := New(Config{Provider: "tavily", Endpoint: "https://api.tavily.com"}, slog.Default()); err == nil {
		t.Fatal("New() succeeded without an API key")
	}
}
