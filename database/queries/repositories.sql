-- name: AddRepository :exec
INSERT INTO repositories (name, branch, updated_at, indexed_at, had_error)
VALUES (?, ?, ?, 0, FALSE);

-- name: UpdateRepository :exec
UPDATE repositories
SET branch=?,
    updated_at=?
WHERE name = ?;

-- name: RemoveRepository :exec
DELETE
FROM repositories
WHERE id = ?;

-- name: GetNonProcessedRepo :one
SELECT *
FROM repositories
WHERE indexed_at < updated_at
LIMIT 1;

-- name: UpdateIndexedAt :exec
UPDATE repositories
SET indexed_at = ?
WHERE id = ?;

-- name: AddIndexedFile :execlastid
INSERT INTO files_index (repository_id, path, lines)
VALUES (?, ?, ?);

-- name: RandomIndexedFile :one
SELECT *
FROM files_index
WHERE lines > ?
ORDER BY random()
LIMIT 1;
