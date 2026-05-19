package skills

import (
	"strings"
	"testing"
)

func TestSkillDraft_Validate(t *testing.T) {
	tests := []struct {
		name    string
		draft   SkillDraft
		wantErr string
	}{
		{
			name:    "empty name",
			draft:   SkillDraft{Description: "desc", Body: "body"},
			wantErr: "name is required",
		},
		{
			name:    "empty description",
			draft:   SkillDraft{Name: "test", Body: "body"},
			wantErr: "description is required",
		},
		{
			name:    "empty body",
			draft:   SkillDraft{Name: "test", Description: "desc"},
			wantErr: "body is required",
		},
		{
			name:  "valid draft",
			draft: SkillDraft{Name: "test", Description: "desc", Body: "body"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.draft.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestSkillDraft_RenderMarkdown(t *testing.T) {
	t.Run("full draft with all fields", func(t *testing.T) {
		draft := SkillDraft{
			Name:        "my-skill",
			Description: "A test skill",
			Tags:        []string{"test", "demo"},
			Keywords:    []string{"hello", "world"},
			Patterns:    []string{"do .*", "run .*"},
			Priority:    10,
			Body:        "This is the skill body.\n\nWith multiple paragraphs.",
		}

		got := draft.RenderMarkdown()

		expects := []string{
			"---\n",
			"name: my-skill\n",
			"description: A test skill\n",
			"tags: [test, demo]\n",
			"triggers:\n",
			"  keywords: [hello, world]\n",
			"  patterns: [do .*, run .*]\n",
			"priority: 10\n",
			"---\n",
			"\nThis is the skill body.",
		}
		for _, exp := range expects {
			if !strings.Contains(got, exp) {
				t.Errorf("expected output to contain %q\ngot:\n%s", exp, got)
			}
		}
	})

	t.Run("minimal draft no tags triggers priority", func(t *testing.T) {
		draft := SkillDraft{
			Name:        "minimal",
			Description: "Minimal skill",
			Body:        "Just a body.",
		}

		got := draft.RenderMarkdown()

		if strings.Contains(got, "tags:") {
			t.Error("should not contain tags")
		}
		if strings.Contains(got, "triggers:") {
			t.Error("should not contain triggers")
		}
		if strings.Contains(got, "priority:") {
			t.Error("should not contain priority")
		}
		if !strings.Contains(got, "name: minimal\n") {
			t.Error("should contain name")
		}
		if !strings.Contains(got, "description: Minimal skill\n") {
			t.Error("should contain description")
		}
		if !strings.Contains(got, "Just a body.") {
			t.Error("should contain body")
		}
	})

	t.Run("draft with only keywords", func(t *testing.T) {
		draft := SkillDraft{
			Name:        "kw-only",
			Description: "Keywords only",
			Keywords:    []string{"alpha", "beta"},
			Body:        "Body text.",
		}

		got := draft.RenderMarkdown()

		if !strings.Contains(got, "triggers:\n") {
			t.Error("should contain triggers section")
		}
		if !strings.Contains(got, "  keywords: [alpha, beta]\n") {
			t.Error("should contain keywords")
		}
		if strings.Contains(got, "patterns:") {
			t.Error("should not contain patterns")
		}
	})

	t.Run("draft with only patterns", func(t *testing.T) {
		draft := SkillDraft{
			Name:        "pat-only",
			Description: "Patterns only",
			Patterns:    []string{"run .*"},
			Body:        "Body text.",
		}

		got := draft.RenderMarkdown()

		if !strings.Contains(got, "triggers:\n") {
			t.Error("should contain triggers section")
		}
		if !strings.Contains(got, "  patterns: [run .*]\n") {
			t.Error("should contain patterns")
		}
		if strings.Contains(got, "keywords:") {
			t.Error("should not contain keywords")
		}
	})
}

func TestSkillDraft_RenderMarkdown_Roundtrip(t *testing.T) {
	draft := SkillDraft{
		Name:        "roundtrip-skill",
		Description: "A roundtrip test skill",
		Tags:        []string{"tag1", "tag2"},
		Keywords:    []string{"kw1", "kw2"},
		Patterns:    []string{"pat1", "pat2"},
		Priority:    5,
		Body:        "Roundtrip body content.",
	}

	md := draft.RenderMarkdown()
	skill, err := ParseSkillFile(md)
	if err != nil {
		t.Fatalf("ParseSkillFile failed: %v", err)
	}

	if skill.Name != draft.Name {
		t.Errorf("Name: got %q, want %q", skill.Name, draft.Name)
	}
	if skill.Description != draft.Description {
		t.Errorf("Description: got %q, want %q", skill.Description, draft.Description)
	}
	if len(skill.Tags) != len(draft.Tags) {
		t.Errorf("Tags length: got %d, want %d", len(skill.Tags), len(draft.Tags))
	} else {
		for i, tag := range draft.Tags {
			if skill.Tags[i] != tag {
				t.Errorf("Tags[%d]: got %q, want %q", i, skill.Tags[i], tag)
			}
		}
	}
	if len(skill.Triggers.Keywords) != len(draft.Keywords) {
		t.Errorf("Keywords length: got %d, want %d", len(skill.Triggers.Keywords), len(draft.Keywords))
	} else {
		for i, kw := range draft.Keywords {
			if skill.Triggers.Keywords[i] != kw {
				t.Errorf("Keywords[%d]: got %q, want %q", i, skill.Triggers.Keywords[i], kw)
			}
		}
	}
	if len(skill.Triggers.Patterns) != len(draft.Patterns) {
		t.Errorf("Patterns length: got %d, want %d", len(skill.Triggers.Patterns), len(draft.Patterns))
	} else {
		for i, pat := range draft.Patterns {
			if skill.Triggers.Patterns[i] != pat {
				t.Errorf("Patterns[%d]: got %q, want %q", i, skill.Triggers.Patterns[i], pat)
			}
		}
	}
	if skill.Priority != draft.Priority {
		t.Errorf("Priority: got %d, want %d", skill.Priority, draft.Priority)
	}
	if skill.Body != draft.Body {
		t.Errorf("Body: got %q, want %q", skill.Body, draft.Body)
	}
}

func TestSkillDraft_ToSkill(t *testing.T) {
	draft := SkillDraft{
		Name:        "to-skill-test",
		Description: "Conversion test",
		Tags:        []string{"a", "b"},
		Keywords:    []string{"k1"},
		Patterns:    []string{"p1"},
		Priority:    3,
		Body:        "Skill body.",
	}

	skill := draft.ToSkill("project")

	if skill.Name != draft.Name {
		t.Errorf("Name: got %q, want %q", skill.Name, draft.Name)
	}
	if skill.Description != draft.Description {
		t.Errorf("Description: got %q, want %q", skill.Description, draft.Description)
	}
	if skill.Source != "project" {
		t.Errorf("Source: got %q, want %q", skill.Source, "project")
	}
	if len(skill.Tags) != len(draft.Tags) {
		t.Fatalf("Tags length: got %d, want %d", len(skill.Tags), len(draft.Tags))
	}
	for i, tag := range draft.Tags {
		if skill.Tags[i] != tag {
			t.Errorf("Tags[%d]: got %q, want %q", i, skill.Tags[i], tag)
		}
	}
	if len(skill.Triggers.Keywords) != len(draft.Keywords) {
		t.Fatalf("Keywords length: got %d, want %d", len(skill.Triggers.Keywords), len(draft.Keywords))
	}
	for i, kw := range draft.Keywords {
		if skill.Triggers.Keywords[i] != kw {
			t.Errorf("Keywords[%d]: got %q, want %q", i, skill.Triggers.Keywords[i], kw)
		}
	}
	if len(skill.Triggers.Patterns) != len(draft.Patterns) {
		t.Fatalf("Patterns length: got %d, want %d", len(skill.Triggers.Patterns), len(draft.Patterns))
	}
	for i, pat := range draft.Patterns {
		if skill.Triggers.Patterns[i] != pat {
			t.Errorf("Patterns[%d]: got %q, want %q", i, skill.Triggers.Patterns[i], pat)
		}
	}
	if skill.Priority != draft.Priority {
		t.Errorf("Priority: got %d, want %d", skill.Priority, draft.Priority)
	}
	if skill.Body != draft.Body {
		t.Errorf("Body: got %q, want %q", skill.Body, draft.Body)
	}
}
