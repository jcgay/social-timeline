// Package htmltext converts HTML-formatted Mastodon post content into
// plain text suitable for Markdown output.
package htmltext

import (
	"strings"

	"golang.org/x/net/html"
)

// FromHTML converts an HTML string to trimmed plain text.
// Block elements (p, div) are separated by a blank line; <br> becomes a
// newline; anchor text is rendered as "label (url)" unless label == url.
func FromHTML(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	var b strings.Builder
	walk(doc, &b)
	result := strings.TrimSpace(b.String())
	// collapse 3+ consecutive newlines to 2
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return result
}

func walk(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "br":
			b.WriteString("\n")
			return
		case "a":
			href := attrVal(n, "href")
			label := strings.TrimSpace(textContent(n))
			if label == "" || label == href {
				b.WriteString(href)
			} else {
				b.WriteString(label)
				b.WriteString(" (")
				b.WriteString(href)
				b.WriteString(")")
			}
			return
		case "p", "div":
			var inner strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, &inner)
			}
			text := strings.TrimSpace(inner.String())
			if text != "" {
				b.WriteString(text)
				b.WriteString("\n\n")
			}
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, b)
	}
}

func textContent(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		} else {
			b.WriteString(textContent(c))
		}
	}
	return b.String()
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
