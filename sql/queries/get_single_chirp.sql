-- name: GetOnehirp :one
SELECT * 
FROM chirps
WHERE id = $1;
