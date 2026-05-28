package memory

import "testing"

func TestGetContextWindow(t *testing.T) {
	tests := []struct {
		model    string
		override int
		want     int
	}{
		{"deepseek-chat", 0, 65536},
		{"gpt-4o", 0, 128000},
		{"claude-sonnet-4-6", 0, 200000},
		{"unknown-model", 0, 8192},
		{"deepseek-chat", 32000, 32000},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := GetContextWindow(tt.model, tt.override)
			if got != tt.want {
				t.Errorf("GetContextWindow(%q, %d) = %d, want %d", tt.model, tt.override, got, tt.want)
			}
		})
	}
}
