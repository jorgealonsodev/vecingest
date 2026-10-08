-- offices: id is itself the tenant root (design D-5).
-- name: InsertOffice :one
INSERT INTO offices (id, name, cif, email, phone, address, logo_key, collegiate_number)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetOfficeByID :one
SELECT * FROM offices WHERE id = $1 AND deleted_at IS NULL;
