// Package watcher runs the background polling loop for watch tasks.
package watcher

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gmailmanager/internal/imapx"
	"gmailmanager/internal/matcher"
	"gmailmanager/internal/notify"
	"gmailmanager/internal/secret"
	"gmailmanager/internal/store"
)

// MessageLimit is the number of recent messages inspected per poll.
const MessageLimit = 5

// minInterval guards against hammering Gmail by mistake.
const minInterval = 30 * time.Second

// Watcher polls watch tasks on their own schedules.
type Watcher struct {
	store   *store.Store
	box     *secret.Box
	fetcher *imapx.Fetcher
	notify  *notify.Client
	sem     chan struct{}

	mu      sync.Mutex
	running map[int64]bool
	started time.Time
	lastRun time.Time
	ctx     context.Context
}

// New builds a watcher. concurrency limits parallel IMAP sessions.
func New(st *store.Store, box *secret.Box, fetcher *imapx.Fetcher, concurrency int) *Watcher {
	if concurrency < 1 {
		concurrency = 3
	}
	return &Watcher{
		store:   st,
		box:     box,
		fetcher: fetcher,
		notify:  notify.New(),
		sem:     make(chan struct{}, concurrency),
		running: make(map[int64]bool),
		started: time.Now(),
	}
}

// Status is a small snapshot for the UI.
type Status struct {
	StartedAt time.Time `json:"startedAt"`
	LastTick  time.Time `json:"lastTick"`
	Active    int       `json:"active"`
}

// Status reports the loop state.
func (w *Watcher) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Status{StartedAt: w.started, LastTick: w.lastRun, Active: len(w.running)}
}

// Start launches the polling loop until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	w.ctx = ctx
	w.mu.Unlock()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		w.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				log.Printf("watcher stopped")
				return
			case <-ticker.C:
				w.tick(ctx)
			}
		}
	}()
	log.Printf("watcher started (interval check 5s, %d recent messages per poll)", MessageLimit)
}

func (w *Watcher) tick(ctx context.Context) {
	tasks, err := w.store.EnabledWatches(ctx)
	if err != nil {
		log.Printf("watcher: list tasks: %v", err)
		return
	}
	w.mu.Lock()
	w.lastRun = time.Now()
	w.mu.Unlock()

	now := time.Now().Unix()
	for _, task := range tasks {
		if task.Interval <= 0 || task.AccountID == 0 {
			continue
		}
		interval := int64(task.Interval)
		if interval < int64(minInterval.Seconds()) {
			interval = int64(minInterval.Seconds())
		}
		if task.LastRunAt > 0 && now-task.LastRunAt < interval {
			continue
		}
		if w.isRunning(task.ID) {
			continue
		}
		w.goRun(ctx, task)
	}
}

func (w *Watcher) isRunning(id int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running[id]
}

func (w *Watcher) goRun(ctx context.Context, task store.Watch) {
	w.mu.Lock()
	w.running[task.ID] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.running, task.ID)
			w.mu.Unlock()
		}()
		select {
		case w.sem <- struct{}{}:
			defer func() { <-w.sem }()
		case <-ctx.Done():
			return
		}
		if _, err := w.RunNow(ctx, task); err != nil {
			log.Printf("watcher: task %d: %v", task.ID, err)
		}
	}()
}

// RunResult is the outcome of one poll.
type RunResult struct {
	Task       store.Watch     `json:"task"`
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	Fetched    int             `json:"fetched"`
	Hits       []imapx.Summary `json:"hits"`
	Notified   bool            `json:"notified"`
	NotifyErr  string          `json:"notifyError,omitempty"`
	DurationMS int64           `json:"durationMs"`
}

// RunNow executes one poll immediately, ignoring the schedule.
func (w *Watcher) RunNow(ctx context.Context, task store.Watch) (RunResult, error) {
	start := time.Now()
	res := RunResult{Task: task, Hits: []imapx.Summary{}}

	finish := func(status string, errMsg string) (RunResult, error) {
		res.DurationMS = time.Since(start).Milliseconds()
		res.OK = status == "ok"
		res.Error = errMsg
		_ = w.store.SetWatchRun(ctx, store.WatchRun{
			ID:         task.ID,
			Status:     status,
			Error:      errMsg,
			HitCount:   len(res.Hits),
			DurationMS: res.DurationMS,
			Notified:   res.Notified,
		})
		return res, nil
	}

	if !task.Enabled {
		return finish("disabled", "任务已停用")
	}
	if task.AccountID == 0 {
		return finish("error", "未绑定账号")
	}

	acc, err := w.store.Get(ctx, task.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return finish("error", "账号已被删除")
	}
	if err != nil {
		return finish("error", err.Error())
	}
	appPass, err := w.box.Decrypt(acc.AppPassword)
	if err != nil {
		return finish("error", "应用专用密码解密失败: "+err.Error())
	}
	if strings.TrimSpace(appPass) == "" {
		return finish("error", "该账号未配置应用专用密码")
	}
	rule, err := matcher.CompileRule(task.Subject, task.From)
	if err != nil {
		return finish("error", "匹配规则无效: "+err.Error())
	}

	folder := task.Folder
	if folder == "" {
		folder = imapx.FolderInbox
	}
	cred := imapx.Credentials{Email: acc.Email, AppPassword: strings.ReplaceAll(appPass, " ", "")}
	fetchRes, err := w.fetcher.Latest(ctx, cred, MessageLimit, folder)
	if err != nil && fetchRes.Error == "" {
		fetchRes.Error = err.Error()
	}
	if fetchRes.Error != "" {
		return finish("error", fetchRes.Error)
	}
	res.Fetched = len(fetchRes.Messages)
	for _, m := range fetchRes.Messages {
		if rule.Match(m.Subject, m.From) {
			res.Hits = append(res.Hits, m)
		}
	}

	// Keep the account row in sync so the UI shows fresh state.
	mark := store.MarkFetch{ID: acc.ID, Status: "ok"}
	if len(fetchRes.Messages) > 0 {
		top := fetchRes.Messages[0]
		mark.Subject, mark.From, mark.Date, mark.Seen = top.Subject, top.From, top.Date, top.Seen
	}
	_ = w.store.SetFetchResult(ctx, mark)

	if len(res.Hits) == 0 {
		return finish("ok", "")
	}

	hits := make([]notify.Hit, 0, len(res.Hits))
	for _, m := range res.Hits {
		hits = append(hits, notify.Hit{
			AccountEmail: acc.Email,
			Subject:      m.Subject,
			From:         m.From,
			Date:         m.Date,
			Folder:       m.FolderLabel,
			UID:          m.UID,
			Snippet:      m.Snippet,
		})
	}
	name := task.Name
	if strings.TrimSpace(name) == "" {
		name = acc.Email
	}
	if err := w.notify.Notify(ctx, task.WebhookURL, name, hits); err != nil {
		res.NotifyErr = err.Error()
		log.Printf("watcher: task %d webhook: %v", task.ID, err)
		return finish("error", "命中 "+fmt.Sprint(len(hits))+" 封，但 webhook 推送失败: "+err.Error())
	}
	res.Notified = true
	log.Printf("watcher: task %d (%s) hit %d message(s), webhook sent", task.ID, acc.Email, len(hits))
	return finish("ok", "")
}

// TestWebhook sends a sample notification so users can verify a webhook URL.
func (w *Watcher) TestWebhook(ctx context.Context, url string) error {
	return w.notify.Notify(ctx, url, "测试任务", []notify.Hit{{
		AccountEmail: "demo@gmail.com",
		Subject:      "这是一条测试通知",
		From:         "Gmail Manager <noreply@gmail.com>",
		Date:         time.Now().Unix(),
		Folder:       "收件箱",
		Snippet:      "如果你看到这条消息，说明 webhook 配置成功。",
	}})
}

// Trigger fires a task in the background (manual "run now" from the UI).
func (w *Watcher) Trigger(task store.Watch) bool {
	if w.isRunning(task.ID) {
		return false
	}
	ctx := w.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	w.goRun(ctx, task)
	return true
}
