package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	chatbotauth "github.com/automatiza-mg/seizeiro/internal/auth/chatbot"
	"github.com/automatiza-mg/seizeiro/internal/config"
	"github.com/automatiza-mg/seizeiro/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

type application struct {
	cfg         *config.Config
	pool        *pgxpool.Pool
	views       fs.FS
	chatbotauth *chatbotauth.Service
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.NewFromEnv()
	if err != nil {
		return err
	}

	pool, err := database.New(ctx, cfg.PostgresURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	encKey, err := cfg.Key()
	if err != nil {
		return err
	}

	chatAuth, err := chatbotauth.NewService(pool, encKey)
	if err != nil {
		return err
	}

	app := &application{
		cfg:         cfg,
		pool:        pool,
		views:       os.DirFS("web/views"),
		chatbotauth: chatAuth,
	}

	srv := &http.Server{
		Addr:    ":4000",
		Handler: app.routes(),
	}

	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()

	if err := srv.Shutdown(stopCtx); err != nil {
		return err
	}
	return nil
}
