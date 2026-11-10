// Package web registers the HTML (htmx) routes.
package web

import (
	"net/http"
	"net/url"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/assets"
	"git.cu3rd.ru/cu3rd/dash/routers/api"
	"git.cu3rd.ru/cu3rd/dash/routers/web/admin"
	"git.cu3rd.ru/cu3rd/dash/routers/web/auth"
	"git.cu3rd.ru/cu3rd/dash/routers/web/home"
	auth_service "git.cu3rd.ru/cu3rd/dash/services/auth"
	"git.cu3rd.ru/cu3rd/dash/services/context"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// forms are small; the largest body is a JSON dump uploaded on the settings page
const maxFormBytes = 2 << 20

// Routes returns the router for the web UI.
func Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer, securityHeaders)

	// public/ contains assets/ (webpack output), so the URL path maps 1:1
	r.Handle("/assets/*", http.FileServerFS(assets.Public()))
	r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.GetEngine(r.Context()).Ping(); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	// machine endpoints: the webhook authenticates with a bearer token, not a cookie
	r.Get("/api/summary", api.Summary)
	r.Post("/api/donations", api.DonationWebhook)

	r.Group(func(r chi.Router) {
		// rejects cross-site POST/PATCH/DELETE using Sec-Fetch-Site / Origin
		r.Use(http.NewCrossOriginProtection().Handler, limitBody, loadDoer)

		r.Method(http.MethodGet, "/", context.Handler(home.Home))
		r.Method(http.MethodGet, "/signin", context.Handler(auth.SignIn))
		r.Method(http.MethodPost, "/signin/dev", context.Handler(auth.DevSignIn))
		r.Method(http.MethodPost, "/signin/password", context.Handler(auth.PasswordSignIn))
		r.Method(http.MethodGet, "/auth/login", context.Handler(auth.Login))
		r.Method(http.MethodGet, "/auth/callback", context.Handler(auth.Callback))
		r.Method(http.MethodPost, "/signout", context.Handler(auth.SignOut))

		r.Route("/admin", func(r chi.Router) {
			r.Use(reqRole(member_model.RoleViewer))
			r.Method(http.MethodGet, "/", context.Handler(admin.Dashboard))
			r.Method(http.MethodGet, "/expenses", context.Handler(admin.Expenses))
			r.Method(http.MethodGet, "/expenses/{id}", context.Handler(admin.Expense))
			r.Method(http.MethodGet, "/payments", context.Handler(admin.Payments))
			r.Method(http.MethodGet, "/donations", context.Handler(admin.Donations))
			r.Method(http.MethodGet, "/categories", context.Handler(admin.Categories))
			r.Method(http.MethodGet, "/audit", context.Handler(admin.Audit))

			r.Group(func(r chi.Router) {
				r.Use(reqRole(member_model.RoleEditor))
				r.Method(http.MethodGet, "/expenses/new", context.Handler(admin.NewExpense))
				r.Method(http.MethodPost, "/expenses/new", context.Handler(admin.CreateExpense))
				r.Method(http.MethodGet, "/expenses/{id}/edit", context.Handler(admin.EditExpense))
				r.Method(http.MethodPost, "/expenses/{id}/edit", context.Handler(admin.UpdateExpense))
				r.Method(http.MethodPost, "/expenses/{id}/delete", context.Handler(admin.DeleteExpense))
				r.Method(http.MethodPost, "/expenses/{id}/payments", context.Handler(admin.CreatePayment))
				r.Method(http.MethodPost, "/payments/{id}/delete", context.Handler(admin.DeletePayment))
				r.Method(http.MethodPost, "/donations", context.Handler(admin.CreateDonation))
				r.Method(http.MethodGet, "/donations/{id}/edit", context.Handler(admin.EditDonation))
				r.Method(http.MethodPost, "/donations/{id}/edit", context.Handler(admin.UpdateDonation))
				r.Method(http.MethodPost, "/donations/{id}/delete", context.Handler(admin.DeleteDonation))
				r.Method(http.MethodPost, "/categories", context.Handler(admin.CreateCategory))
				r.Method(http.MethodPost, "/categories/{id}", context.Handler(admin.UpdateCategory))
				r.Method(http.MethodPost, "/categories/{id}/delete", context.Handler(admin.DeleteCategory))
			})

			r.Group(func(r chi.Router) {
				r.Use(reqRole(member_model.RoleOwner))
				r.Method(http.MethodGet, "/members", context.Handler(admin.Members))
				r.Method(http.MethodPost, "/members", context.Handler(admin.SaveMember))
				r.Method(http.MethodPost, "/members/{login}/delete", context.Handler(admin.DeleteMember))
				r.Method(http.MethodGet, "/settings", context.Handler(admin.Settings))
				r.Method(http.MethodPost, "/settings", context.Handler(admin.SaveSettings))
				r.Method(http.MethodPost, "/settings/rates", context.Handler(admin.UpdateRates))
				r.Method(http.MethodGet, "/settings/export", context.Handler(admin.ExportData))
				r.Method(http.MethodPost, "/settings/import", context.Handler(admin.ImportData))
			})
		})
	})
	r.NotFound(context.Handler(home.NotFound).ServeHTTP)
	return r
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
		next.ServeHTTP(w, r)
	})
}

// loadDoer puts the signed-in member, if any, into the request.
func loadDoer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m, avatar := auth_service.Doer(r); m != nil {
			r = context.WithDoer(r, m, avatar)
		}
		next.ServeHTTP(w, r)
	})
}

// reqRole lets through members with at least role; others are sent to sign in or get 403.
func reqRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return context.Handler(func(ctx *context.Context) {
			m := ctx.Doer()
			if m == nil {
				ctx.Redirect("/signin?next=" + url.QueryEscape(ctx.Req.URL.RequestURI()))
				return
			}
			if !m.Can(role) {
				ctx.Data["Title"] = "Нет прав"
				ctx.Data["Message"] = "Нужна роль «" + member_model.RoleLabel(role) + "», у вас — «" + member_model.RoleLabel(m.Role) + "»."
				if ctx.IsHTMX() {
					ctx.PlainText(http.StatusForbidden, ctx.Data["Message"].(string))
					return
				}
				ctx.HTML(http.StatusForbidden, "status")
				return
			}
			next.ServeHTTP(ctx.Resp, ctx.Req)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
