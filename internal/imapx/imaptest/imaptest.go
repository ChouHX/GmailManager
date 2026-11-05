// Package imaptest provides a minimal in-memory IMAP server for tests.
package imaptest

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/backendutil"
	"github.com/emersion/go-imap/server"
	"github.com/emersion/go-message/textproto"
)

var crlfcrlf = []byte{13, 10, 13, 10}

// Message is one stored message.
type Message struct {
	UID   uint32
	Date  time.Time
	Flags []string
	Body  []byte
}

// Mailbox is one mailbox with its messages.
type Mailbox struct {
	Name     string
	Attrs    []string
	Messages []*Message
}

// Server is a running test IMAP server.
type Server struct {
	Addr     string
	Host     string
	Port     int
	Username string
	Password string
	srv      *server.Server
	be       *backendImpl
}

// AddUser registers an extra login that shares the same mailboxes.
func (s *Server) AddUser(username, password string) {
	s.be.users[username] = &user{name: username, password: password, boxes: s.be.users[s.Username].boxes}
}

// Close stops the server.
func (s *Server) Close() error { return s.srv.Close() }

// Start boots a server on a random loopback port.
func Start(username, password string, boxes []*Mailbox) (*Server, error) {
	u := &user{name: username, password: password, boxes: boxes}
	be := &backendImpl{users: map[string]*user{username: u}}
	srv := server.New(be)
	srv.AllowInsecureAuth = true
	srv.ErrorLog = nil
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() { _ = srv.Serve(ln) }()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return nil, err
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return nil, err
	}
	return &Server{Addr: ln.Addr().String(), Host: host, Port: port, Username: username, Password: password, srv: srv, be: be}, nil
}

// GmailBoxes returns INBOX + Spam + Trash with sample messages.
func GmailBoxes() []*Mailbox {
	now := time.Now()
	return []*Mailbox{
		{
			Name:  "INBOX",
			Attrs: []string{imap.HasNoChildrenAttr},
			Messages: []*Message{
				{UID: 11, Date: now.Add(-3 * time.Hour), Body: TextMessage("Inbox older", "a@example.com", now.Add(-3*time.Hour), "older body")},
				{UID: 12, Date: now.Add(-1 * time.Hour), Flags: []string{imap.SeenFlag}, Body: TextMessage("Inbox newest", "b@example.com", now.Add(-1*time.Hour), "newest body")},
			},
		},
		{
			Name:  "[Gmail]/Spam",
			Attrs: []string{imap.JunkAttr, imap.HasNoChildrenAttr},
			Messages: []*Message{
				{UID: 21, Date: now.Add(-30 * time.Minute), Body: TextMessage("Spam message", "c@spam.example", now.Add(-30*time.Minute), "spam body")},
			},
		},
		{
			Name:  "[Gmail]/Trash",
			Attrs: []string{imap.TrashAttr, imap.HasNoChildrenAttr},
			Messages: []*Message{
				{UID: 31, Date: now.Add(-48 * time.Hour), Body: TextMessage("Trash message", "d@example.com", now.Add(-48*time.Hour), "trash body")},
			},
		},
	}
}

// TextMessage builds a raw text/plain message.
func TextMessage(subject, from string, date time.Time, body string) []byte {
	return []byte(fmt.Sprintf("From: %s\r\nTo: me@example.com\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d@example.com>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, subject, date.Format(time.RFC1123Z), date.UnixNano(), body))
}

// HTMLMessage builds a raw text/html message.
func HTMLMessage(subject, from string, date time.Time, html string) []byte {
	return []byte(fmt.Sprintf("From: %s\r\nTo: me@example.com\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d@example.com>\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		from, subject, date.Format(time.RFC1123Z), date.UnixNano(), html))
}

// ---- backend ----

type backendImpl struct{ users map[string]*user }

func (b *backendImpl) Login(_ *imap.ConnInfo, username, password string) (backend.User, error) {
	if u, ok := b.users[username]; ok && u.password == password {
		return u, nil
	}
	return nil, backend.ErrInvalidCredentials
}

type user struct {
	name     string
	password string
	boxes    []*Mailbox
}

func (u *user) Username() string { return u.name }

func (u *user) ListMailboxes(subscribed bool) ([]backend.Mailbox, error) {
	out := make([]backend.Mailbox, 0, len(u.boxes))
	for _, b := range u.boxes {
		out = append(out, &mbox{b})
	}
	return out, nil
}

func (u *user) GetMailbox(name string) (backend.Mailbox, error) {
	for _, b := range u.boxes {
		if b.Name == name {
			return &mbox{b}, nil
		}
	}
	return nil, backend.ErrNoSuchMailbox
}

func (u *user) CreateMailbox(string) error         { return errors.New("read-only backend") }
func (u *user) DeleteMailbox(string) error         { return errors.New("read-only backend") }
func (u *user) RenameMailbox(string, string) error { return errors.New("read-only backend") }
func (u *user) Logout() error                      { return nil }

type mbox struct{ box *Mailbox }

func (m *mbox) Name() string { return m.box.Name }

func (m *mbox) Info() (*imap.MailboxInfo, error) {
	return &imap.MailboxInfo{Name: m.box.Name, Attributes: m.box.Attrs, Delimiter: "/"}, nil
}

func (m *mbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	st := imap.NewMailboxStatus(m.box.Name, items)
	st.Flags = m.box.Attrs
	st.PermanentFlags = []string{imap.TryCreateFlag}
	st.Messages = uint32(len(m.box.Messages))
	st.Unseen = uint32(len(m.box.Messages))
	st.UidNext = uint32(len(m.box.Messages) + 1)
	st.UidValidity = 1
	return st, nil
}

func (m *mbox) SetSubscribed(bool) error { return nil }
func (m *mbox) Check() error             { return nil }

func (m *mbox) ListMessages(uid bool, seqset *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)
	for i, msg := range m.box.Messages {
		seq := uint32(i + 1)
		if uid {
			if !seqset.Contains(msg.UID) {
				continue
			}
		} else if !seqset.Contains(seq) {
			continue
		}
		out := imap.NewMessage(seq, items)
		header, body := split(msg.Body)
		for _, item := range items {
			switch item {
			case imap.FetchUid:
				out.Uid = msg.UID
			case imap.FetchFlags:
				out.Flags = msg.Flags
			case imap.FetchInternalDate:
				out.InternalDate = msg.Date
			case imap.FetchRFC822Size:
				out.Size = uint32(len(msg.Body))
			case imap.FetchEnvelope:
				env, _ := backendutil.FetchEnvelope(header)
				out.Envelope = env
			case imap.FetchBodyStructure:
				bs, _ := backendutil.FetchBodyStructure(header, bytes.NewReader(body), true)
				out.BodyStructure = bs
			default:
				section, err := imap.ParseBodySectionName(item)
				if err != nil {
					continue
				}
				lit, err := backendutil.FetchBodySection(header, bytes.NewReader(body), section)
				if err != nil {
					continue
				}
				out.Body = map[*imap.BodySectionName]imap.Literal{section: lit}
			}
		}
		ch <- out
	}
	return nil
}

func (m *mbox) SearchMessages(uid bool, _ *imap.SearchCriteria) ([]uint32, error) {
	out := make([]uint32, 0, len(m.box.Messages))
	for i, msg := range m.box.Messages {
		if uid {
			out = append(out, msg.UID)
		} else {
			out = append(out, uint32(i+1))
		}
	}
	return out, nil
}

func (m *mbox) CreateMessage(flags []string, date time.Time, body imap.Literal) error {
	raw, _ := io.ReadAll(body)
	m.box.Messages = append(m.box.Messages, &Message{UID: uint32(len(m.box.Messages) + 1), Date: date, Flags: flags, Body: raw})
	return nil
}

func (m *mbox) UpdateMessagesFlags(bool, *imap.SeqSet, imap.FlagsOp, []string) error { return nil }
func (m *mbox) CopyMessages(bool, *imap.SeqSet, string) error                        { return nil }
func (m *mbox) Expunge() error                                                       { return nil }

func split(raw []byte) (textproto.Header, []byte) {
	idx := bytes.Index(raw, crlfcrlf)
	if idx < 0 {
		h, _ := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(raw)))
		return h, nil
	}
	h, _ := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(raw[:idx+4])))
	return h, raw[idx+4:]
}
