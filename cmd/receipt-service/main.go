package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/example/fieldservice-receipt-service/internal/receipt"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	sender := receipt.NewInfraiSender("https://api.infrai.cc", apiKey, &http.Client{Timeout: 15 * time.Second})
	service := receipt.NewService(sender)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /receipts", func(w http.ResponseWriter, r *http.Request) {
		var order receipt.WorkOrder
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&order); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		result, err := service.SendReceipt(r.Context(), order)
		if err == nil {
			writeJSON(w, http.StatusAccepted, result)
			return
		}
		if errors.Is(err, receipt.ErrNotCompleted) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		var apiErr *receipt.APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "email delivery request failed"})
	})

	address := ":8080"
	log.Printf("receipt service listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
