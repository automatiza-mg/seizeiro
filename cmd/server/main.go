package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/automatiza-mg/seizeiro/internal/config"
	"github.com/automatiza-mg/seizeiro/internal/database"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.NewFromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	pool, err := database.New(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	<-ctx.Done()
	return nil
}
