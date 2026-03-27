// apps/agent/internal/config/config.go
package config

import "os"

type Config struct {
	APIBaseURL     string // NestJS API URL for claiming
	BrokerURL      string // WebSocket URL for tunnel (overridden by claim response)
	DataDir        string // Credential storage directory
	HATarget       string // Local HA web interface (http://homeassistant:8123)
	IngressPort    string // Port for ingress panel
	HostnameDomain string // Domain for public URLs (e.g. homegate.example)
	AgentVersion   string
}

func Load() *Config {
	return &Config{
		APIBaseURL:     envStr("API_BASE_URL", "https://api.homegate.example"),
		BrokerURL:      envStr("BROKER_URL", ""),
		DataDir:        envStr("DATA_DIR", "/data"),
		HATarget:       envStr("HA_TARGET", "http://homeassistant:8123"),
		IngressPort:    envStr("INGRESS_PORT", "8080"),
		HostnameDomain: envStr("HOSTNAME_DOMAIN", "homegate.example"),
		AgentVersion:   envStr("AGENT_VERSION", "1.0.0"),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
