package memory

import "errors"

var (
	ErrNoActiveSession     = errors.New("no active session")
	ErrSessionNotFound     = errors.New("session not found")
	ErrEmbeddingFailed     = errors.New("embedding generation failed")
	ErrDatabaseUnavailable = errors.New("database unavailable")
	ErrCompactionFailed    = errors.New("compaction failed")
)
