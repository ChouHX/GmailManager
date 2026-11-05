package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gmailmanager/internal/imapx"
	"gmailmanager/internal/store"
)

// linkBaseURL returns the externally reachable origin, honouring a reverse
// proxy (X-Forwarded-*) and falling back to the request's own host.
func (s *Server) linkBaseURL(r *http.Request) string {
	if s.cfg.PublicBaseURL != "" {
		return strings.TrimRight(s.cfg.PublicBaseURL, "/")
	}
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		proto = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return proto + "://" + host
}

func (s *Server) publicURL(r *http.Request, token string) string {
	return s.linkBaseURL(r) + "/f/" + token
}

// ---------- admin: link management ----------

type linkDTO struct {
	AccountID int64  `json:"accountId"`
	Email     string `json:"email"`
	Token     string `json:"token"`
	URL       string `json:"url"`
	CreatedAt int64  `json:"createdAt"`
	LastUsed  int64  `json:"lastUsedAt"`
	UseCount  int64  `json:"useCount"`
}

// GET /api/accounts/{id}/link — current (or freshly created) link.
func (s *Server) handleAccountLink(w http.ResponseWriter, r *http.Request) {
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

	link, err := s.store.EnsureLink(r.Context(), acc.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, linkDTO{
		AccountID: acc.ID,
		Email:     acc.Email,
		Token:     link.Token,
		URL:       s.publicURL(r, link.Token),
		CreatedAt: link.CreatedAt,
		LastUsed:  link.LastUsedAt,
		UseCount:  link.UseCount,
	})
}

// POST /api/accounts/links — bulk variant.
type bulkLinkReq struct {
	IDs []int64 `json:"ids"`
}

func (s *Server) handleBulkLinks(w http.ResponseWriter, r *http.Request) {
	var req bulkLinkReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "请选择账号")
		return
	}
	if len(req.IDs) > 500 {
		writeErr(w, http.StatusBadRequest, "单次最多处理 500 个账号")
		return
	}

	accounts, err := s.store.GetMany(r.Context(), req.IDs)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]linkDTO, 0, len(accounts))
	for _, acc := range accounts {
		link, err := s.store.EnsureLink(r.Context(), acc.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, linkDTO{
			AccountID: acc.ID,
			Email:     acc.Email,
			Token:     link.Token,
			URL:       s.publicURL(r, link.Token),
			CreatedAt: link.CreatedAt,
			LastUsed:  link.LastUsedAt,
			UseCount:  link.UseCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": out})
}

// POST /api/accounts/{id}/link/revoke — invalidate the current link.
func (s *Server) handleRevokeLink(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := s.store.RevokeLink(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/accounts/links/export?ids=1,2 — plain-text link list for download.
func (s *Server) handleExportLinks(w http.ResponseWriter, r *http.Request) {
	var ids []int64
	if v := r.URL.Query().Get("ids"); v != "" {
		for _, part := range strings.Split(v, ",") {
			if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				ids = append(ids, n)
			}
		}
	}
	if len(ids) == 0 {
		page := 1
		for {
			items, total, err := s.store.List(r.Context(), store.ListParams{Page: page, PageSize: 200})
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			for _, a := range items {
				ids = append(ids, a.ID)
			}
			if len(ids) >= total || len(items) == 0 {
				break
			}
			page++
		}
	}

	accounts, err := s.store.GetMany(r.Context(), ids)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var b strings.Builder
	b.WriteString("# Gmail Manager 取件链接\n")
	b.WriteString("# 生成时间: " + time.Now().Format("2006-01-02 15:04:05") + "\n")
	b.WriteString("# 格式: 邮箱<TAB>取件链接\n")
	for _, acc := range accounts {
		link, err := s.store.EnsureLink(r.Context(), acc.ID)
		if err != nil {
			continue
		}
		b.WriteString(acc.Email + "\t" + s.publicURL(r, link.Token) + "\n")
	}

	filename := "gmail-fetch-links-" + time.Now().Format("20060102-150405") + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// ---------- public: no auth ----------

// resolveLink loads the account behind a public token.
func (s *Server) resolveLink(w http.ResponseWriter, r *http.Request) (store.FetchLink, store.Account, bool) {
	token := r.PathValue("token")
	link, err := s.store.LinkByToken(r.Context(), token)
	if errors.Is(err, store.ErrLinkNotFound) {
		writeErr(w, http.StatusNotFound, "链接无效或已失效")
		return store.FetchLink{}, store.Account{}, false
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.FetchLink{}, store.Account{}, false
	}
	acc, err := s.store.Get(r.Context(), link.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "账号已被删除")
		return store.FetchLink{}, store.Account{}, false
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.FetchLink{}, store.Account{}, false
	}
	s.store.TouchLink(r.Context(), link.ID)
	return link, acc, true
}

// GET /api/public/f/{token} — account summary shown on the public page.
func (s *Server) handlePublicInfo(w http.ResponseWriter, r *http.Request) {
	_, acc, ok := s.resolveLink(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":    acc.Email,
		"note":     acc.Note,
		"label":    "取件",
		"lifetime": "仅可读取该账号最新邮件",
	})
}

// GET /api/public/f/{token}/messages — newest messages, no login required.
func (s *Server) handlePublicMessages(w http.ResponseWriter, r *http.Request) {
	_, acc, ok := s.resolveLink(w, r)
	if !ok {
		return
	}
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = imapx.FolderAuto
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 10)

	cred, err := s.creds(acc)
	if err != nil {
		writeJSON(w, http.StatusOK, imapx.FetchResult{
			Email: acc.Email, AccountID: acc.ID, OK: false, Error: err.Error(),
			Messages: []imapx.Summary{}, FetchedAt: time.Now().Unix(),
		})
		return
	}
	if !s.acquire(w, r) {
		return
	}
	defer s.release()

	res, err := s.fetcher.Latest(r.Context(), cred, limit, folder)
	res.AccountID = acc.ID
	if err != nil && res.Error == "" {
		res.Error = err.Error()
	}
	res.OK = res.Error == ""
	if res.Messages == nil {
		res.Messages = []imapx.Summary{}
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /api/public/f/{token}/message — one message body, no login required.
func (s *Server) handlePublicMessage(w http.ResponseWriter, r *http.Request) {
	_, acc, ok := s.resolveLink(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseUint(r.URL.Query().Get("uid"), 10, 32)
	if err != nil || uid == 0 {
		writeErr(w, http.StatusBadRequest, "无效的邮件 UID")
		return
	}
	mailbox := r.URL.Query().Get("mailbox")

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
