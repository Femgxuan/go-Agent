package prompt

import (
	"strings"
	"testing"
)

func TestPromptBuilder_Build_ContainsAllSections(t *testing.T) {
	b := NewHermesBuilder()
	b.SetSoul("You are a test agent.")
	b.SetPlatform("CLI")
	b.SetMemory("test memory content", "test user profile")
	b.SetSkillsIndex("- skill1: test skill")
	b.SetContextFiles("AGENTS.md content")
	b.SetToolRules("Always use tools properly.")
	b.SetToolSchemas(`{"tools": []}`)

	result := b.Build()

	checks := []string{
		"You are a test agent.",
		"CLI",
		"test memory content",
		"test user profile",
		"skill1: test skill",
		"AGENTS.md content",
		"Always use tools properly.",
		`{"tools": []}`,
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("Build() missing expected content: %q", check)
		}
	}
}

func TestPromptBuilder_Build_Frozen(t *testing.T) {
	b := NewHermesBuilder()
	b.SetSoul("first")

	r1 := b.Build()

	b.SetSoul("second") // Should NOT affect already-built result
	r2 := b.Build()

	if r1 != r2 {
		t.Error("Build() should be frozen after first call (sync.Once)")
	}
	if strings.Contains(r2, "second") {
		t.Error("Build() should not include changes after first call")
	}
}

func TestPromptBuilder_Build_Ordering(t *testing.T) {
	b := NewHermesBuilder()
	b.SetSoul("SECTION_A")
	b.SetPlatform("SECTION_B")
	b.SetMemory("SECTION_C", "SECTION_D")
	b.SetSkillsIndex("SECTION_E")
	b.SetContextFiles("SECTION_F")
	b.SetToolRules("SECTION_G")
	b.SetToolSchemas("SECTION_H")

	result := b.Build()

	idxA := strings.Index(result, "SECTION_A")
	idxB := strings.Index(result, "SECTION_B")
	idxC := strings.Index(result, "SECTION_C")
	idxD := strings.Index(result, "SECTION_D")
	idxE := strings.Index(result, "SECTION_E")
	idxF := strings.Index(result, "SECTION_F")
	idxG := strings.Index(result, "SECTION_G")
	idxH := strings.Index(result, "SECTION_H")

	if !(idxA < idxB && idxB < idxC && idxC < idxD && idxD < idxE && idxE < idxF && idxF < idxG && idxG < idxH) {
		t.Error("Build() sections are not in the correct order")
	}
}
