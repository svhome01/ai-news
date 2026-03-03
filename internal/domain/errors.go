package domain

import "errors"

// Sentinel errors used across layers.
var (
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrInvalidInput   = errors.New("invalid input")
	ErrReservedWord   = errors.New("reserved word")
	ErrPipelineActive = errors.New("pipeline already running")
)
