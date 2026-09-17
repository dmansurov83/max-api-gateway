package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/you/max-api-self/internal/api"
	"github.com/you/max-api-self/internal/config"
	"github.com/you/max-api-self/internal/core"
)

func main() {
	cfg := config.Load("config.yaml")

	if cfg.MaxToken == "" || cfg.MaxDeviceID == "" {
		log.Fatal("MAX_TOKEN and MAX_DEVICE_ID are required (set in config.yaml or env)")
	}

	core := core.New(&core.Config{
		Token:    cfg.MaxToken,
		DeviceID: cfg.MaxDeviceID,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := core.Connect(ctx); err != nil {
		log.Fatalf("failed to connect to MAX: %v", err)
	}
	defer core.Close()

	go func() {
		for pkt := range core.Packets() {
			log.Printf("incoming packet: opcode=%d", pkt.Opcode)
		}
	}()

	go func() {
		for err := range core.Errors() {
			log.Printf("core error: %v", err)
		}
	}()

	router := api.NewRouter(core, cfg.APIToken)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	go func() {
		log.Printf("listening on :%d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Print("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	core.Close()
}
