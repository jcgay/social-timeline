package htmltext

import (
	"testing"
)

func TestFromHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain paragraph", `<p>hello world</p>`, "hello world"},
		{"two paragraphs", `<p>line1</p><p>line2</p>`, "line1\n\nline2"},
		{"br tag", `first<br>second`, "first\nsecond"},
		{"br self-close", `first<br />second`, "first\nsecond"},
		{"anchor with label", `see <a href="https://example.com">example</a>`, "see example (https://example.com)"},
		{"anchor same href and text", `<a href="https://example.com">https://example.com</a>`, "https://example.com"},
		{"mention nested span", `<p>hi <span class="h-card"><a href="https://m.s/@bob">@<span>bob</span></a></span></p>`, "hi @bob (https://m.s/@bob)"},
		{"html entities", `&amp;&lt;&gt;&quot;&#39;`, `&<>"'`},
		{"empty string", ``, ``},
		{"empty paragraph", `<p></p>`, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromHTML(tc.in)
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
