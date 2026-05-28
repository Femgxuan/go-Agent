package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/fengxuan/go-agent/client"
)

// mockClassifierLLM returns a fixed response for testing.
type mockClassifierLLM struct {
	response string
}

func (m *mockClassifierLLM) ChatCompletion(ctx context.Context, req client.ChatRequest) (<-chan client.StreamChunk, error) {
	ch := make(chan client.StreamChunk, 2)
	ch <- client.StreamChunk{Delta: m.response}
	close(ch)
	return ch, nil
}

func TestClassify_Targets(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantTarget string
		wantOK     bool
	}{
		{
			name:       "soul target",
			response:   `{"target":"soul","content":"§ 说话要简洁，不要用emoji","reason":"定义沟通风格"}`,
			wantTarget: "soul",
			wantOK:     true,
		},
		{
			name:       "memory target",
			response:   `{"target":"memory","content":"§ 项目使用 PostgreSQL 作为主数据库","reason":"项目技术栈"}`,
			wantTarget: "memory",
			wantOK:     true,
		},
		{
			name:       "user target",
			response:   `{"target":"user","content":"§ 用户偏好使用 Go 写后端","reason":"用户技术偏好"}`,
			wantTarget: "user",
			wantOK:     true,
		},
		{
			name:       "none target",
			response:   `{"target":"none","content":"","reason":"临时任务，无需记忆"}`,
			wantTarget: "none",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClassifier(&mockClassifierLLM{response: tt.response})
			result, ok := c.Classify(context.Background(), "test input", "test reply")
			if ok != tt.wantOK {
				t.Errorf("Classify() ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && result.Target != tt.wantTarget {
				t.Errorf("Classify() target = %q, want %q", result.Target, tt.wantTarget)
			}
		})
	}
}

func TestClassify_PromptParsing(t *testing.T) {
	// Test with markdown code fences around JSON
	response := "```json\n{\"target\":\"user\",\"content\":\"§ 用户在北京\",\"reason\":\"时区信息\"}\n```"
	c := NewClassifier(&mockClassifierLLM{response: response})
	result, ok := c.Classify(context.Background(), "我在北京", "好的")
	if !ok {
		t.Fatal("Classify() should return ok=true")
	}
	if result.Target != "user" {
		t.Errorf("target = %q, want %q", result.Target, "user")
	}
	if !strings.HasPrefix(result.Content, "§") {
		t.Errorf("content should start with §, got %q", result.Content)
	}
}
