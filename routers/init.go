// Package routers assembles the HTTP handler.
package routers

import (
	"net/http"

	"git.cu3rd.ru/cu3rd/dash/routers/web"
)

// NormalRoutes returns the full application handler.
func NormalRoutes() http.Handler {
	return web.Routes()
}
