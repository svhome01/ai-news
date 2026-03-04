-- Migration 003: Add per-category TTS speed scale.
-- Existing rows get DEFAULT 1.0 (normal speed).
ALTER TABLE category_settings ADD COLUMN speed_scale REAL NOT NULL DEFAULT 1.0;
