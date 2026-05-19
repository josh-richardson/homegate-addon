package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	body := os.Getenv("MOCK_HA_BODY")
	if body == "" {
		body = "homegate-mock-ha-ok"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8123"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	})

	addr := ":" + port
	log.Printf("mock-ha listening on %s, body=%q", addr, body)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
