package chatbot

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/automatiza-mg/seizeiro/internal/auth"
	"github.com/automatiza-mg/seizeiro/internal/postgres"
	"github.com/automatiza-mg/seizeiro/internal/security"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("usuario not found")
)

type Token struct {
	PlainText    string    `json:"token,omitempty"`
	Plataforma   string    `json:"plataforma"`
	PlataformaID string    `json:"plataforma_id"`
	ExpiraEm     time.Time `json:"expira_em"`
}

type Usuario struct {
	Plataforma   string    `json:"plataforma"`
	PlataformaID string    `json:"plataforma_id"`
	SEIUsuario   string    `json:"sei_usuario"`
	SEISenha     string    `json:"sei_senha"`
	CriadoEm     time.Time `json:"criado_em"`
}

type Service struct {
	pool   *pgxpool.Pool
	q      *postgres.Queries
	encKey []byte
}

func NewService(pool *pgxpool.Pool, encKey []byte) (*Service, error) {
	if len(encKey) != 32 {
		return nil, fmt.Errorf("key must have 32 bytes, got: %d", len(encKey))
	}

	return &Service{
		pool:   pool,
		q:      postgres.New(pool),
		encKey: encKey,
	}, nil
}

// CreateToken cria um novo token de registro para um usuário do chatbot. Tokens anteriores do mesmo
// usuário são invalidados, e tokens expirados de qualquer usuário são removidos de forma oportunista.
func (s *Service) CreateToken(ctx context.Context, plataforma, plataformaID string) (*Token, error) {
	b := security.RandomBytes(32)
	plainText := base64.RawURLEncoding.EncodeToString(b)
	expiraEm := time.Now().Add(12 * time.Hour)
	hash := sha256.Sum256([]byte(plainText))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.q.WithTx(tx)

	err = q.DeleteTokensChatbot(ctx, postgres.DeleteTokensChatbotParams{
		Plataforma:   plataforma,
		PlataformaID: plataformaID,
	})
	if err != nil {
		return nil, fmt.Errorf("delete tokens chatbot: %w", err)
	}

	err = q.SaveTokenChatbot(ctx, postgres.SaveTokenChatbotParams{
		Hash:         hash[:],
		Plataforma:   plataforma,
		PlataformaID: plataformaID,
		ExpiraEm:     pgtype.Timestamptz{Time: expiraEm, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("save token chatbot: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &Token{
		PlainText:    plainText,
		Plataforma:   plataforma,
		PlataformaID: plataformaID,
		ExpiraEm:     expiraEm,
	}, nil
}

type CreateUsuarioParams struct {
	Token      string
	SEIUsuario string
	SEISenha   string
}

// CreateUsuario registra as credenciais SEI de um usuário do chatbot a partir de um token criado por
// [Service.CreateToken]. Caso o usuário já exista, suas credenciais são atualizadas. O token é de uso
// único e é consumido após o registro. Retorna [auth.ErrInvalidToken] caso o token seja inválido ou
// expirado.
func (s *Service) CreateUsuario(ctx context.Context, params CreateUsuarioParams) error {
	hash := sha256.Sum256([]byte(params.Token))

	tokenRow, err := s.q.GetTokenChatbot(ctx, hash[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrInvalidToken
		}
		return fmt.Errorf("get token chatbot: %w", err)
	}

	senha, err := s.encrypt([]byte(params.SEISenha))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.q.WithTx(tx)

	err = q.SaveUsuarioChatbot(ctx, postgres.SaveUsuarioChatbotParams{
		Plataforma:   tokenRow.Plataforma,
		PlataformaID: tokenRow.PlataformaID,
		SEIUsuario:   params.SEIUsuario,
		SEISenha:     senha,
	})
	if err != nil {
		return fmt.Errorf("save usuario chatbot: %w", err)
	}

	if err := q.DeleteTokenChatbot(ctx, hash[:]); err != nil {
		return fmt.Errorf("delete token chatbot: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (s *Service) GetUsuario(ctx context.Context, plataforma, plataformaID string) (*Usuario, error) {
	row, err := s.q.GetUsuarioChatbot(ctx, postgres.GetUsuarioChatbotParams{
		Plataforma:   plataforma,
		PlataformaID: plataformaID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	senha, err := s.decrypt(row.SEISenha)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return &Usuario{
		Plataforma:   row.Plataforma,
		PlataformaID: row.PlataformaID,
		SEIUsuario:   row.SEIUsuario,
		SEISenha:     string(senha),
		CriadoEm:     row.CriadoEm.Time,
	}, nil
}

type TokenData struct {
	Plataforma   string `json:"plataforma"`
	PlataformaID string `json:"plataforma_id"`
}

// GetTokenData retorna os dados de cadastro de um token.
// Se o token for inválido, retorna [auth.ErrInvalidToken].
func (s *Service) GetTokenData(ctx context.Context, token string) (*TokenData, error) {
	hash := sha256.Sum256([]byte(token))
	row, err := s.q.GetTokenChatbot(ctx, hash[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrInvalidToken
		}
		return nil, err
	}

	return &TokenData{
		Plataforma:   row.Plataforma,
		PlataformaID: row.PlataformaID,
	}, nil
}
