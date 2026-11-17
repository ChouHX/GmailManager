package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidWebhookURL(t *testing.T) {
	good := []string{
		"https://discord.com/api/webhooks/123/abc",
		"https://discordapp.com/api/webhooks/1/x",
		"https://ptb.discord.com/api/webhooks/1/x",
	}
	for _, u := range good {
		if !ValidWebhookURL(u) {
			t.Fatalf("%q should be valid", u)
		}
	}
	for _, u := range []string{"", "nope", "ftp://discord.com/api/webhooks/1/x", "https://example.com/hook"} {
		if ValidWebhookURL(u) {
			t.Fatalf("%q should be invalid", u)
		}
	}
}

func TestNotifyPostsEmbeds(t *testing.T) {
	var got payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("bad payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New()
	err := c.Notify(context.Background(), srv.URL+"/api/webhooks/1/x", "监控", []Hit{
		{AccountEmail: "a@gmail.com", Subject: "验证码 123456", From: "noreply@bank.com", Date: 1700000000, Folder: "收件箱", Snippet: "你的验证码是 123456"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Embeds) != 1 || !strings.Contains(got.Embeds[0].Title, "验证码") {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if !strings.Contains(got.Content, "监控") {
		t.Fatalf("unexpected content: %q", got.Content)
	}
	if len(got.Embeds[0].Fields) == 0 || got.Embeds[0].Fields[0].Value != "noreply@bank.com" {
		t.Fatalf("unexpected fields: %+v", got.Embeds[0].Fields)
	}
}

func TestNotifyGroupsAndErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	hits := make([]Hit, 12)
	for i := range hits {
		hits[i] = Hit{Subject: "s", From: "f"}
	}
	if err := New().Notify(context.Background(), srv.URL+"/api/webhooks/1/x", "", hits); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 batched calls for 12 hits, got %d", calls)
	}
	if err := New().Notify(context.Background(), "bad", "", hits[:1]); err == nil {
		t.Fatal("expected error for invalid url")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusForbidden)
	}))
	defer bad.Close()
	if err := New().Notify(context.Background(), bad.URL+"/api/webhooks/1/x", "", hits[:1]); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}
