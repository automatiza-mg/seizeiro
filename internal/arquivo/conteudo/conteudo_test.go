package conteudo

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/automatiza-mg/seizeiro/internal/arquivo"
	"github.com/automatiza-mg/seizeiro/internal/blob"
	"github.com/automatiza-mg/seizeiro/internal/database"
	"github.com/automatiza-mg/seizeiro/internal/llm"
	"github.com/automatiza-mg/seizeiro/internal/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

var ti *database.TestInstance

func TestMain(m *testing.M) {
	ti = database.MustTestInstance()
	code := m.Run()

	if err := ti.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	os.Exit(code)
}

// Implementa a interface OCR para testes, sem chamadas externas.
type fakeOCR struct {
	text         string
	analyzeErr   error
	pollErr      error
	analyzeCalls int
}

func (f *fakeOCR) AnalyzeDocument(ctx context.Context, r io.Reader, contentType string) (string, error) {
	f.analyzeCalls++
	if f.analyzeErr != nil {
		return "", f.analyzeErr
	}
	// Consome o reader como o cliente real faria.
	if _, err := io.Copy(io.Discard, r); err != nil {
		return "", err
	}
	return "operation-location", nil
}

func (f *fakeOCR) PollResult(ctx context.Context, location string) (string, error) {
	if f.pollErr != nil {
		return "", f.pollErr
	}
	return f.text, nil
}

// fakeEmbedder implementa llm.Embedder para testes, sem chamadas externas.
type fakeEmbedder struct {
	dims  int
	err   error
	calls int
}

func (f *fakeEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = make([]float32, f.dims)
	}
	return embeddings, nil
}

func (f *fakeEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return make([]float32, f.dims), nil
}

type fixture struct {
	pool     *pgxpool.Pool
	service  *Service
	arquivos *arquivo.Service
	ocr      *fakeOCR
	embedder *fakeEmbedder
}

func newFixture(tb testing.TB) *fixture {
	tb.Helper()

	pool := ti.NewPool(tb)
	storage, err := blob.NewFilesystemStorage(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	ocr := &fakeOCR{text: "# Documento\n\nTexto extraído via OCR."}
	// 1536 dimensões para casar com o schema VECTOR(1536).
	embedder := &fakeEmbedder{dims: 1536}

	tokens, err := llm.NewTokenCounter("text-embedding-3-small")
	if err != nil {
		tb.Fatal(err)
	}

	// Client insert-only: sem workers nem queues, apenas para enfileirar tasks.
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		tb.Fatal(err)
	}

	return &fixture{
		pool:     pool,
		service:  NewService(pool, ocr, storage, embedder, tokens, riverClient),
		arquivos: arquivo.NewService(pool, storage, riverClient),
		ocr:      ocr,
		embedder: embedder,
	}
}

// createArquivo cria um arquivo de teste e retorna seu ID.
func (f *fixture) createArquivo(tb testing.TB, content, contentType string) int64 {
	tb.Helper()

	arq, err := f.arquivos.CreateArquivo(tb.Context(), strings.NewReader(content), contentType)
	if err != nil {
		tb.Fatal(err)
	}
	return arq.ID
}

// getConteudo lê o conteúdo extraído diretamente do banco de dados.
func (f *fixture) getConteudo(tb testing.TB, arquivoID int64, metodo string) postgres.ArquivoConteudo {
	tb.Helper()

	row, err := f.service.q.GetArquivoConteudo(tb.Context(), postgres.GetArquivoConteudoParams{
		ArquivoID: arquivoID,
		Metodo:    metodo,
	})
	if err != nil {
		tb.Fatal(err)
	}
	return row
}

func TestExtractConteudo_OCR(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	id := f.createArquivo(t, "%PDF-1.4 conteúdo binário", "application/pdf")

	if err := f.service.ExtractConteudo(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	row := f.getConteudo(t, id, MetodoOCR)
	if row.Formato != FormatoMarkdown {
		t.Fatalf("want formato %q, got %q", FormatoMarkdown, row.Formato)
	}
	if row.Conteudo != f.ocr.text {
		t.Fatalf("want conteudo %q, got %q", f.ocr.text, row.Conteudo)
	}
	if f.ocr.analyzeCalls != 1 {
		t.Fatalf("want 1 analyze call, got %d", f.ocr.analyzeCalls)
	}
}

func TestExtractConteudo_Plain(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	const content = "olá mundo, texto puro"
	id := f.createArquivo(t, content, "text/plain; charset=utf-8")

	if err := f.service.ExtractConteudo(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	row := f.getConteudo(t, id, MetodoPlain)
	if row.Formato != FormatoPlain {
		t.Fatalf("want formato %q, got %q", FormatoPlain, row.Formato)
	}
	if row.Conteudo != content {
		t.Fatalf("want conteudo %q, got %q", content, row.Conteudo)
	}
	if f.ocr.analyzeCalls != 0 {
		t.Fatalf("want 0 analyze calls, got %d", f.ocr.analyzeCalls)
	}
}

func TestExtractConteudo_HTMLMarkdown(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	const content = `<html><body><h1>Título</h1><p>Olá <strong>mundo</strong></p></body></html>`
	id := f.createArquivo(t, content, "text/html; charset=utf-8")

	if err := f.service.ExtractConteudo(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	row := f.getConteudo(t, id, MetodoHTMLMarkdown)
	if row.Formato != FormatoMarkdown {
		t.Fatalf("want formato %q, got %q", FormatoMarkdown, row.Formato)
	}
	if !strings.Contains(row.Conteudo, "# Título") {
		t.Fatalf("want conteudo with %q, got %q", "# Título", row.Conteudo)
	}
	if !strings.Contains(row.Conteudo, "**mundo**") {
		t.Fatalf("want conteudo with %q, got %q", "**mundo**", row.Conteudo)
	}
	if f.ocr.analyzeCalls != 0 {
		t.Fatalf("want 0 analyze calls, got %d", f.ocr.analyzeCalls)
	}
}

func TestExtractConteudo_Idempotent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	id := f.createArquivo(t, "%PDF-1.4 conteúdo binário", "application/pdf")

	if err := f.service.ExtractConteudo(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	// A segunda extração não deve reprocessar nem chamar o OCR novamente.
	if err := f.service.ExtractConteudo(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	if f.ocr.analyzeCalls != 1 {
		t.Fatalf("want 1 analyze call, got %d", f.ocr.analyzeCalls)
	}
}

func TestExtractConteudo_NotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	err := f.service.ExtractConteudo(t.Context(), 123456789)
	if !errors.Is(err, arquivo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
