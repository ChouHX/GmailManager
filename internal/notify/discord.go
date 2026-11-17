// Package notify pushes Discord webhook messages.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Hit is one matched message rendered into a webhook notification.
type Hit struct {
	AccountEmail string
	Subject      string
	From         string
	Date         int64
	Folder       string
	UID          uint32
	Snippet      string
}

// Client posts to Discord webhook URLs.
type Client struct {
	http    *http.Client
	baseURL string // overridable in tests
}

// New creates a client with a sane timeout.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}, baseURL: "https://discord.com"}
}

// ValidWebhookURL reports whether the string looks like a Discord webhook URL.
func ValidWebhookURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	return strings.Contains(lower, "/api/webhooks/")
}

type field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type embed struct {
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	URL         string  `json:"url,omitempty"`
	Color       int     `json:"color"`
	Fields      []field `json:"fields,omitempty"`
	Footer      *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type payload struct {
	Username string  `json:"username,omitempty"`
	Content  string  `json:"content,omitempty"`
	Embeds   []embed `json:"embeds,omitempty"`
}

// Notify sends one task's hits. Multiple hits are grouped into a single
// message (Discord allows at most 10 embeds per request).
func (c *Client) Notify(ctx context.Context, webhookURL, taskName string, hits []Hit) error {
	if len(hits) == 0 {
		return nil
	}
	if !ValidWebhookURL(webhookURL) {
		return errors.New("webhook 地址无效")
	}
	const maxEmbeds = 10
	for start := 0; start < len(hits); start += maxEmbeds {
		end := start + maxEmbeds
		if end > len(hits) {
			end = len(hits)
		}
		if err := c.post(ctx, webhookURL, c.buildPayload(taskName, hits[start:end], len(hits))); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) buildPayload(taskName string, hits []Hit, total int) payload {
	p := payload{Username: "Gmail Manager"}
	if taskName != "" {
		p.Content = fmt.Sprintf("**%s** · 命中 %d 封邮件", taskName, total)
	} else {
		p.Content = fmt.Sprintf("**邮件命中** · 共 %d 封", total)
	}
	for _, h := range hits {
		e := embed{
			Title: truncate(h.Subject, 240),
			Color: 0x2563eb,
		}
		if e.Title == "" {
			e.Title = "(无主题)"
		}
		if h.Snippet != "" {
			e.Description = truncate(h.Snippet, 300)
		}
		fields := []field{
			{Name: "发件人", Value: truncate(orDash(h.From), 200), Inline: true},
			{Name: "邮箱", Value: truncate(orDash(h.AccountEmail), 200), Inline: true},
		}
		if h.Date > 0 {
			fields = append(fields, field{Name: "时间", Value: time.Unix(h.Date, 0).Format("2006-01-02 15:04:05"), Inline: true})
		}
		if h.Folder != "" {
			fields = append(fields, field{Name: "位置", Value: h.Folder, Inline: true})
		}
		e.Fields = fields
		e.Footer = &struct {
			Text string `json:"text"`
		}{Text: "Gmail Manager 定时轮询"}
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
		p.Embeds = append(p.Embeds, e)
	}
	return p
}

func (c *Client) post(ctx context.Context, webhookURL string, p payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求 webhook 失败: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("webhook 返回 %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
