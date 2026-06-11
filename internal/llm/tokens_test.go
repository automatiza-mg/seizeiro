package llm

import "testing"

func TestTokenCounter_Count(t *testing.T) {
	t.Parallel()

	counter, err := NewTokenCounter("text-embedding-3-small")
	if err != nil {
		t.Fatalf("new token counter: %v", err)
	}

	if got := counter.Count(""); got != 0 {
		t.Fatalf("Count(\"\") = %d, want 0", got)
	}

	if got := counter.Count("hello world"); got <= 0 {
		t.Fatalf("Count(\"hello world\") = %d, want positive", got)
	}

	// Mais texto deve resultar em mais tokens.
	short := counter.Count("hello")
	long := counter.Count("hello world, this is a longer piece of text")
	if long <= short {
		t.Fatalf("expected long (%d) > short (%d)", long, short)
	}
}

func TestNewTokenCounter_UnknownModelFallsBack(t *testing.T) {
	t.Parallel()

	// Um modelo desconhecido recorre ao encoding padrão em vez de falhar.
	counter, err := NewTokenCounter("modelo-inexistente")
	if err != nil {
		t.Fatalf("expected fallback, got error: %v", err)
	}
	if got := counter.Count("hello world"); got <= 0 {
		t.Fatalf("Count = %d, want positive", got)
	}
}
