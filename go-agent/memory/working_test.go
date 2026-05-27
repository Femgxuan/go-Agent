package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkingMemory_Add(t *testing.T) {
	wm := NewWorkingMemory(100, &SimpleTokenizer{})

	msg := Message{
		Role:      "user",
		Content:   "hello",
		Timestamp: time.Now(),
	}

	err := wm.Add(msg)
	require.NoError(t, err)

	window := wm.GetWindow(100)
	assert.Len(t, window, 1)
	assert.Equal(t, "hello", window[0].Content)
}

func TestWorkingMemory_SlidingWindow(t *testing.T) {
	wm := NewWorkingMemory(10, &SimpleTokenizer{}) // 10 tokens限制

	// 添加多条消息
	wm.Add(Message{Role: "user", Content: "hello world"})    // 2 tokens
	wm.Add(Message{Role: "assistant", Content: "hi there"})  // 2 tokens
	wm.Add(Message{Role: "user", Content: "how are you"})    // 3 tokens
	wm.Add(Message{Role: "assistant", Content: "I am fine"}) // 3 tokens

	window := wm.GetWindow(10)
	// 应该只保留最后几条，总tokens不超过10
	totalTokens := 0
	for _, msg := range window {
		totalTokens += wm.tokenizer.Count(msg.Content)
	}
	assert.LessOrEqual(t, totalTokens, 10)
}

func TestWorkingMemory_Remember(t *testing.T) {
	wm := NewWorkingMemory(100, &SimpleTokenizer{})

	// Remember关键信息
	wm.Remember("project", "Go Agent")

	window := wm.GetWindow(0) // 0表示使用默认限制
	found := false
	for _, msg := range window {
		if strings.Contains(msg.Content, "Go Agent") {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestWorkingMemory_TokenCount(t *testing.T) {
	wm := NewWorkingMemory(100, &SimpleTokenizer{})

	wm.Add(Message{Role: "user", Content: "hello world"})   // 2 tokens
	wm.Add(Message{Role: "assistant", Content: "hi there"}) // 2 tokens

	assert.Equal(t, 4, wm.TokenCount())
}
