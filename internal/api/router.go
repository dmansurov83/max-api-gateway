package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/you/max-api-self/internal/core"
)

func NewRouter(core *core.MaxCore, apiToken string) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	if apiToken != "" {
		r.Use(authMiddleware(apiToken))
	}

	r.Get("/health", handleHealth(core))
	r.Post("/send", handleSend(core))
	r.Post("/upload", handleUpload(core))

	return r
}