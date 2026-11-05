package imapx

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"gmailmanager/internal/imapx/imaptest"
)

// startTestServer boots an in-memory IMAP server on a random port.
func startTestServer(t *testing.T, boxes []*imaptest.Mailbox) (string, *imaptest.Server) {
	t.Helper()
	srv, err := imaptest.Start("user@gmail.com", "apppass", boxes)
	if err != nil {
		t.Fatalf("start imap server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv.Addr, srv
}

func defaultBoxes() []*imaptest.Mailbox { return imaptest.GmailBoxes() }

func sampleMessage(subject, from string, date time.Time, body string) []byte {
	return imaptest.TextMessage(subject, from, date, body)
}

func newTestFetcher(addr string) *Fetcher {
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return New(Config{Host: host, Port: port, Timeout: 10 * time.Second, Plaintext: true})
}

// ---- tests ----

func TestLatestMergesInboxSpamAndTrash(t *testing.T) {
	addr, _ := startTestServer(t, defaultBoxes())
	f := newTestFetcher(addr)

	res, err := f.Latest(context.Background(), Credentials{Email: "user@gmail.com", AppPassword: "apppass"}, 10, FolderAuto)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected ok, got error %q", res.Error)
	}
	if len(res.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(res.Messages))
	}
	// Sorted newest first: spam(-30m), inbox newest(-1h), inbox older(-3h), trash(-48h)
	if res.Messages[0].Subject != "Spam message" {
		t.Errorf("first subject = %q, want spam message", res.Messages[0].Subject)
	}
	if res.Messages[1].Subject != "Inbox newest" {
		t.Errorf("second subject = %q", res.Messages[1].Subject)
	}
	if res.Messages[3].Subject != "Trash message" {
		t.Errorf("last subject = %q", res.Messages[3].Subject)
	}
	labels := map[string]bool{}
	for _, m := range res.Messages {
		labels[m.FolderLabel] = true
	}
	for _, want := range []string{"收件箱", "垃圾邮件", "已删除"} {
		if !labels[want] {
			t.Errorf("missing folder label %q", want)
		}
	}
	if res.Messages[1].Snippet == "" {
		t.Errorf("expected snippet for inbox message")
	}
	if !res.Messages[1].Seen {
		t.Errorf("expected \\Seen flag to be detected")
	}
}

func TestLatestRespectsLimitAndFolder(t *testing.T) {
	addr, _ := startTestServer(t, defaultBoxes())
	f := newTestFetcher(addr)
	cred := Credentials{Email: "user@gmail.com", AppPassword: "apppass"}

	res, err := f.Latest(context.Background(), cred, 1, FolderAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Subject != "Spam message" {
		t.Fatalf("limit=1 should return only the newest message, got %+v", res.Messages)
	}

	res, err = f.Latest(context.Background(), cred, 10, FolderInbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("inbox only should return 2, got %d", len(res.Messages))
	}
	for _, m := range res.Messages {
		if m.FolderLabel != "收件箱" {
			t.Errorf("unexpected label %q", m.FolderLabel)
		}
	}

	res, err = f.Latest(context.Background(), cred, 10, FolderSpam)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Subject != "Spam message" {
		t.Fatalf("spam only returned %+v", res.Messages)
	}
}

func TestLatestBadPassword(t *testing.T) {
	addr, _ := startTestServer(t, defaultBoxes())
	f := newTestFetcher(addr)

	res, err := f.Latest(context.Background(), Credentials{Email: "user@gmail.com", AppPassword: "wrong"}, 10, FolderAuto)
	if err == nil {
		t.Fatal("expected login error")
	}
	if res.OK {
		t.Fatal("result should not be ok")
	}
	if !strings.Contains(res.Error, "登录失败") {
		t.Errorf("expected friendly error, got %q", res.Error)
	}
}

func TestVerify(t *testing.T) {
	addr, _ := startTestServer(t, defaultBoxes())
	f := newTestFetcher(addr)

	if err := f.Verify(context.Background(), Credentials{Email: "user@gmail.com", AppPassword: "apppass"}); err != nil {
		t.Fatalf("verify should succeed: %v", err)
	}
	if err := f.Verify(context.Background(), Credentials{Email: "user@gmail.com", AppPassword: "nope"}); err == nil {
		t.Fatal("verify should fail with a bad password")
	}
}

func TestMessageDetail(t *testing.T) {
	boxes := defaultBoxes()
	html := "<html><body><p>Hello <b>world</b></p><script>alert(1)</script></body></html>"
	boxes[0].Messages = append(boxes[0].Messages, &imaptest.Message{
		UID:  13,
		Date: time.Now(),
		Body: []byte("From: Sender <s@example.com>\r\nTo: me@example.com\r\nSubject: HTML mail\r\nDate: " +
			time.Now().Format(time.RFC1123Z) + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + html),
	})
	addr, _ := startTestServer(t, boxes)
	f := newTestFetcher(addr)
	cred := Credentials{Email: "user@gmail.com", AppPassword: "apppass"}

	detail, err := f.Message(context.Background(), cred, "INBOX", 13)
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	if detail.Subject != "HTML mail" {
		t.Errorf("subject = %q", detail.Subject)
	}
	if !strings.Contains(detail.HTML, "Hello") {
		t.Errorf("html body missing: %q", detail.HTML)
	}
	if strings.Contains(detail.HTML, "<script") {
		t.Errorf("script tag should be stripped: %q", detail.HTML)
	}
	if detail.From != "s@example.com" || detail.FromName != "Sender" {
		t.Errorf("from parse wrong: %q %q", detail.From, detail.FromName)
	}
}

func TestBatchPreservesOrder(t *testing.T) {
	addr, _ := startTestServer(t, defaultBoxes())
	f := newTestFetcher(addr)

	creds := []Credentials{
		{Email: "user@gmail.com", AppPassword: "apppass"},
		{Email: "user@gmail.com", AppPassword: "bad"},
	}
	results := f.Batch(context.Background(), creds, 1, FolderAuto, 2)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].OK {
		t.Errorf("first should succeed: %+v", results[0])
	}
	if results[1].OK {
		t.Errorf("second should fail")
	}
}
