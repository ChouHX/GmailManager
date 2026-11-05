// Package store persists accounts in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when an email already exists.
var ErrDuplicate = errors.New("duplicate email")

// Account is a stored mailbox credential set.
type Account struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	TOTPSecret  string `json:"totpSecret"`
	AppPassword string `json:"appPassword"`
	Note        string `json:"note"`

	LastFetchAt      int64  `json:"lastFetchAt"`
	LastFetchStatus  string `json:"lastFetchStatus"`
	LastFetchError   string `json:"lastFetchError"`
	LastFetchSubject string `json:"lastFetchSubject"`
	LastFetchFrom    string `json:"lastFetchFrom"`
	LastFetchDate    int64  `json:"lastFetchDate"`
	LastFetchSeen    bool   `json:"lastFetchSeen"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open initialises the database file and schema.
func Open(path string) (*Store, error) {
	dsn := path
	if !strings.HasPrefix(path, "file:") {
		dsn = "file:" + url.PathEscape(path)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_enc TEXT NOT NULL DEFAULT '',
	totp_enc TEXT NOT NULL DEFAULT '',
	app_password_enc TEXT NOT NULL DEFAULT '',
	note TEXT NOT NULL DEFAULT '',
	last_fetch_at INTEGER NOT NULL DEFAULT 0,
	last_fetch_status TEXT NOT NULL DEFAULT '',
	last_fetch_error TEXT NOT NULL DEFAULT '',
	last_fetch_subject TEXT NOT NULL DEFAULT '',
	last_fetch_from TEXT NOT NULL DEFAULT '',
	last_fetch_date INTEGER NOT NULL DEFAULT 0,
	last_fetch_seen INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_email ON accounts(email);

CREATE TABLE IF NOT EXISTS fetch_links (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	account_id INTEGER NOT NULL,
	token TEXT NOT NULL UNIQUE,
	label TEXT NOT NULL DEFAULT '',
	revoked INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0,
	use_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_links_account ON fetch_links(account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_links_token ON fetch_links(token);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Additive migration for databases created before last_fetch_seen existed.
	if err := s.addColumnIfMissing("accounts", "last_fetch_seen", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

func (s *Store) addColumnIfMissing(table, column, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

const accountColumns = `id, email, password_enc, totp_enc, app_password_enc, note,
	last_fetch_at, last_fetch_status, last_fetch_error, last_fetch_subject, last_fetch_from, last_fetch_date, last_fetch_seen,
	created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(row scanner) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Email, &a.Password, &a.TOTPSecret, &a.AppPassword, &a.Note,
		&a.LastFetchAt, &a.LastFetchStatus, &a.LastFetchError, &a.LastFetchSubject, &a.LastFetchFrom, &a.LastFetchDate, &a.LastFetchSeen,
		&a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// ListParams controls pagination and filtering.
type ListParams struct {
	Page     int
	PageSize int
	Query    string
}

// Normalize clamps pagination values.
func (p *ListParams) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 10
	}
	if p.PageSize > 200 {
		p.PageSize = 200
	}
	p.Query = strings.TrimSpace(p.Query)
}

// List returns a page of accounts plus the total count.
func (s *Store) List(ctx context.Context, p ListParams) ([]Account, int, error) {
	p.Normalize()
	where := ""
	var args []any
	if p.Query != "" {
		where = " WHERE email LIKE ? OR note LIKE ?"
		like := "%" + p.Query + "%"
		args = append(args, like, like)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := "SELECT " + accountColumns + " FROM accounts" + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, p.PageSize, (p.Page-1)*p.PageSize)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Account, 0, p.PageSize)
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// Get returns one account by id.
func (s *Store) Get(ctx context.Context, id int64) (Account, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+accountColumns+" FROM accounts WHERE id = ?", id)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// GetMany returns accounts matching the given ids.
func (s *Store) GetMany(ctx context.Context, ids []int64) ([]Account, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		holders[i] = "?"
		args[i] = id
	}
	q := "SELECT " + accountColumns + " FROM accounts WHERE id IN (" + strings.Join(holders, ",") + ") ORDER BY id DESC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Create inserts a new account. Encrypted columns must already be encrypted.
func (s *Store) Create(ctx context.Context, a Account) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts (email, password_enc, totp_enc, app_password_enc, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		normalizeEmail(a.Email), a.Password, a.TOTPSecret, a.AppPassword, a.Note, now, now)
	if err != nil {
		if isUniqueErr(err) {
			return 0, ErrDuplicate
		}
		return 0, err
	}
	return res.LastInsertId()
}

// Update writes the editable fields of an account.
func (s *Store) Update(ctx context.Context, a Account) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET email = ?, password_enc = ?, totp_enc = ?, app_password_enc = ?, note = ?, updated_at = ?
		 WHERE id = ?`,
		normalizeEmail(a.Email), a.Password, a.TOTPSecret, a.AppPassword, a.Note, time.Now().Unix(), a.ID)
	if err != nil {
		if isUniqueErr(err) {
			return ErrDuplicate
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes one account.
func (s *Store) Delete(ctx context.Context, id int64) error {
	if err := s.DeleteLinksForAccount(ctx, id); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMany removes accounts by id and returns the number removed.
func (s *Store) DeleteMany(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		holders[i] = "?"
		args[i] = id
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM fetch_links WHERE account_id IN ("+strings.Join(holders, ",")+")", args...); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE id IN ("+strings.Join(holders, ",")+")", args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Upsert inserts or updates by email. It reports whether a row was inserted.
func (s *Store) Upsert(ctx context.Context, a Account, overwrite bool) (bool, error) {
	now := time.Now().Unix()
	existing, err := s.byEmail(ctx, a.Email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if errors.Is(err, ErrNotFound) {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO accounts (email, password_enc, totp_enc, app_password_enc, note, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			normalizeEmail(a.Email), a.Password, a.TOTPSecret, a.AppPassword, a.Note, now, now); err != nil {
			return false, err
		}
		return true, nil
	}
	if !overwrite {
		return false, nil
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE accounts SET password_enc = ?, totp_enc = ?, app_password_enc = ?, note = ?, updated_at = ?
		 WHERE id = ?`,
		a.Password, a.TOTPSecret, a.AppPassword, coalesce(a.Note, existing.Note), now, existing.ID)
	return false, err
}

func (s *Store) byEmail(ctx context.Context, email string) (Account, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+accountColumns+" FROM accounts WHERE email = ?", normalizeEmail(email))
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// MarkFetch records the outcome of a mailbox fetch.
type MarkFetch struct {
	ID      int64
	Status  string
	Error   string
	Subject string
	From    string
	Date    int64
	Seen    bool
}

// SetFetchResult stores the last fetch state for an account.
func (s *Store) SetFetchResult(ctx context.Context, m MarkFetch) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET last_fetch_at = ?, last_fetch_status = ?, last_fetch_error = ?,
			last_fetch_subject = ?, last_fetch_from = ?, last_fetch_date = ? WHERE id = ?`,
		time.Now().Unix(), m.Status, truncate(m.Error, 500), truncate(m.Subject, 300), truncate(m.From, 200), m.Date, m.ID)
	return err
}

// Stats is the dashboard summary.
type Stats struct {
	Total        int `json:"total"`
	WithAppPass  int `json:"withAppPassword"`
	FetchOK      int `json:"fetchOk"`
	FetchFailed  int `json:"fetchFailed"`
	FetchedToday int `json:"fetchedToday"`
}

// Stats aggregates counters for the UI header.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN app_password_enc != '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN last_fetch_status = 'ok' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN last_fetch_status = 'error' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN last_fetch_at >= ? THEN 1 ELSE 0 END), 0)
		FROM accounts`, startOfToday()).Scan(&st.Total, &st.WithAppPass, &st.FetchOK, &st.FetchFailed, &st.FetchedToday)
	if err != nil {
		return st, err
	}
	return st, nil
}

func startOfToday() int64 {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func coalesce(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

// ParseImportLine splits an import line into its credential fields.
func ParseImportLine(line string) (fields []string, ok bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, false
	}
	sep := "----"
	if !strings.Contains(line, sep) {
		switch {
		case strings.Contains(line, "\t"):
			sep = "\t"
		case strings.Count(line, "|") >= 3:
			sep = "|"
		case strings.Count(line, ",") >= 3:
			sep = ","
		default:
			return nil, false
		}
	}
	parts := strings.Split(line, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	for len(parts) < 4 {
		parts = append(parts, "")
	}
	if strings.TrimSpace(parts[0]) == "" {
		return nil, false
	}
	return parts[:4], true
}

// FormatExportLine renders a credential row in the import format.
func FormatExportLine(email, password, totp, appPassword string) string {
	return fmt.Sprintf("%s----%s----%s----%s", email, password, totp, appPassword)
}
