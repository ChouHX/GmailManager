package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gmailmanager/internal/watcher"
)

// webhookRecorder captures Discord webhook payloads.
type webhookRecorder struct {
	mu     sync.Mutex
	bodies []string
}

func (w *webhookRecorder) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	buf := make([]byte, r.ContentLength)
	_, _ = r.Body.Read(buf)
	w.mu.Lock()
	w.bodies = append(w.bodies, string(buf))
	w.mu.Unlock()
	rw.WriteHeader(http.StatusNoContent)
}

func (w *webhookRecorder) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.bodies)
}

func (w *webhookRecorder) last() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.bodies) == 0 {
		return ""
	}
	return w.bodies[len(w.bodies)-1]
}

func TestWatchCRUD(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	code, body := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "user@gmail.com", "password": "pw", "appPassword": "apppass",
	})
	if code != 201 {
		t.Fatalf("create account: %d %s", code, body)
	}

	hook := "https://discord.com/api/webhooks/1/abc"
	// Missing match condition.
	code, body = e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "intervalSeconds": 60, "webhookUrl": hook,
	})
	if code != 400 {
		t.Fatalf("expected 400 for empty conditions, got %d %s", code, body)
	}
	// Bad webhook.
	code, body = e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "subject": "invoice", "intervalSeconds": 60, "webhookUrl": "nope",
	})
	if code != 400 {
		t.Fatalf("expected 400 for bad webhook, got %d %s", code, body)
	}
	// Too short interval.
	code, body = e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "subject": "invoice", "intervalSeconds": 5, "webhookUrl": hook,
	})
	if code != 400 {
		t.Fatalf("expected 400 for short interval, got %d %s", code, body)
	}

	code, body = e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "name": "发票监控", "subject": "invoice", "from": "@bank.com",
		"folder": "inbox", "intervalSeconds": 300, "webhookUrl": hook,
	})
	if code != 201 {
		t.Fatalf("create watch: %d %s", code, body)
	}

	code, body = e.do(t, "GET", "/api/watches", nil)
	if code != 200 {
		t.Fatalf("list watches: %d %s", code, body)
	}
	var list struct {
		Items []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Email    string `json:"email"`
			Enabled  bool   `json:"enabled"`
			Subject  string `json:"subject"`
			Interval int    `json:"intervalSeconds"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Email != "user@gmail.com" || !list.Items[0].Enabled {
		t.Fatalf("unexpected list: %s", body)
	}

	// Toggle off.
	code, body = e.do(t, "POST", "/api/watches/1/toggle", nil)
	if code != 200 || !strings.Contains(string(body), `"enabled":false`) {
		t.Fatalf("toggle: %d %s", code, body)
	}

	// Update.
	code, body = e.do(t, "PUT", "/api/watches/1", map[string]any{
		"accountId": 1, "name": "发票监控2", "subject": "re:^invoice-\\d+$", "folder": "auto",
		"intervalSeconds": 120, "webhookUrl": hook, "enabled": true,
	})
	if code != 200 {
		t.Fatalf("update watch: %d %s", code, body)
	}

	code, body = e.do(t, "DELETE", "/api/watches/1", nil)
	if code != 200 {
		t.Fatalf("delete watch: %d %s", code, body)
	}
	code, body = e.do(t, "DELETE", "/api/watches/1", nil)
	if code != 404 {
		t.Fatalf("expected 404 after delete, got %d %s", code, body)
	}
}

func TestWatchRunPushesWebhook(t *testing.T) {
	e, imapSrv := newIMAPEnv(t)
	imapSrv.AddUser("other@gmail.com", "apppass")
	e.login(t)

	rec := &webhookRecorder{}
	hookSrv := httptest.NewServer(rec)
	t.Cleanup(hookSrv.Close)
	hook := hookSrv.URL + "/api/webhooks/1/token"

	if code, body := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "user@gmail.com", "password": "pw", "appPassword": "apppass",
	}); code != 201 {
		t.Fatalf("create account: %d %s", code, body)
	}

	code, body := e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "name": "收件箱监控", "subject": "inbox", "folder": "inbox",
		"intervalSeconds": 60, "webhookUrl": hook,
	})
	if code != 201 {
		t.Fatalf("create watch: %d %s", code, body)
	}

	code, body = e.do(t, "POST", "/api/watches/1/run", nil)
	if code != 200 {
		t.Fatalf("run watch: %d %s", code, body)
	}
	var res struct {
		OK       bool `json:"ok"`
		Fetched  int  `json:"fetched"`
		Notified bool `json:"notified"`
		Hits     []struct {
			Subject string `json:"subject"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || !res.Notified || len(res.Hits) != 2 || res.Fetched != 2 {
		t.Fatalf("unexpected run result: %s", body)
	}
	if rec.count() != 1 {
		t.Fatalf("expected 1 webhook call, got %d", rec.count())
	}
	if !strings.Contains(rec.last(), "Inbox newest") {
		t.Fatalf("webhook payload missing subject: %s", rec.last())
	}

	// A non-matching run must not push anything.
	if code, body := e.do(t, "PUT", "/api/watches/1", map[string]any{
		"accountId": 1, "name": "收件箱监控", "subject": "no-such-subject", "folder": "inbox",
		"intervalSeconds": 60, "webhookUrl": hook,
	}); code != 200 {
		t.Fatalf("update watch: %d %s", code, body)
	}
	code, body = e.do(t, "POST", "/api/watches/1/run", nil)
	if code != 200 {
		t.Fatalf("run watch: %d %s", code, body)
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 0 || res.Notified {
		t.Fatalf("expected no hits, got %s", body)
	}
	if rec.count() != 1 {
		t.Fatalf("webhook should not fire again, got %d calls", rec.count())
	}
}

func TestWebhookTestEndpoint(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	rec := &webhookRecorder{}
	hookSrv := httptest.NewServer(rec)
	t.Cleanup(hookSrv.Close)

	code, body := e.do(t, "POST", "/api/watches/webhook-test", map[string]string{
		"url": hookSrv.URL + "/api/webhooks/9/test",
	})
	if code != 200 || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("webhook test: %d %s", code, body)
	}
	if rec.count() != 1 {
		t.Fatalf("expected 1 call, got %d", rec.count())
	}
	if !strings.Contains(rec.last(), "测试通知") {
		t.Fatalf("unexpected payload: %s", rec.last())
	}

	code, body = e.do(t, "POST", "/api/watches/webhook-test", map[string]string{"url": "bad"})
	if code != 400 {
		t.Fatalf("expected 400 for invalid url, got %d %s", code, body)
	}
}

func TestWatchesRequireAuth(t *testing.T) {
	e := newTestEnv(t)
	if code, _ := e.do(t, "GET", "/api/watches", nil); code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
}

func TestWatchDeletedWithAccount(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)
	if code, body := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "user@gmail.com", "password": "pw", "appPassword": "apppass",
	}); code != 201 {
		t.Fatalf("create account: %d %s", code, body)
	}
	if code, body := e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "subject": "x", "intervalSeconds": 60,
		"webhookUrl": "https://discord.com/api/webhooks/1/abc",
	}); code != 201 {
		t.Fatalf("create watch: %d %s", code, body)
	}
	if code, body := e.do(t, "DELETE", "/api/accounts/1", nil); code != 200 {
		t.Fatalf("delete account: %d %s", code, body)
	}
	code, body := e.do(t, "GET", "/api/watches", nil)
	if code != 200 {
		t.Fatalf("list: %d %s", code, body)
	}
	if !strings.Contains(string(body), `"items":[]`) {
		t.Fatalf("expected empty watch list, got %s", body)
	}
	_ = watcher.MessageLimit
}

func TestClearAccountPool(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	for _, email := range []string{"a@gmail.com", "b@gmail.com", "c@gmail.com"} {
		if code, body := e.do(t, "POST", "/api/accounts", map[string]string{
			"email": email, "password": "pw", "appPassword": "apppass",
		}); code != 201 {
			t.Fatalf("create account: %d %s", code, body)
		}
	}
	if code, body := e.do(t, "POST", "/api/watches", map[string]any{
		"accountId": 1, "subject": "x", "intervalSeconds": 60,
		"webhookUrl": "https://discord.com/api/webhooks/1/abc",
	}); code != 201 {
		t.Fatalf("create watch: %d %s", code, body)
	}
	if code, body := e.do(t, "GET", "/api/accounts/1/link", nil); code != 200 {
		t.Fatalf("ensure link: %d %s", code, body)
	}

	// Wrong / missing confirmation phrase must be rejected.
	for _, body := range []map[string]string{{}, {"confirm": "清空吧"}, {"confirm": "clear"}} {
		code, resp := e.do(t, "POST", "/api/accounts/clear", body)
		if code != 400 {
			t.Fatalf("expected 400 for %v, got %d %s", body, code, resp)
		}
	}
	if code, body := e.do(t, "GET", "/api/accounts", nil); code != 200 || !strings.Contains(string(body), `"total":3`) {
		t.Fatalf("accounts should be untouched: %d %s", code, body)
	}

	code, body := e.do(t, "POST", "/api/accounts/clear", map[string]string{"confirm": "清空"})
	if code != 200 || !strings.Contains(string(body), `"deleted":3`) {
		t.Fatalf("clear: %d %s", code, body)
	}

	code, body = e.do(t, "GET", "/api/accounts", nil)
	if code != 200 || !strings.Contains(string(body), `"total":0`) {
		t.Fatalf("expected empty pool, got %d %s", code, body)
	}
	code, body = e.do(t, "GET", "/api/watches", nil)
	if code != 200 || !strings.Contains(string(body), `"items":[]`) {
		t.Fatalf("watches should be cleared too, got %d %s", code, body)
	}
	code, body = e.do(t, "GET", "/api/accounts/links/export", nil)
	if code != 200 {
		t.Fatalf("links export: %d %s", code, body)
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			t.Fatalf("links should be cleared too, got line %q", line)
		}
	}

	// Clearing an already empty pool is a no-op, not an error.
	code, body = e.do(t, "POST", "/api/accounts/clear", map[string]string{"confirm": "清空"})
	if code != 200 || !strings.Contains(string(body), `"deleted":0`) {
		t.Fatalf("second clear: %d %s", code, body)
	}
}

func TestClearAccountPoolRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	if code, _ := e.do(t, "POST", "/api/accounts/clear", map[string]string{"confirm": "清空"}); code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
}
