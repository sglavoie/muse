-- +goose Up
ALTER TABLE queue ADD COLUMN deleted_at        INTEGER;              -- soft-delete; NULL means active
ALTER TABLE queue ADD COLUMN shuffle_position  INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE queue DROP COLUMN shuffle_position;
ALTER TABLE queue DROP COLUMN deleted_at;
