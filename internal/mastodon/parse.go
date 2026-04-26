package mastodon

import (
	"strings"
	"time"

	"github.com/jcgay/social-timeline/internal/htmltext"
	"github.com/jcgay/social-timeline/internal/timeline"
)

type account struct {
	ID   string `json:"id"`
	Acct string `json:"acct"`
}

type mention struct {
	ID   string `json:"id"`
	Acct string `json:"acct"`
}

type mediaAttachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type status struct {
	ID                 string            `json:"id"`
	CreatedAt          time.Time         `json:"created_at"`
	URL                string            `json:"url"`
	Content            string            `json:"content"`
	InReplyToID        *string           `json:"in_reply_to_id"`
	InReplyToAccountID *string           `json:"in_reply_to_account_id"`
	Mentions           []mention         `json:"mentions"`
	Account            account           `json:"account"`
	MediaAttachments   []mediaAttachment `json:"media_attachments"`
	Reblog             *status           `json:"reblog"`
}

// statusesToPosts maps a list of API statuses to neutral Posts.
// instanceHost is used to fully-qualify bare local handles.
func statusesToPosts(in []status, instanceHost string) []timeline.Post {
	out := make([]timeline.Post, 0, len(in))
	for _, s := range in {
		out = append(out, statusToPost(s, instanceHost))
	}
	return out
}

func statusToPost(s status, instanceHost string) timeline.Post {
	wrapperAuthor := qualify(s.Account.Acct, instanceHost)
	if s.Reblog != nil {
		inner := s.Reblog
		return timeline.Post{
			Platform:       "mastodon",
			Author:         wrapperAuthor,
			CreatedAt:      s.CreatedAt.UTC(),
			Text:           htmltext.FromHTML(inner.Content),
			URL:            inner.URL,
			Type:           timeline.PostRepost,
			OriginalAuthor: qualify(inner.Account.Acct, instanceHost),
			MediaURLs:      mediaURLs(inner.MediaAttachments),
		}
	}
	p := timeline.Post{
		Platform:  "mastodon",
		Author:    wrapperAuthor,
		CreatedAt: s.CreatedAt.UTC(),
		Text:      htmltext.FromHTML(s.Content),
		URL:       s.URL,
		Type:      timeline.PostOriginal,
		MediaURLs: mediaURLs(s.MediaAttachments),
	}
	if s.InReplyToID != nil && *s.InReplyToID != "" {
		if orig := replyAuthor(s, instanceHost); orig != "" {
			p.Type = timeline.PostReply
			p.OriginalAuthor = orig
		}
	}
	return p
}

func qualify(acct, instanceHost string) string {
	if acct == "" {
		return ""
	}
	if strings.Contains(acct, "@") {
		return acct
	}
	return acct + "@" + instanceHost
}

func mediaURLs(ms []mediaAttachment) []string {
	if len(ms) == 0 {
		return nil
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if m.URL != "" {
			out = append(out, m.URL)
		}
	}
	return out
}

func replyAuthor(s status, instanceHost string) string {
	if s.InReplyToAccountID == nil {
		return ""
	}
	for _, m := range s.Mentions {
		if m.ID == *s.InReplyToAccountID {
			return qualify(m.Acct, instanceHost)
		}
	}
	return ""
}
