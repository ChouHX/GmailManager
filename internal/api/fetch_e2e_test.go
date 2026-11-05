package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gmailmanager/internal/config"
	"gmailmanager/internal/imapx/imaptest"
	"gmailmanager/internal/secret"
	"gmailmanager/internal/store"
)

// newIMAPEnv boots an in-memory IMAP server and points the API at it.
func newIMAPEnv(t *testing.T) (*testEnv, *imaptest.Server) {
	t.Helper()
	imapSrv, err := imaptest.Start("user@gmail.com", "apppass", imaptest.GmailBoxes())
	if err != nil {
		t.Fatalf("start imap: %v", err)
	}
	t.Cleanup(func() { _ = imapSrv.Close() })

	dir := t.TempDir()
	cfg := config.Config{
		Addr:             ":0",
		DataDir:          dir,
		DBPath:           filepath.Join(dir, "test.db"),
		AdminUser:        "admin",
		AdminPassword:    "secret-pass",
		SessionTTL:       time.Hour,
		IMAPHost:         imapSrv.Host,
		IMAPPort:         imapSrv.Port,
		FetchConcurrency: 3,
		FetchTimeout:     10 * time.Second,
		FetchLimit:       10,
		// Test IMAP server is plaintext on loopback.
		PlaintextIMAP: true,
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secret.New("test-secret", dir)
	if err != nil {
		t.Fatal(err)
	}
	apiServer := New(cfg, st, box, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("spa"))
	}), BuildInfo{Version: "test", Commit: "test"})
	srv := httptestServer(t, apiServer.Handler())
	jar := newJar()
	return &testEnv{srv: srv, client: newClient(jar), store: st, cfg: cfg}, imapSrv
}

func TestFetchEndToEnd(t *testing.T) {
	e, imapSrv := newIMAPEnv(t)
	imapSrv.AddUser("other@gmail.com", "apppass")
	e.login(t)

	code, body := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "user@gmail.com", "password": "pw", "appPassword": "apppass",
	})
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}

	// Single fetch: 10 newest across inbox + spam + trash
	code, body = e.do(t, "GET", "/api/fetch/1?limit=10", nil)
	if code != 200 {
		t.Fatalf("fetch: %d %s", code, body)
	}
	var res struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		AccountID int64  `json:"accountId"`
		Messages  []struct {
			UID         uint32 `json:"uid"`
			Subject     string `json:"subject"`
			Mailbox     string `json:"mailbox"`
			FolderLabel string `json:"folderLabel"`
			Snippet     string `json:"snippet"`
			Seen        bool   `json:"seen"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v %s", err, body)
	}
	if !res.OK {
		t.Fatalf("fetch failed: %s", body)
	}
	if res.AccountID != 1 {
		t.Errorf("accountId = %d", res.AccountID)
	}
	if len(res.Messages) != 4 {
		t.Fatalf("expected 4 messages (inbox 2 + spam 1 + trash 1), got %d: %s", len(res.Messages), body)
	}
	if res.Messages[0].Subject != "Spam message" {
		t.Errorf("newest should be the spam message, got %q", res.Messages[0].Subject)
	}
	labels := map[string]bool{}
	for _, m := range res.Messages {
		labels[m.FolderLabel] = true
	}
	for _, want := range []string{"收件箱", "垃圾邮件", "已删除"} {
		if !labels[want] {
			t.Errorf("missing label %q in %s", want, body)
		}
	}

	// Fetch status is persisted for the list view
	acc, _ := e.store.Get(t.Context(), 1)
	if acc.LastFetchStatus != "ok" || acc.LastFetchSubject == "" || acc.LastFetchAt == 0 {
		t.Fatalf("fetch result not persisted: %+v", acc)
	}

	// Folder filter
	code, body = e.do(t, "GET", "/api/fetch/1?limit=10&folder=inbox", nil)
	var inboxRes struct {
		Messages []struct {
			FolderLabel string `json:"folderLabel"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &inboxRes)
	if code != 200 || len(inboxRes.Messages) != 2 {
		t.Fatalf("inbox filter: %d %s", code, body)
	}
	for _, m := range inboxRes.Messages {
		if m.FolderLabel != "收件箱" {
			t.Errorf("unexpected label %q", m.FolderLabel)
		}
	}

	// Message detail
	code, body = e.do(t, "GET", "/api/fetch/1/message?mailbox=INBOX&uid=12", nil)
	if code != 200 {
		t.Fatalf("message: %d %s", code, body)
	}
	var detail struct {
		Subject string `json:"subject"`
		Text    string `json:"text"`
		HTML    string `json:"html"`
		From    string `json:"from"`
	}
	_ = json.Unmarshal(body, &detail)
	if detail.Subject != "Inbox newest" {
		t.Errorf("detail subject = %q", detail.Subject)
	}
	if !strings.Contains(detail.Text, "newest body") {
		t.Errorf("detail text = %q", detail.Text)
	}
	if detail.From != "b@example.com" {
		t.Errorf("detail from = %q", detail.From)
	}

	// Unknown UID -> 502 (server rejects)
	if code, _ := e.do(t, "GET", "/api/fetch/1/message?mailbox=INBOX&uid=999", nil); code != 502 {
		t.Errorf("missing message should be 502, got %d", code)
	}

	// Batch fetch: one newest message per account
	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "other@gmail.com", "appPassword": "apppass"}); code != 201 {
		t.Fatal("create second account")
	}
	code, body = e.do(t, "POST", "/api/fetch/batch", map[string]any{"ids": []int64{1, 2}, "limit": 1})
	if code != 200 {
		t.Fatalf("batch: %d %s", code, body)
	}
	var batch struct {
		Results []struct {
			AccountID int64  `json:"accountId"`
			OK        bool   `json:"ok"`
			Error     string `json:"error"`
			Messages  []struct {
				Subject string `json:"subject"`
			} `json:"messages"`
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &batch)
	if len(batch.Results) != 2 {
		t.Fatalf("batch results: %s", body)
	}
	for _, r := range batch.Results {
		if !r.OK {
			t.Errorf("account %d failed: %s", r.AccountID, r.Error)
		}
		if len(r.Messages) != 1 {
			t.Errorf("account %d should return exactly 1 message, got %d", r.AccountID, len(r.Messages))
		}
	}
}

func TestFetchEndToEndBadPassword(t *testing.T) {
	e, _ := newIMAPEnv(t)
	e.login(t)

	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "user@gmail.com", "appPassword": "wrong"}); code != 201 {
		t.Fatal("create")
	}
	code, body := e.do(t, "GET", "/api/fetch/1?limit=5", nil)
	if code != 200 {
		t.Fatalf("fetch: %d %s", code, body)
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &res)
	if res.OK {
		t.Fatalf("should fail with wrong app password: %s", body)
	}
	if !strings.Contains(res.Error, "登录失败") {
		t.Errorf("expected friendly login error, got %q", res.Error)
	}

	acc, _ := e.store.Get(t.Context(), 1)
	if acc.LastFetchStatus != "error" || acc.LastFetchError == "" {
		t.Fatalf("failure not persisted: %+v", acc)
	}

	// Verify endpoint reports the same problem
	code, body = e.do(t, "POST", "/api/accounts/1/verify", nil)
	var verify struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &verify)
	if code != 200 || verify.OK {
		t.Fatalf("verify should fail: %d %s", code, body)
	}
	if !strings.Contains(verify.Message, "登录失败") {
		t.Errorf("verify message = %q", verify.Message)
	}
}

func TestVerifyEndToEndSuccess(t *testing.T) {
	e, _ := newIMAPEnv(t)
	e.login(t)
	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "user@gmail.com", "appPassword": "apppass"}); code != 201 {
		t.Fatal("create")
	}
	code, body := e.do(t, "POST", "/api/accounts/1/verify", nil)
	var verify struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &verify)
	if code != 200 || !verify.OK {
		t.Fatalf("verify should succeed: %d %s", code, body)
	}
}
