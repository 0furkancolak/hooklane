package http

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// dashboardCSP allows only same-origin scripts, styles and API calls; the
// dashboard has no inline scripts and loads nothing from other origins.
const dashboardCSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

// WithDashboard serves the built dashboard (web/dist) from dir at "/".
func WithDashboard(dir string) Option { return func(o *options) { o.dashboardDir = dir } }

func mountDashboard(app *fiber.App, dir string) {
	app.Get("/*", static.New(dir, static.Config{
		// API and probe paths never fall through to static files.
		Next: func(c fiber.Ctx) bool {
			p := c.Path()
			return strings.HasPrefix(p, "/v1/") || p == "/v1" || p == "/healthz" || p == "/readyz" || p == "/openapi.yaml"
		},
		IndexNames: []string{"index.html"},
		ModifyResponse: func(c fiber.Ctx) error {
			c.Set("Content-Security-Policy", dashboardCSP)
			c.Set("X-Frame-Options", "DENY")
			c.Set("Referrer-Policy", "no-referrer")
			// Vite fingerprints everything under /assets, so those files can
			// be cached forever; index.html keeps no-store to pick up releases.
			if strings.HasPrefix(c.Path(), "/assets/") {
				c.Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			return nil
		},
	}))
}
