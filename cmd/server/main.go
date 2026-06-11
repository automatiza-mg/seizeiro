package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/automatiza-mg/seizeiro/internal/arquivo"
	"github.com/automatiza-mg/seizeiro/internal/arquivo/conteudo"
	"github.com/automatiza-mg/seizeiro/internal/blob"
	"github.com/automatiza-mg/seizeiro/internal/config"
	"github.com/automatiza-mg/seizeiro/internal/database"
	"github.com/automatiza-mg/seizeiro/internal/docintel"
	"github.com/automatiza-mg/seizeiro/internal/llm"
	"github.com/automatiza-mg/seizeiro/internal/postgres/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
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

	if err := riverUp(ctx, pool); err != nil {
		return fmt.Errorf("river up: %w", err)
	}

	embedder, err := llm.NewOpenAIEmbedder(llm.OpenAIParams{
		APIKey:     cfg.OpenAI.APIKey,
		BaseURL:    cfg.OpenAI.BaseURL,
		Model:      cfg.OpenAI.EmbeddingModel,
		Dimensions: cfg.OpenAI.EmbeddingDimensions,
		BatchSize:  cfg.OpenAI.EmbeddingBatchSize,
	})
	if err != nil {
		return fmt.Errorf("embedder: %w", err)
	}

	tokenCounter, err := llm.NewTokenCounter(cfg.OpenAI.EmbeddingModel)
	if err != nil {
		return fmt.Errorf("token counter: %w", err)
	}

	storage, err := newStorage(cfg.Storage)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}

	ocr := docintel.NewClient(cfg.DocIntel.Endpoint, cfg.DocIntel.Key)

	workers := river.NewWorkers()
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	conteudoService := conteudo.NewService(pool, ocr, storage, embedder, tokenCounter, riverClient)
	_ = arquivo.NewService(pool, storage, riverClient)

	river.AddWorker(workers, conteudo.NewExtractConteudoWorker(conteudoService))
	river.AddWorker(workers, conteudo.NewChunkConteudoWorker(conteudoService))

	if err := riverClient.Start(ctx); err != nil {
		return fmt.Errorf("river start: %w", err)
	}

	<-ctx.Done()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()

	if err := riverClient.Stop(stopCtx); err != nil {
		return fmt.Errorf("river stop: %w", err)
	}

	return nil
}

// Cria o backend de armazenamento de acordo com a configuração.
func newStorage(cfg config.Storage) (blob.Storage, error) {
	if cfg.AzureAccount != "" {
		return blob.NewAzureStorage(cfg.AzureAccount, cfg.AzureContainer)
	}
	return blob.NewFilesystemStorage(cfg.FilesystemRoot)
}

// Aplica as migrações do River adaptando [pgxpool.Pool] para [sql.DB].
func riverUp(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	return migrations.RiverUp(ctx, db)
}
