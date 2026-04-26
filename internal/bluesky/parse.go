package bluesky

import (
	"strings"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

type author struct {
	Handle string `json:"handle"`
}

type postRecord struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Reply     *struct {
		Parent struct {
			URI string `json:"uri"`
		} `json:"parent"`
	} `json:"reply"`
}

type embedView struct {
	Type   string `json:"$type"`
	Images []struct {
		Fullsize string `json:"fullsize"`
	} `json:"images"`
	Thumbnail string `json:"thumbnail"`
}

type feedPost struct {
	URI       string     `json:"uri"`
	Author    author     `json:"author"`
	Record    postRecord `json:"record"`
	Embed     *embedView `json:"embed"`
	IndexedAt time.Time  `json:"indexedAt"`
}

type repostReason struct {
	Type      string    `json:"$type"`
	By        author    `json:"by"`
	IndexedAt time.Time `json:"indexedAt"`
}

type replyContext struct {
	Parent struct {
		Author author `json:"author"`
	} `json:"parent"`
}

type feedItem struct {
	Post   feedPost      `json:"post"`
	Reason *repostReason `json:"reason,omitempty"`
	Reply  *replyContext `json:"reply,omitempty"`
}

type feedResponse struct {
	Cursor string     `json:"cursor"`
	Feed   []feedItem `json:"feed"`
}

func feedToPosts(items []feedItem) []timeline.Post {
	out := make([]timeline.Post, 0, len(items))
	for _, it := range items {
		out = append(out, itemToPost(it))
	}
	return out
}

func itemToPost(it feedItem) timeline.Post {
	innerPermalink := permalink(it.Post.Author.Handle, it.Post.URI)
	media := mediaFromEmbed(it.Post.Embed)

	if it.Reason != nil && strings.HasSuffix(it.Reason.Type, "reasonRepost") {
		return timeline.Post{
			Platform:       "bluesky",
			Author:         it.Reason.By.Handle,
			CreatedAt:      it.Reason.IndexedAt.UTC(),
			Text:           it.Post.Record.Text,
			URL:            innerPermalink,
			Type:           timeline.PostRepost,
			OriginalAuthor: it.Post.Author.Handle,
			MediaURLs:      media,
		}
	}
	p := timeline.Post{
		Platform:  "bluesky",
		Author:    it.Post.Author.Handle,
		CreatedAt: it.Post.Record.CreatedAt.UTC(),
		Text:      it.Post.Record.Text,
		URL:       innerPermalink,
		Type:      timeline.PostOriginal,
		MediaURLs: media,
	}
	if it.Post.Record.Reply != nil {
		p.Type = timeline.PostReply
		if it.Reply != nil {
			p.OriginalAuthor = it.Reply.Parent.Author.Handle
		}
	}
	return p
}

// permalink converts an at:// URI to a https://bsky.app/... URL.
func permalink(handle, atURI string) string {
	rkey := atURI
	if i := strings.LastIndex(atURI, "/"); i >= 0 {
		rkey = atURI[i+1:]
	}
	return "https://bsky.app/profile/" + handle + "/post/" + rkey
}

func mediaFromEmbed(e *embedView) []string {
	if e == nil {
		return nil
	}
	switch {
	case strings.Contains(e.Type, "embed.images"):
		out := make([]string, 0, len(e.Images))
		for _, img := range e.Images {
			if img.Fullsize != "" {
				out = append(out, img.Fullsize)
			}
		}
		return out
	case strings.Contains(e.Type, "embed.video"):
		if e.Thumbnail != "" {
			return []string{e.Thumbnail}
		}
	}
	return nil
}
