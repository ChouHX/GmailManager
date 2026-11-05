// Package api exposes the HTTP JSON API and serves the embedded SPA.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gmailmanager/internal/config"
	"gmailmanager/internal/imapx"
	"gmailmanager/internal/secret"
	"gmailmanager/internal/store"
	"gmailmanager/internal/totp"
)

// BuildInfo is injected at link time and surfaced by /api/health.
type BuildInfo struct {
	Version string
	Commit  string
}

// Server wires together storage, IMAP and the HTTP handlers.
type Server struct {
	cfg      config.Config
	store    *store.Store
	box      *secret.Box
	fetcher  *imapx.Fetcher
	sessions *sessionManager
	assets   http.Handler
	fetchSem chan struct{}
	build    BuildInfo
}

// New builds the API server. assets serves the built frontend.
func New(cfg config.Config, st *store.Store, box *secret.Box, assets http.Handler, build BuildInfo) *Server {
	if build.Version == "" {
		build.Version = "dev"
	}
	if build.Commit == "" {
		build.Commit = "none"
	}
	s := &Server{
		cfg:      cfg,
		store:    st,
		box:      box,
		fetcher:  imapx.New(imapx.Config{Host: cfg.IMAPHost, Port: cfg.IMAPPort, Timeout: cfg.FetchTimeout, Plaintext: cfg.PlaintextIMAP}),
		sessions: newSessionManager(cfg.Secret, cfg.SessionTTL, cfg.CookieSecure),
		assets:   assets,
		fetchSem: make(chan struct{}, maxInt(2, cfg.FetchConcurrency)),
		build:    build,
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Handler returns the root mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"time":    time.Now().Unix(),
			"version": s.build.Version,
			"commit":  s.build.Commit,
		})
	})
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/me", s.handleMe)

	mux.HandleFunc("GET /api/stats", s.auth(s.handleStats))

	mux.HandleFunc("GET /api/accounts", s.auth(s.handleListAccounts))
	mux.HandleFunc("POST /api/accounts", s.auth(s.handleCreateAccount))
	mux.HandleFunc("PUT /api/accounts/{id}", s.auth(s.handleUpdateAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", s.auth(s.handleDeleteAccount))
	mux.HandleFunc("POST /api/accounts/batch-delete", s.auth(s.handleBatchDelete))
	mux.HandleFunc("POST /api/accounts/import", s.auth(s.handleImport))
	mux.HandleFunc("GET /api/accounts/export", s.auth(s.handleExport))
	mux.HandleFunc("POST /api/accounts/{id}/verify", s.auth(s.handleVerify))
	mux.HandleFunc("GET /api/accounts/{id}/totp", s.auth(s.handleTOTP))

	mux.HandleFunc("POST /api/fetch/batch", s.auth(s.handleBatchFetch))
	mux.HandleFunc("GET /api/fetch/{id}", s.auth(s.handleAccountFetch))
	mux.HandleFunc("GET /api/fetch/{id}/message", s.auth(s.handleMessage))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "接口不存在")
	})

	mux.Handle("/", s.assets)
	return s.logRequests(s.securityHeaders(mux))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

type ctxKey string

const userKey ctxKey = "user"

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.sessions.user(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "请先登录")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

type apiError struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func readJSON(r *http.Request, v any) error {
	defer io.Copy(io.Discard, r.Body)
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	return dec.Decode(v)
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// accountDTO is the API representation of an account. Secrets are masked
// unless reveal is explicitly requested.
type accountDTO struct {
	ID               int64  `json:"id"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	TOTPSecret       string `json:"totpSecret"`
	AppPassword      string `json:"appPassword"`
	Note             string `json:"note"`
	HasPassword      bool   `json:"hasPassword"`
	HasTOTP          bool   `json:"hasTotp"`
	HasAppPassword   bool   `json:"hasAppPassword"`
	LastFetchAt      int64  `json:"lastFetchAt"`
	LastFetchStatus  string `json:"lastFetchStatus"`
	LastFetchError   string `json:"lastFetchError"`
	LastFetchSubject string `json:"lastFetchSubject"`
	LastFetchFrom    string `json:"lastFetchFrom"`
	LastFetchDate    int64  `json:"lastFetchDate"`
	LastFetchSeen    bool   `json:"lastFetchSeen"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
}

const mask = "••••••••"

func (s *Server) toDTO(a store.Account, reveal bool) accountDTO {
	dto := accountDTO{
		ID:               a.ID,
		Email:            a.Email,
		Note:             a.Note,
		HasPassword:      a.Password != "",
		HasTOTP:          a.TOTPSecret != "",
		HasAppPassword:   a.AppPassword != "",
		LastFetchAt:      a.LastFetchAt,
		LastFetchStatus:  a.LastFetchStatus,
		LastFetchError:   a.LastFetchError,
		LastFetchSubject: a.LastFetchSubject,
		LastFetchFrom:    a.LastFetchFrom,
		LastFetchDate:    a.LastFetchDate,
		LastFetchSeen:    a.LastFetchSeen,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
	if reveal {
		dto.Password, _ = s.box.Decrypt(a.Password)
		dto.TOTPSecret, _ = s.box.Decrypt(a.TOTPSecret)
		dto.AppPassword, _ = s.box.Decrypt(a.AppPassword)
	} else {
		if dto.HasPassword {
			dto.Password = mask
		}
		if dto.HasTOTP {
			dto.TOTPSecret = mask
		}
		if dto.HasAppPassword {
			dto.AppPassword = mask
		}
	}
	return dto
}

// creds decrypts the credentials needed for IMAP.
func (s *Server) creds(a store.Account) (imapx.Credentials, error) {
	appPass, err := s.box.Decrypt(a.AppPassword)
	if err != nil {
		return imapx.Credentials{}, err
	}
	if strings.TrimSpace(appPass) == "" {
		return imapx.Credentials{}, errors.New("该账号未配置应用专用密码")
	}
	return imapx.Credentials{Email: a.Email, AppPassword: strings.ReplaceAll(appPass, " ", "")}, nil
}

// ---------- auth handlers ----------

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.sessions.throttle(ip) {
		writeErr(w, http.StatusTooManyRequests, "尝试次数过多，请 5 分钟后再试")
		return
	}
	var req loginReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username != s.cfg.AdminUser || req.Password != s.cfg.AdminPassword {
		s.sessions.fail(ip)
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.sessions.reset(ip)
	s.sessions.issue(w, req.Username)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":             req.Username,
		"defaultPasswordInUse": s.cfg.IsDefaultAdminPassword(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.clear(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := s.sessions.user(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":             user,
		"defaultPasswordInUse": s.cfg.IsDefaultAdminPassword(),
	})
}

// ---------- account handlers ----------

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.Stats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	p := store.ListParams{Page: page, PageSize: pageSize, Query: q.Get("q")}
	items, total, err := s.store.List(r.Context(), p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Normalize()
	out := make([]accountDTO, 0, len(items))
	for _, a := range items {
		out = append(out, s.toDTO(a, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    out,
		"total":    total,
		"page":     p.Page,
		"pageSize": p.PageSize,
	})
}

type accountReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	TOTPSecret  string `json:"totpSecret"`
	AppPassword string `json:"appPassword"`
	Note        string `json:"note"`
}

func validEmail(e string) bool {
	e = strings.TrimSpace(e)
	if len(e) < 5 || strings.ContainsAny(e, " \t\r\n") {
		return false
	}
	at := strings.LastIndex(e, "@")
	if at <= 0 || at == len(e)-1 {
		return false
	}
	return strings.Contains(e[at+1:], ".")
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req accountReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if !validEmail(req.Email) {
		writeErr(w, http.StatusBadRequest, "邮箱地址格式不正确")
		return
	}
	if strings.TrimSpace(req.AppPassword) == "" {
		writeErr(w, http.StatusBadRequest, "请填写应用专用密码")
		return
	}
	acc := store.Account{Email: req.Email, Note: strings.TrimSpace(req.Note), TOTPSecret: totp.Normalize(req.TOTPSecret)}
	var err error
	if acc.Password, err = s.box.Encrypt(strings.TrimSpace(req.Password)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acc.TOTPSecret, err = s.box.Encrypt(acc.TOTPSecret); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acc.AppPassword, err = s.box.Encrypt(strings.ReplaceAll(strings.TrimSpace(req.AppPassword), " ", "")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, err := s.store.Create(r.Context(), acc)
	if errors.Is(err, store.ErrDuplicate) {
		writeErr(w, http.StatusConflict, "该邮箱已存在")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	existing, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req accountReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if !validEmail(req.Email) {
		writeErr(w, http.StatusBadRequest, "邮箱地址格式不正确")
		return
	}
	acc := store.Account{ID: id, Email: req.Email, Note: strings.TrimSpace(req.Note)}
	acc.Password = existing.Password
	acc.TOTPSecret = existing.TOTPSecret
	acc.AppPassword = existing.AppPassword

	if req.Password != "" && req.Password != mask {
		if acc.Password, err = s.box.Encrypt(strings.TrimSpace(req.Password)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.TOTPSecret != mask {
		enc, err := s.box.Encrypt(totp.Normalize(req.TOTPSecret))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		acc.TOTPSecret = enc
	}
	if req.AppPassword != "" && req.AppPassword != mask {
		enc, err := s.box.Encrypt(strings.ReplaceAll(strings.TrimSpace(req.AppPassword), " ", ""))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		acc.AppPassword = enc
	}

	if err := s.store.Update(r.Context(), acc); errors.Is(err, store.ErrDuplicate) {
		writeErr(w, http.StatusConflict, "该邮箱已存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := s.store.Delete(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	n, err := s.store.DeleteMany(r.Context(), req.IDs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	var accounts []store.Account
	if idsParam != "" {
		var ids []int64
		for _, part := range strings.Split(idsParam, ",") {
			if v, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				ids = append(ids, v)
			}
		}
		accounts, _ = s.store.GetMany(r.Context(), ids)
	} else {
		page := 1
		for {
			items, total, err := s.store.List(r.Context(), store.ListParams{Page: page, PageSize: 200})
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			accounts = append(accounts, items...)
			if len(accounts) >= total || len(items) == 0 {
				break
			}
			page++
		}
	}
	var b strings.Builder
	for _, a := range accounts {
		password, _ := s.box.Decrypt(a.Password)
		totpSecret, _ := s.box.Decrypt(a.TOTPSecret)
		appPassword, _ := s.box.Decrypt(a.AppPassword)
		b.WriteString(store.FormatExportLine(a.Email, password, totpSecret, appPassword))
		b.WriteString("\n")
	}
	filename := "gmail-accounts-" + time.Now().Format("20060102-150405") + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, b.String())
}

type importReq struct {
	Content   string `json:"content"`
	Overwrite bool   `json:"overwrite"`
	SkipBad   bool   `json:"skipBad"`
}

type importRowResult struct {
	Email   string `json:"email"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var req importReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	lines := strings.Split(strings.ReplaceAll(req.Content, "\r\n", "\n"), "\n")
	var (
		created, updated, skipped, failed int
		rows                              []importRowResult
	)
	for _, line := range lines {
		fields, ok := store.ParseImportLine(line)
		if !ok {
			if strings.TrimSpace(line) != "" {
				failed++
				rows = append(rows, importRowResult{Email: truncateLine(line), Status: "failed", Message: "格式不正确，应为 邮箱----密码----2FA----应用密码"})
			}
			continue
		}
		email, password, totpSecret, appPassword := fields[0], fields[1], fields[2], fields[3]
		if !validEmail(email) {
			failed++
			rows = append(rows, importRowResult{Email: email, Status: "failed", Message: "邮箱格式不正确"})
			continue
		}
		acc := store.Account{Email: email}
		var err error
		if acc.Password, err = s.box.Encrypt(password); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if acc.TOTPSecret, err = s.box.Encrypt(totp.Normalize(totpSecret)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if acc.AppPassword, err = s.box.Encrypt(strings.ReplaceAll(appPassword, " ", "")); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		inserted, err := s.store.Upsert(r.Context(), acc, req.Overwrite)
		if err != nil {
			failed++
			rows = append(rows, importRowResult{Email: email, Status: "failed", Message: err.Error()})
			continue
		}
		switch {
		case inserted:
			created++
			rows = append(rows, importRowResult{Email: email, Status: "created", Message: "已新增"})
		case req.Overwrite:
			updated++
			rows = append(rows, importRowResult{Email: email, Status: "updated", Message: "已更新"})
		default:
			skipped++
			rows = append(rows, importRowResult{Email: email, Status: "skipped", Message: "已存在，跳过"})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"failed":  failed,
		"rows":    rows,
	})
}

func truncateLine(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return string(r)
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	acc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cred, err := s.creds(acc)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	select {
	case s.fetchSem <- struct{}{}:
		defer func() { <-s.fetchSem }()
	case <-r.Context().Done():
		writeErr(w, http.StatusRequestTimeout, "请求已取消")
		return
	}
	if err := s.fetcher.Verify(r.Context(), cred); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "IMAP 登录成功"})
}

func (s *Server) handleTOTP(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	acc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sec, err := s.box.Decrypt(acc.TOTPSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sec == "" {
		writeErr(w, http.StatusBadRequest, "该账号未配置 2FA 密钥")
		return
	}
	code, remaining, err := totp.Now(sec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "2FA 密钥无效: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "remaining": remaining, "period": totp.Period})
}

// ---------- fetch handlers ----------

func (s *Server) acquire(w http.ResponseWriter, r *http.Request) bool {
	select {
	case s.fetchSem <- struct{}{}:
		return true
	case <-r.Context().Done():
		writeErr(w, http.StatusRequestTimeout, "请求已取消")
		return false
	}
}

func (s *Server) release() { <-s.fetchSem }

func (s *Server) handleAccountFetch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	acc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), s.cfg.FetchLimit)
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = imapx.FolderAuto
	}
	cred, err := s.creds(acc)
	if err != nil {
		s.markFailure(r.Context(), acc.ID, err.Error())
		writeJSON(w, http.StatusOK, imapx.FetchResult{Email: acc.Email, AccountID: acc.ID, OK: false, Error: err.Error(), Messages: []imapx.Summary{}, FetchedAt: time.Now().Unix()})
		return
	}
	if !s.acquire(w, r) {
		return
	}
	defer s.release()

	res, err := s.fetcher.Latest(r.Context(), cred, limit, folder)
	res.AccountID = acc.ID
	if err != nil {
		s.markFailure(r.Context(), acc.ID, err.Error())
	}
	if res.OK {
		m := store.MarkFetch{ID: acc.ID, Status: "ok"}
		if len(res.Messages) > 0 {
			top := res.Messages[0]
			m.Subject = top.Subject
			m.From = top.From
			m.Date = top.Date
			m.Seen = top.Seen
		}
		_ = s.store.SetFetchResult(r.Context(), m)
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) markFailure(ctx context.Context, id int64, msg string) {
	log.Printf("fetch failed for account %d: %s", id, msg)
	_ = s.store.SetFetchResult(ctx, store.MarkFetch{ID: id, Status: "error", Error: msg})
}

type batchFetchReq struct {
	IDs    []int64 `json:"ids"`
	Limit  int     `json:"limit"`
	Folder string  `json:"folder"`
}

func (s *Server) handleBatchFetch(w http.ResponseWriter, r *http.Request) {
	var req batchFetchReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "请选择账号")
		return
	}
	if len(req.IDs) > 50 {
		writeErr(w, http.StatusBadRequest, "单次最多批量取件 50 个账号")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 1
	}
	if req.Folder == "" {
		req.Folder = imapx.FolderAuto
	}
	accounts, err := s.store.GetMany(r.Context(), req.IDs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Build credentials; accounts without app password fail fast.
	results := make([]imapx.FetchResult, len(accounts))
	credByID := make(map[int64]imapx.Credentials, len(accounts))
	for _, a := range accounts {
		if c, err := s.creds(a); err == nil {
			credByID[a.ID] = c
		}
	}

	concurrency := s.cfg.FetchConcurrency
	if concurrency < 1 {
		concurrency = 4
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, a := range accounts {
		wg.Add(1)
		go func(i int, a store.Account) {
			defer wg.Done()
			cred, ok := credByID[a.ID]
			if !ok {
				msg := "该账号未配置应用专用密码"
				results[i] = imapx.FetchResult{AccountID: a.ID, Email: a.Email, OK: false, Error: msg, Messages: []imapx.Summary{}, FetchedAt: time.Now().Unix()}
				s.markFailure(r.Context(), a.ID, msg)
				return
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := s.fetcher.Latest(r.Context(), cred, req.Limit, req.Folder)
			res.AccountID = a.ID
			if err != nil && res.Error == "" {
				res.Error = err.Error()
			}
			res.OK = res.Error == ""
			if res.Messages == nil {
				res.Messages = []imapx.Summary{}
			}
			if res.OK {
				m := store.MarkFetch{ID: a.ID, Status: "ok"}
				if len(res.Messages) > 0 {
					m.Subject = res.Messages[0].Subject
					m.From = res.Messages[0].From
					m.Date = res.Messages[0].Date
					m.Seen = res.Messages[0].Seen
				}
				_ = s.store.SetFetchResult(r.Context(), m)
			} else {
				s.markFailure(r.Context(), a.ID, res.Error)
			}
			results[i] = res
		}(i, a)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	uid, err := strconv.ParseUint(r.URL.Query().Get("uid"), 10, 32)
	if err != nil || uid == 0 {
		writeErr(w, http.StatusBadRequest, "无效的邮件 UID")
		return
	}
	mailbox := r.URL.Query().Get("mailbox")
	acc, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cred, err := s.creds(acc)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.acquire(w, r) {
		return
	}
	defer s.release()

	detail, err := s.fetcher.Message(r.Context(), cred, mailbox, uint32(uid))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	if n > 100 {
		return 100
	}
	return n
}

// LogStartup prints a short banner.
func (s *Server) LogStartup() {
	log.Printf("admin user: %s", s.cfg.AdminUser)
	if s.cfg.IsDefaultAdminPassword() {
		log.Printf("WARNING: using default admin password, set GM_ADMIN_PASSWORD")
	}
	log.Printf("imap: %s:%d, concurrency %d", s.cfg.IMAPHost, s.cfg.IMAPPort, s.cfg.FetchConcurrency)
}
