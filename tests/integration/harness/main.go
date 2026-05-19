// Homegate addon end-to-end test harness.
//
// Drives a fresh signup + site creation + addon link/claim + tunnel
// round-trip against a live API environment (staging or production).
// The Mailtrap sandbox endpoint captures the signup-confirmation email
// so the harness can extract the token without a real mailbox.
//
// Required env vars:
//
//	E2E_ENV                  staging|production
//	E2E_PASSWORD             password for the throwaway test user
//	MAILTRAP_API_TOKEN       account-level Mailtrap token (read inbox)
//	MAILTRAP_ACCOUNT_ID      Mailtrap account id
//	MAILTRAP_INBOX_ID        sandbox inbox id receiving signup mail
//	E2E_SIGNUP_DOMAIN        domain part for throwaway emails
//
// Optional:
//
//	E2E_CAPTCHA_TOKEN        Turnstile token (when API has captcha on)
//	E2E_COMPOSE_FILE         path to compose.yml (default ../compose.yml)
//	E2E_DATA_DIR             host path bind-mounted as addon /data (default ../data)
//	E2E_MOCK_HA_BODY         expected mock-ha body (default homegate-e2e-ok)
//	E2E_LINK_TIMEOUT         seconds to wait for link-request file (default 30)
//	E2E_TUNNEL_TIMEOUT       seconds to wait for device online (default 60)
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type envConfig struct {
	apiBase  string
	domain   string
	separator string
}

var envs = map[string]envConfig{
	"staging":    {apiBase: "https://homegate-test.website/api", domain: "homegate-test.website", separator: "."},
	"production": {apiBase: "https://homegate.network/api", domain: "homegate.network", separator: "."},
}

type harness struct {
	env          envConfig
	password     string
	email        string
	httpClient   *http.Client
	mailtrap     mailtrapClient
	composeFile  string
	dataDir      string
	mockHABody   string
	linkTimeout  time.Duration
	tunnelTimeout time.Duration
	captchaToken string

	siteID       string
	siteHostname string
	requestID    string
	composeUp    bool
}

func main() {
	h, err := newHarness()
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
	defer h.cleanup()

	steps := []struct {
		name string
		fn   func() error
	}{
		{"signup-confirm-login", h.signupConfirmLogin},
		{"create-site", h.createSite},
		{"start-addon", h.startAddon},
		{"read-link-request", h.readLinkRequest},
		{"confirm-link", h.confirmLink},
		{"wait-tunnel", h.waitForTunnel},
		{"round-trip", h.roundTripHTTP},
	}
	for _, s := range steps {
		log.Printf("==> %s", s.name)
		if err := s.fn(); err != nil {
			log.Fatalf("step %s: %v", s.name, err)
		}
	}
	log.Println("==> E2E PASSED")
}

func newHarness() (*harness, error) {
	envName := os.Getenv("E2E_ENV")
	env, ok := envs[envName]
	if !ok {
		return nil, fmt.Errorf("E2E_ENV must be staging|production, got %q", envName)
	}

	required := []string{"E2E_PASSWORD", "MAILTRAP_API_TOKEN", "MAILTRAP_ACCOUNT_ID", "MAILTRAP_INBOX_ID", "E2E_SIGNUP_DOMAIN"}
	for _, k := range required {
		if os.Getenv(k) == "" {
			return nil, fmt.Errorf("missing required env var %s", k)
		}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	composeFile := envOr("E2E_COMPOSE_FILE", "../compose.yml")
	dataDir := envOr("E2E_DATA_DIR", "../data")
	absData, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}

	return &harness{
		env:           env,
		password:      os.Getenv("E2E_PASSWORD"),
		email:         fmt.Sprintf("e2e-%d@%s", time.Now().UnixNano(), os.Getenv("E2E_SIGNUP_DOMAIN")),
		httpClient:    &http.Client{Jar: jar, Timeout: 30 * time.Second},
		mailtrap:      newMailtrap(os.Getenv("MAILTRAP_API_TOKEN"), os.Getenv("MAILTRAP_ACCOUNT_ID"), os.Getenv("MAILTRAP_INBOX_ID")),
		composeFile:   composeFile,
		dataDir:       absData,
		mockHABody:    envOr("E2E_MOCK_HA_BODY", "homegate-e2e-ok"),
		linkTimeout:   parseSecondsOr("E2E_LINK_TIMEOUT", 30),
		tunnelTimeout: parseSecondsOr("E2E_TUNNEL_TIMEOUT", 60),
		captchaToken:  os.Getenv("E2E_CAPTCHA_TOKEN"),
	}, nil
}

func (h *harness) signupConfirmLogin() error {
	log.Printf("test user: %s", h.email)

	signupBody := map[string]string{"email": h.email, "password": h.password}
	if h.captchaToken != "" {
		signupBody["captchaToken"] = h.captchaToken
	}
	if _, err := h.apiPOST("/auth/register", signupBody, nil); err != nil {
		return fmt.Errorf("register: %w", err)
	}

	token, err := h.mailtrap.WaitForConfirmationToken(h.email, 60*time.Second)
	if err != nil {
		return fmt.Errorf("wait for email: %w", err)
	}
	log.Printf("got confirmation token (len=%d)", len(token))

	if _, err := h.apiPOST("/auth/confirm-email", map[string]string{"token": token}, nil); err != nil {
		return fmt.Errorf("confirm-email: %w", err)
	}

	loginBody := map[string]string{"email": h.email, "password": h.password}
	if h.captchaToken != "" {
		loginBody["captchaToken"] = h.captchaToken
	}
	if _, err := h.apiPOST("/auth/login", loginBody, nil); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

func (h *harness) createSite() error {
	var resp struct {
		ID       string `json:"id"`
		Hostname struct {
			FQDN string `json:"fqdn"`
		} `json:"hostname"`
	}
	if _, err := h.apiPOST("/sites", map[string]string{"name": "e2e-site"}, &resp); err != nil {
		return err
	}
	if resp.ID == "" || resp.Hostname.FQDN == "" {
		return fmt.Errorf("unexpected create-site response: %+v", resp)
	}
	h.siteID = resp.ID
	h.siteHostname = resp.Hostname.FQDN
	log.Printf("site %s (%s) created", h.siteID, h.siteHostname)
	return nil
}

func (h *harness) startAddon() error {
	if err := os.RemoveAll(h.dataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(h.dataDir, 0o755); err != nil {
		return err
	}
	// Write /data/options.json so run.sh picks the right environment
	envName := "staging"
	if h.env.domain == envs["production"].domain {
		envName = "production"
	}
	opts := map[string]string{"environment": envName}
	if err := writeJSON(filepath.Join(h.dataDir, "options.json"), opts); err != nil {
		return err
	}

	cmd := exec.Command("docker", "compose", "-f", h.composeFile, "up", "-d", "--build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "MOCK_HA_BODY="+h.mockHABody)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	h.composeUp = true
	return nil
}

func (h *harness) readLinkRequest() error {
	path := filepath.Join(h.dataDir, "link-request.json")
	deadline := time.Now().Add(h.linkTimeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var st struct {
				RequestID string `json:"requestId"`
			}
			if err := json.Unmarshal(data, &st); err == nil && st.RequestID != "" {
				h.requestID = st.RequestID
				log.Printf("addon link request: %s", st.RequestID)
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for %s after %s", path, h.linkTimeout)
}

func (h *harness) confirmLink() error {
	body := map[string]string{"requestId": h.requestID, "siteId": h.siteID}
	if _, err := h.apiPOST("/device-auth/link-confirm", body, nil); err != nil {
		return err
	}
	log.Printf("link confirmed for site %s", h.siteID)
	return nil
}

func (h *harness) waitForTunnel() error {
	deadline := time.Now().Add(h.tunnelTimeout)
	for time.Now().Before(deadline) {
		var resp struct {
			IsOnline bool `json:"isOnline"`
		}
		if _, err := h.apiGET("/sites/"+h.siteID, &resp); err == nil && resp.IsOnline {
			log.Println("device online")
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("device did not come online within %s", h.tunnelTimeout)
}

func (h *harness) roundTripHTTP() error {
	url := "https://" + h.siteHostname + "/"
	log.Printf("GET %s", url)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), h.mockHABody) {
		return fmt.Errorf("body %q does not contain %q", body, h.mockHABody)
	}
	log.Println("round-trip OK")
	return nil
}

func (h *harness) cleanup() {
	if h.composeUp {
		cmd := exec.Command("docker", "compose", "-f", h.composeFile, "down", "-v")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
	if h.siteID != "" {
		_, _ = h.apiDELETE("/sites/" + h.siteID)
	}
}

// --- HTTP helpers -----------------------------------------------------------

func (h *harness) apiPOST(path string, body any, out any) (*http.Response, error) {
	return h.apiRequest("POST", path, body, out)
}

func (h *harness) apiGET(path string, out any) (*http.Response, error) {
	return h.apiRequest("GET", path, nil, out)
}

func (h *harness) apiDELETE(path string) (*http.Response, error) {
	return h.apiRequest("DELETE", path, nil, nil)
}

func (h *harness) apiRequest(method, path string, body any, out any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, h.env.apiBase+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return resp, fmt.Errorf("%s %s -> %d: %s", method, path, resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp, fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp, nil
}

// --- Mailtrap sandbox client ------------------------------------------------

type mailtrapClient struct {
	token     string
	accountID string
	inboxID   string
	http      *http.Client
}

func newMailtrap(token, accountID, inboxID string) mailtrapClient {
	return mailtrapClient{
		token:     token,
		accountID: accountID,
		inboxID:   inboxID,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

type mailtrapMessage struct {
	ID       int    `json:"id"`
	ToEmail  string `json:"to_email"`
	HTMLPath string `json:"html_path"`
	Subject  string `json:"subject"`
	SentAt   string `json:"sent_at"`
}

func (m mailtrapClient) WaitForConfirmationToken(recipient string, timeout time.Duration) (string, error) {
	tokenRe := regexp.MustCompile(`[?&]token=([A-Za-z0-9._\-]+)`)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msgs, err := m.listMessages()
		if err != nil {
			log.Printf("mailtrap list err (will retry): %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for _, msg := range msgs {
			if !strings.EqualFold(msg.ToEmail, recipient) {
				continue
			}
			html, err := m.fetchHTML(msg.HTMLPath)
			if err != nil {
				return "", fmt.Errorf("fetch html for msg %d: %w", msg.ID, err)
			}
			if match := tokenRe.FindStringSubmatch(html); match != nil {
				return match[1], nil
			}
			return "", fmt.Errorf("no token query param in confirmation email %d", msg.ID)
		}
		time.Sleep(2 * time.Second)
	}
	return "", errors.New("timed out waiting for confirmation email")
}

func (m mailtrapClient) listMessages() ([]mailtrapMessage, error) {
	url := fmt.Sprintf("https://mailtrap.io/api/accounts/%s/inboxes/%s/messages", m.accountID, m.inboxID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Api-Token", m.token)
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list messages %d: %s", resp.StatusCode, body)
	}
	var msgs []mailtrapMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (m mailtrapClient) fetchHTML(htmlPath string) (string, error) {
	url := "https://mailtrap.io" + htmlPath
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Api-Token", m.token)
	resp, err := m.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetch html %d: %s", resp.StatusCode, body)
	}
	return string(body), nil
}

// --- misc -------------------------------------------------------------------

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseSecondsOr(k string, def int) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return time.Duration(def) * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("invalid %s=%q, using default %d", k, v, def)
		return time.Duration(def) * time.Second
	}
	return time.Duration(n) * time.Second
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
