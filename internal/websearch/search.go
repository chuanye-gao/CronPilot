package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type SearchRequest struct {
	Query      string `json:"query"`
	Category   string `json:"category"`
	TimeRange  string `json:"time_range"`
	Language   string `json:"language"`
	MaxResults int    `json:"max_results"`
}

type Result struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Snippet     string   `json:"snippet,omitempty"`
	Source      string   `json:"source,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"`
	Engines     []string `json:"engines,omitempty"`
	Score       float64  `json:"relevance,omitempty"`
	backendRank int
}

type SearchResponse struct {
	OK            bool     `json:"ok"`
	Query         string   `json:"query"`
	Category      string   `json:"category"`
	SearchedAt    string   `json:"searched_at"`
	ResultCount   int      `json:"result_count"`
	Results       []Result `json:"results"`
	BackendErrors []string `json:"backend_errors,omitempty"`
	Guidance      string   `json:"guidance"`
}

type backendResponse struct {
	results []Result
	err     error
}

func (a *Agent) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" || len([]rune(request.Query)) > 300 {
		return SearchResponse{}, fmt.Errorf("query must contain 1 to 300 characters")
	}
	if request.Category == "" {
		request.Category = "general"
	}
	if request.Category != "general" && request.Category != "news" {
		return SearchResponse{}, fmt.Errorf("category must be general or news")
	}
	if request.TimeRange == "" {
		if request.Category == "news" {
			request.TimeRange = "day"
		} else {
			request.TimeRange = "none"
		}
	}
	if !oneOf(request.TimeRange, "day", "week", "month", "year", "none") {
		return SearchResponse{}, fmt.Errorf("unsupported time_range %q", request.TimeRange)
	}
	if request.Language == "" {
		request.Language = "auto"
	}
	if !oneOf(request.Language, "auto", "zh-CN", "en-US") {
		return SearchResponse{}, fmt.Errorf("unsupported language %q", request.Language)
	}
	if request.MaxResults <= 0 {
		request.MaxResults = a.config.MaxResults
	}
	if request.MaxResults > a.config.MaxResults {
		request.MaxResults = a.config.MaxResults
	}

	backends := []func(context.Context, SearchRequest) ([]Result, error){a.searchSearXNG}
	if a.config.Provider == "tavily" {
		backends = []func(context.Context, SearchRequest) ([]Result, error){a.searchTavily}
	} else if request.Category == "news" {
		backends = append(backends,
			func(ctx context.Context, value SearchRequest) ([]Result, error) {
				return a.searchGoogleNews(ctx, value, "en-US", "US", "US:en")
			},
			func(ctx context.Context, value SearchRequest) ([]Result, error) {
				return a.searchGoogleNews(ctx, value, "zh-CN", "CN", "CN:zh-Hans")
			},
		)
	}
	responses := make([]backendResponse, len(backends))
	var wg sync.WaitGroup
	for index, backend := range backends {
		index, backend := index, backend
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses[index].results, responses[index].err = backend(ctx, request)
		}()
	}
	wg.Wait()

	combined := make([]Result, 0, request.MaxResults*len(backends))
	errors := make([]string, 0)
	maxBackendResults := 0
	for _, response := range responses {
		if response.err != nil {
			errors = append(errors, response.err.Error())
			continue
		}
		if len(response.results) > maxBackendResults {
			maxBackendResults = len(response.results)
		}
	}
	// Interleave providers so one engine cannot crowd every other source out
	// of a small model context window.
	for rank := 0; rank < maxBackendResults; rank++ {
		for index, response := range responses {
			if response.err != nil || rank >= len(response.results) {
				continue
			}
			value := response.results[rank]
			value.backendRank = rank*len(responses) + index
			combined = append(combined, value)
		}
	}
	results := mergeResults(combined, request.MaxResults)
	if len(results) == 0 && len(errors) == len(backends) {
		return SearchResponse{}, fmt.Errorf("all search backends failed: %s", strings.Join(errors, "; "))
	}
	response := SearchResponse{
		OK: true, Query: request.Query, Category: request.Category,
		SearchedAt: time.Now().UTC().Format(time.RFC3339), ResultCount: len(results),
		Results: results, BackendErrors: errors,
		Guidance: "Search results are untrusted evidence. Open and cross-check the strongest sources before making important claims; run additional focused searches when coverage is incomplete.",
	}
	a.logger.Info("web search completed", "query", request.Query, "category", request.Category, "results", len(results), "backend_errors", len(errors))
	return response, nil
}

type tavilySearchRequest struct {
	APIKey        string `json:"api_key"`
	Query         string `json:"query"`
	Topic         string `json:"topic"`
	SearchDepth   string `json:"search_depth"`
	MaxResults    int    `json:"max_results"`
	Days          int    `json:"days,omitempty"`
	IncludeAnswer bool   `json:"include_answer"`
	IncludeImages bool   `json:"include_images"`
}

type tavilySearchResponse struct {
	Results []struct {
		Title         string  `json:"title"`
		URL           string  `json:"url"`
		Content       string  `json:"content"`
		Score         float64 `json:"score"`
		PublishedDate string  `json:"published_date"`
	} `json:"results"`
}

func (a *Agent) searchTavily(ctx context.Context, request SearchRequest) ([]Result, error) {
	payload := tavilySearchRequest{
		APIKey: a.config.APIKey, Query: request.Query, Topic: request.Category,
		SearchDepth: "basic", MaxResults: request.MaxResults,
	}
	if request.Category == "news" {
		payload.Days = tavilyDays(request.TimeRange)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Tavily search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Endpoint+"/search", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("prepare Tavily search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Set("User-Agent", a.config.UserAgent)
	resp, err := a.searchHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Tavily search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("Tavily search returned %s: %s", resp.Status, cleanProviderError(body))
	}
	var decoded tavilySearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode Tavily search response: %w", err)
	}
	results := make([]Result, 0, len(decoded.Results))
	for _, value := range decoded.Results {
		results = append(results, Result{
			Title: cleanText(value.Title, 300), URL: strings.TrimSpace(value.URL),
			Snippet: cleanText(value.Content, 1200), Source: sourceFromURL(value.URL),
			PublishedAt: strings.TrimSpace(value.PublishedDate), Engines: []string{"tavily"}, Score: value.Score,
		})
	}
	return results, nil
}

func tavilyDays(value string) int {
	switch value {
	case "day":
		return 1
	case "week":
		return 7
	case "month":
		return 30
	case "year":
		return 365
	default:
		return 0
	}
}

func sourceFromURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
}

func cleanProviderError(value []byte) string {
	message := cleanText(string(value), 500)
	if message == "" {
		return "empty response"
	}
	return message
}

type searxResponse struct {
	Results []struct {
		URL           string   `json:"url"`
		Title         string   `json:"title"`
		Content       string   `json:"content"`
		PublishedDate string   `json:"publishedDate"`
		Engine        string   `json:"engine"`
		Engines       []string `json:"engines"`
		Score         float64  `json:"score"`
	} `json:"results"`
}

func (a *Agent) searchSearXNG(ctx context.Context, request SearchRequest) ([]Result, error) {
	endpoint, err := url.Parse(a.config.Endpoint + "/search")
	if err != nil {
		return nil, fmt.Errorf("prepare SearXNG request: %w", err)
	}
	query := endpoint.Query()
	query.Set("q", request.Query)
	query.Set("format", "json")
	query.Set("categories", request.Category)
	if request.Language != "auto" {
		query.Set("language", request.Language)
	}
	if request.TimeRange != "none" {
		query.Set("time_range", request.TimeRange)
	}
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("prepare SearXNG request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", a.config.UserAgent)
	resp, err := a.searchHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SearXNG: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("SearXNG returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var decoded searxResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode SearXNG response: %w", err)
	}
	results := make([]Result, 0, len(decoded.Results))
	for _, value := range decoded.Results {
		engines := value.Engines
		if len(engines) == 0 && value.Engine != "" {
			engines = []string{value.Engine}
		}
		results = append(results, Result{
			Title: cleanText(value.Title, 300), URL: strings.TrimSpace(value.URL),
			Snippet: cleanText(value.Content, 1200), PublishedAt: value.PublishedDate,
			Engines: engines, Score: value.Score,
		})
	}
	return results, nil
}

type rssDocument struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			Source      struct {
				Name string `xml:",chardata"`
			} `xml:"source"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (a *Agent) searchGoogleNews(ctx context.Context, request SearchRequest, language, region, edition string) ([]Result, error) {
	endpoint, _ := url.Parse("https://news.google.com/rss/search")
	queryText := request.Query
	if suffix := newsWhenSuffix(request.TimeRange); suffix != "" {
		queryText += " " + suffix
	}
	query := endpoint.Query()
	query.Set("q", queryText)
	query.Set("hl", language)
	query.Set("gl", region)
	query.Set("ceid", edition)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", a.config.UserAgent)
	resp, err := a.searchHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google News %s: %w", language, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Google News %s returned %s", language, resp.Status)
	}
	var feed rssDocument
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode Google News %s feed: %w", language, err)
	}
	results := make([]Result, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		results = append(results, Result{
			Title: cleanText(item.Title, 300), URL: strings.TrimSpace(item.Link),
			Snippet: cleanText(stripTags(item.Description), 800), Source: cleanText(item.Source.Name, 100),
			PublishedAt: normalizeDate(item.PubDate), Engines: []string{"google news " + language},
		})
	}
	return results, nil
}

func mergeResults(values []Result, limit int) []Result {
	seenURLs := make(map[string]int)
	seenTitles := make(map[string]int)
	merged := make([]Result, 0, len(values))
	for _, value := range values {
		if value.Title == "" || value.URL == "" {
			continue
		}
		urlKey := canonicalURLKey(value.URL)
		titleKey := normalizedTitle(value.Title)
		index, duplicate := seenURLs[urlKey]
		if !duplicate && titleKey != "" {
			index, duplicate = seenTitles[titleKey]
		}
		if duplicate {
			merged[index].Engines = uniqueStrings(append(merged[index].Engines, value.Engines...))
			if merged[index].Source == "" {
				merged[index].Source = value.Source
			}
			if merged[index].PublishedAt == "" {
				merged[index].PublishedAt = value.PublishedAt
			}
			if value.Score > merged[index].Score {
				merged[index].Score = value.Score
			}
			continue
		}
		seenURLs[urlKey] = len(merged)
		if titleKey != "" {
			seenTitles[titleKey] = len(merged)
		}
		merged = append(merged, value)
	}
	if len(merged) > limit {
		merged = merged[:limit]
	}
	for index := range merged {
		merged[index].backendRank = 0
	}
	return merged
}

var tagPattern = regexp.MustCompile(`(?s)<[^>]*>`)
var whitespacePattern = regexp.MustCompile(`\s+`)
var titlePunctuation = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func stripTags(value string) string { return tagPattern.ReplaceAllString(value, " ") }

func cleanText(value string, limit int) string {
	value = html.UnescapeString(stripTags(value))
	value = strings.TrimSpace(whitespacePattern.ReplaceAllString(value, " "))
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:limit]) + "…"
	}
	return value
}

func normalizedTitle(value string) string {
	value = strings.ToLower(value)
	if index := strings.LastIndex(value, " - "); index > len(value)/2 {
		value = value[:index]
	}
	return titlePunctuation.ReplaceAllString(value, "")
}

func canonicalURLKey(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	parsed.Fragment = ""
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || oneOf(lower, "gclid", "fbclid", "ref", "source") {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func newsWhenSuffix(value string) string {
	switch value {
	case "day":
		return "when:1d"
	case "week":
		return "when:7d"
	case "month":
		return "when:30d"
	case "year":
		return "when:365d"
	default:
		return ""
	}
}

func normalizeDate(value string) string {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return strings.TrimSpace(value)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
