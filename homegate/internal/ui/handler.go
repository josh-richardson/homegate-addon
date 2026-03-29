// apps/agent/internal/ui/handler.go
package ui

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

type templateData struct {
	State        string
	FQDN         string
	Version      string
	Error        string
	DashboardURL string
}

type Handler struct {
	tmpl         *template.Template
	domain       string
	separator    string
	version      string
	dashboardURL string

	mu    sync.RWMutex
	state string
	label string
	error string

	OnClaim func(token string) error
	OnRetry func()
}

func NewHandler(domain, separator, version, dashboardURL string) *Handler {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	return &Handler{
		tmpl:         tmpl,
		domain:       domain,
		separator:    separator,
		version:      version,
		dashboardURL: dashboardURL,
		state:        "unclaimed",
	}
}

func (h *Handler) SetState(state, label, errMsg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state = state
	h.label = label
	h.error = errMsg
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Use HasSuffix so routes work behind HA ingress proxy which may
	// preserve the /api/hassio_ingress/<token>/ prefix.
	switch {
	case r.Method == "POST" && hasSuffix(r.URL.Path, "/claim"):
		h.handleClaim(w, r)
	case r.Method == "POST" && hasSuffix(r.URL.Path, "/retry"):
		h.handleRetry(w, r)
	default:
		h.renderStatus(w)
	}
}

func hasSuffix(path, suffix string) bool {
	return path == suffix || strings.HasSuffix(path, suffix)
}

func (h *Handler) renderStatus(w http.ResponseWriter) {
	h.mu.RLock()
	fqdn := ""
	if h.label != "" {
		fqdn = h.label + h.separator + h.domain
	}
	data := templateData{
		State:        h.state,
		FQDN:         fqdn,
		Version:      h.version,
		Error:        h.error,
		DashboardURL: h.dashboardURL,
	}
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "status.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "template error", 500)
	}
}

func (h *Handler) handleClaim(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if token == "" {
		h.mu.Lock()
		h.error = "Token is required"
		h.mu.Unlock()
		h.renderStatus(w)
		return
	}

	if h.OnClaim != nil {
		if err := h.OnClaim(token); err != nil {
			h.mu.Lock()
			h.error = err.Error()
			h.mu.Unlock()
			h.renderStatus(w)
			return
		}
	}

	http.Redirect(w, r, ingressRoot(r.URL.Path, "/claim"), http.StatusSeeOther)
}

func (h *Handler) handleRetry(w http.ResponseWriter, r *http.Request) {
	if h.OnRetry != nil {
		h.OnRetry()
	}
	http.Redirect(w, r, ingressRoot(r.URL.Path, "/retry"), http.StatusSeeOther)
}

// ingressRoot returns the addon's root path by stripping the action suffix.
// e.g. "/api/hassio_ingress/<token>/claim" → "/api/hassio_ingress/<token>/"
func ingressRoot(path, suffix string) string {
	if strings.HasSuffix(path, suffix) {
		return strings.TrimSuffix(path, suffix) + "/"
	}
	return "./"
}
