package api

import (
	"errors"
	"net/http"
	"strings"

	"gmailmanager/internal/imapx"
	"gmailmanager/internal/notify"
	"gmailmanager/internal/store"
)

// MinWatchInterval / MaxWatchInterval bound the polling period (seconds).
const (
	MinWatchInterval = 30
	MaxWatchInterval = 24 * 60 * 60
)

type watchReq struct {
	AccountID       int64  `json:"accountId"`
	Name            string `json:"name"`
	Enabled         *bool  `json:"enabled"`
	Subject         string `json:"subject"`
	From            string `json:"from"`
	Folder          string `json:"folder"`
	IntervalSeconds int    `json:"intervalSeconds"`
	WebhookURL      string `json:"webhookUrl"`
}

func (r watchReq) apply(w store.Watch) store.Watch {
	w.AccountID = r.AccountID
	w.Name = strings.TrimSpace(r.Name)
	w.Subject = strings.TrimSpace(r.Subject)
	w.From = strings.TrimSpace(r.From)
	w.Folder = strings.TrimSpace(r.Folder)
	if w.Folder == "" {
		w.Folder = imapx.FolderInbox
	}
	w.Interval = r.IntervalSeconds
	w.WebhookURL = strings.TrimSpace(r.WebhookURL)
	if r.Enabled != nil {
		w.Enabled = *r.Enabled
	}
	return w
}

func validFolder(f string) bool {
	switch f {
	case imapx.FolderInbox, imapx.FolderSpam, imapx.FolderTrash, imapx.FolderAuto:
		return true
	}
	return false
}

// validate returns a user-facing error message, or "" when the task is valid.
func (r watchReq) validate() string {
	if r.AccountID == 0 {
		return "请选择要轮询的账号"
	}
	if strings.TrimSpace(r.Subject) == "" && strings.TrimSpace(r.From) == "" {
		return "请至少填写邮件标题或发件人条件"
	}
	if r.IntervalSeconds < MinWatchInterval {
		return "轮询间隔不能小于 30 秒"
	}
	if r.IntervalSeconds > MaxWatchInterval {
		return "轮询间隔不能超过 24 小时"
	}
	if r.Folder != "" && !validFolder(r.Folder) {
		return "取件范围无效"
	}
	if !notify.ValidWebhookURL(r.WebhookURL) {
		return "请填写有效的 Discord webhook 链接"
	}
	return ""
}

// GET /api/watches
func (s *Server) handleListWatches(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListWatches(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]any{"items": tasks}
	if s.watcher != nil {
		out["status"] = s.watcher.Status()
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/watches
func (s *Server) handleCreateWatch(w http.ResponseWriter, r *http.Request) {
	var req watchReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if msg := req.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := s.store.Get(r.Context(), req.AccountID); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusBadRequest, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	task := req.apply(store.Watch{Enabled: true})
	id, err := s.store.CreateWatch(r.Context(), task)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// PUT /api/watches/{id}
func (s *Server) handleUpdateWatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	existing, err := s.store.GetWatch(r.Context(), id)
	if errors.Is(err, store.ErrWatchNotFound) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req watchReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if msg := req.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := s.store.Get(r.Context(), req.AccountID); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusBadRequest, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	task := req.apply(existing)
	if err := s.store.UpdateWatch(r.Context(), task); errors.Is(err, store.ErrWatchNotFound) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/watches/{id}
func (s *Server) handleDeleteWatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := s.store.DeleteWatch(r.Context(), id); errors.Is(err, store.ErrWatchNotFound) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/watches/{id}/toggle — flip the enabled flag.
func (s *Server) handleToggleWatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	task, err := s.store.GetWatch(r.Context(), id)
	if errors.Is(err, store.ErrWatchNotFound) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	task.Enabled = !task.Enabled
	if err := s.store.UpdateWatch(r.Context(), task); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": task.Enabled})
}

// POST /api/watches/{id}/run — poll once, right now.
func (s *Server) handleRunWatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	task, err := s.store.GetWatch(r.Context(), id)
	if errors.Is(err, store.ErrWatchNotFound) {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.watcher == nil {
		writeErr(w, http.StatusServiceUnavailable, "轮询器未启动")
		return
	}
	res, err := s.watcher.RunNow(r.Context(), task)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if updated, err := s.store.GetWatch(r.Context(), id); err == nil {
		res.Task = updated
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /api/watches/webhook-test — send a test message to a webhook.
func (s *Server) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if !notify.ValidWebhookURL(req.URL) {
		writeErr(w, http.StatusBadRequest, "请填写有效的 Discord webhook 链接")
		return
	}
	if s.watcher == nil {
		writeErr(w, http.StatusServiceUnavailable, "轮询器未启动")
		return
	}
	if err := s.watcher.TestWebhook(r.Context(), req.URL); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "测试消息已发送"})
}
