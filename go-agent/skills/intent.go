package skills

import (
	"regexp"
	"strings"
	"unicode"
)

// CreateSkillIntent holds the result of intent detection for "create skill" requests.
type CreateSkillIntent struct {
	Detected  bool
	SkillName string   // extracted name if present, empty otherwise
	Keywords  []string // extracted keywords/tags from input
}

var chinesePhrases = []string{
	"保存成skill", "保存为skill", "创建skill", "创建技能",
	"沉淀为技能", "沉淀成技能", "保存成技能", "保存为技能",
	"生成skill", "生成技能", "新建skill", "新建技能",
	"添加skill", "添加技能",
}

var englishPhrases = []string{
	"save as skill", "create skill", "create a skill",
	"save skill", "new skill", "add skill",
	"make a skill", "make skill",
}

var patternRegex = regexp.MustCompile(`(?i)(save|create|make|add|generate|build)\s+(this\s+)?(as\s+)?(a\s+)?skill`)

// Name extraction patterns
var chineseNamePatterns = []string{"叫", "命名为", "名字是"}
var englishNameRegex = regexp.MustCompile(`(?i)(?:named|called|name\s+it|name:\s*)\s*(\S+)`)

// Keyword extraction patterns
var chineseKeywordPatterns = []string{"关键词", "标签"}
var englishKeywordRegex = regexp.MustCompile(`(?i)(?:tags|keywords):\s*(.+)`)

// DetectCreateSkillIntent detects whether the user wants to create/save a skill.
// Pure rule-based, no LLM.
func DetectCreateSkillIntent(input string) CreateSkillIntent {
	result := CreateSkillIntent{}
	lower := strings.ToLower(input)

	// Check Chinese phrases
	for _, phrase := range chinesePhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			result.Detected = true
			break
		}
	}

	// Check English phrases
	if !result.Detected {
		for _, phrase := range englishPhrases {
			if strings.Contains(lower, phrase) {
				result.Detected = true
				break
			}
		}
	}

	// Check regex pattern
	if !result.Detected {
		if patternRegex.MatchString(input) {
			result.Detected = true
		}
	}

	if !result.Detected {
		return result
	}

	// Extract name
	result.SkillName = extractName(input)

	// Extract keywords
	result.Keywords = extractKeywords(input)

	return result
}

func extractName(input string) string {
	// Try Chinese name patterns
	for _, pat := range chineseNamePatterns {
		idx := strings.Index(input, pat)
		if idx >= 0 {
			after := input[idx+len(pat):]
			after = strings.TrimLeftFunc(after, unicode.IsSpace)
			name := extractUntilPunctuation(after)
			if name != "" {
				return name
			}
		}
	}

	// Try English name patterns
	m := englishNameRegex.FindStringSubmatch(input)
	if m != nil {
		name := strings.TrimSpace(m[1])
		name = extractUntilPunctuation(name)
		if name != "" {
			return name
		}
	}

	return ""
}

func extractUntilPunctuation(s string) string {
	var b strings.Builder
	for _, r := range s {
		if isPunctuation(r) {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func isPunctuation(r rune) bool {
	if unicode.IsPunct(r) && r != '-' && r != '_' {
		return true
	}
	// Chinese punctuation and common delimiters
	switch r {
	case '，', '。', '！', '？', '、', '；', '：', '\n', '\r', '\t':
		return true
	}
	return false
}

func extractKeywords(input string) []string {
	// Try Chinese keyword patterns
	for _, pat := range chineseKeywordPatterns {
		idx := strings.Index(input, pat)
		if idx >= 0 {
			after := input[idx+len(pat):]
			// Skip optional colon/punctuation
			after = strings.TrimLeftFunc(after, func(r rune) bool {
				return r == '：' || r == ':' || unicode.IsSpace(r)
			})
			return splitKeywords(after)
		}
	}

	// Try English keyword patterns
	m := englishKeywordRegex.FindStringSubmatch(input)
	if m != nil {
		return splitKeywords(m[1])
	}

	return nil
}

func splitKeywords(s string) []string {
	// Split by Chinese comma, English comma
	parts := regexp.MustCompile(`[,，]`).Split(s, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Stop at punctuation that indicates end of keyword list
		p = extractUntilPunctuation(p)
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
