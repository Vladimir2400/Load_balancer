package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	port := "8081"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	hostname, _ := os.Hostname()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Backend [%s:%s]: Received request: %s %s from %s", hostname, port, r.Method, r.URL.Path, r.RemoteAddr)
		time.Sleep(50 * time.Millisecond)
		fmt.Fprintf(w, "Hello from backend server %s on port %s!", hostname, port)
	})

	log.Printf("Backend server %s starting on port %s...", hostname, port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("FATAL: Failed to start backend server on port %s: %v", port, err)
	}
}
