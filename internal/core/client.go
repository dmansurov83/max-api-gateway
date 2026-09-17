package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/MrCatchParkington/go-max-client/maxclient"
)

type MaxCore struct {
	client *maxclient.Client
	cfg    *Config
	log    *slog.Logger
}

type Config struct {
	Token    string
	DeviceID string
}

func New(cfg *Config) *MaxCore {
	return &MaxCore{
		cfg: cfg,
		log: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}
}

func (c *MaxCore) Connect(ctx context.Context) error {
	c.client = maxclient.New(
		maxclient.WithAutoReconnect(true),
		maxclient.WithReconnectBackoff(2*time.Second, 60*time.Second),
		maxclient.WithLogger(c.log),
	)

	if err := c.client.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	if err := c.client.AuthToken(ctx, c.cfg.Token, c.cfg.DeviceID); err != nil {
		c.client.Close()
		return fmt.Errorf("auth: %w", err)
	}

	c.log.Info("authenticated and connected")
	return nil
}

func (c *MaxCore) SendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := c.client.SendMessage(ctx, chatID, text)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

func (c *MaxCore) SendFile(ctx context.Context, chatID int64, filename string, reader io.Reader) error {
	attach, err := c.client.UploadPhoto(ctx, filename, reader)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	_, err = c.client.SendMessage(ctx, chatID, "", maxclient.SendMessageOpts{
		Attaches: []maxclient.Attachment{*attach},
	})
	if err != nil {
		return fmt.Errorf("send with attach: %w", err)
	}
	return nil
}

func (c *MaxCore) IsConnected() bool {
	if c.client == nil {
		return false
	}
	// Best-effort: try a lightweight ping. If it fails, not connected.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.client.InvokeMethod(ctx, 1, map[string]any{"interactive": false}) // keepalive opcode
	return err == nil
}

func (c *MaxCore) Packets() <-chan *maxclient.Packet {
	if c.client == nil {
		return nil
	}
	return c.client.Packets()
}

func (c *MaxCore) Errors() <-chan error {
	if c.client == nil {
		return nil
	}
	return c.client.Errors()
}

func (c *MaxCore) Close() {
	if c.client != nil {
		c.client.Close()
	}
}