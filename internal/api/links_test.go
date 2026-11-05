package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestShareLinkEndToEnd covers the full public-link flow against a real
// in-memory IMAP server: admin generates a link, an anonymous client uses it.
func TestShareLinkEndToEnd(t *testing.T) {
	e, imapSrv := newIMAPEnv(t)
	imapSrv.AddUser("other@gmail.com", "apppass")
	e.login(t)

	if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{
		"email": "user@gmail.com", "appPassword": "apppass",
	}); code != 201 {
		t.Fatal("create account")
	}

	// --- admin generates a link ------------------------------------------
	code, body := e.do(t, "GET", "/api/accounts/1/link", nil)
	if code != 200 {
		t.Fatalf("link: %d %s", code, body)
	}
	var link struct {
		AccountID int64  `json:"accountId"`
		Email     string `json:"email"`
		Token     string `json:"token"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(body, &link); err != nil {
		t.Fatalf("decode: %v %s", err, body)
	}
	if link.Token == "" || link.AccountID != 1 {
		t.Fatalf("bad link payload: %s", body)
	}
	if !strings.Contains(link.URL, "/f/"+link.Token) {
		t.Errorf("url should embed the token: %q", link.URL)
	}

	// Generating twice returns the same token (stable link).
	_, body2 := e.do(t, "GET", "/api/accounts/1/link", nil)
	var link2 struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(body2, &link2)
	if link2.Token != link.Token {
		t.Errorf("token should be stable: %q vs %q", link.Token, link2.Token)
	}

	// --- anonymous client: no cookies at all ----------------------------
	anon := newClient(newJar())
	pub := func(path string) (*http.Response, []byte) {
		res, err := anon.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var buf strings.Builder
		_, _ = copyTo(&buf, res.Body)
		return res, []byte(buf.String())
	}

	// Info
	res, raw := pub("/api/public/f/" + link.Token)
	if res.StatusCode != 200 {
		t.Fatalf("public info: %d %s", res.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "user@gmail.com") {
		t.Errorf("info should name the account: %s", raw)
	}

	// Messages (newest 10, includes spam/trash)
	res, raw = pub("/api/public/f/" + link.Token + "/messages?limit=10")
	if res.StatusCode != 200 {
		t.Fatalf("public messages: %d %s", res.StatusCode, raw)
	}
	var fres struct {
		OK       bool `json:"ok"`
		Messages []struct {
			UID     uint32 `json:"uid"`
			Subject string `json:"subject"`
			Mailbox string `json:"mailbox"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &fres)
	if !fres.OK {
		t.Fatalf("public fetch failed: %s", raw)
	}
	if len(fres.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %s", len(fres.Messages), raw)
	}

	// Single message body
	target := fres.Messages[0]
	res, raw = pub("/api/public/f/" + link.Token + "/message?uid=" +
		itoa(int64(target.UID)) + "&mailbox=" + target.Mailbox)
	if res.StatusCode != 200 {
		t.Fatalf("public message: %d %s", res.StatusCode, raw)
	}
	if !strings.Contains(string(raw), `"subject"`) {
		t.Errorf("detail should include a subject: %s", raw)
	}

	// --- revoke: the same token must stop working ------------------------
	if code, _ := e.do(t, "POST", "/api/accounts/1/link/revoke", nil); code != 200 {
		t.Fatal("revoke failed")
	}
	res, raw = pub("/api/public/f/" + link.Token)
	if res.StatusCode != 404 {
		t.Fatalf("revoked token should 404, got %d %s", res.StatusCode, raw)
	}

	// --- private endpoints stay protected --------------------------------
	res, _ = pub("/api/accounts")
	if res.StatusCode != 401 {
		t.Fatalf("admin API must reject anonymous, got %d", res.StatusCode)
	}
}

func TestBulkLinksAndExport(t *testing.T) {
	e := newTestEnv(t)
	e.login(t)

	for _, email := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		if code, _ := e.do(t, "POST", "/api/accounts", map[string]string{
			"email": email, "appPassword": "app",
		}); code != 201 {
			t.Fatal("create", email)
		}
	}

	code, body := e.do(t, "POST", "/api/accounts/links", map[string]any{"ids": []int64{1, 3}})
	if code != 200 {
		t.Fatalf("bulk links: %d %s", code, body)
	}
	var bulk struct {
		Links []struct {
			AccountID int64  `json:"accountId"`
			URL       string `json:"url"`
		} `json:"links"`
	}
	_ = json.Unmarshal(body, &bulk)
	if len(bulk.Links) != 2 {
		t.Fatalf("expected 2 links: %s", body)
	}
	for _, l := range bulk.Links {
		if l.URL == "" {
			t.Errorf("empty url for account %d", l.AccountID)
		}
	}

	// Export for the same selection.
	code, body = e.do(t, "GET", "/api/accounts/links/export?ids=1,3", nil)
	if code != 200 {
		t.Fatalf("export: %d %s", code, body)
	}
	text := string(body)
	for _, email := range []string{"a@x.com", "c@x.com"} {
		if !strings.Contains(text, email) {
			t.Errorf("export missing %s:\n%s", email, text)
		}
	}
	if strings.Contains(text, "b@x.com") {
		t.Errorf("export should only include the selection:\n%s", text)
	}
	if lines := strings.Count(strings.TrimSpace(text), "\n") + 1; lines < 3 {
		t.Errorf("export should have a header block plus rows:\n%s", text)
	}
}

func TestInvalidTokenIs404(t *testing.T) {
	e := newTestEnv(t)
	anon := newClient(newJar())
	for _, path := range []string{
		"/api/public/f/deadbeef",
		"/api/public/f/deadbeef/messages",
		"/api/public/f/deadbeef/message?uid=1",
	} {
		res, err := anon.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Errorf("%s: want 404, got %d", path, res.StatusCode)
		}
	}
}

func TestPublicPageServesSPA(t *testing.T) {
	e := newTestEnv(t)
	res, err := e.client.Get(e.srv.URL + "/f/sometoken")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("public page should serve the SPA, got %d", res.StatusCode)
	}
}

func copyTo(dst *strings.Builder, src interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 4096)
	var n int64
	for {
		c, err := src.Read(buf)
		dst.Write(buf[:c])
		n += int64(c)
		if err != nil {
			return n, nil
		}
	}
}

var _ = httptest.NewRecorder
