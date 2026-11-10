// Package context is the per-request web context: handlers take *Context and fill Data for templates.
package context

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	"git.cu3rd.ru/cu3rd/dash/modules/templates"

	"github.com/go-chi/chi/v5"
)

// Context wraps a request and the template data.
type Context struct {
	Resp http.ResponseWriter
	Req  *http.Request
	Data map[string]any
}

// Handler is a route handler taking the web context.
type Handler func(*Context)

// ServeHTTP lets a Handler be mounted on chi directly.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h(&Context{Resp: w, Req: r, Data: map[string]any{}})
}

// IsHTMX reports whether the request was issued by htmx (HX-Request header).
// Boosted navigations want the full page, so they are not counted.
func (ctx *Context) IsHTMX() bool {
	return ctx.Req.Header.Get("HX-Request") == "true" && ctx.Req.Header.Get("HX-Boosted") != "true"
}

type doerKey struct{}

type doer struct {
	member *member_model.Member
	avatar string
}

// WithDoer stores the signed-in member in the request (set by the auth middleware).
func WithDoer(r *http.Request, m *member_model.Member, avatar string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), doerKey{}, doer{m, avatar}))
}

// Doer is the signed-in member, or nil.
func (ctx *Context) Doer() *member_model.Member {
	d, _ := ctx.Req.Context().Value(doerKey{}).(doer)
	return d.member
}

// DoerLogin is the signed-in login, for the audit log.
func (ctx *Context) DoerLogin() string {
	if m := ctx.Doer(); m != nil {
		return m.Login
	}
	return ""
}

const flashCookie = "dash_flash"

// Flash shows msg on the next rendered page (after a redirect).
func (ctx *Context) Flash(msg string) {
	http.SetCookie(ctx.Resp, &http.Cookie{
		Name: flashCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(msg)), Path: setting.AppSubURL + "/", MaxAge: 60,
		HttpOnly: true, Secure: setting.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (ctx *Context) takeFlash() string {
	c, err := ctx.Req.Cookie(flashCookie)
	if err != nil {
		return ""
	}
	http.SetCookie(ctx.Resp, &http.Cookie{Name: flashCookie, Path: setting.AppSubURL + "/", MaxAge: -1, HttpOnly: true, Secure: setting.CookieSecure, SameSite: http.SameSiteLaxMode})
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	return string(b)
}

// HTML renders a template with ctx.Data.
func (ctx *Context) HTML(status int, name string) {
	// always typed, so templates can call {{.Doer.Can "editor"}} for anonymous visitors too
	d, _ := ctx.Req.Context().Value(doerKey{}).(doer)
	ctx.Data["Doer"], ctx.Data["Avatar"] = d.member, d.avatar
	if _, ok := ctx.Data["Flash"]; !ok {
		ctx.Data["Flash"] = ctx.takeFlash()
	}
	ctx.Resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	// render to a buffer first so a template error never sends a half-written page
	var buf bytes.Buffer
	if err := templates.Renderer().HTML(&buf, name, ctx.Data); err != nil {
		ctx.ServerError("render", err)
		return
	}
	ctx.Resp.WriteHeader(status)
	_, _ = buf.WriteTo(ctx.Resp)
}

// Redirect sends the client to url: for htmx requests via HX-Redirect,
// otherwise with a 303 so that a POST becomes a GET.
func (ctx *Context) Redirect(url string) {
	url = setting.AppURL(url)
	if ctx.IsHTMX() {
		ctx.Resp.Header().Set("HX-Redirect", url)
		ctx.Resp.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(ctx.Resp, ctx.Req, url, http.StatusSeeOther)
}

// PlainText writes a text response; htmx shows non-2xx bodies as error toasts.
func (ctx *Context) PlainText(status int, text string) {
	ctx.Resp.Header().Set("Content-Type", "text/plain; charset=utf-8")
	ctx.Resp.WriteHeader(status)
	_, _ = ctx.Resp.Write([]byte(text))
}

// NotFoundOrServerError maps model errors to 404 / 422 / 500.
func (ctx *Context) NotFoundOrServerError(logMsg string, err error) {
	switch {
	case errors.Is(err, db.ErrNotExist):
		ctx.PlainText(http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrInvalidArgument):
		ctx.PlainText(http.StatusUnprocessableEntity, err.Error())
	default:
		ctx.ServerError(logMsg, err)
	}
}

// ServerError logs err and responds with 500 without leaking details.
func (ctx *Context) ServerError(logMsg string, err error) {
	slog.Error(logMsg, "err", err, "method", ctx.Req.Method, "path", ctx.Req.URL.Path)
	ctx.PlainText(http.StatusInternalServerError, "Internal Server Error")
}

// PathParamInt64 returns a chi URL parameter as int64 (0 if invalid).
func (ctx *Context) PathParamInt64(name string) int64 {
	v, _ := strconv.ParseInt(chi.URLParam(ctx.Req, name), 10, 64)
	return v
}

// PathParam returns a chi URL parameter.
func (ctx *Context) PathParam(name string) string { return chi.URLParam(ctx.Req, name) }

// FormString returns a trimmed form value.
func (ctx *Context) FormString(name string) string { return strings.TrimSpace(ctx.Req.FormValue(name)) }

// FormInt64 returns a form value as int64 (0 if invalid).
func (ctx *Context) FormInt64(name string) int64 {
	v, _ := strconv.ParseInt(ctx.FormString(name), 10, 64)
	return v
}

// FormBool reports whether a checkbox is checked.
func (ctx *Context) FormBool(name string) bool { return ctx.Req.FormValue(name) != "" }

// RedirectWithFlash redirects and shows msg on the next page
// (HX-Redirect is a full page load too, so the cookie is read there).
func (ctx *Context) RedirectWithFlash(url, msg string) {
	ctx.Flash(msg)
	ctx.Redirect(url)
}
