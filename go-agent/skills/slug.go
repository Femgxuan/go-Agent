package skills

import (
	"strings"
	"unicode"
)

func IsValidSlugChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		r == '-' ||
		(r >= 0x4E00 && r <= 0x9FFF)
}

func Slugify(name string) string {
	name = strings.ToLower(name)

	var b strings.Builder
	for _, r := range name {
		if IsValidSlugChar(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '.' || unicode.IsSpace(r) {
			b.WriteByte('-')
		} else {
			b.WriteByte('-')
		}
	}

	slug := collapseHyphens(b.String())
	slug = strings.Trim(slug, "-")

	if slug == "" {
		return "unnamed-skill"
	}

	if len(slug) > 64 {
		slug = truncateAt64(slug)
	}

	return slug
}

func collapseHyphens(s string) string {
	var b strings.Builder
	prev := false
	for _, r := range s {
		if r == '-' {
			if !prev {
				b.WriteByte('-')
			}
			prev = true
		} else {
			b.WriteRune(r)
			prev = false
		}
	}
	return b.String()
}

// Truncate to at most 64 bytes, cutting at the last safe boundary
// (hyphen or before a multi-byte char) to avoid splitting words/chars.
func truncateAt64(s string) string {
	if len(s) <= 64 {
		return s
	}
	cut := 64
	for cut > 0 && !isCharBoundary(s, cut) {
		cut--
	}
	result := s[:cut]
	result = strings.TrimRight(result, "-")
	if result == "" {
		return "unnamed-skill"
	}
	return result
}

func isCharBoundary(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	return (s[i] & 0xC0) != 0x80
}

func UniqueSlug(name string, existingNames []string) string {
	slug := Slugify(name)
	existing := make(map[string]struct{}, len(existingNames))
	for _, n := range existingNames {
		existing[n] = struct{}{}
	}

	if _, ok := existing[slug]; !ok {
		return slug
	}

	for i := 2; ; i++ {
		candidate := slug + "-" + itoa(i)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
