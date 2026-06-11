package llm

import (
	"fmt"

	"github.com/pkoukk/tiktoken-go"
	tiktokenloader "github.com/pkoukk/tiktoken-go-loader"
)

// Encoding usado pelos modelos de embedding mais recentes (text-embedding-3-*),
// que nem sempre constam no mapa de modelos do tiktoken.
const defaultEncoding = "cl100k_base"

func init() {
	// Usa o loader offline para evitar o download do dicionário BPE em runtime.
	tiktoken.SetBpeLoader(tiktokenloader.NewOfflineLoader())
}

// TokenCounter conta os tokens de um texto de acordo com o tokenizer de um modelo.
type TokenCounter struct {
	enc *tiktoken.Tiktoken
}

// NewTokenCounter cria um [TokenCounter] para o modelo informado. Quando o
// modelo não é reconhecido, usa o encoding padrão ([defaultEncoding]).
func NewTokenCounter(model string) (*TokenCounter, error) {
	enc, err := tiktoken.EncodingForModel(model)
	if err != nil {
		// Modelos novos podem não estar no mapa do tiktoken; recorre ao
		// encoding padrão dos modelos de embedding.
		enc, err = tiktoken.GetEncoding(defaultEncoding)
		if err != nil {
			return nil, fmt.Errorf("get encoding %q: %w", defaultEncoding, err)
		}
	}
	return &TokenCounter{enc: enc}, nil
}

// Count retorna a quantidade de tokens em text.
func (c *TokenCounter) Count(text string) int {
	return len(c.enc.Encode(text, nil, nil))
}
