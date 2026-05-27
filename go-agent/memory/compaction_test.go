package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDecayCalculator_Calculate(t *testing.T) {
	calculator := &DecayCalculator{halfLife: 30 * 24 * time.Hour} // 30天半衰期

	tests := []struct {
		name      string
		createdAt time.Time
		expected  float64
	}{
		{"just created", time.Now(), 1.0},
		{"30 days ago", time.Now().AddDate(0, 0, -30), 0.5},
		{"60 days ago", time.Now().AddDate(0, 0, -60), 0.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculator.Calculate(tt.createdAt)
			assert.InDelta(t, tt.expected, score, 0.01)
		})
	}
}
