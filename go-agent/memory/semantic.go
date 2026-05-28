package memory

import "context"

// SemanticStore manages synthesized knowledge: user preferences, facts, learned patterns.
// This replaces the old LongTermMemory interface with a cleaner contract.
type SemanticStore interface {
	// Remember stores a synthesized fact.
	Remember(ctx context.Context, fact Fact) error

	// Recall retrieves relevant facts for a query.
	Recall(ctx context.Context, query string, topK int) ([]Fact, error)

	// Update updates specific fields of a fact.
	Update(ctx context.Context, id string, updates map[string]any) error

	// Delete deletes a fact by ID.
	Delete(ctx context.Context, id string) error

	// ForgetByFilter deletes facts matching the filter criteria.
	ForgetByFilter(ctx context.Context, filter ForgetFilter) error
}
