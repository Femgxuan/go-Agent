package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleTokenizer_Count(t *testing.T) {
	tokenizer := &SimpleTokenizer{}

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"english words", "hello world", 2},
		{"chinese chars", "你好世界", 8}, // 每个中文字符≈2 tokens
		{"mixed content", "hello 世界", 5}, // 1 english word + 2 chinese chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Count(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTiktokenTokenizer_Count(t *testing.T) {
	tk, err := NewTiktokenTokenizer("gpt-4o")
	if err != nil {
		t.Skip("tiktoken not available:", err)
	}

	tests := []struct {
		name  string
		input string
		min   int
	}{
		{"empty", "", 0},
		{"english", "Hello, world!", 3},
		{"chinese", "你好世界", 2},
		{"mixed", "Hello 你好 world 世界", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tk.Count(tt.input)
			if got < tt.min {
				t.Errorf("Count(%q) = %d, want >= %d", tt.input, got, tt.min)
			}
		})
	}
}
