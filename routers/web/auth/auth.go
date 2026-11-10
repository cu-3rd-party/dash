// Package auth holds the sign-in handlers: Forgejo OAuth2, a password for one member
// (stands without OAuth) and, locally, a password-less dev sign-in.
package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	audit_model "git.cu3rd.ru/cu3rd/dash/models/audit"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	auth_service "git.cu3rd.ru/cu3rd/dash/services/auth"
	"git.cu3rd.ru/cu3rd/dash/services/context"
)

const tplSignIn = "auth/signin"

func renderSignIn(ctx *context.Context, status int, next, errMsg string) {
	ctx.Data["Title"] = "Вход"
	ctx.Data["Next"] = auth_service.SafeNext(next)
	ctx.Data["Error"] = errMsg
	ctx.Data["OAuth"] = setting.OAuthEnabled()
	ctx.Data["DevLogin"] = setting.DevLogin
	ctx.Data["PasswordLogin"] = setting.PasswordLoginEnabled()
	ctx.Data["ForgejoURL"] = setting.ForgejoURL
	ctx.HTML(status, tplSignIn)
}

// SignIn renders the sign-in page: GET /signin
func SignIn(ctx *context.Context) {
	next := ctx.Req.URL.Query().Get("next")
	if ctx.Doer() != nil {
		ctx.Redirect(auth_service.SafeNext(next))
		return
	}
	errMsg := ctx.Req.URL.Query().Get("error")
	// with Forgejo as the only way in there is nothing to choose: go straight there.
	// An error from the callback is still shown, otherwise it would loop back to Forgejo.
	if errMsg == "" && setting.OAuthEnabled() && !setting.DevLogin && !setting.PasswordLoginEnabled() {
		ctx.Redirect("/auth/login?next=" + url.QueryEscape(auth_service.SafeNext(next)))
		return
	}
	renderSignIn(ctx, http.StatusOK, next, errMsg)
}

// Login sends the browser to Forgejo: GET /auth/login
func Login(ctx *context.Context) {
	if !setting.OAuthEnabled() {
		renderSignIn(ctx, http.StatusNotFound, "", "Вход через Forgejo не настроен (DASH_OAUTH_CLIENT_ID / DASH_OAUTH_CLIENT_SECRET).")
		return
	}
	http.Redirect(ctx.Resp, ctx.Req, auth_service.AuthorizeURL(ctx.Resp, ctx.Req.URL.Query().Get("next")), http.StatusSeeOther)
}

// Callback finishes the OAuth2 flow: GET /auth/callback
func Callback(ctx *context.Context) {
	login, avatar, next, err := auth_service.Callback(ctx.Resp, ctx.Req)
	if err != nil {
		var signInErr auth_service.ErrSignIn
		msg := "Не удалось получить данные пользователя из Forgejo, попробуйте ещё раз."
		if errors.As(err, &signInErr) {
			msg = signInErr.Reason
		} else {
			slog.Warn("oauth callback", "err", err)
		}
		ctx.Redirect("/signin?error=" + url.QueryEscape(msg))
		return
	}
	if !finishSignIn(ctx, login, avatar) {
		return
	}
	ctx.Redirect(auth_service.SafeNext(next))
}

func finishSignIn(ctx *context.Context, login, avatar string) bool {
	m, err := member_model.GetByLogin(ctx.Req.Context(), login)
	if err != nil || member_model.RoleLevel(m.Role) == 0 {
		slog.Info("sign-in denied", "login", login)
		renderSignIn(ctx, http.StatusForbidden, "", "У пользователя "+login+" нет доступа к админке финансов. Попросите владельца добавить вас.")
		return false
	}
	auth_service.SignIn(ctx.Resp, m.Login, avatar)
	audit_model.Record(ctx.Req.Context(), m.Login, "auth.signin", "")
	return true
}

// DevSignIn signs in without a password when DASH_DEV_LOGIN is on: POST /signin/dev
func DevSignIn(ctx *context.Context) {
	if !setting.DevLogin {
		ctx.PlainText(http.StatusNotFound, "dev-вход выключен")
		return
	}
	login, err := member_model.NormalizeLogin(ctx.FormString("login"))
	if err != nil {
		renderSignIn(ctx, http.StatusUnprocessableEntity, ctx.FormString("next"), err.Error())
		return
	}
	if !finishSignIn(ctx, login, "") {
		return
	}
	ctx.Redirect(auth_service.SafeNext(ctx.FormString("next")))
}

// PasswordSignIn checks the configured login and password: POST /signin/password
func PasswordSignIn(ctx *context.Context) {
	next := ctx.FormString("next")
	if !setting.PasswordLoginEnabled() {
		renderSignIn(ctx, http.StatusNotFound, next, "Вход по паролю выключен.")
		return
	}
	login := ctx.FormString("login")
	ok, err := auth_service.CheckPassword(ctx.Req, login, ctx.Req.FormValue("password"))
	switch {
	case err != nil:
		renderSignIn(ctx, http.StatusTooManyRequests, next, err.Error())
		return
	case !ok:
		slog.Info("password sign-in failed", "login", login)
		renderSignIn(ctx, http.StatusForbidden, next, "Неверный логин или пароль.")
		return
	}
	if !finishSignIn(ctx, setting.PasswordLogin, "") {
		return
	}
	ctx.Redirect(auth_service.SafeNext(next))
}

// SignOut ends the session: POST /signout
func SignOut(ctx *context.Context) {
	auth_service.SignOut(ctx.Resp)
	ctx.Redirect("/")
}
