package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/automatiza-mg/seizeiro/internal/postgres"
	"github.com/automatiza-mg/seizeiro/internal/security"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool *pgxpool.Pool
	q    *postgres.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
		q:    postgres.New(pool),
	}
}

type LoginParams struct {
	CPF   string
	Senha string
}

// Principal representa um usuário autenticado junto com o token emitido para a sessão.
type Principal struct {
	Usuario *Usuario
	Token   *Token
}

// Login autentica as credenciais de um usuário, retornando seus dados e um token de autenticação.
//
// Caso as credenciais informadas sejam inválidas (CPF ou Senha), retorna [ErrInvalidCredentials].
// Se o CPF informado for inválido, retorna [ErrInvalidCPF].
// Se o usuário ainda não possuir uma senha cadastrada, retorna [ErrNoSenha].
func (s *Service) Login(ctx context.Context, params LoginParams) (*Principal, error) {
	cpf := normalizeCPF(params.CPF)
	if err := ValidateCPF(cpf); err != nil {
		return nil, fmt.Errorf("validate cpf: %w", err)
	}

	row, err := s.q.GetUsuarioByCPF(ctx, cpf)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get usuario by cpf: %w", err)
	}

	// Usuário ainda não possui uma senha cadastrada
	if !row.HashSenha.Valid {
		return nil, ErrNoSenha
	}

	ok, err := security.VerifyPassword(row.HashSenha.String, params.Senha)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	usuario := usuarioFromDB(row)

	token, err := s.CreateToken(ctx, CreateTokenParams{
		UsuarioID: usuario.ID,
		Escopo:    EscopoAuth,
		TTL:       24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}

	return &Principal{
		Usuario: &usuario,
		Token:   token,
	}, nil
}
