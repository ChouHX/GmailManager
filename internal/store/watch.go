package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrWatchNotFound is returned when a watch task does not exist.
var ErrWatchNotFound = errors.New("watch not found")

// AllAccountsID as AccountID means the task polls the whole pool.
const AllAccountsID int64 = 0

// Watch is a scheduled IMAP poll. A task matches when the subject and/or
// sender patterns hit; every hit pushes a Discord webhook. AccountID 0 means
// "every account in the pool".
type Watch struct {
	ID         int64  `json:"id"`
	AccountID  int64  `json:"accountId"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Subject    string `json:"subject"`
	From       string `json:"from"`
	Folder     string `json:"folder"`
	Interval   int    `json:"intervalSeconds"`
	WebhookURL string `json:"webhookUrl"`

	LastRunAt       int64  `json:"lastRunAt"`
	LastStatus      string `json:"lastStatus"`
	LastError       string `json:"lastError"`
	LastHitAt       int64  `json:"lastHitAt"`
	LastHitCount    int    `json:"lastHitCount"`
	TotalHits       int64  `json:"totalHits"`
	LastNotified    int64  `json:"lastNotifiedAt"`
	LastRunDuration int64  `json:"lastDurationMs"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

const watchColumns = `w.id, w.account_id, CASE WHEN w.account_id = 0 THEN '' ELSE COALESCE(a.email, '') END, w.name, w.enabled, w.subject, w.from_addr,
	w.folder, w.interval_seconds, w.webhook_url, w.last_run_at, w.last_status, w.last_error,
	w.last_hit_at, w.last_hit_count, w.total_hits, w.last_notified_at, w.last_duration_ms,
	w.created_at, w.updated_at`

func scanWatch(row scanner) (Watch, error) {
	var w Watch
	err := row.Scan(&w.ID, &w.AccountID, &w.Email, &w.Name, &w.Enabled, &w.Subject, &w.From,
		&w.Folder, &w.Interval, &w.WebhookURL, &w.LastRunAt, &w.LastStatus, &w.LastError,
		&w.LastHitAt, &w.LastHitCount, &w.TotalHits, &w.LastNotified, &w.LastRunDuration,
		&w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// CreateWatch inserts a task and returns its id.
func (s *Store) CreateWatch(ctx context.Context, w Watch) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO watches (account_id, name, enabled, subject, from_addr, folder, interval_seconds, webhook_url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.AccountID, w.Name, w.Enabled, w.Subject, w.From, w.Folder, w.Interval, w.WebhookURL, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateWatch writes the editable fields of a task.
func (s *Store) UpdateWatch(ctx context.Context, w Watch) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE watches SET account_id = ?, name = ?, enabled = ?, subject = ?, from_addr = ?,
			folder = ?, interval_seconds = ?, webhook_url = ?, updated_at = ?
		 WHERE id = ?`,
		w.AccountID, w.Name, w.Enabled, w.Subject, w.From, w.Folder, w.Interval, w.WebhookURL,
		time.Now().Unix(), w.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWatchNotFound
	}
	return nil
}

// DeleteWatch removes a task.
func (s *Store) DeleteWatch(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM watches WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrWatchNotFound
	}
	return nil
}

// GetWatch returns one task by id.
func (s *Store) GetWatch(ctx context.Context, id int64) (Watch, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+watchColumns+` FROM watches w
		LEFT JOIN accounts a ON a.id = w.account_id WHERE w.id = ?`, id)
	w, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return w, ErrWatchNotFound
	}
	return w, err
}

// ListWatches returns every task, newest first.
func (s *Store) ListWatches(ctx context.Context) ([]Watch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+watchColumns+` FROM watches w
		LEFT JOIN accounts a ON a.id = w.account_id ORDER BY w.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Watch{}
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// EnabledWatches returns the tasks the scheduler should consider.
func (s *Store) EnabledWatches(ctx context.Context) ([]Watch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+watchColumns+` FROM watches w
		LEFT JOIN accounts a ON a.id = w.account_id WHERE w.enabled = 1 ORDER BY w.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Watch{}
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// WatchRun records the outcome of one poll.
type WatchRun struct {
	ID         int64
	Status     string
	Error      string
	HitCount   int
	DurationMS int64
	Notified   bool
}

// SetWatchRun stores the result of one poll.
func (s *Store) SetWatchRun(ctx context.Context, r WatchRun) error {
	now := time.Now().Unix()
	hits := 0
	if r.HitCount > 0 {
		hits = r.HitCount
	}
	notified := 0
	if r.Notified {
		notified = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE watches SET last_run_at = ?, last_status = ?, last_error = ?, last_duration_ms = ?,
			last_hit_count = ?, last_hit_at = CASE WHEN ? > 0 THEN ? ELSE last_hit_at END,
			total_hits = total_hits + ?, last_notified_at = CASE WHEN ? = 1 THEN ? ELSE last_notified_at END
		 WHERE id = ?`,
		now, r.Status, truncate(r.Error, 500), r.DurationMS, hits, hits, now, hits, notified, now, r.ID)
	return err
}

// DeleteWatchesForAccount removes the tasks bound to a deleted account.
func (s *Store) DeleteWatchesForAccount(ctx context.Context, accountID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM watches WHERE account_id = ?`, accountID)
	return err
}
