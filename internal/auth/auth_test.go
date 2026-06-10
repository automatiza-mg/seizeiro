package auth

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/automatiza-mg/seizeiro/internal/database"
	"github.com/google/go-cmp/cmp"
)

var ti *database.TestInstance

func newTestService(tb testing.TB) *Service {
	tb.Helper()
	pool := ti.NewPool(tb)
	return NewService(pool)
}

func TestMain(m *testing.M) {
	ti = database.MustTestInstance()
	code := m.Run()

	if err := ti.Close(context.Background()); err != nil {
		log.Fatal(err)
	}

	os.Exit(code)
}

func TestLogin(t *testing.T) {
	t.Parallel()
	service := newTestService(t)

	usuario, err := service.CreateUsuario(t.Context(), CreateUsuarioParams{
		Nome:  "Fulano da Silva",
		CPF:   "123.456.789-09",
		Email: "fulano.silva@planejamento.mg.gov.br",
		Senha: "Abc123123",
	})
	if err != nil {
		t.Fatal(err)
	}

	principal, err := service.Login(t.Context(), LoginParams{
		CPF:   "123.456.789-09",
		Senha: "Abc123123",
	})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(usuario, principal.Usuario); diff != "" {
		t.Fatalf("usuario mismatch:\n%s", diff)
	}
	if principal.Token == nil {
		t.Fatal("expected token, got nil")
	}
	if principal.Token.PlainText == "" {
		t.Fatal("expected non-empty token")
	}

	// O token emitido deve ser válido e pertencer ao usuário autenticado.
	owner, err := service.GetTokenOwner(t.Context(), principal.Token.PlainText, EscopoAuth)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(usuario, owner); diff != "" {
		t.Fatalf("token owner mismatch:\n%s", diff)
	}
}

func TestLogin_InvalidCPF(t *testing.T) {
	t.Parallel()
	service := newTestService(t)

	_, err := service.Login(t.Context(), LoginParams{
		CPF:   "000.000.000-00",
		Senha: "Abc123123",
	})
	if !errors.Is(err, ErrInvalidCPF) {
		t.Fatalf("expected ErrInvalidCPF, got %v", err)
	}
}

func TestLogin_InvalidCredentials_UnknownCPF(t *testing.T) {
	t.Parallel()
	service := newTestService(t)

	// CPF válido, porém não cadastrado.
	_, err := service.Login(t.Context(), LoginParams{
		CPF:   "529.988.310-28",
		Senha: "Abc123123",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_InvalidCredentials_WrongPassword(t *testing.T) {
	t.Parallel()
	service := newTestService(t)

	_, err := service.CreateUsuario(t.Context(), CreateUsuarioParams{
		Nome:  "Fulano da Silva",
		CPF:   "123.456.789-09",
		Email: "fulano.silva@planejamento.mg.gov.br",
		Senha: "Abc123123",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Login(t.Context(), LoginParams{
		CPF:   "123.456.789-09",
		Senha: "SenhaErrada123",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_ErrNoSenha(t *testing.T) {
	t.Parallel()
	service := newTestService(t)

	// Usuário criado sem senha cadastrada.
	_, err := service.CreateUsuario(t.Context(), CreateUsuarioParams{
		Nome:  "Fulano da Silva",
		CPF:   "123.456.789-09",
		Email: "fulano.silva@planejamento.mg.gov.br",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Login(t.Context(), LoginParams{
		CPF:   "123.456.789-09",
		Senha: "Abc123123",
	})
	if !errors.Is(err, ErrNoSenha) {
		t.Fatalf("expected ErrNoSenha, got %v", err)
	}
}
