package websearch

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"testing"
)

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
	agent, err := New(Config{Provider: "tavily", Endpoint: "http://search:8080", APIKey: "test-key"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []SearchRequest{{}, {Query: "x", Category: "images"}, {Query: "x", TimeRange: "hour"}} {
		if _, err := agent.Search(context.Background(), request); err == nil {
			t.Fatalf("Search(%s) succeeded", fmt.Sprintf("%#v", request))
		}
	}
}
