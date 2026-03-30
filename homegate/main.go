package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/homegate/agent/internal/config"
	"github.com/homegate/agent/internal/credentials"
	"github.com/homegate/agent/internal/link"
	"github.com/homegate/agent/internal/tunnel"
	"github.com/homegate/agent/internal/ui"
)

var version = "dev"

func main() {
	cfg := config.Load()

	credStore := credentials.NewStore(cfg.DataDir)
	linkStore := link.NewStore(cfg.DataDir)
	uiHandler := ui.NewHandler(cfg.HostnameDomain, cfg.HostnameSeparator, version, cfg.DashboardURL)

	var mu sync.Mutex
	var client *tunnel.Client

	setClient := func(c *tunnel.Client) {
		mu.Lock()
		defer mu.Unlock()
		client = c
	}

	getClient := func() *tunnel.Client {
		mu.Lock()
		defer mu.Unlock()
		return client
	}

	uiHandler.OnRetry = func() {
		if c := getClient(); c != nil {
			c.Close()
			setClient(nil)
		}
		credStore.Clear()
		linkStore.Clear()
		uiHandler.SetVerificationURL("")
		uiHandler.SetState("initializing", "", "")

		go startLinkFlow(cfg, linkStore, credStore, uiHandler, setClient)
	}

	creds, err := credStore.Load()
	if err != nil {
		log.Printf("no stored credentials: %v", err)
	}

	if creds != nil {
		brokerURL := creds.BrokerURL
		if cfg.BrokerURL != "" {
			brokerURL = cfg.BrokerURL
		}

		c := tunnel.NewClient(brokerURL, creds.JWT, cfg.HATarget)
		setClient(c)
		go func() {
			if err := c.Connect(); err != nil {
				log.Printf("tunnel error: %v", err)
				uiHandler.SetState("failed", "", err.Error())
			}
		}()

		go pollTunnelState(c, uiHandler)
	} else {
		go startLinkFlow(cfg, linkStore, credStore, uiHandler, setClient)
	}

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	if c := getClient(); c != nil {
		c.Close()
	}
	server.Close()
}

func newLinkRequest(cfg *config.Config, linkStore *link.Store, uiHandler *ui.Handler) (*link.LinkState, error) {
	linkStore.Clear()
	deviceUUID := generateUUID()
	result, err := link.CreateRequest(cfg.APIBaseURL, deviceUUID)
	if err != nil {
		return nil, err
	}

	state := &link.LinkState{
		DeviceUUID:      deviceUUID,
		RequestID:       result.RequestID,
		VerificationURL: result.VerificationURL,
		ExpiresAt:       result.ExpiresAt,
	}
	if err := linkStore.Save(state); err != nil {
		log.Printf("failed to save link state: %v", err)
	}

	uiHandler.SetVerificationURL(state.VerificationURL)
	uiHandler.SetState("waiting", "", "")
	return state, nil
}

func startLinkFlow(
	cfg *config.Config,
	linkStore *link.Store,
	credStore *credentials.Store,
	uiHandler *ui.Handler,
	setClient func(*tunnel.Client),
) {
	state, err := linkStore.Load()
	if err != nil || state.IsExpired() {
		state = nil
	}

	if state == nil {
		state, err = newLinkRequest(cfg, linkStore, uiHandler)
		if err != nil {
			log.Printf("failed to create link request: %v", err)
			uiHandler.SetState("failed", "", fmt.Sprintf("Failed to create link request: %v", err))
			return
		}
	} else {
		uiHandler.SetVerificationURL(state.VerificationURL)
		uiHandler.SetState("waiting", "", "")
	}

	for {
		if state.IsExpired() {
			log.Println("link request expired, generating new one")
			uiHandler.SetState("initializing", "", "")

			state, err = newLinkRequest(cfg, linkStore, uiHandler)
			if err != nil {
				log.Printf("failed to create link request: %v", err)
				uiHandler.SetState("failed", "", fmt.Sprintf("Failed to create link request: %v", err))
				return
			}
		}

		status, err := link.PollStatus(cfg.APIBaseURL, state.RequestID)
		if err != nil {
			log.Printf("poll error: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if status.Status == "completed" {
			log.Println("link confirmed, saving credentials")

			newCreds := &credentials.Credentials{
				DeviceID:  status.DeviceID,
				JWT:       status.DeviceJWT,
				BrokerURL: status.BrokerURL,
			}
			if err := credStore.Save(newCreds); err != nil {
				log.Printf("failed to save credentials: %v", err)
				uiHandler.SetState("failed", "", "Failed to save credentials")
				return
			}
			linkStore.Clear()

			c := tunnel.NewClient(status.BrokerURL, status.DeviceJWT, cfg.HATarget)
			setClient(c)
			go func() {
				if err := c.Connect(); err != nil {
					log.Printf("tunnel error: %v", err)
					uiHandler.SetState("failed", "", err.Error())
				}
			}()

			go pollTunnelState(c, uiHandler)
			return
		}

		if status.Status == "expired" {
			state.ExpiresAt = time.Now().Add(-1 * time.Second).Format(time.RFC3339)
			continue
		}

		time.Sleep(10 * time.Second)
	}
}

func pollTunnelState(client *tunnel.Client, uiHandler *ui.Handler) {
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
		time.Sleep(100 * time.Millisecond)
	}
}

func generateUUID() string {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		panic(fmt.Sprintf("failed to generate UUID: %v", err))
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
