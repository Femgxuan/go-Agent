package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ShortTermMemory manages short-term memory (session history persistence).
type ShortTermMemory interface {
	// Save saves one interaction.
	Save(ctx context.Context, sid SessionID, interaction Interaction) error

	// Load loads session history.
	Load(ctx context.Context, sid SessionID, limit int) ([]Interaction, error)

	// ListSessions lists all sessions.
	ListSessions(ctx context.Context, userID string) ([]SessionID, error)

	// DeleteSession deletes a session.
	DeleteSession(ctx context.Context, sid SessionID) error
}

// SessionFile is the structure of a single session file.
type SessionFile struct {
	ID        SessionID     `json:"id"`
	UserID    string        `json:"user_id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []Interaction `json:"messages"`
}

// FileShortTermMemory is a file-based short-term memory implementation.
type FileShortTermMemory struct {
	mu       sync.RWMutex
	basePath string
	current  *SessionFile
}

// NewFileShortTermMemory creates a new FileShortTermMemory.
func NewFileShortTermMemory(basePath string) *FileShortTermMemory {
	return &FileShortTermMemory{
		basePath: basePath,
	}
}

// Save saves one interaction to the current session.
func (f *FileShortTermMemory) Save(ctx context.Context, sid SessionID, interaction Interaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// If new session or session changed, load or create.
	if f.current == nil || f.current.ID != sid {
		session, err := f.loadOrCreate(sid)
		if err != nil {
			return err
		}
		f.current = session
	}

	// Append interaction.
	f.current.Messages = append(f.current.Messages, interaction)
	f.current.UpdatedAt = time.Now()

	// Write to file.
	return f.saveToFile()
}

// Load loads session history.
func (f *FileShortTermMemory) Load(ctx context.Context, sid SessionID, limit int) ([]Interaction, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// If it's the current session, return directly.
	if f.current != nil && f.current.ID == sid {
		if limit > 0 && len(f.current.Messages) > limit {
			return f.current.Messages[len(f.current.Messages)-limit:], nil
		}
		return f.current.Messages, nil
	}

	// Otherwise load from file.
	session, err := f.loadFromFile(sid)
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(session.Messages) > limit {
		return session.Messages[len(session.Messages)-limit:], nil
	}
	return session.Messages, nil
}

// ListSessions lists all sessions.
func (f *FileShortTermMemory) ListSessions(ctx context.Context, userID string) ([]SessionID, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	entries, err := os.ReadDir(f.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions directory: %w", err)
	}

	var sessions []SessionID
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			sid := SessionID(name[:len(name)-5]) // Remove .json suffix.
			sessions = append(sessions, sid)
		}
	}

	return sessions, nil
}

// DeleteSession deletes a session.
func (f *FileShortTermMemory) DeleteSession(ctx context.Context, sid SessionID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// If it's the current session, clear it.
	if f.current != nil && f.current.ID == sid {
		f.current = nil
	}

	// Delete file.
	filePath := filepath.Join(f.basePath, string(sid)+".json")
	return os.Remove(filePath)
}

// loadOrCreate loads or creates a session file.
func (f *FileShortTermMemory) loadOrCreate(sid SessionID) (*SessionFile, error) {
	filePath := filepath.Join(f.basePath, string(sid)+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new session.
			return &SessionFile{
				ID:        sid,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Messages:  []Interaction{},
			}, nil
		}
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session SessionFile
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}

// loadFromFile loads a session from file.
func (f *FileShortTermMemory) loadFromFile(sid SessionID) (*SessionFile, error) {
	filePath := filepath.Join(f.basePath, string(sid)+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session SessionFile
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}

// saveToFile saves the current session to file.
func (f *FileShortTermMemory) saveToFile() error {
	if f.current == nil {
		return nil
	}

	// Ensure directory exists.
	if err := os.MkdirAll(f.basePath, 0755); err != nil {
		return fmt.Errorf("creating sessions directory: %w", err)
	}

	filePath := filepath.Join(f.basePath, string(f.current.ID)+".json")
	data, err := json.MarshalIndent(f.current, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}
