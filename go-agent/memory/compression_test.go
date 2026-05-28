package memory

import (
	"context"
	"testing"
)

type mockSummarizer struct {
	summary string
	err     error
}

func (m *mockSummarizer) Summarize(ctx context.Context, msgs []Message) (string, error) {
	return m.summary, m.err
}

func TestContextCompressor_Check(t *testing.T) {
	tk := &SimpleTokenizer{}
	wm := NewWorkingMemory(100, tk)

	for i := 0; i < 20; i++ {
		wm.Add(Message{Role: "user", Content: "this is a test message with enough tokens"})
	}

	comp := &ContextCompressor{
		working:    wm,
		summarizer: &mockSummarizer{summary: "compressed summary"},
		tokenizer:  tk,
		maxTokens:  100,
		threshold:  0.85,
	}

	needs, err := comp.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Error("expected compression needed")
	}
}

func TestContextCompressor_AvailableBudget(t *testing.T) {
	tk := &SimpleTokenizer{}
	wm := NewWorkingMemory(1000, tk)

	comp := &ContextCompressor{
		working:    wm,
		summarizer: &mockSummarizer{},
		tokenizer:  tk,
		maxTokens:  8192,
		threshold:  0.85,
	}

	budget := comp.AvailableBudget()
	if budget <= 0 || budget > 8192 {
		t.Errorf("unexpected budget: %d", budget)
	}
	// 8192 * 0.20 = 1638 (reserve), 8192 - 1638 = 6554
	if budget != 6554 {
		t.Errorf("expected 6554, got %d", budget)
	}
}

func TestContextCompressor_Compress(t *testing.T) {
	tk := &SimpleTokenizer{}
	wm := NewWorkingMemory(1000, tk)

	// Add enough messages to compress
	for i := 0; i < 10; i++ {
		wm.Add(Message{Role: "user", Content: "this is a test message with enough tokens to fill up"})
	}

	comp := &ContextCompressor{
		working:    wm,
		summarizer: &mockSummarizer{summary: "summary of old messages"},
		tokenizer:  tk,
		maxTokens:  1000,
		threshold:  0.85,
	}

	saved, err := comp.Compress(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should have saved some tokens
	if saved < 0 {
		t.Errorf("unexpected negative saved tokens: %d", saved)
	}

	// Working memory should now have fewer messages
	msgs := wm.GetMessages()
	if len(msgs) >= 10 {
		t.Errorf("expected fewer messages after compression, got %d", len(msgs))
	}
}
