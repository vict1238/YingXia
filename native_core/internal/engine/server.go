// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	service                           *Service
	HTTP                              *http.Server
	listener                          net.Listener
	origin, cookieName, session, csrf string
	lastSeen                          atomic.Int64
	hadClient                         atomic.Bool
	networkSlots                      chan struct{}
	onQuit                            func()
}

func newServer(service *Service, port int) (*Server, error) {
	ln, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if e != nil {
		return nil, e
	}
	s := &Server{service: service, listener: ln, origin: "http://" + ln.Addr().String(), cookieName: fmt.Sprintf("yx_session_%d", ln.Addr().(*net.TCPAddr).Port), session: token(), csrf: token(), networkSlots: make(chan struct{}, 4)}
	s.lastSeen.Store(time.Now().Unix())
	s.HTTP = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	return s, nil
}
func (s *Server) shutdown() {
	s.service.searches.cancelActive()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = s.HTTP.Shutdown(ctx)
}
func setHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; media-src 'self' https: http: blob:; frame-src https: http:; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
}
func sendJSON(w http.ResponseWriter, status int, d any) {
	b, e := json.Marshal(d)
	if e != nil {
		status = 500
		b = []byte(`{"ok":false,"error":"internal","message":"无法编码响应。"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", str(len(b)))
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
func sendError(w http.ResponseWriter, e error) {
	var a *AppError
	if !errors.As(e, &a) {
		log.Printf("internal error type=%T", e)
		a = &AppError{"内部错误或本地数据无法保存；请检查数据目录权限。", "internal", 500}
	}
	sendJSON(w, a.Status, Obj{"ok": false, "error": a.Code, "message": a.Message})
}
func (s *Server) common(r *http.Request) error {
	if r.Host != strings.TrimPrefix(s.origin, "http://") {
		return problem("Host 校验失败。", "host", 403)
	}
	if o := r.Header.Get("Origin"); o != "" && o != s.origin {
		return problem("跨站请求已拒绝。", "origin", 403)
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return problem("跨站请求已拒绝。", "origin", 403)
	}
	return nil
}
func (s *Server) authenticated(r *http.Request, write bool) error {
	c, e := r.Cookie(s.cookieName)
	if e != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.session)) != 1 {
		return problem("会话失效，请重新打开软件。", "session", 403)
	}
	if write && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-YingXia-CSRF")), []byte(s.csrf)) != 1 {
		return problem("安全令牌失效，请刷新软件。", "csrf", 403)
	}
	s.lastSeen.Store(time.Now().Unix())
	s.hadClient.Store(true)
	return nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	defer func() {
		if recover() != nil {
			log.Print("local handler panic recovered")
			sendError(w, problem("内部错误，请重新启动软件。", "internal", 500))
		}
	}()
	if e := s.common(r); e != nil {
		sendError(w, e)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		sendError(w, problem("不支持此请求方法。", "method", 405))
		return
	}
	p := r.URL.Path
	if r.Method == http.MethodPost {
		if e := s.authenticated(r, true); e != nil {
			sendError(w, e)
			return
		}
		ctype, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if ctype != "application/json" {
			sendError(w, problem("只接受 JSON。", "content_type", 415))
			return
		}
		if r.ContentLength <= 0 || r.ContentLength > 65536 {
			sendError(w, problem("请求大小超限。", "too_large", 413))
			return
		}
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		if e != nil {
			sendError(w, problem("请求内容不正确。", "bad_json"))
			return
		}
		d, e := decodeObject(raw)
		if e != nil {
			sendError(w, problem("请求内容应为 JSON 对象。", "bad_json"))
			return
		}
		network := p == "/api/search" || p == "/api/resolve" || p == "/api/diagnose" || p == "/api/favorite" || p == "/api/bind"
		if network {
			select {
			case s.networkSlots <- struct{}{}:
				defer func() { <-s.networkSlots }()
			default:
				sendError(w, problem("请求较多，请等待当前查询完成。", "busy", 429))
				return
			}
		}
		result, e := s.post(r.Context(), p, d)
		if e != nil {
			sendError(w, e)
			return
		}
		sendJSON(w, 200, Obj{"ok": true, "data": result})
		if p == "/api/quit" {
			if s.onQuit != nil {
				go s.onQuit()
			} else {
				go s.shutdown()
			}
		}
		return
	}
	if e := s.authenticated(r, false); e != nil {
		sendError(w, e)
		return
	}
	if p == "/api/media" {
		if e := s.service.requireNetwork(); e != nil {
			sendError(w, e)
			return
		}
		s.service.media.serve(w, r)
		return
	}
	if strings.HasPrefix(p, "/api/") {
		var result any
		var e error
		switch p {
		case "/api/search/status":
			result, e = s.service.searches.status(r.URL.Query().Get("id"))
		case "/api/state":
			result = s.service.State()
		case "/api/sources":
			result = s.service.manager.publicState()
		case "/api/library":
			result = Obj{"favorites": s.service.store.Favorites(), "history": s.service.store.History()}
		case "/api/detail":
			select {
			case s.networkSlots <- struct{}{}:
				defer func() { <-s.networkSlots }()
			default:
				sendError(w, problem("请求较多，请稍后再试。", "busy", 429))
				return
			}
			result, e = s.service.DetailPublic(r.Context(), r.URL.Query().Get("id"))
		case "/api/bindings":
			result = s.service.store.Bindings(r.URL.Query().Get("id"), r.URL.Query().Get("episode"))
		case "/api/report":
			result = s.service.Report()
		default:
			e = problem("接口不存在。", "not_found", 404)
		}
		if e != nil {
			sendError(w, e)
			return
		}
		sendJSON(w, 200, Obj{"ok": true, "data": result})
		return
	}
	sendError(w, problem("接口不存在。", "not_found", 404))
}
func (s *Server) post(ctx context.Context, p string, d Obj) (any, error) {
	v := s.service
	switch p {
	case "/api/search/start":
		return v.searches.start(d)
	case "/api/search/cancel":
		return v.searches.cancel(sget(d, "id"))
	case "/api/sources/update":
		return v.manager.update(ctx)
	case "/api/sources/add":
		return v.manager.addCMS(d)
	case "/api/sources/remove":
		return v.manager.removeCMS(sget(d, "id"))
	case "/api/search":
		return v.Search(ctx, d)
	case "/api/settings":
		return v.Settings(d)
	case "/api/import-platform":
		return v.Import(d)
	case "/api/bind":
		return v.Bind(ctx, d)
	case "/api/match":
		_, _, ep, e := v.Select(ctx, d)
		return ep, e
	case "/api/resolve":
		return v.Resolve(ctx, d)
	case "/api/favorite":
		f, e := v.Detail(ctx, sget(d, "film_id"))
		if e != nil {
			return nil, e
		}
		saved, e := v.store.Toggle(f.public())
		return Obj{"saved": saved}, e
	case "/api/progress":
		pos := d["position"]
		if pos == nil {
			pos = 0
		}
		e := v.store.Progress(sget(d, "history_id"), sget(d, "evidence"), pos)
		return Obj{}, e
	case "/api/clear-history":
		return Obj{}, v.store.ClearHistory()
	case "/api/diagnose":
		return v.Diagnose(ctx)
	case "/api/ping":
		return Obj{}, nil
	case "/api/quit":
		return Obj{"closing": true}, nil
	default:
		return nil, problem("接口不存在。", "not_found", 404)
	}
}
