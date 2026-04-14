-- name: GetSecretByPeekId :one
SELECT *
FROM secrets
WHERE peek_id = ?
LIMIT 1;

-- name: GetSecretByEditId :one
SELECT *
FROM secrets
WHERE edit_id = ?
LIMIT 1;

-- name: InsertSecret :one
INSERT INTO secrets (edit_id, peek_id, title, content, password, expire_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateSecretByEditId :one
UPDATE secrets
SET title = ?, content = ?
WHERE edit_id = ?
RETURNING *;