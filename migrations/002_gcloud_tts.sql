-- Migration 002: Add 'gcloud' to tts_engine CHECK constraint on category_settings.
-- This runs only when PRAGMA user_version < 2 (managed by repository/db.go).
-- Strategy: create new table → copy data → drop old → rename.

CREATE TABLE IF NOT EXISTS category_settings_v2 (
    category                  TEXT    PRIMARY KEY,
    display_name              TEXT    NOT NULL,
    articles_per_episode      INTEGER NOT NULL DEFAULT 10,
    summary_chars_per_article INTEGER NOT NULL DEFAULT 200,
    language                  TEXT    NOT NULL DEFAULT 'ja'
                                      CHECK (language IN ('ja', 'en')),
    tts_engine                TEXT    NOT NULL DEFAULT 'voicevox'
                                      CHECK (tts_engine IN ('voicevox', 'edge-tts', 'gcloud')),
    voicevox_speaker_id       INTEGER NOT NULL DEFAULT 3,
    tts_voice                 TEXT,
    enabled                   INTEGER NOT NULL DEFAULT 1,
    sort_order                INTEGER NOT NULL DEFAULT 0,
    created_at                TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
INSERT OR IGNORE INTO category_settings_v2 SELECT * FROM category_settings;
DROP TABLE IF EXISTS category_settings;
ALTER TABLE category_settings_v2 RENAME TO category_settings;
CREATE INDEX IF NOT EXISTS idx_category_settings_enabled    ON category_settings (enabled);
CREATE INDEX IF NOT EXISTS idx_category_settings_sort_order ON category_settings (sort_order)
