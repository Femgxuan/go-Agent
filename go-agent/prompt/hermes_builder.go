package prompt

import (
	"fmt"
	"strings"
	"sync"
)

// HermesBuilder assembles a system prompt in the Hermes order:
// 1. Persona (SOUL.md)
// 2. Platform hints
// 3. Memory guidance (MEMORY.md § USER.md frozen snapshot)
// 4. Session search hint
// 5. Skills guidance (Level-0 index)
// 6. Context files (AGENTS.md)
// 7. Tool-use enforcement rules
// 8. Tool schemas
//
// Once Build() is called, the result is frozen for the session lifetime (sync.Once).
// This ensures byte-static system prompt for prompt caching.
type HermesBuilder struct {
	mu sync.Mutex

	soul         string
	platform     string
	memory       string // MEMORY.md content
	user         string // USER.md content
	skillsIndex  string
	contextFiles string
	toolRules    string
	toolSchemas  string

	built     string
	builtOnce sync.Once
}

// NewHermesBuilder creates a new HermesBuilder.
func NewHermesBuilder() *HermesBuilder {
	return &HermesBuilder{}
}

// SetSoul sets the persona section (SOUL.md content).
func (b *HermesBuilder) SetSoul(soul string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.soul = soul
}

// SetPlatform sets the platform hints section.
func (b *HermesBuilder) SetPlatform(platform string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.platform = platform
}

// SetMemory sets the frozen snapshot (MEMORY.md + USER.md).
func (b *HermesBuilder) SetMemory(memory, user string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.memory = memory
	b.user = user
}

// SetSkillsIndex sets the Level-0 skills index string.
func (b *HermesBuilder) SetSkillsIndex(index string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.skillsIndex = index
}

// SetContextFiles sets the context files section (AGENTS.md).
func (b *HermesBuilder) SetContextFiles(files string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.contextFiles = files
}

// SetToolRules sets the tool-use enforcement rules.
func (b *HermesBuilder) SetToolRules(rules string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.toolRules = rules
}

// SetToolSchemas sets the tool JSON schemas.
func (b *HermesBuilder) SetToolSchemas(schemas string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.toolSchemas = schemas
}

// Build assembles the system prompt. The result is frozen after the first call.
func (b *HermesBuilder) Build() string {
	b.builtOnce.Do(func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		var sb strings.Builder

		// 1. Persona
		if b.soul != "" {
			sb.WriteString(b.soul)
			sb.WriteString("\n\n")
		}

		// 2. Platform hints
		if b.platform != "" {
			sb.WriteString(fmt.Sprintf("## Platform\nYou are running in: %s\n\n", b.platform))
		}

		// 3. Memory guidance (frozen snapshot)
		if b.memory != "" || b.user != "" {
			sb.WriteString("## Memory\n")
			if b.memory != "" {
				sb.WriteString(b.memory)
			}
			if b.user != "" {
				if b.memory != "" {
					sb.WriteString("\n§\n")
				}
				sb.WriteString(b.user)
			}
			sb.WriteString("\n\n")
		}

		// 4. Session search hint
		sb.WriteString("## Session History\n")
		sb.WriteString("You can use the `session_search` tool to query past conversations for relevant context.\n\n")

		// 5. Skills guidance
		if b.skillsIndex != "" {
			sb.WriteString(b.skillsIndex)
			sb.WriteString("\n")
		}

		// 6. Context files
		if b.contextFiles != "" {
			sb.WriteString("## Project Context\n")
			sb.WriteString(b.contextFiles)
			sb.WriteString("\n\n")
		}

		// 7. Tool-use enforcement
		if b.toolRules != "" {
			sb.WriteString("## Tool Usage Rules\n")
			sb.WriteString(b.toolRules)
			sb.WriteString("\n\n")
		}

		// 8. Tool schemas
		if b.toolSchemas != "" {
			sb.WriteString("## Available Tools\n")
			sb.WriteString(b.toolSchemas)
			sb.WriteString("\n")
		}

		b.built = sb.String()
	})

	return b.built
}
