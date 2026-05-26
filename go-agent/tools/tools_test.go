package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---- TavilySearch ----

func TestTavilySearch(t *testing.T) {
	// Mock HTTP server returning a valid Tavily-like response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{"title": "Go Programming", "url": "https://go.dev", "content": "Go is an open source language."},
				{"title": "Golang Blog", "url": "https://go.dev/blog", "content": "Latest news about Go."},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tool := NewTavilySearch("test-api-key", srv.URL)
	result, err := tool.Execute(context.Background(), map[string]any{
		"query":       "Go programming",
		"max_results": 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "Go Programming") {
		t.Errorf("expected result to contain 'Go Programming', got: %s", result)
	}
}

// ---- ShellExec ----

func TestShellExec(t *testing.T) {
	tool := NewShellExec(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
		"timeout": 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "hello\n"
	if runtime.GOOS == "windows" {
		expected = "hello\r\n"
	}
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestShellExecBlocked(t *testing.T) {
	tool := NewShellExec([]string{"sudo", "rm -rf /"})
	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "sudo rm -rf /",
	})
	if err == nil {
		t.Fatal("expected error for blocked command, got nil")
	}
}

func TestShellExecTimeout(t *testing.T) {
	tool := NewShellExec(nil)
	sleepCmd := "sleep 10"
	if runtime.GOOS == "windows" {
		sleepCmd = "timeout /t 10 /nobreak"
	}
	_, err := tool.Execute(context.Background(), map[string]any{
		"command": sleepCmd,
		"timeout": 1,
	})
	if err == nil {
		t.Fatal("expected error for timed-out command, got nil")
	}
}

// ---- ReadFile ----

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	content := "hello world"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	tool := NewReadFile(1 << 20) // 1 MB
	result, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Errorf("expected %q, got %q", content, result)
	}
}

func TestReadFilePathTraversal(t *testing.T) {
	tool := NewReadFile(1 << 20)
	_, err := tool.Execute(context.Background(), map[string]any{"path": "/tmp/../etc/passwd"})
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestReadFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(path, []byte("abcdefgh"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	tool := NewReadFile(4) // max 4 bytes, file is 8
	_, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err == nil {
		t.Fatal("expected error for file too large, got nil")
	}
}

// ---- WriteFile ----

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "output.txt")
	content := "written content"

	tool := NewWriteFile()
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": content,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result message")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(got) != content {
		t.Errorf("expected %q, got %q", content, string(got))
	}
}
