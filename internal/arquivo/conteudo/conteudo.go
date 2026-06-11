// Package conteudo extrai o conteúdo textual de arquivos e o persiste no
// banco de dados.
//
// O método de extração é resolvido a partir do MIME type do arquivo:
//
//   - text/plain: o conteúdo é lido diretamente do storage ([MetodoPlain]).
//   - text/html: o conteúdo é convertido para Markdown ([MetodoHTMLMarkdown]).
//   - demais tipos: o texto é extraído via OCR usando a Azure Document
//     Intelligence ([MetodoOCR]).
package conteudo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"

	"github.com/automatiza-mg/seizeiro/internal/arquivo"
	"github.com/automatiza-mg/seizeiro/internal/blob"
	"github.com/automatiza-mg/seizeiro/internal/database"
	"github.com/automatiza-mg/seizeiro/internal/docintel"
	"github.com/automatiza-mg/seizeiro/internal/llm"
	"github.com/automatiza-mg/seizeiro/internal/markdown"
	"github.com/automatiza-mg/seizeiro/internal/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
)

const (
	MetodoOCR          = "ocr"
	MetodoPlain        = "plain"
	MetodoHTMLMarkdown = "html_markdown"

	FormatoPlain    = "plain"
	FormatoMarkdown = "markdown"
)

var (
	// ErrNotFound é retornado quando o conteúdo não existe.
	ErrNotFound = errors.New("conteudo: not found")
)

var _ TokenCounter = (*llm.TokenCounter)(nil)
var _ OCR = (*docintel.Client)(nil)

// OCR extrai texto de documentos via análise assíncrona.
type OCR interface {
	// AnalyzeDocument inicia a análise e retorna a location da operação.
	AnalyzeDocument(ctx context.Context, r io.Reader, contentType string) (string, error)
	// PollResult aguarda a conclusão da operação e retorna o texto extraído.
	PollResult(ctx context.Context, location string) (string, error)
}

// TokenCounter conta os tokens de um texto.
type TokenCounter interface {
	Count(text string) int
}

type Service struct {
	pool     *pgxpool.Pool
	q        *postgres.Queries
	ocr      OCR
	storage  blob.Storage
	embedder llm.Embedder
	tokens   TokenCounter
	river    *river.Client[pgx.Tx]
}

func NewService(pool *pgxpool.Pool, ocr OCR, storage blob.Storage, embedder llm.Embedder, tokens TokenCounter, river *river.Client[pgx.Tx]) *Service {
	return &Service{
		pool:     pool,
		q:        postgres.New(pool),
		ocr:      ocr,
		storage:  storage,
		embedder: embedder,
		tokens:   tokens,
		river:    river,
	}
}

// ExtractConteudo extrai o conteúdo textual de um arquivo e o persiste em
// arquivos_conteudo. O método de extração é resolvido a partir do MIME type
// do arquivo. É idempotente: caso o conteúdo já tenha sido extraído com o
// mesmo método, retorna nil sem reprocessar.
//
// Retorna [arquivo.ErrNotFound] caso o arquivo não exista.
func (s *Service) ExtractConteudo(ctx context.Context, arquivoID int64) error {
	row, err := s.q.GetArquivo(ctx, arquivoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return arquivo.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get arquivo: %w", err)
	}

	mediaType, _, err := mime.ParseMediaType(row.MimeType)
	if err != nil {
		return &arquivo.UnsupportedError{ContentType: row.MimeType}
	}

	var method, format string
	switch mediaType {
	case "text/plain":
		method, format = MetodoPlain, FormatoPlain
	case "text/html":
		method, format = MetodoHTMLMarkdown, FormatoMarkdown
	default:
		method, format = MetodoOCR, FormatoMarkdown
	}

	// Evita reprocessar (e pagar por) uma extração já concluída.
	_, err = s.q.GetArquivoConteudo(ctx, postgres.GetArquivoConteudoParams{
		ArquivoID: arquivoID,
		Metodo:    method,
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("get arquivo conteudo: %w", err)
	}

	rc, err := s.storage.Get(ctx, row.ChaveStorage)
	if err != nil {
		return fmt.Errorf("storage get: %w", err)
	}
	defer rc.Close()

	var text string
	switch method {
	case MetodoPlain:
		b, err := io.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("read all: %w", err)
		}
		text = string(b)
	case MetodoHTMLMarkdown:
		text, err = markdown.ConvertHTML(rc, row.MimeType, markdown.WithoutImg())
		if err != nil {
			return fmt.Errorf("convert html: %w", err)
		}
	case MetodoOCR:
		location, err := s.ocr.AnalyzeDocument(ctx, rc, row.MimeType)
		if err != nil {
			return fmt.Errorf("analyze document: %w", err)
		}

		text, err = s.ocr.PollResult(ctx, location)
		if err != nil {
			return fmt.Errorf("poll result: %w", err)
		}
	}

	// Salva o conteúdo e enfileira o chunking na mesma transação: a task de
	// chunking existe se, e somente se, o conteúdo foi persistido.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	saved, err := s.q.WithTx(tx).SaveArquivoConteudo(ctx, postgres.SaveArquivoConteudoParams{
		ArquivoID: arquivoID,
		Metodo:    method,
		Formato:   format,
		Conteudo:  text,
	})
	if err != nil {
		if database.IsUniqueError(err, "arquivos_conteudo_arquivo_id_metodo_key") {
			return nil
		}
		return fmt.Errorf("save arquivo conteudo: %w", err)
	}

	_, err = s.river.InsertTx(ctx, tx, arquivo.ChunkArgs{ConteudoID: saved.ID}, nil)
	if err != nil {
		return fmt.Errorf("insert chunk task: %w", err)
	}

	return tx.Commit(ctx)
}

// ChunkConteudo divide o conteúdo extraído de um arquivo em chunks, gera os
// embeddings de cada chunk e os persiste em arquivos_conteudo_chunks. É
// idempotente: caso o conteúdo já possua chunks, retorna nil sem reprocessar.
//
// Retorna [ErrNotFound] caso o conteúdo não exista.
func (s *Service) ChunkConteudo(ctx context.Context, conteudoID int64) error {
	row, err := s.q.GetArquivoConteudoByID(ctx, conteudoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get conteudo: %w", err)
	}

	// Evita reprocessar (e pagar por) embeddings já gerados.
	count, err := s.q.CountArquivoConteudoChunks(ctx, conteudoID)
	if err != nil {
		return fmt.Errorf("count chunks: %w", err)
	}
	if count > 0 {
		return nil
	}

	chunks, err := splitText(row.Conteudo)
	if err != nil {
		return fmt.Errorf("split text: %w", err)
	}
	if len(chunks) == 0 {
		return nil
	}

	embeddings, err := s.embedder.EmbedDocuments(ctx, chunks)
	if err != nil {
		return fmt.Errorf("embed documents: %w", err)
	}
	if len(embeddings) != len(chunks) {
		return fmt.Errorf("expected %d embeddings, got %d", len(chunks), len(embeddings))
	}

	// Persiste todos os chunks em uma única transação: em caso de falha
	// parcial, nenhum chunk fica visível e o retry recomeça do zero.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.q.WithTx(tx)
	for i, chunk := range chunks {
		err := qtx.SaveArquivoConteudoChunk(ctx, postgres.SaveArquivoConteudoChunkParams{
			ConteudoID: conteudoID,
			Indice:     int32(i),
			Conteudo:   chunk,
			Tokens:     countTokens(s.tokens, chunk),
			Embedding:  pgvector.NewVector(embeddings[i]),
		})
		if err != nil {
			if database.IsUniqueError(err, "arquivos_conteudo_chunks_conteudo_id_indice_key") {
				return nil
			}
			return fmt.Errorf("save chunk %d: %w", i, err)
		}
	}

	return tx.Commit(ctx)
}

// Conta os tokens de text como telemetria opcional.
//
// A contagem é best-effort: na ausência de um contador, retorna um
// valor nulo (NULL no banco) sem interromper o fluxo.
func countTokens(counter TokenCounter, text string) pgtype.Int4 {
	if counter == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(counter.Count(text)), Valid: true}
}
