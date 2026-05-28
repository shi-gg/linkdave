package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://localhost:8080/health")
	if err != nil || resp.StatusCode != http.StatusNoContent {
		os.Exit(1)
	}
	defer resp.Body.Close()
}
