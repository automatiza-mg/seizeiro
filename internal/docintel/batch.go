package docintel

import (
	"bytes"
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

// AzureBlobSource especifica um container (opcionalmente filtrado por prefixo) cujos
// documentos serão processados em lote.
type AzureBlobSource struct {
	ContainerURL string `json:"containerUrl"`
	Prefix       string `json:"prefix,omitempty"`
}

// AzureBlobFileListSource especifica documentos a serem processados em lote por meio de
// um arquivo JSONL armazenado na raiz do container.
type AzureBlobFileListSource struct {
	ContainerURL string `json:"containerUrl"`
	FileList     string `json:"fileList"`
}

// AnalyzeBatchParams agrupa os parâmetros para iniciar uma análise em lote.
//
// Exatamente uma fonte deve ser informada: [AzureBlobSource] (todos os documentos de um
// container ou prefixo) ou [AzureBlobFileListSource] (documentos específicos listados em
// um arquivo JSONL). Caso ambas ou nenhuma sejam informadas, [AnalyzeBatch] retorna
// [ErrInvalidBatchRequest].
type AnalyzeBatchParams struct {
	AzureBlobSource         *AzureBlobSource         `json:"azureBlobSource,omitempty"`
	AzureBlobFileListSource *AzureBlobFileListSource `json:"azureBlobFileListSource,omitempty"`

	ResultContainerURL string `json:"resultContainerUrl"`
	ResultPrefix       string `json:"resultPrefix,omitempty"`
	OverwriteExisting  bool   `json:"overwriteExisting,omitempty"`
}

// Valida os parâmetros de forma defensiva, retornando [ErrInvalidBatchRequest]
// com contexto quando alguma condição não é satisfeita.
func (p AnalyzeBatchParams) validate() error {
	switch {
	case p.AzureBlobSource == nil && p.AzureBlobFileListSource == nil:
		return fmt.Errorf("%w: a source is required", ErrInvalidBatchRequest)
	case p.AzureBlobSource != nil && p.AzureBlobFileListSource != nil:
		return fmt.Errorf("%w: only one source is allowed", ErrInvalidBatchRequest)
	}

	if p.AzureBlobSource != nil && p.AzureBlobSource.ContainerURL == "" {
		return fmt.Errorf("%w: azureBlobSource.containerUrl is required", ErrInvalidBatchRequest)
	}
	if p.AzureBlobFileListSource != nil {
		if p.AzureBlobFileListSource.ContainerURL == "" {
			return fmt.Errorf("%w: azureBlobFileListSource.containerUrl is required", ErrInvalidBatchRequest)
		}
		if p.AzureBlobFileListSource.FileList == "" {
			return fmt.Errorf("%w: azureBlobFileListSource.fileList is required", ErrInvalidBatchRequest)
		}
	}

	if p.ResultContainerURL == "" {
		return fmt.Errorf("%w: resultContainerUrl is required", ErrInvalidBatchRequest)
	}

	return nil
}

// AnalyzeBatch inicia a análise em lote de documentos armazenados no Azure Blob Storage,
// extraindo texto em formato markdown usando a API da Azure Document Intelligence.
//
// Os documentos não são enviados diretamente: a fonte e o destino dos resultados são
// containers do Blob Storage informados em params. Os resultados de cada documento são
// gravados como arquivos no container de destino e não fazem parte da resposta.
//
// Retorna o local da operação para ser consultado usando [Client.GetBatchResult].
// Retorna [ErrInvalidBatchRequest] caso os parâmetros sejam inválidos.
func (c *Client) AnalyzeBatch(ctx context.Context, params AnalyzeBatchParams) (string, error) {
	if err := params.validate(); err != nil {
		return "", err
	}

	body, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("marshal batch request: %w", err)
	}

	q := make(url.Values)
	q.Set("api-version", apiVersion)
	q.Set("outputContentFormat", "markdown")

	endpoint := strings.TrimSuffix(c.endpoint, "/")
	url := fmt.Sprintf("%s/documentintelligence/documentModels/%s:analyzeBatch?%s", endpoint, modelID, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

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

// GetBatchResult retorna o status e, quando concluído, o resultado da análise em lote.
func (c *Client) GetBatchResult(ctx context.Context, location string) (*BatchAnalyzeOperation, error) {
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
		var op BatchAnalyzeOperation
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
func (c *Client) PollBatchResult(ctx context.Context, location string) (*BatchAnalyzeOperation, error) {
	// TODO: Remover este timeout estendido quando o polling for movido para o serviço de
	// análise de documentos, que deve controlar o tempo limite de forma adequada.
	p := poller.New(2*time.Second, 30*time.Minute, func(ctx context.Context) poller.Result[*BatchAnalyzeOperation] {
		op, err := c.GetBatchResult(ctx, location)
		if err != nil {
			statusErr, ok := errors.AsType[*StatusError](err)
			if ok && statusErr.Retryable() {
				return poller.Result[*BatchAnalyzeOperation]{Done: false}
			}
			return poller.Result[*BatchAnalyzeOperation]{Done: true, Err: err}
		}

		switch op.Status {
		case StatusCompleted, StatusSucceeded:
			return poller.Result[*BatchAnalyzeOperation]{Done: true, Value: op}
		case StatusFailed, StatusCanceled, StatusSkipped:
			return poller.Result[*BatchAnalyzeOperation]{Done: true, Err: &AnalyzeError{Status: op.Status, Err: op.Error}}
		case StatusRunning, StatusNotStarted:
			return poller.Result[*BatchAnalyzeOperation]{Done: false}
		default:
			return poller.Result[*BatchAnalyzeOperation]{Done: true, Err: fmt.Errorf("unexpected status: %s", op.Status)}
		}
	})

	op, err := p.Poll(ctx)
	if err != nil {
		return nil, err
	}
	return op, nil
}
