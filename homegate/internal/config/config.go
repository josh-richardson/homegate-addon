// apps/agent/internal/config/config.go
package config

import (
	"os"
	"strings"
)

type Config struct {
	APIBaseURL        string // NestJS API URL for claiming
	BrokerURL         string // WebSocket URL for tunnel (overridden by claim response)
	DataDir           string // Credential storage directory
	HATarget          string // Local HA web interface (http://homeassistant:8123)
	IngressPort       string // Port for ingress panel
	HostnameDomain    string // Domain for public URLs (e.g. homegate.example)
	HostnameSeparator string // Separator between label and domain (e.g. "-")
	DashboardURL      string // Web dashboard URL
	AgentVersion      string
}

func Load() *Config {
	apiBaseURL := envStr("API_BASE_URL", "https://api.homegate.example")
	dashboardURL := envStr("DASHBOARD_URL", strings.TrimSuffix(apiBaseURL, "/api"))

	return &Config{
		APIBaseURL:        apiBaseURL,
		BrokerURL:         envStr("BROKER_URL", ""),
		DataDir:           envStr("DATA_DIR", "/data"),
		HATarget:          envStr("HA_TARGET", "http://homeassistant:8123"),
		IngressPort:       envStr("INGRESS_PORT", "8080"),
		HostnameDomain:    envStr("HOSTNAME_DOMAIN", "homegate.example"),
		HostnameSeparator: envStr("HOSTNAME_SEPARATOR", "."),
		DashboardURL:      dashboardURL,
		AgentVersion:      envStr("AGENT_VERSION", "1.0.0"),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
