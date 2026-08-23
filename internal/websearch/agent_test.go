package websearch

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSearchUsesLocalBackendAndDeduplicates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("format") != "json" {
			t.Fatalf("request = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"First story - Example","url":"https://example.com/story?utm_source=test","content":"first summary","engine":"alpha","score":0.9},
			{"title":"First story - Other","url":"https://example.com/story","content":"duplicate","engine":"beta","score":0.8},
			{"title":"Second story","url":"https://news.example.org/second","content":"second summary","engines":["gamma"],"score":0.7}
		]}`))
	}))
	defer server.Close()

	agent, err := New(Config{Endpoint: server.URL, MaxResults: 10}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	response, err := agent.Search(context.Background(), SearchRequest{Query: "test", Category: "general"})
	if err != nil {
		t.Fatal(err)
	}
	if response.ResultCount != 2 {
		t.Fatalf("results = %#v", response.Results)
	}
	if got := strings.Join(response.Results[0].Engines, ","); !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Fatalf("merged engines = %q", got)
	}
}

func TestExtractPagePrefersArticleAndMetadata(t *testing.T) {
	page, err := extractPage(strings.NewReader(`<!doctype html><html><head><title>Example</title><meta property="article:published_time" content="2026-08-23T10:00:00Z"><link rel="canonical" href="/canonical"></head><body><nav>menu</nav><article><h1>Headline</h1><p>Useful paragraph.</p><script>ignore me</script></article><footer>footer</footer></body></html>`), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "Example" || page.PublishedAt != "2026-08-23T10:00:00Z" || page.CanonicalURL != "/canonical" {
		t.Fatalf("metadata = %#v", page)
	}
	if !strings.Contains(page.Content, "Useful paragraph") || strings.Contains(page.Content, "ignore me") || strings.Contains(page.Content, "menu") {
		t.Fatalf("content = %q", page.Content)
	}
}

type fixedResolver struct{ addresses []net.IPAddr }

func (r fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, nil
}

func TestValidatePublicURLBlocksPrivateNetworks(t *testing.T) {
	privateURL, _ := url.Parse("http://service.example/data")
	err := validatePublicURL(context.Background(), privateURL, fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}})
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("private URL error = %v", err)
	}
	publicURL, _ := url.Parse("https://news.example/article")
	err = validatePublicURL(context.Background(), publicURL, fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}})
	if err != nil {
		t.Fatalf("public URL error = %v", err)
	}
}

func TestSearchRejectsInvalidArguments(t *testing.T) {
	agent, err := New(Config{Endpoint: "http://search:8080"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []SearchRequest{{}, {Query: "x", Category: "images"}, {Query: "x", TimeRange: "hour"}} {
		if _, err := agent.Search(context.Background(), request); err == nil {
			t.Fatalf("Search(%s) succeeded", fmt.Sprintf("%#v", request))
		}
	}
}
