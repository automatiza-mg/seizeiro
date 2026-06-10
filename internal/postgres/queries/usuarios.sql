-- SaveUsuario salva um novo usuário no banco de dados.
-- name: SaveUsuario :one
INSERT INTO usuarios (nome, cpf, email, email_verificado, hash_senha)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- GetUsuario busca um usuário pelo ID informado.
-- name: GetUsuario :one
SELECT * FROM usuarios WHERE id = $1;

-- GetUsuarioByCPF busca um usuário pelo CPF informado.
-- name: GetUsuarioByCPF :one
SELECT * FROM usuarios WHERE cpf = $1;

-- UpdateSenhaUsuario atualiza o hash da senha de um usuário.
-- name: UpdateSenhaUsuario :exec
UPDATE usuarios SET
    hash_senha = $2,
    atualizado_em = CURRENT_TIMESTAMP
WHERE id = $1;
