// apps/agent/internal/ui/handler.go
package ui

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

type templateData struct {
	State   string
	Label   string
	Domain  string
	Version string
	Error   string
}

type Handler struct {
	tmpl    *template.Template
	domain  string
	version string

	mu    sync.RWMutex
	state string
	label string
	error string

	OnClaim func(token string) error
	OnRetry func()
}

func NewHandler(domain, version string) *Handler {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	return &Handler{
		tmpl:    tmpl,
		domain:  domain,
		version: version,
		state:   "unclaimed",
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
	switch {
	case r.Method == "POST" && r.URL.Path == "/claim":
		h.handleClaim(w, r)
	case r.Method == "POST" && r.URL.Path == "/retry":
		h.handleRetry(w, r)
	default:
		h.renderStatus(w)
	}
}

func (h *Handler) renderStatus(w http.ResponseWriter) {
	h.mu.RLock()
	data := templateData{
		State:   h.state,
		Label:   h.label,
		Domain:  h.domain,
		Version: h.version,
		Error:   h.error,
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

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleRetry(w http.ResponseWriter, r *http.Request) {
	if h.OnRetry != nil {
		h.OnRetry()
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
