package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/you/max-api-self/internal/core"
)

type sendRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func handleHealth(core *core.MaxCore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"status":    "ok",
			"connected": core.IsConnected(),
		}
		if !core.IsConnected() {
			status["status"] = "degraded"
		}
		json.NewEncoder(w).Encode(status)
	}
}

func handleSend(core *core.MaxCore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req sendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "invalid JSON body"})
			return
		}

		if req.Text == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "text is required"})
			return
		}

		if err := core.SendMessage(r.Context(), req.ChatID, req.Text); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func handleUpload(core *core.MaxCore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "invalid multipart form"})
			return
		}

		chatIDStr := r.FormValue("chat_id")
		chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "invalid chat_id"})
			return
		}

file, header, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "file is required"})
			return
		}
		defer file.Close()

		if err := core.SendFile(r.Context(), chatID, header.Filename, file); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}