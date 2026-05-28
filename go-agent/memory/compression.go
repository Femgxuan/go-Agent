package memory

import "context"

// Summarizer compresses a set of messages into a summary string.
type Summarizer interface {
	Summarize(ctx context.Context, msgs []Message) (string, error)
}

// ContextCompressor monitors working memory token usage and triggers compression
// when usage exceeds a configurable threshold.
type ContextCompressor struct {
	working    WorkingMemory
	summarizer Summarizer
	tokenizer  Tokenizer
	maxTokens  int
	threshold  float64
}

// NewContextCompressor creates a new compressor.
func NewContextCompressor(working WorkingMemory, summarizer Summarizer, tokenizer Tokenizer, maxTokens int, threshold float64) *ContextCompressor {
	return &ContextCompressor{
		working:    working,
		summarizer: summarizer,
		tokenizer:  tokenizer,
		maxTokens:  maxTokens,
		threshold:  threshold,
	}
}

// Check returns true if working memory token usage exceeds the threshold.
func (c *ContextCompressor) Check(ctx context.Context) (bool, error) {
	return c.working.NeedsCompression(c.threshold), nil
}

// Compress compresses early messages in working memory, replacing them with a summary.
// Takes the oldest 70% of messages, summarizes them, and replaces them with a
// single compressed message.
func (c *ContextCompressor) Compress(ctx context.Context) (int, error) {
	msgs := c.working.GetMessages()
	if len(msgs) < 2 {
		return 0, nil
	}

	// Split: oldest 70% for summarization, newest 30% to keep
	splitIdx := len(msgs) * 70 / 100
	if splitIdx < 1 {
		splitIdx = 1
	}
	oldMsgs := msgs[:splitIdx]
	newMsgs := msgs[splitIdx:]

	// Calculate tokens saved
	oldTokens := 0
	for _, m := range oldMsgs {
		oldTokens += c.tokenizer.Count(m.Content)
	}

	// Summarize old messages
	summary, err := c.summarizer.Summarize(ctx, oldMsgs)
	if err != nil {
		return 0, err
	}

	// Build new message list: summary + recent messages
	compressed := []Message{
		{Role: "system", Content: "[Context compressed] " + summary},
	}
	compressed = append(compressed, newMsgs...)

	c.working.ReplaceMessages(compressed)
	c.working.SetCompressedSummary(summary)

	newTokens := c.tokenizer.Count(summary)
	saved := oldTokens - newTokens
	if saved < 0 {
		saved = 0
	}

	return saved, nil
}

// AvailableBudget returns the token budget available for working memory messages.
func (c *ContextCompressor) AvailableBudget() int {
	reserve := int(float64(c.maxTokens) * 0.20)
	budget := c.maxTokens - reserve
	if budget < 1024 {
		budget = 1024
	}
	return budget
}
