// Package imapx fetches messages from Gmail over IMAP using app passwords.
package imapx

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/microcosm-cc/bluemonday"
)

// Folder keys understood by the API.
const (
	FolderAuto  = "auto"  // INBOX + spam + trash, merged by date
	FolderInbox = "inbox" // INBOX only
	FolderSpam  = "spam"
	FolderTrash = "trash"
)

func init() {
	// Teach go-message how to decode non UTF-8 charsets.
	message.CharsetReader = charset.Reader
	imap.CharsetReader = charset.Reader
}

// Credentials describes a mailbox login.
type Credentials struct {
	Email       string
	AppPassword string
}

// Config controls connection and fetch behaviour.
type Config struct {
	Host      string
	Port      int
	Timeout   time.Duration
	UserAgent string
	// Plaintext disables TLS. Only for tests; Gmail always requires TLS.
	Plaintext bool
}

// Fetcher connects to Gmail IMAP.
type Fetcher struct {
	cfg Config
}

// New creates a Fetcher.
func New(cfg Config) *Fetcher {
	if cfg.Host == "" {
		cfg.Host = "imap.gmail.com"
	}
	if cfg.Port == 0 {
		cfg.Port = 993
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Fetcher{cfg: cfg}
}

// Summary is the lightweight message header shown in list views.
type Summary struct {
	UID         uint32   `json:"uid"`
	Mailbox     string   `json:"mailbox"`
	Subject     string   `json:"subject"`
	From        string   `json:"from"`
	FromName    string   `json:"fromName"`
	To          string   `json:"to"`
	Date        int64    `json:"date"`
	Size        uint32   `json:"size"`
	Seen        bool     `json:"seen"`
	Flags       []string `json:"flags"`
	Snippet     string   `json:"snippet,omitempty"`
	MessageID   string   `json:"messageId,omitempty"`
	HasAttachmt bool     `json:"hasAttachment"`
	FolderLabel string   `json:"folderLabel"`
}

// Attachment describes one non-inline part.
type Attachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	PartPath string `json:"partPath"`
}

// Detail is the full message body plus attachments.
type Detail struct {
	Summary
	Text        string       `json:"text"`
	HTML        string       `json:"html"`
	RawHTML     bool         `json:"rawHtml"`
	Attachments []Attachment `json:"attachments"`
}

// FetchResult is the outcome of one account fetch.
type FetchResult struct {
	Email      string    `json:"email"`
	AccountID  int64     `json:"accountId"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	Messages   []Summary `json:"messages"`
	FetchedAt  int64     `json:"fetchedAt"`
	DurationMS int64     `json:"durationMs"`
}

var sanitizer = newSanitizer()

func newSanitizer() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("a", "b", "blockquote", "br", "caption", "code", "col", "colgroup", "dd", "div",
		"dl", "dt", "em", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "i", "img",
		"li", "ol", "p", "pre", "s", "small", "span", "strong", "sub", "sup", "table", "tbody", "td",
		"tfoot", "th", "thead", "tr", "u", "ul")
	p.AllowAttrs("href", "title").OnElements("a")
	p.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	p.AllowAttrs("align", "colspan", "rowspan", "width", "height", "border", "cellpadding", "cellspacing", "bgcolor", "valign").OnElements("table", "td", "th", "tr", "tbody", "thead", "tfoot")
	p.AllowAttrs("class", "style", "dir", "align").Globally()
	p.AllowStyles("color", "background-color", "font-weight", "font-style", "font-size", "font-family",
		"text-align", "text-decoration", "margin", "margin-left", "margin-right", "margin-top",
		"margin-bottom", "padding", "padding-left", "padding-right", "padding-top", "padding-bottom",
		"border", "border-collapse", "border-spacing", "width", "height", "max-width", "line-height",
		"vertical-align", "display", "white-space", "word-break", "overflow-wrap").Globally()
	p.AllowURLSchemes("http", "https", "mailto", "data")
	p.AllowDataAttributes()
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnFullyQualifiedLinks(true)
	return p
}

func (f *Fetcher) dial(ctx context.Context) (*client.Client, error) {
	addr := net.JoinHostPort(f.cfg.Host, strconv.Itoa(f.cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var c *client.Client
	var err error
	if f.cfg.Plaintext {
		c, err = client.DialWithDialer(dialer, addr)
	} else {
		c, err = client.DialWithDialerTLS(dialer, addr, &tls.Config{ServerName: f.cfg.Host, MinVersion: tls.VersionTLS12})
	}
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	c.Timeout = f.cfg.Timeout
	c.ErrorLog = log.New(io.Discard, "", 0)
	// Make sure a cancelled context tears the connection down.
	go func() {
		select {
		case <-ctx.Done():
			c.Terminate()
		case <-c.LoggedOut():
		}
	}()
	return c, nil
}

// Verify logs in and selects INBOX, closing the connection afterwards.
func (f *Fetcher) Verify(ctx context.Context, cred Credentials) error {
	ctx, cancel := context.WithTimeout(ctx, f.cfg.Timeout)
	defer cancel()

	c, err := f.dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		c.Logout()
		c.Terminate()
	}()
	if err := c.Login(cred.Email, cred.AppPassword); err != nil {
		return friendlyLoginErr(err)
	}
	if _, err := c.Select(imap.InboxName, true); err != nil {
		return fmt.Errorf("select INBOX: %w", err)
	}
	return nil
}

func friendlyLoginErr(err error) error {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "invalid credentials"),
		strings.Contains(msg, "authentication failed"),
		strings.Contains(msg, "username and password not accepted"),
		strings.Contains(msg, "invalid username or password"):
		return errors.New("登录失败：应用专用密码无效（请确认已开启两步验证并使用 16 位 App Password）")
	case strings.Contains(msg, "application-specific password required"):
		return errors.New("登录失败：需要应用专用密码，请勿使用账号主密码")
	case strings.Contains(msg, "too many"):
		return errors.New("登录失败：尝试次数过多，请稍后再试")
	case strings.Contains(msg, "lookup failed"):
		return errors.New("登录失败：账号不存在或已被停用（Lookup failed）")
	default:
		return fmt.Errorf("登录失败：%w", err)
	}
}

// mailboxLabel maps a raw IMAP mailbox name to its UI label.
func mailboxLabel(name string) string {
	lower := strings.ToLower(name)
	switch {
	case lower == "inbox":
		return "收件箱"
	case strings.Contains(lower, "spam"), strings.Contains(lower, "junk"):
		return "垃圾邮件"
	case strings.Contains(lower, "trash"), strings.Contains(lower, "deleted"):
		return "已删除"
	case strings.Contains(lower, "sent"):
		return "已发送"
	case strings.Contains(lower, "draft"):
		return "草稿"
	default:
		return name
	}
}

// mailboxTarget is a mailbox we want to read.
type mailboxTarget struct {
	name  string
	label string
}

func (f *Fetcher) resolveMailboxes(c *client.Client, folder string) []mailboxTarget {
	switch strings.ToLower(folder) {
	case FolderInbox:
		return []mailboxTarget{{imap.InboxName, "收件箱"}}
	case FolderSpam:
		return []mailboxTarget{{findSpam(c), "垃圾邮件"}}
	case FolderTrash:
		return []mailboxTarget{{findTrash(c), "已删除"}}
	default:
		return []mailboxTarget{
			{imap.InboxName, "收件箱"},
			{findSpam(c), "垃圾邮件"},
			{findTrash(c), "已删除"},
		}
	}
}

func listMailboxes(c *client.Client) []imap.MailboxInfo {
	ch := make(chan *imap.MailboxInfo, 64)
	go func() {
		_ = c.List("", "*", ch)
	}()
	var out []imap.MailboxInfo
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, *m)
		case <-deadline:
			return out
		}
	}
}

func hasAttr(m imap.MailboxInfo, attr string) bool {
	for _, a := range m.Attributes {
		if strings.EqualFold(a, attr) {
			return true
		}
	}
	return false
}

func findSpam(c *client.Client) string {
	boxes := listMailboxes(c)
	for _, m := range boxes {
		if hasAttr(m, imap.JunkAttr) {
			return m.Name
		}
	}
	for _, name := range []string{"[Gmail]/Spam", "[Google Mail]/Spam", "Spam", "Junk"} {
		for _, m := range boxes {
			if strings.EqualFold(m.Name, name) {
				return m.Name
			}
		}
	}
	return "[Gmail]/Spam"
}

func findTrash(c *client.Client) string {
	boxes := listMailboxes(c)
	for _, m := range boxes {
		if hasAttr(m, imap.TrashAttr) {
			return m.Name
		}
	}
	for _, name := range []string{"[Gmail]/Trash", "[Google Mail]/Trash", "Trash", "Deleted Messages"} {
		for _, m := range boxes {
			if strings.EqualFold(m.Name, name) {
				return m.Name
			}
		}
	}
	return "[Gmail]/Trash"
}

// Latest returns up to limit newest messages across the requested folder.
func (f *Fetcher) Latest(ctx context.Context, cred Credentials, limit int, folder string) (FetchResult, error) {
	start := time.Now()
	res := FetchResult{Email: cred.Email}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	ctx, cancel := context.WithTimeout(ctx, f.cfg.Timeout)
	defer cancel()

	c, err := f.dial(ctx)
	if err != nil {
		res.Error = err.Error()
		res.FetchedAt = time.Now().Unix()
		res.DurationMS = time.Since(start).Milliseconds()
		return res, err
	}
	defer func() {
		c.Logout()
		c.Terminate()
	}()

	if err := c.Login(cred.Email, cred.AppPassword); err != nil {
		err = friendlyLoginErr(err)
		res.Error = err.Error()
		res.FetchedAt = time.Now().Unix()
		res.DurationMS = time.Since(start).Milliseconds()
		return res, err
	}

	var all []Summary
	for _, box := range f.resolveMailboxes(c, folder) {
		msgs, err := f.fetchMailbox(c, box, limit)
		if err != nil {
			if len(all) == 0 {
				res.Error = fmt.Sprintf("%s: %v", box.label, err)
			}
			continue
		}
		all = append(all, msgs...)
	}

	sort.SliceStable(all, func(i, j int) bool { return all[i].Date > all[j].Date })
	if len(all) > limit {
		all = all[:limit]
	}
	res.Messages = all
	res.OK = res.Error == ""
	res.FetchedAt = time.Now().Unix()
	res.DurationMS = time.Since(start).Milliseconds()
	return res, nil
}

func (f *Fetcher) fetchMailbox(c *client.Client, box mailboxTarget, limit int) ([]Summary, error) {
	status, err := c.Select(box.name, true)
	if err != nil {
		return nil, err
	}
	if status.Messages == 0 {
		return nil, nil
	}
	from := uint32(1)
	if status.Messages > uint32(limit) {
		from = status.Messages - uint32(limit) + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, status.Messages)

	items := []imap.FetchItem{
		imap.FetchUid,
		imap.FetchFlags,
		imap.FetchInternalDate,
		imap.FetchRFC822Size,
		imap.FetchEnvelope,
		imap.FetchBodyStructure,
	}
	section := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.TextSpecifier}, Peek: true, Partial: []int{0, 2048}}
	items = append(items, section.FetchItem())

	ch := make(chan *imap.Message, 32)
	done := make(chan error, 1)
	go func() { done <- c.Fetch(seqset, items, ch) }()

	var out []Summary
	for msg := range ch {
		out = append(out, toSummary(msg, box, section))
	}
	if err := <-done; err != nil {
		return out, err
	}
	return out, nil
}

func toSummary(msg *imap.Message, box mailboxTarget, section *imap.BodySectionName) Summary {
	s := Summary{UID: msg.Uid, Mailbox: box.name, FolderLabel: box.label, Size: msg.Size}
	if !msg.InternalDate.IsZero() {
		s.Date = msg.InternalDate.Unix()
	}
	if msg.Envelope != nil {
		s.Subject = msg.Envelope.Subject
		s.MessageID = msg.Envelope.MessageId
		if !msg.Envelope.Date.IsZero() && s.Date == 0 {
			s.Date = msg.Envelope.Date.Unix()
		}
		if len(msg.Envelope.From) > 0 {
			from := msg.Envelope.From[0]
			s.From = from.Address()
			s.FromName = from.PersonalName
		}
		if len(msg.Envelope.To) > 0 {
			var tos []string
			for _, a := range msg.Envelope.To {
				tos = append(tos, a.Address())
			}
			s.To = strings.Join(tos, ", ")
		}
	}
	for _, fl := range msg.Flags {
		if strings.EqualFold(fl, imap.SeenFlag) {
			s.Seen = true
		}
	}
	s.Flags = msg.Flags
	if bs := msg.BodyStructure; bs != nil {
		s.HasAttachmt = bodyHasAttachment(bs)
	}
	if lit := msg.GetBody(section); lit != nil {
		if b, err := io.ReadAll(io.LimitReader(lit, 4096)); err == nil {
			s.Snippet = makeSnippet(string(b))
		}
	}
	return s
}

func bodyHasAttachment(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	if strings.EqualFold(bs.Disposition, "attachment") {
		return true
	}
	if strings.HasPrefix(strings.ToLower(bs.MIMEType+"/"+bs.MIMESubType), "image/") && strings.EqualFold(bs.Disposition, "inline") {
		return false
	}
	for _, p := range bs.Parts {
		if bodyHasAttachment(p) {
			return true
		}
	}
	return false
}

func makeSnippet(raw string) string {
	txt := htmlToText(raw)
	txt = strings.Join(strings.Fields(txt), " ")
	r := []rune(txt)
	if len(r) > 200 {
		return string(r[:200])
	}
	return txt
}

func htmlToText(raw string) string {
	repl := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</div>", "\n", "</tr>", "\n", "</td>", " ")
	s := repl.Replace(raw)
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'").Replace(out)
	return out
}

// Message fetches the full content of one message.
func (f *Fetcher) Message(ctx context.Context, cred Credentials, mailbox string, uid uint32) (Detail, error) {
	ctx, cancel := context.WithTimeout(ctx, f.cfg.Timeout)
	defer cancel()

	c, err := f.dial(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer func() {
		c.Logout()
		c.Terminate()
	}()

	if err := c.Login(cred.Email, cred.AppPassword); err != nil {
		return Detail{}, friendlyLoginErr(err)
	}
	if mailbox == "" {
		mailbox = imap.InboxName
	}
	if _, err := c.Select(mailbox, true); err != nil {
		return Detail{}, fmt.Errorf("select %s: %w", mailbox, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, imap.FetchRFC822Size, section.FetchItem()}

	ch := make(chan *imap.Message, 4)
	done := make(chan error, 1)
	go func() { done <- c.UidFetch(seqset, items, ch) }()

	var msg *imap.Message
	for m := range ch {
		msg = m
	}
	if err := <-done; err != nil {
		return Detail{}, err
	}
	if msg == nil {
		return Detail{}, errors.New("邮件不存在或已被删除")
	}

	// Read the body first: GetBody returns a one-shot reader, and toSummary
	// would otherwise consume it before we can parse the message.
	var raw []byte
	if lit := msg.GetBody(section); lit != nil {
		raw, err = io.ReadAll(lit)
		if err != nil {
			return Detail{}, fmt.Errorf("读取邮件内容失败: %w", err)
		}
	}

	detail := Detail{Summary: toSummary(msg, mailboxTarget{mailbox, mailboxLabel(mailbox)}, section)}
	if len(raw) == 0 {
		return detail, nil
	}
	parseBody(&detail, raw)
	if detail.Snippet == "" {
		detail.Snippet = makeSnippet(detail.Text)
	}
	return detail, nil
}

func parseBody(detail *Detail, raw []byte) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil && mr == nil {
		detail.Text = string(raw)
		return
	}
	if mr == nil {
		return
	}
	defer mr.Close()

	var textBuf, htmlBuf strings.Builder
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		body, _ := io.ReadAll(io.LimitReader(p.Body, 4<<20))

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, params, _ := h.ContentType()
			switch {
			case strings.HasPrefix(ct, "text/html"):
				if htmlBuf.Len() == 0 {
					htmlBuf.Write(body)
				}
			case strings.HasPrefix(ct, "text/plain"):
				if textBuf.Len() == 0 {
					textBuf.Write(body)
				}
			case strings.HasPrefix(ct, "image/"):
				name := params["name"]
				if name == "" {
					name = "image"
				}
				detail.Attachments = append(detail.Attachments, Attachment{
					Filename: name,
					MimeType: ct,
					Size:     len(body),
				})
			}
		case *mail.AttachmentHeader:
			name, _ := h.Filename()
			ct, _, _ := h.ContentType()
			if name == "" {
				name = "attachment"
			}
			detail.Attachments = append(detail.Attachments, Attachment{
				Filename: name,
				MimeType: ct,
				Size:     len(body),
			})
		}
	}

	detail.Text = textBuf.String()
	detail.HTML = sanitizer.Sanitize(htmlBuf.String())
	detail.RawHTML = false
	if detail.Text == "" && detail.HTML != "" {
		detail.Text = htmlToText(detail.HTML)
	}
}

// Batch fetches many accounts concurrently, preserving input order.
func (f *Fetcher) Batch(ctx context.Context, creds []Credentials, limit int, folder string, concurrency int) []FetchResult {
	if concurrency < 1 {
		concurrency = 4
	}
	if concurrency > len(creds) {
		concurrency = len(creds)
	}
	results := make([]FetchResult, len(creds))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, cred := range creds {
		wg.Add(1)
		go func(i int, cred Credentials) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := f.Latest(ctx, cred, limit, folder)
			if err != nil && res.Error == "" {
				res.Error = err.Error()
			}
			if res.Email == "" {
				res.Email = cred.Email
			}
			res.OK = res.Error == ""
			results[i] = res
		}(i, cred)
	}
	wg.Wait()
	return results
}
