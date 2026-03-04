package migrations

import _ "embed"

// SQL1 contains the initial schema DDL (migration 001).
//
//go:embed 001_initial.sql
var SQL1 string

// SQL2 contains the migration that adds 'gcloud' to the tts_engine CHECK constraint (migration 002).
//
//go:embed 002_gcloud_tts.sql
var SQL2 string

// SQL3 contains the migration that adds per-category speed_scale to category_settings (migration 003).
//
//go:embed 003_per_category_speed.sql
var SQL3 string
