package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

// ErrLinkNotFound is returned when a share link token is unknown.
var ErrLinkNotFound = errors.New("link not found")

// FetchLink is a public, account-scoped token that lets anyone with the URL
// read that account's latest messages without logging in.
type FetchLink struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"accountId"`
	Token     string `json:"token"`
	Label     string `json:"label"`
	Revoked   bool   `json:"revoked"`

	CreatedAt  int64 `json:"createdAt"`
	LastUsedAt int64 `json:"lastUsedAt"`
	UseCount   int64 `json:"useCount"`
}

const linkColumns = `id, account_id, token, label, revoked, created_at, last_used_at, use_count`

func scanLink(row scanner) (FetchLink, error) {
	var l FetchLink
	err := row.Scan(&l.ID, &l.AccountID, &l.Token, &l.Label, &l.Revoked, &l.CreatedAt, &l.LastUsedAt, &l.UseCount)
	return l, err
}

// newToken returns a URL-safe random token (~128 bits).
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// EnsureLink returns the active link for an account, creating one if needed.
func (s *Store) EnsureLink(ctx context.Context, accountID int64) (FetchLink, error) {
	link, err := s.activeLink(ctx, accountID)
	if err == nil {
		return link, nil
	}
	if !errors.Is(err, ErrLinkNotFound) {
		return FetchLink{}, err
	}

	token, err := newToken()
	if err != nil {
		return FetchLink{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO fetch_links (account_id, token, label, revoked, created_at, last_used_at, use_count)
		 VALUES (?, ?, '', 0, ?, 0, 0)`, accountID, token, now)
	if err != nil {
		return FetchLink{}, err
	}
	id, _ := res.LastInsertId()
	return FetchLink{ID: id, AccountID: accountID, Token: token, CreatedAt: now}, nil
}

func (s *Store) activeLink(ctx context.Context, accountID int64) (FetchLink, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+linkColumns+` FROM fetch_links WHERE account_id = ? AND revoked = 0
		 ORDER BY id DESC LIMIT 1`, accountID)
	l, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrLinkNotFound
	}
	return l, err
}

// LinkByToken resolves a public token to its account.
func (s *Store) LinkByToken(ctx context.Context, token string) (FetchLink, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+linkColumns+` FROM fetch_links WHERE token = ? AND revoked = 0 LIMIT 1`,
		strings.TrimSpace(token))
	l, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrLinkNotFound
	}
	return l, err
}

// TouchLink records that a public link was used.
func (s *Store) TouchLink(ctx context.Context, id int64) {
	_, _ = s.db.ExecContext(ctx,
		`UPDATE fetch_links SET last_used_at = ?, use_count = use_count + 1 WHERE id = ?`,
		time.Now().Unix(), id)
}

// LinksForAccounts returns the active link of each account (missing = none).
func (s *Store) LinksForAccounts(ctx context.Context, ids []int64) (map[int64]FetchLink, error) {
	out := make(map[int64]FetchLink, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		holders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+linkColumns+` FROM fetch_links
		 WHERE revoked = 0 AND account_id IN (`+strings.Join(holders, ",")+`)
		 ORDER BY id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out[l.AccountID] = l
	}
	return out, rows.Err()
}

// RevokeLink disables the current link of an account.
func (s *Store) RevokeLink(ctx context.Context, accountID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE fetch_links SET revoked = 1 WHERE account_id = ? AND revoked = 0`, accountID)
	return err
}

// RevokeAllLinks disables every link, e.g. after rotating the server secret.
func (s *Store) RevokeAllLinks(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE fetch_links SET revoked = 1 WHERE revoked = 0`)
	return err
}

// DeleteLinksForAccount cleans up link rows when an account is removed.
func (s *Store) DeleteLinksForAccount(ctx context.Context, accountID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM fetch_links WHERE account_id = ?`, accountID)
	return err
}
