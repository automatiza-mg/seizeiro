package main

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	api := humago.New(mux, huma.DefaultConfig("SEIzeiro", "0.1.0"))
	registerCreateChatbotCadastro(api, "/api/v1", app.cfg.BaseURL, app.chatbotauth)

	mux.HandleFunc("GET /cadastro", app.handleCadastro)
	mux.HandleFunc("POST /cadastro", app.handleCadastroPost)
	mux.HandleFunc("GET /cadastro/sucesso", app.handleCadastroSucesso)
	mux.HandleFunc("GET /cadastro/invalido", app.handleCadastroTokenInvalido)

	return mux
}
