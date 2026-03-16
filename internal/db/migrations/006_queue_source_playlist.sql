-- +goose Up
ALTER TABLE queue ADD COLUMN source_playlist TEXT NOT NULL DEFAULT '';
