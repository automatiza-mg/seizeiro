package docintel

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAnalyzeDocument_Accepted(t *testing.T) {
	t.Parallel()

	const wantLocation = "https://example.com/documentintelligence/documentModels/prebuilt-layout/analyzeResults/123?api-version=2024-11-30"

	var gotBody string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = string(b)
		w.Header().Set("Operation-Location", wantLocation)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-key")

	location, err := client.AnalyzeDocument(t.Context(), strings.NewReader("document-bytes"), "application/pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if location != wantLocation {
		t.Fatalf("location = %q, want %q", location, wantLocation)
	}
	if gotContentType != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", gotContentType)
	}
	if gotBody != "document-bytes" {
		t.Fatalf("body = %q, want %q", gotBody, "document-bytes")
	}
}

func TestAnalyzeDocument_UnexpectedStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "bad request")
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-key")

	_, err := client.AnalyzeDocument(t.Context(), strings.NewReader("document-bytes"), "application/pdf")
	statusErr, ok := errors.AsType[*StatusError](err)
	if !ok {
		t.Fatalf("error = %v, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", statusErr.StatusCode, http.StatusBadRequest)
	}
}

func TestGetAnalyzeResult_Succeeded(t *testing.T) {
	t.Parallel()

	const payload = `{
		"status": "succeeded",
		"createdDateTime": "2025-01-01T00:00:00Z",
		"lastUpdatedDateTime": "2025-01-01T00:01:00Z",
		"analyzeResult": {
			"apiVersion": "2024-11-30",
			"modelId": "prebuilt-layout",
			"content": "# Título\n\nConteúdo extraído.",
			"contentFormat": "markdown"
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, payload)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-key")

	op, err := client.GetAnalyzeResult(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if op.Status != StatusSucceeded {
		t.Fatalf("Status = %q, want %q", op.Status, StatusSucceeded)
	}

	want := AnalyzeResult{
		APIVersion:    "2024-11-30",
		ModelID:       "prebuilt-layout",
		Content:       "# Título\n\nConteúdo extraído.",
		ContentFormat: "markdown",
	}
	if diff := cmp.Diff(want, op.AnalyzeResult); diff != "" {
		t.Fatalf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestGetAnalyzeResult_UnexpectedStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-key")

	_, err := client.GetAnalyzeResult(t.Context(), srv.URL)
	statusErr, ok := errors.AsType[*StatusError](err)
	if !ok {
		t.Fatalf("error = %v, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want %d", statusErr.StatusCode, http.StatusNotFound)
	}
}
