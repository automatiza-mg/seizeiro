package blob

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func newTestStorage(t *testing.T) *FilesystemStorage {
	t.Helper()
	s, err := NewFilesystemStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFilesystemStorage_Put(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)

	const content = "conteúdo do arquivo"
	// Usa uma key aninhada para exercitar a criação de diretórios intermediários.
	if err := s.Put(t.Context(), "dir/sub/file.txt", "text/plain", strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	rc, err := s.Get(t.Context(), "dir/sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("want %q, got %q", content, string(got))
	}
}

func TestFilesystemStorage_Get(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)

	const content = "olá mundo"
	if err := s.Put(t.Context(), "file.txt", "text/plain", strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}

	rc, err := s.Get(t.Context(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("want %q, got %q", content, string(got))
	}

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		_, err := s.Get(t.Context(), "inexistente.txt")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestFilesystemStorage_Delete(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)

	if err := s.Put(t.Context(), "file.txt", "text/plain", strings.NewReader("dados")); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(t.Context(), "file.txt"); err != nil {
		t.Fatal(err)
	}

	// Após o delete, a key não deve mais existir.
	if _, err := s.Get(t.Context(), "file.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	t.Run("idempotent", func(t *testing.T) {
		t.Parallel()

		// Delete de uma key inexistente deve retornar nil.
		if err := s.Delete(t.Context(), "inexistente.txt"); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}
