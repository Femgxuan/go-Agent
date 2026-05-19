package skills

import (
	"regexp"
	"sort"
	"strings"
)

// Index holds an indexed set of skills for efficient matching.
type Index struct {
	skills     []*Skill
	MaxResults int
}

// NewIndex creates a new Index from the provided skills slice.
// Default MaxResults is 3.
func NewIndex(skills []*Skill) *Index {
	return &Index{
		skills:     skills,
		MaxResults: 3,
	}
}

// Reload replaces the skills in the index.
func (idx *Index) Reload(skills []*Skill) {
	idx.skills = skills
}

// List returns a copy of all skills in the index.
func (idx *Index) List() []*Skill {
	result := make([]*Skill, len(idx.skills))
	copy(result, idx.skills)
	return result
}

// MatchByName returns a slice of 0 or 1 skills with an exact name match.
func (idx *Index) MatchByName(name string) []*Skill {
	for _, s := range idx.skills {
		if s.Name == name {
			return []*Skill{s}
		}
	}
	return []*Skill{}
}

type scoredSkill struct {
	skill *Skill
	score int
}

// Match performs multi-strategy deterministic matching:
//  1. Keyword match: input contains keyword → score +10
//  2. Pattern match: input matches regex pattern → score +8
//  3. Tag match: input token equals tag → score +3
//  4. Description substring: input token found in description → score +1
//
// Results are sorted by score desc, then priority desc. Capped at MaxResults.
func (idx *Index) Match(input string) []*Skill {
	lowerInput := strings.ToLower(input)
	inputTokens := strings.Fields(lowerInput)

	var scored []scoredSkill

	for _, s := range idx.skills {
		score := 0

		// 1. Keyword match
		for _, kw := range s.Triggers.Keywords {
			if strings.Contains(lowerInput, strings.ToLower(kw)) {
				score += 10
				break
			}
		}

		// 2. Pattern match
		for _, pat := range s.Triggers.Patterns {
			re, err := regexp.Compile(pat)
			if err == nil && re.MatchString(lowerInput) {
				score += 8
				break
			}
		}

		// 3. Tag match
		for _, token := range inputTokens {
			for _, tag := range s.Tags {
				if token == strings.ToLower(tag) {
					score += 3
					goto doneTag
				}
			}
		}
	doneTag:

		// 4. Description substring
		lowerDesc := strings.ToLower(s.Description)
		for _, token := range inputTokens {
			if strings.Contains(lowerDesc, token) {
				score += 1
				break
			}
		}

		if score > 0 {
			scored = append(scored, scoredSkill{skill: s, score: score})
		}
	}

	// Sort by score desc, then priority desc
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].skill.Priority > scored[j].skill.Priority
	})

	// Cap at MaxResults
	limit := idx.MaxResults
	if limit > len(scored) {
		limit = len(scored)
	}

	result := make([]*Skill, limit)
	for i := 0; i < limit; i++ {
		result[i] = scored[i].skill
	}
	return result
}
