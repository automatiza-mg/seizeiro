package conteudo

import (
	"errors"
	"strings"
	"testing"

	"github.com/automatiza-mg/seizeiro/internal/arquivo"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
)

func TestSplitText(t *testing.T) {
	t.Parallel()

	t.Run("texto longo gera múltiplos chunks", func(t *testing.T) {
		t.Parallel()

		text := strings.Repeat("parágrafo de exemplo. ", 200)
		chunks, err := splitText(text)
		if err != nil {
			t.Fatal(err)
		}
		if len(chunks) < 2 {
			t.Fatalf("want múltiplos chunks, got %d", len(chunks))
		}
		for i, c := range chunks {
			if c == "" {
				t.Fatalf("chunk %d vazio", i)
			}
		}
	})

	t.Run("texto curto gera um chunk", func(t *testing.T) {
		t.Parallel()

		chunks, err := splitText("texto curto")
		if err != nil {
			t.Fatal(err)
		}
		if len(chunks) != 1 {
			t.Fatalf("want 1 chunk, got %d", len(chunks))
		}
	})
}

// extractAndGetConteudoID extrai o conteúdo de um arquivo e retorna o ID do
// conteúdo persistido.
func (f *fixture) extractAndGetConteudoID(tb testing.TB, content, contentType string) int64 {
	tb.Helper()

	arquivoID := f.createArquivo(tb, content, contentType)
	if err := f.service.ExtractConteudo(tb.Context(), arquivoID); err != nil {
		tb.Fatal(err)
	}

	var metodo string
	switch contentType {
	case "text/plain":
		metodo = MetodoPlain
	default:
		metodo = MetodoOCR
	}
	return f.getConteudo(tb, arquivoID, metodo).ID
}

func TestExtractConteudo_EnqueuesChunkTask(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	conteudoID := f.extractAndGetConteudoID(t, "texto puro", "text/plain")

	job := rivertest.RequireInserted(t.Context(), t, riverpgxv5.New(f.pool), arquivo.ChunkArgs{}, nil)
	if job.Args.ConteudoID != conteudoID {
		t.Fatalf("want ConteudoID %d, got %d", conteudoID, job.Args.ConteudoID)
	}
}

func TestChunkConteudo(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	conteudoID := f.extractAndGetConteudoID(t, "texto puro para chunking", "text/plain")

	if err := f.service.ChunkConteudo(t.Context(), conteudoID); err != nil {
		t.Fatal(err)
	}

	count, err := f.service.q.CountArquivoConteudoChunks(t.Context(), conteudoID)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("want chunks persistidos, got 0")
	}
	if f.embedder.calls != 1 {
		t.Fatalf("want 1 embed call, got %d", f.embedder.calls)
	}
}

func TestChunkConteudo_Idempotent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	conteudoID := f.extractAndGetConteudoID(t, "texto puro para chunking", "text/plain")

	if err := f.service.ChunkConteudo(t.Context(), conteudoID); err != nil {
		t.Fatal(err)
	}
	// A segunda chamada não deve reprocessar nem chamar o embedder novamente.
	if err := f.service.ChunkConteudo(t.Context(), conteudoID); err != nil {
		t.Fatal(err)
	}

	if f.embedder.calls != 1 {
		t.Fatalf("want 1 embed call, got %d", f.embedder.calls)
	}
}

func TestChunkConteudo_NotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	err := f.service.ChunkConteudo(t.Context(), 123456789)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrConteudoNotFound, got %v", err)
	}
}
