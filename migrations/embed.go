package migrations

import _ "embed"

// SQL contains the full DDL for the initial schema migration.
//
//go:embed 001_initial.sql
var SQL string
