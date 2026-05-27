package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MetaMemory manages meta-memories (self-reflections).
type MetaMemory interface {
	// Record records a reflection.
	Record(ctx context.Context, reflection Reflection) error

	// Recent retrieves the most recent reflections.
	Recent(ctx context.Context, limit int) ([]Reflection, error)

	// Search searches for relevant reflections.
	Search(ctx context.Context, query string, limit int) ([]Reflection, error)
}

// FileMetaMemory is a file-based meta-memory implementation.
type FileMetaMemory struct {
	mu       sync.RWMutex
	basePath string
}

// NewFileMetaMemory creates a new FileMetaMemory.
func NewFileMetaMemory(basePath string) *FileMetaMemory {
	return &FileMetaMemory{
		basePath: basePath,
	}
}

// Record records a reflection.
func (f *FileMetaMemory) Record(ctx context.Context, reflection Reflection) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Ensure directory exists.
	if err := os.MkdirAll(f.basePath, 0755); err != nil {
		return fmt.Errorf("creating reflections directory: %w", err)
	}

	// Organize by date.
	dateStr := reflection.CreatedAt.Format("20060102")
	filePath := filepath.Join(f.basePath, dateStr+".json")

	// Load or create file.
	var reflections []Reflection
	data, err := os.ReadFile(filePath)
	if err == nil {
		json.Unmarshal(data, &reflections)
	}

	// Append reflection.
	reflections = append(reflections, reflection)

	// Write file.
	data, err = json.MarshalIndent(reflections, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling reflections: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

// Recent retrieves the most recent reflections.
func (f *FileMetaMemory) Recent(ctx context.Context, limit int) ([]Reflection, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Read reflection files from the last 7 days.
	var all []Reflection
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		filePath := filepath.Join(f.basePath, dateStr+".json")

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var reflections []Reflection
		json.Unmarshal(data, &reflections)
		all = append(all, reflections...)
	}

	// Sort by time descending, return the most recent limit entries.
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	return all, nil
}

// Search searches for relevant reflections.
func (f *FileMetaMemory) Search(ctx context.Context, query string, limit int) ([]Reflection, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Read reflection files from the last 7 days.
	var all []Reflection
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		filePath := filepath.Join(f.basePath, dateStr+".json")

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var reflections []Reflection
		json.Unmarshal(data, &reflections)
		all = append(all, reflections...)
	}

	// Simple keyword search.
	var results []Reflection
	queryLower := strings.ToLower(query)
	for _, ref := range all {
		if strings.Contains(strings.ToLower(ref.Content), queryLower) {
			results = append(results, ref)
		}
	}

	// Sort by time descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
