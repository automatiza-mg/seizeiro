package database

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		constraintName string
		want           bool
	}{
		{
			name:           "unique violation com constraint correta",
			err:            &pgconn.PgError{Code: uniqueViolationCode, ConstraintName: "usuarios_cpf_key"},
			constraintName: "usuarios_cpf_key",
			want:           true,
		},
		{
			name:           "unique violation com constraint diferente",
			err:            &pgconn.PgError{Code: uniqueViolationCode, ConstraintName: "usuarios_email_key"},
			constraintName: "usuarios_cpf_key",
			want:           false,
		},
		{
			name:           "código de erro diferente",
			err:            &pgconn.PgError{Code: "23503", ConstraintName: "usuarios_cpf_key"},
			constraintName: "usuarios_cpf_key",
			want:           false,
		},
		{
			name:           "erro nil",
			err:            nil,
			constraintName: "usuarios_cpf_key",
			want:           false,
		},
		{
			name:           "erro que não é PgError",
			err:            errors.New("erro genérico"),
			constraintName: "usuarios_cpf_key",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUniqueError(tt.err, tt.constraintName)

			if got != tt.want {
				t.Fatalf("IsUniqueError() = %v, want %v", got, tt.want)
			}
		})
	}
}
