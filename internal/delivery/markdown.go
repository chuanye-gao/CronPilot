package delivery

import (
	"bytes"
	"html"

	"github.com/yuin/goldmark"
)

// markdownRenderer converts task output from Markdown to HTML for email
// clients. goldmark is CommonMark-compliant and escapes raw HTML by default,
// so model output cannot inject markup into the email.
var markdownRenderer = goldmark.New()

func renderMarkdown(input string) string {
	var buffer bytes.Buffer
	if err := markdownRenderer.Convert([]byte(input), &buffer); err != nil {
		return "<pre style=\"white-space:pre-wrap;word-break:break-word;font:13px/1.6 ui-monospace,SFMono-Regular,Consolas,monospace;color:#c9d1cc\">" + html.EscapeString(input) + "</pre>"
	}
	return buffer.String()
}
