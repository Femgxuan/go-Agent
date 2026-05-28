package memory

import (
	"fmt"
	"sync"
)

// WorkingMemory管理工作记忆（Token窗口）
type WorkingMemory interface {
	// Add添加消息到工作记忆
	Add(msg Message) error

	// GetWindow获取当前窗口内的消息
	GetWindow(maxTokens int) []Message

	// Remember标记关键信息，永不丢弃
	Remember(key, value string)

	// TokenCount计算当前工作记忆的Token数
	TokenCount() int

	// NeedsCompression returns true if token usage exceeds the given threshold.
	NeedsCompression(threshold float64) bool

	// GetCompressedSummary returns the compressed summary from previous compression.
	GetCompressedSummary() string

	// SetCompressedSummary sets the compressed summary after compression.
	SetCompressedSummary(summary string)

	// GetMessages returns all messages (for compression).
	GetMessages() []Message

	// ReplaceMessages replaces all messages (after compression).
	ReplaceMessages(msgs []Message)
}

// WorkingMemoryImpl实现WorkingMemory接口
type WorkingMemoryImpl struct {
	mu                sync.RWMutex
	messages          []Message
	keyFacts          map[string]string
	tokenizer         Tokenizer
	maxTokens         int
	compressedSummary string
}

// NewWorkingMemory创建一个新的WorkingMemory
func NewWorkingMemory(maxTokens int, tokenizer Tokenizer) *WorkingMemoryImpl {
	return &WorkingMemoryImpl{
		keyFacts:  make(map[string]string),
		tokenizer: tokenizer,
		maxTokens: maxTokens,
	}
}

// Add添加消息到工作记忆
func (w *WorkingMemoryImpl) Add(msg Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.messages = append(w.messages, msg)
	return nil
}

// GetWindow获取当前窗口内的消息
func (w *WorkingMemoryImpl) GetWindow(maxTokens int) []Message {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if maxTokens <= 0 {
		maxTokens = w.maxTokens
	}

	var result []Message
	tokenCount := 0

	// 从最新消息向前遍历
	for i := len(w.messages) - 1; i >= 0; i-- {
		msg := w.messages[i]
		msgTokens := w.tokenizer.Count(msg.Content)

		if tokenCount+msgTokens > maxTokens {
			break // 超出Token限制，停止
		}

		result = append([]Message{msg}, result...) // 插入到头部
		tokenCount += msgTokens
	}

	// 追加keyFacts（始终保留）
	for key, value := range w.keyFacts {
		result = append([]Message{{
			Role:    "system",
			Content: fmt.Sprintf("[记住] %s: %s", key, value),
		}}, result...)
	}

	return result
}

// Remember标记关键信息，永不丢弃
func (w *WorkingMemoryImpl) Remember(key, value string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.keyFacts[key] = value
}

// TokenCount计算当前工作记忆的Token数
func (w *WorkingMemoryImpl) TokenCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()

	count := 0
	for _, msg := range w.messages {
		count += w.tokenizer.Count(msg.Content)
	}
	return count
}

// NeedsCompression returns true if token usage exceeds the given threshold.
func (w *WorkingMemoryImpl) NeedsCompression(threshold float64) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.maxTokens <= 0 || len(w.messages) == 0 {
		return false
	}

	totalTokens := 0
	for _, msg := range w.messages {
		totalTokens += w.tokenizer.Count(msg.Content)
	}

	usage := float64(totalTokens) / float64(w.maxTokens)
	return usage >= threshold
}

// GetCompressedSummary returns the compressed summary from previous compression.
func (w *WorkingMemoryImpl) GetCompressedSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.compressedSummary
}

// SetCompressedSummary sets the compressed summary after compression.
func (w *WorkingMemoryImpl) SetCompressedSummary(summary string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.compressedSummary = summary
}

// GetMessages returns all messages (for compression).
func (w *WorkingMemoryImpl) GetMessages() []Message {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]Message, len(w.messages))
	copy(result, w.messages)
	return result
}

// ReplaceMessages replaces all messages (after compression).
func (w *WorkingMemoryImpl) ReplaceMessages(msgs []Message) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.messages = msgs
}
