package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// https://www.postgresql.org/docs/current/errcodes-appendix.html
	uniqueViolationCode = "23505"
)

// IsUniqueError verifica se o erro informado é uma unique_violation para a constraint informada.
//
// O nome da constraint gerada pelo PostgreSQL segue o seguinte padrão: {tabela}_{coluna}_key.
//
//	ok := IsUniqueError(err, "usuarios_cpf_key") // Tabela: "usuarios" Coluna: "cpf".
func IsUniqueError(err error, constraintName string) bool {
	pgError, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return false
	}
	return pgError.Code == uniqueViolationCode && pgError.ConstraintName == constraintName
}
