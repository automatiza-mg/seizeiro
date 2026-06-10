# Contribuindo

## Convenções de Código

### Idioma

- Identificadores e mensagens de erro são escritos em inglês.
- Substantivos do domínio permanecem em português (`Usuario`, `Senha`, `Escopo`).
- Comentários e Godoc são escritos em português.

### Lidando com Erros

- Sempre adicione contexto aos erros: `fmt.Errorf("descricao: %w")`.
- Compare erros com `errors.Is`, `errors.AsType` ou `errors.As`.
- Valide todos os inputs de forma defensiva.
- Declare erros sentinela como `var ErrXxx = errors.New(...)` no pacote.
- Use structs que implementam `Error()` quando o erro precisa carregar dados.
- Traduza erros do banco para erros do domínio (ex: `pgx.ErrNoRows` para
  `ErrUsuarioNotFound`).

### Construtores e Injeção de Dependência

- Construtores se chamam `New` ou `New<Tipo>` e retornam um ponteiro.
- Variantes que entram em pânico ou falham em fatal usam o prefixo `Must`.
- Parâmetros agrupados usam structs nomeadas `<Verbo><Substantivo>Params`.

## Configuração

A aplicação é configurada por variáveis de ambiente, carregadas a partir de um
arquivo `.env` na raiz do projeto. As variáveis disponíveis estão documentadas
no arquivo `.env.example`.

- As configurações são definidas na struct `Config` em
  `internal/config/config.go` e lidas via tag `env`.
- Use `,notEmpty` na tag `env` para marcar uma variável como obrigatória.
- Agrupe configurações relacionadas em structs próprias (ex:
  `DocumentIntelligence`) embutidas em `Config`.
- Nomeie as variáveis de ambiente em `SCREAMING_SNAKE_CASE`, prefixadas pelo
  serviço quando aplicável (ex: `AZURE_DOCINTEL_KEY`).
- Toda nova variável deve ser adicionada ao arquivo `.env.example`.

## Banco de Dados

### Schema

A gestão do schema do banco de dados é feita através de migrações
usando a ferramente [goose](https://pressly.github.io/goose/). A ferramenta
está disponível no projeto como uma [go tool](https://pkg.go.dev/cmd/go#hdr-Run_specified_go_tool).

Para criar uma nova migração, execute o comando abaixo:

```bash
go tool goose -dir internal/postgres/migrations create <nome> sql
```

Para aplicar as migrações ao banco de dados, execute o comando abaixo:

```bash
go tool goose -dir internal/postgres/migrations postgres <database_url> up
```

### Queries

As queries do banco de dados são compiladas de arquivos `.sql` pela ferramenta
[sqlc](https://sqlc.dev/). Para compilar novas queries, execute o comando abaixo:

```bash
go tool sqlc generate # ou make sql/build
```

Consulte a [documentação](https://docs.sqlc.dev/en/stable/tutorials/getting-started-postgresql.html) do
sqlc se precisar adicionar novas queries.

- Edite os arquivos `.sql` em `internal/postgres/queries`, nunca os `.sql.go`
  gerados.
- Use os verbos `Save` (insert), `Get` (select) e `Update<Campo>` (update) ao
  nomear queries.

## Testes

- Use apenas a `testing` da biblioteca padrão e `github.com/google/go-cmp/cmp`
  para comparar structs.
- Chame `t.Parallel()` no início de cada teste e subteste sempre que possível.
- Testes orientados a tabela usam um slice `tests` de structs com o campo `name`
  iterado com `t.Run(tt.name, ...)`.
- Use `testing/synctest` para testar lógica dependente de tempo.
- Use `t.Context()` em vez de `context.Background()`.
- Helpers recebem `testing.TB` e chamam `tb.Helper()`.
- Testes de banco usam `TestMain` com `dockertest` e podem ser pulados com
  `-short` ou a variável de ambiente `SKIP_DATABASE_TESTS`.

## Documentação

### Godoc

Todos os tipos e funções exportadas têm um comentário Godoc começando com o nome
do identificador e escrito em português:

```go
// WeakPasswordError é o erro retornado quando a senha informada por um usuário é fraca.
type WeakPasswordError struct {
  Violations []string
}

// Description retorna uma descrição amigável para o usuário final com os erros.
func (w *WeakPasswordError) Description() string {
  // ...
}
```

Cada pacote tem um comentário de doc no arquivo principal, iniciado por
`Package <nome>`:

```go
// Package poller fornece um utilitário genérico para executar uma função
// repetidamente em intervalos regulares até que ela sinalize conclusão ou que
// um tempo limite seja atingido.
package poller
```

As queries do banco também são documentadas: o comentário em português acima da
anotação `-- name:` é levado para o Godoc do método gerado pelo sqlc.

### Comentários

- Escreva os comentários em português.
- Explique _porque_, nunca _o que_.
- Prefira deixar o código claro a comentá-lo; comente apenas o que não é óbvio.
- Use comentários inline para justificar decisões não evidentes.
- Referencie identificadores com colchetes para que o Godoc gere os links:
  `[ValidatePassword]`, `[*WeakPasswordError]`, `[error]`.
- Documente sentinelas, panics e condições de erro relevantes (ex: "Retorna
  [ErrInvalidCPF] caso o CPF seja inválido.").
- Encerre os comentários com ponto final.
- Nunca divida seções com números (`// --- 1. Autenticação ---`).
