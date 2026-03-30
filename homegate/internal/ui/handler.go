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
	RegistrationURL string
	VerificationURL string
	LinkStep        string
	LinkStepAction  string
	RetryAction     string
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
	linkStep        string

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
		linkStep:     "ask",
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

func (h *Handler) ResetLinkStep() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.linkStep = "ask"
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST" && hasSuffix(r.URL.Path, "/retry"):
		h.handleRetry(w, r)
	case r.Method == "POST" && hasSuffix(r.URL.Path, "/link-step"):
		h.handleLinkStep(w, r)
	default:
		h.renderStatus(w, r)
	}
}

func hasSuffix(path, suffix string) bool {
	return path == suffix || strings.HasSuffix(path, suffix)
}

func (h *Handler) renderStatus(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	fqdn := ""
	if h.label != "" {
		fqdn = h.label + h.separator + h.domain
	}
	root := ingressRoot(r.URL.Path, "")
	data := templateData{
		State:           h.state,
		FQDN:            fqdn,
		Version:         h.version,
		Error:           h.error,
		DashboardURL:    h.dashboardURL,
		RegistrationURL: "https://test.homegate.network/register",
		VerificationURL: h.verificationURL,
		LinkStep:        h.linkStep,
		LinkStepAction:  root + "link-step",
		RetryAction:     root + "retry",
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

func (h *Handler) handleLinkStep(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	switch r.FormValue("step") {
	case "has-account", "registered":
		h.linkStep = "link"
	case "needs-account":
		h.linkStep = "register"
	}
	h.mu.Unlock()

	http.Redirect(w, r, ingressRoot(r.URL.Path, "/link-step"), http.StatusSeeOther)
}

func ingressRoot(path, suffix string) string {
	if suffix != "" && strings.HasSuffix(path, suffix) {
		return strings.TrimSuffix(path, suffix) + "/"
	}
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}
