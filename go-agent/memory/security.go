package memory

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	injectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above)\s+(instructions?|rules?|prompts?)`),
		regexp.MustCompile(`(?i)you\s+are\s+now\s+`),
		regexp.MustCompile(`(?i)system:\s`),
		regexp.MustCompile(`(?i)new\s+instructions?:`),
		regexp.MustCompile(`(?i)disregard\s+(all\s+)?(previous|prior)`),
		regexp.MustCompile(`(?i)override\s+(your\s+)?instructions?`),
	}

	exfiltrationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(curl|wget|fetch)\s+.*\$`),
		regexp.MustCompile(`(?i)(curl|wget|fetch)\s+.*\{`),
		regexp.MustCompile(`(?i)send\s+(data|info|secret)\s+to\s+`),
	}

	persistencePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(eval|exec|subprocess)\s*\(`),
		regexp.MustCompile(`(?i)write\s+to\s+.*(\.bashrc|\.zshrc|\.profile|startup)`),
		regexp.MustCompile(`(?i)add\s+to\s+(crontab|startup|autostart)`),
	}
)

// ScanForInjection checks content for prompt injection, data exfiltration,
// persistence backdoors, and invisible unicode characters.
func ScanForInjection(content string) []SecurityWarning {
	var warnings []SecurityWarning
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNum := i + 1

		for _, p := range injectionPatterns {
			if p.MatchString(line) {
				warnings = append(warnings, SecurityWarning{
					Type:    "injection",
					Detail:  p.FindString(line),
					LineNum: lineNum,
				})
			}
		}

		for _, p := range exfiltrationPatterns {
			if p.MatchString(line) {
				warnings = append(warnings, SecurityWarning{
					Type:    "exfiltration",
					Detail:  p.FindString(line),
					LineNum: lineNum,
				})
			}
		}

		for _, p := range persistencePatterns {
			if p.MatchString(line) {
				warnings = append(warnings, SecurityWarning{
					Type:    "persistence",
					Detail:  p.FindString(line),
					LineNum: lineNum,
				})
			}
		}
	}

	for _, r := range content {
		if isInvisibleUnicode(r) {
			warnings = append(warnings, SecurityWarning{
				Type:   "unicode",
				Detail: "invisible character detected",
			})
			break
		}
	}

	return warnings
}

func isInvisibleUnicode(r rune) bool {
	// Zero-width and invisible characters
	invisibleRunes := map[rune]bool{
		0x200B: true, // Zero Width Space
		0x200C: true, // Zero Width Non-Joiner
		0x200D: true, // Zero Width Joiner
		0x2060: true, // Word Joiner
		0x2061: true, // Function Application
		0x2062: true, // Invisible Times
		0x2063: true, // Invisible Separator
		0x2064: true, // Invisible Plus
		0xFEFF: true, // BOM / Zero Width No-Break Space
		0x00AD: true, // Soft Hyphen
		0x034F: true, // Combining Grapheme Joiner
		0x061C: true, // Arabic Letter Mark
		0x115F: true, // Hangul Choseong Filler
		0x1160: true, // Hangul Jungseong Filler
		0x17B4: true, // Khmer Vowel Inherent Aq
		0x17B5: true, // Khmer Vowel Inherent Aa
		0x180E: true, // Mongolian Vowel Separator
	}
	if invisibleRunes[r] {
		return true
	}
	if unicode.Is(unicode.Bidi_Control, r) {
		return true
	}
	return false
}
