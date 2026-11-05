package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gmailmanager/internal/config"
	"gmailmanager/internal/secret"
	"gmailmanager/internal/store"
)

type testEnv struct {
	srv    *httptest.Server
	client *http.Client
	store  *store.Store
	cfg    config.Config
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		Addr:             ":0",
		DataDir:          dir,
		DBPath:           filepath.Join(dir, "test.db"),
		AdminUser:        "admin",
		AdminPassword:    "secret-pass",
		SessionTTL:       time.Hour,
		IMAPHost:         "127.0.0.1",
		IMAPPort:         1, // nothing listens here; fetch fails fast
		FetchConcurrency: 2,
		FetchTimeout:     2 * time.Second,
		FetchLimit:       10,
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
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<!doctype html><title>spa</title>")
	})
	api := New(cfg, st, box, assets, BuildInfo{Version: "test", Commit: "test"})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	return &testEnv{srv: srv, client: &http.Client{Jar: jar, Timeout: 20 * time.Second}, store: st, cfg: cfg}
}

func httptestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func newJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}

func newClient(jar http.CookieJar) *http.Client {
	return &http.Client{Jar: jar, Timeout: 20 * time.Second}
}

func (e *testEnv) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, data
}

func (e *testEnv) login(t *testing.T) {
	t.Helper()
	code, body := e.do(t, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "secret-pass"})
	if code != 200 {
		t.Fatalf("login status %d: %s", code, body)
	}
}

func TestAuthFlow(t *testing.T) {
	e := newTestEnv(t)

	if code, _ := e.do(t, "GET", "/api/accounts", nil); code != 401 {
		t.Fatalf("unauthenticated list should be 401, got %d", code)
	}

	code, body := e.do(t, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "wrong"})
	if code != 401 {
		t.Fatalf("bad login status %d", code)
	}
	if !strings.Contains(string(body), "用户名或密码错误") {
		t.Fatalf("unexpected error: %s", body)
	}

	e.login(t)
	code, body = e.do(t, "GET", "/api/auth/me", nil)
	if code != 200 || !strings.Contains(string(body), "admin") {
		t.Fatalf("me: %d %s", code, body)
	}

	if code, _ := e.do(t, "POST", "/api/auth/logout", nil); code != 200 {
		t.Fatal("logout failed")
	}
	if code, _ := e.do(t, "GET", "/api/accounts", nil); code != 401 {
		t.Fatalf("after logout should be 401, got %d", code)
	}
}

func TestAccountCRUDAndExport(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	code, body := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "User@Gmail.com", "password": "pw", "totpSecret": "jbswy3dpehpk3pxp",
		"appPassword": "abcd efgh ijkl mnop", "note": "n1",
	})
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	var created struct{ ID int64 }
	_ = json.Unmarshal(body, &created)

	// Duplicate
	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "user@gmail.com", "appPassword": "x"}); code != 409 {
		t.Fatalf("duplicate should be 409, got %d", code)
	}

	// Invalid email
	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "nope", "appPassword": "x"}); code != 400 {
		t.Fatalf("invalid email should be 400, got %d", code)
	}

	// Missing app password
	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "a@b.com"}); code != 400 {
		t.Fatalf("missing app password should be 400, got %d", code)
	}

	code, body = e.do(t, "GET", "/api/accounts?page=1&pageSize=10", nil)
	if code != 200 {
		t.Fatalf("list: %d", code)
	}
	var page struct {
		Items []struct {
			ID             int64  `json:"id"`
			Email          string `json:"email"`
			Password       string `json:"password"`
			AppPassword    string `json:"appPassword"`
			HasAppPassword bool   `json:"hasAppPassword"`
		} `json:"items"`
		Total int `json:"total"`
	}
	_ = json.Unmarshal(body, &page)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("list payload: %s", body)
	}
	if page.Items[0].Email != "user@gmail.com" {
		t.Errorf("email should be normalized, got %q", page.Items[0].Email)
	}
	if page.Items[0].Password != mask || page.Items[0].AppPassword != mask {
		t.Errorf("secrets must be masked in list: %s", body)
	}

	// Update keeps masked secrets
	code, body = e.do(t, "PUT", "/api/accounts/1", map[string]string{
		"email": "user@gmail.com", "password": mask, "totpSecret": mask, "appPassword": mask, "note": "updated",
	})
	if code != 200 {
		t.Fatalf("update: %d %s", code, body)
	}
	acc, _ := e.store.Get(t.Context(), created.ID)
	if acc.AppPassword == "" || acc.AppPassword == mask {
		t.Fatalf("app password should still be encrypted, got %q", acc.AppPassword)
	}

	// Export round trip
	code, body = e.do(t, "GET", "/api/accounts/export", nil)
	if code != 200 {
		t.Fatalf("export: %d", code)
	}
	want := "user@gmail.com----pw----JBSWY3DPEHPK3PXP----abcdefghijklmnop\n"
	if string(body) != want {
		t.Fatalf("export = %q, want %q", body, want)
	}

	// TOTP endpoint
	code, body = e.do(t, "GET", "/api/accounts/1/totp", nil)
	if code != 200 || !strings.Contains(string(body), "code") {
		t.Fatalf("totp: %d %s", code, body)
	}

	// Delete
	if code, _ := e.do(t, "DELETE", "/api/accounts/1", nil); code != 200 {
		t.Fatal("delete failed")
	}
	if code, _ := e.do(t, "DELETE", "/api/accounts/1", nil); code != 404 {
		t.Fatal("second delete should 404")
	}
}

func TestImportFlow(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	content := strings.Join([]string{
		"a@gmail.com----p1----SECRET1----app1",
		"b@gmail.com----p2--------app2",
		"broken-line",
		"c@gmail.com----p3----SECRET3----app3",
	}, "\n")

	code, body := e.do(t, "POST", "/api/accounts/import", map[string]any{"content": content, "overwrite": false})
	if code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	if raw["created"].(float64) != 3 || raw["failed"].(float64) != 1 {
		t.Fatalf("import summary: %s", body)
	}

	// Re-import without overwrite skips everything
	code, body = e.do(t, "POST", "/api/accounts/import", map[string]any{"content": content, "overwrite": false})
	_ = json.Unmarshal(body, &raw)
	if raw["created"].(float64) != 0 || raw["skipped"].(float64) != 3 {
		t.Fatalf("re-import should skip: %s", body)
	}

	// Overwrite updates
	code, body = e.do(t, "POST", "/api/accounts/import", map[string]any{
		"content": "a@gmail.com----newpass----SECRETNEW----appnew", "overwrite": true,
	})
	_ = json.Unmarshal(body, &raw)
	if raw["updated"].(float64) != 1 {
		t.Fatalf("overwrite should update: %s", body)
	}

	code, body = e.do(t, "GET", "/api/accounts/export", nil)
	if !strings.Contains(string(body), "a@gmail.com----newpass----SECRETNEW----appnew") {
		t.Fatalf("export after overwrite: %s", body)
	}

	// Batch delete
	var page struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	_, body = e.do(t, "GET", "/api/accounts?page=1&pageSize=10", nil)
	_ = json.Unmarshal(body, &page)
	ids := []int64{}
	for _, it := range page.Items {
		ids = append(ids, it.ID)
	}
	code, body = e.do(t, "POST", "/api/accounts/batch-delete", map[string]any{"ids": ids})
	if code != 200 {
		t.Fatalf("batch delete: %d %s", code, body)
	}
	_, body = e.do(t, "GET", "/api/accounts?page=1&pageSize=10", nil)
	_ = json.Unmarshal(body, &raw)
	if raw["total"].(float64) != 0 {
		t.Fatalf("should be empty: %s", body)
	}
}

func TestFetchEndpointReportsConnectionError(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	code, body := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "x@gmail.com", "appPassword": "app",
	})
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}

	code, body = e.do(t, "GET", "/api/fetch/1?limit=10", nil)
	if code != 200 {
		t.Fatalf("fetch: %d %s", code, body)
	}
	var res struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		AccountID int64  `json:"accountId"`
	}
	_ = json.Unmarshal(body, &res)
	if res.OK {
		t.Fatalf("fetch should fail against a closed port: %s", body)
	}
	if res.Error == "" || res.AccountID != 1 {
		t.Fatalf("unexpected fetch payload: %s", body)
	}

	// Account without app password
	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{"email": "y@gmail.com", "appPassword": "app"}); code != 201 {
		t.Fatal("create y")
	}
	acc, _ := e.store.Get(t.Context(), 2)
	acc.AppPassword = ""
	_ = e.store.Update(t.Context(), acc)
	code, body = e.do(t, "GET", "/api/fetch/2", nil)
	if code != 200 || !strings.Contains(string(body), "应用专用密码") {
		t.Fatalf("missing app password: %d %s", code, body)
	}

	// Batch endpoint validates input
	if code, _ := e.do(t, "POST", "/api/fetch/batch", map[string]any{"ids": []int64{}}); code != 400 {
		t.Fatal("empty batch should be 400")
	}
	tooMany := make([]int64, 51)
	if code, _ := e.do(t, "POST", "/api/fetch/batch", map[string]any{"ids": tooMany}); code != 400 {
		t.Fatal("oversized batch should be 400")
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
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &batch)
	if len(batch.Results) != 2 {
		t.Fatalf("batch results: %s", body)
	}
	for _, r := range batch.Results {
		if r.OK || r.Error == "" {
			t.Fatalf("expected failures: %s", body)
		}
	}

	// Message endpoint argument validation
	if code, _ := e.do(t, "GET", "/api/fetch/1/message?uid=abc", nil); code != 400 {
		t.Fatal("bad uid should be 400")
	}
	if code, _ := e.do(t, "GET", "/api/fetch/1/message?uid=5", nil); code != 502 {
		t.Fatalf("unreachable imap should be 502, got %d", code)
	}
}

func TestSPAFallback(t *testing.T) {
	e := newTestEnv(t)
	res, err := e.client.Get(e.srv.URL + "/some/deep/link")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), "spa") {
		t.Fatalf("fallback: %d %s", res.StatusCode, body)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing security header")
	}
}

func TestSessionCookieIsHttpOnly(t *testing.T) {
	e := newTestEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			if !c.HttpOnly {
				t.Error("session cookie must be HttpOnly")
			}
			return
		}
	}
	t.Fatal("session cookie not set")
}
