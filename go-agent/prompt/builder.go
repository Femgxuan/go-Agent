package prompt

import (
	"strings"
	"time"
)

type Builder struct {
	sources []Source
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) AddSource(s Source) {
	b.sources = append(b.sources, s)
}

func (b *Builder) Reset() {
	b.sources = nil
}

func (b *Builder) Build() (string, *Trace) {
	trace := &Trace{
		Timestamp:     time.Now(),
		RulesLoaded:   []string{},
		SkillsMatched: []string{},
		Sources:       []string{},
	}

	var parts []string
	for _, s := range b.sources {
		if s.Content == "" {
			continue
		}
		parts = append(parts, s.Content)
		trace.Sources = append(trace.Sources, s.Name)
		switch s.Type {
		case TypeRules:
			trace.RulesLoaded = append(trace.RulesLoaded, s.Name)
		case TypeSkill:
			trace.SkillsMatched = append(trace.SkillsMatched, s.Name)
		}
	}

	result := strings.Join(parts, "\n\n")
	trace.TotalChars = len(result)
	return result, trace
}
