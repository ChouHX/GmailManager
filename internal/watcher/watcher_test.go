package watcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gmailmanager/internal/imapx"
	"gmailmanager/internal/imapx/imaptest"
	"gmailmanager/internal/secret"
	"gmailmanager/internal/store"
)

type env struct {
	store   *store.Store
	box     *secret.Box
	watcher *Watcher
	imapSrv *imaptest.Server
	hookURL string
	hits    *atomic.Int32
}

func newEnv(t *testing.T) *env {
	t.Helper()
	imapSrv, err := imaptest.Start("user@gmail.com", "apppass", imaptest.GmailBoxes())
	if err != nil {
		t.Fatalf("start imap: %v", err)
	}
	t.Cleanup(func() { _ = imapSrv.Close() })

	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secret.New("test-secret", dir)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := imapx.New(imapx.Config{Host: imapSrv.Host, Port: imapSrv.Port, Timeout: 10 * time.Second, Plaintext: true})

	hits := &atomic.Int32{}
	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hookSrv.Close)

	return &env{
		store:   st,
		box:     box,
		watcher: New(st, box, fetcher, 2),
		imapSrv: imapSrv,
		hookURL: hookSrv.URL + "/api/webhooks/1/token",
		hits:    hits,
	}
}

func (e *env) createAccount(t *testing.T) int64 {
	t.Helper()
	enc, err := e.box.Encrypt("apppass")
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.store.Create(context.Background(), store.Account{Email: "user@gmail.com", AppPassword: enc})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRunNowMatchesSubjectAndFrom(t *testing.T) {
	e := newEnv(t)
	accID := e.createAccount(t)

	task := store.Watch{
		AccountID: accID, Enabled: true, Name: "收件箱",
		Subject: "inbox", From: "b@example.com", Folder: imapx.FolderInbox,
		Interval: 60, WebhookURL: e.hookURL,
	}
	id, err := e.store.CreateWatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	task.ID = id

	res, err := e.watcher.RunNow(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || len(res.Hits) != 1 || res.Hits[0].Subject != "Inbox newest" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !res.Notified || e.hits.Load() != 1 {
		t.Fatalf("expected 1 webhook call, got %d", e.hits.Load())
	}

	saved, err := e.store.GetWatch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastStatus != "ok" || saved.LastHitCount != 1 || saved.TotalHits != 1 || saved.LastNotified == 0 {
		t.Fatalf("store not updated: %+v", saved)
	}
	if saved.LastRunDuration <= 0 {
		t.Fatalf("duration not recorded: %+v", saved)
	}
}

func TestRunNowWebhookFailureIsReported(t *testing.T) {
	e := newEnv(t)
	accID := e.createAccount(t)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	t.Cleanup(broken.Close)

	task := store.Watch{AccountID: accID, Enabled: true, Subject: "inbox", Folder: imapx.FolderInbox, Interval: 60,
		WebhookURL: broken.URL + "/api/webhooks/1/x"}
	id, err := e.store.CreateWatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	task.ID = id

	res, err := e.watcher.RunNow(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Notified || !strings.Contains(res.Error, "webhook 推送失败") {
		t.Fatalf("unexpected result: %+v", res)
	}
	saved, _ := e.store.GetWatch(context.Background(), id)
	if saved.LastStatus != "error" {
		t.Fatalf("expected error status, got %+v", saved)
	}
}

func TestRunNowDisabledAndMissingPassword(t *testing.T) {
	e := newEnv(t)
	accID := e.createAccount(t)

	disabled := store.Watch{ID: 1, AccountID: accID, Enabled: false, Subject: "x", Interval: 60, WebhookURL: e.hookURL}
	if _, err := e.store.CreateWatch(context.Background(), disabled); err != nil {
		t.Fatal(err)
	}
	res, err := e.watcher.RunNow(context.Background(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Error != "任务已停用" {
		t.Fatalf("unexpected: %+v", res)
	}

	empty, err := e.store.Create(context.Background(), store.Account{Email: "nopass@gmail.com"})
	if err != nil {
		t.Fatal(err)
	}
	noPass := store.Watch{AccountID: empty, Enabled: true, Subject: "x", Interval: 60, WebhookURL: e.hookURL}
	if _, err := e.store.CreateWatch(context.Background(), noPass); err != nil {
		t.Fatal(err)
	}
	res, err = e.watcher.RunNow(context.Background(), noPass)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || !strings.Contains(res.Error, "应用专用密码") {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestSchedulerFiresDueTask(t *testing.T) {
	e := newEnv(t)
	accID := e.createAccount(t)

	task := store.Watch{AccountID: accID, Enabled: true, Name: "定时", Subject: "inbox",
		Folder: imapx.FolderInbox, Interval: 60, WebhookURL: e.hookURL}
	id, err := e.store.CreateWatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	// Pretend the task ran 2 minutes ago so the first tick picks it up.
	if _, err := e.store.DB().Exec(`UPDATE watches SET last_run_at = ? WHERE id = ?`, time.Now().Unix()-120, id); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.watcher.Start(ctx)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if e.hits.Load() >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e.hits.Load() != 1 {
		t.Fatalf("expected scheduled webhook call, got %d", e.hits.Load())
	}
	saved, err := e.store.GetWatch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastStatus != "ok" || saved.TotalHits != 2 {
		t.Fatalf("unexpected stored state: %+v", saved)
	}
}

func TestRunNowAllAccountsPollsWholePool(t *testing.T) {
	e := newEnv(t)
	e.imapSrv.AddUser("other@gmail.com", "apppass")

	enc, err := e.box.Encrypt("apppass")
	if err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"user@gmail.com", "other@gmail.com"} {
		if _, err := e.store.Create(context.Background(), store.Account{Email: email, AppPassword: enc}); err != nil {
			t.Fatal(err)
		}
	}
	// An account without an app password must be skipped, not fail the run.
	if _, err := e.store.Create(context.Background(), store.Account{Email: "nopass@gmail.com"}); err != nil {
		t.Fatal(err)
	}

	task := store.Watch{AccountID: store.AllAccountsID, Enabled: true, Subject: "inbox",
		Folder: imapx.FolderInbox, Interval: 60, WebhookURL: e.hookURL}
	id, err := e.store.CreateWatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	task.ID = id

	res, err := e.watcher.RunNow(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok, got %+v", res)
	}
	if res.Accounts != 3 {
		t.Fatalf("expected 3 accounts considered, got %d", res.Accounts)
	}
	// Two mailboxes share the same INBOX, which holds 2 matching messages.
	if len(res.Hits) != 4 {
		t.Fatalf("expected 4 hits, got %d (%+v)", len(res.Hits), res.Hits)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0], "nopass@gmail.com") {
		t.Fatalf("expected the passwordless account to be skipped, got %+v", res.Skipped)
	}
	if !res.Notified || e.hits.Load() != 1 {
		t.Fatalf("expected one webhook call, got %d", e.hits.Load())
	}

	saved, err := e.store.GetWatch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LastStatus != "ok" || saved.TotalHits != 4 {
		t.Fatalf("unexpected stored state: %+v", saved)
	}
	// All-accounts tasks carry no single email.
	if saved.Email != "" {
		t.Fatalf("all-accounts task should not expose an email, got %q", saved.Email)
	}
}

func TestRunNowEmptyPool(t *testing.T) {
	e := newEnv(t)
	task := store.Watch{AccountID: store.AllAccountsID, Enabled: true, Subject: "x",
		Interval: 60, WebhookURL: e.hookURL}
	id, err := e.store.CreateWatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	task.ID = id
	res, err := e.watcher.RunNow(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || !strings.Contains(res.Error, "号池为空") {
		t.Fatalf("unexpected: %+v", res)
	}
}
