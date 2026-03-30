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
	State           string
	FQDN            string
	Version         string
	Error           string
	DashboardURL    string
	VerificationURL string
}

type Handler struct {
	tmpl         *template.Template
	domain       string
	separator    string
	version      string
	dashboardURL string

	mu              sync.RWMutex
	state           string
	label           string
	error           string
	verificationURL string

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
		state:        "initializing",
	}
}

func (h *Handler) SetState(state, label, errMsg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state = state
	h.label = label
	h.error = errMsg
}

func (h *Handler) SetVerificationURL(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.verificationURL = url
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
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
		State:           h.state,
		FQDN:            fqdn,
		Version:         h.version,
		Error:           h.error,
		DashboardURL:    h.dashboardURL,
		VerificationURL: h.verificationURL,
	}
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "status.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "template error", 500)
	}
}

func (h *Handler) handleRetry(w http.ResponseWriter, r *http.Request) {
	if h.OnRetry != nil {
		h.OnRetry()
	}
	http.Redirect(w, r, ingressRoot(r.URL.Path, "/retry"), http.StatusSeeOther)
}

func ingressRoot(path, suffix string) string {
	if strings.HasSuffix(path, suffix) {
		return strings.TrimSuffix(path, suffix) + "/"
	}
	return "./"
}
