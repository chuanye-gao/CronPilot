package delivery

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	input := "# Title\n\n**bold** and [link](https://example.com)\n\n- item one\n- item two\n\n```go\nfmt.Println(\"hi\")\n```"
	output := renderMarkdown(input)
	for _, want := range []string{
		"<h1", "Title",
		"<strong>bold</strong>",
		`href="https://example.com"`,
		"<ul>", "<li>",
		"<code",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderMarkdown() missing %q in:\n%s", want, output)
		}
	}
}

func TestRenderMarkdownEscapesRawHTML(t *testing.T) {
	output := renderMarkdown(`<script>alert("xss")</script>`)
	if strings.Contains(output, "<script>") {
		t.Fatalf("raw HTML should be escaped, got %q", output)
	}
}
