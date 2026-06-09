package docintel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/automatiza-mg/seizeiro/internal/poller"
)

// AnalyzeDocument inicia a análise de um documento (extraindo texto em formato markdown) a partir de um
// [io.Reader] usando a API da Azure Document Intelligence.
//
// Retorna o local da operação para ser consultado usando GetAnalyzeResult.
func (c *Client) AnalyzeDocument(ctx context.Context, r io.Reader, contentType string) (string, error) {
	q := make(url.Values)
	q.Set("locale", "pt-BR")
	q.Set("api-version", apiVersion)
	q.Set("outputContentFormat", "markdown")

	endpoint := strings.TrimSuffix(c.endpoint, "/")
	url := fmt.Sprintf("%s/documentintelligence/documentModels/%s:analyze?%s", endpoint, modelID, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, r)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)

	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// Lê o corpo da requisição e retorna o erro caso status seja diferente de 202.
	if res.StatusCode != http.StatusAccepted {
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return "", err
		}
		return "", &StatusError{StatusCode: res.StatusCode, Body: string(b)}
	}

	operationLocation := res.Header.Get("Operation-Location")
	return operationLocation, nil
}

// GetAnalyzeResult retorna o status e, quando concluído, o resultado da análise do documento.
func (c *Client) GetAnalyzeResult(ctx context.Context, location string) (*AnalyzeOperation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		var op AnalyzeOperation
		err := json.NewDecoder(res.Body).Decode(&op)
		if err != nil {
			return nil, err
		}
		return &op, nil
	default:
		b, _ := io.ReadAll(res.Body)
		return nil, &StatusError{StatusCode: res.StatusCode, Body: string(b)}
	}
}

// TODO: Mover método para serviço de análise de documentos.
func (c *Client) PollResult(ctx context.Context, location string) (string, error) {
	p := poller.New(2*time.Second, 5*time.Minute, func(ctx context.Context) poller.Result[string] {
		op, err := c.GetAnalyzeResult(ctx, location)
		if err != nil {
			statusErr, ok := errors.AsType[*StatusError](err)
			if ok && statusErr.Retryable() {
				return poller.Result[string]{Done: false}
			}
			return poller.Result[string]{Done: true, Err: err}
		}

		switch op.Status {
		case StatusSucceeded:
			return poller.Result[string]{Done: true, Value: op.AnalyzeResult.Content}
		case StatusFailed, StatusCanceled, StatusSkipped:
			return poller.Result[string]{Done: true, Err: &AnalyzeError{Status: op.Status, Err: op.Error}}
		case StatusRunning, StatusNotStarted:
			return poller.Result[string]{Done: false}
		default:
			return poller.Result[string]{Done: true, Err: fmt.Errorf("unexpected status: %s", op.Status)}
		}
	})

	text, err := p.Poll(ctx)
	if err != nil {
		return "", err
	}
	return text, nil
}
