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
