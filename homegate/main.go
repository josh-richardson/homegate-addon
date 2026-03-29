// apps/agent/main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homegate/agent/internal/claim"
	"github.com/homegate/agent/internal/config"
	"github.com/homegate/agent/internal/credentials"
	"github.com/homegate/agent/internal/tunnel"
	"github.com/homegate/agent/internal/ui"
)

func main() {
	cfg := config.Load()

	store := credentials.NewStore(cfg.DataDir)
	uiHandler := ui.NewHandler(cfg.HostnameDomain, cfg.HostnameSeparator, cfg.AgentVersion)

	// Try loading existing credentials
	creds, err := store.Load()
	if err != nil {
		log.Printf("failed to load credentials: %v", err)
	}

	var client *tunnel.Client

	// Wire up claim handler
	uiHandler.OnClaim = func(token string) error {
		result, err := claim.Exchange(cfg.APIBaseURL, token)
		if err != nil {
			return err
		}

		newCreds := &credentials.Credentials{
			DeviceID:  result.DeviceID,
			JWT:       result.DeviceJWT,
			BrokerURL: result.BrokerURL,
		}
		if err := store.Save(newCreds); err != nil {
			return fmt.Errorf("save credentials: %w", err)
		}

		// Start tunnel
		client = tunnel.NewClient(result.BrokerURL, result.DeviceJWT, cfg.HATarget)
		go func() {
			if err := client.Connect(); err != nil {
				log.Printf("tunnel error: %v", err)
				uiHandler.SetState("failed", "", err.Error())
			}
		}()

		// Update UI on connect (poll briefly)
		go func() {
			for i := 0; i < 50; i++ {
				if client.State() == tunnel.StateConnected {
					uiHandler.SetState("connected", client.Label(), "")
					return
				}
				if client.State() == tunnel.StateFailed {
					return
				}
				select {
				case <-client.Done():
					return
				default:
				}
				<-time.After(100 * time.Millisecond)
			}
		}()

		return nil
	}

	// Wire up retry handler
	uiHandler.OnRetry = func() {
		if client != nil {
			client.Close()
			client = nil
		}
		store.Clear()
		uiHandler.SetState("unclaimed", "", "")
	}

	// If we have credentials, start tunnel immediately
	if creds != nil {
		brokerURL := creds.BrokerURL
		if cfg.BrokerURL != "" {
			brokerURL = cfg.BrokerURL // env override
		}

		client = tunnel.NewClient(brokerURL, creds.JWT, cfg.HATarget)

		go func() {
			if err := client.Connect(); err != nil {
				log.Printf("tunnel error: %v", err)
				uiHandler.SetState("failed", "", err.Error())
			}
		}()

		// Poll for connection state
		go func() {
			for i := 0; i < 50; i++ {
				switch client.State() {
				case tunnel.StateConnected:
					uiHandler.SetState("connected", client.Label(), "")
					return
				case tunnel.StateFailed:
					return
				case tunnel.StateReconnecting:
					uiHandler.SetState("reconnecting", "", "")
				}
				<-time.After(100 * time.Millisecond)
			}
		}()
	}

	// Start ingress server
	server := &http.Server{
		Addr:    ":" + cfg.IngressPort,
		Handler: uiHandler,
	}

	go func() {
		log.Printf("ingress panel listening on :%s", cfg.IngressPort)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	if client != nil {
		client.Close()
	}
	server.Close()
}
