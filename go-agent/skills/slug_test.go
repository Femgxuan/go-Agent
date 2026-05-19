package skills

import "testing"

func TestSlugifyBasicASCII(t *testing.T) {
	got := Slugify("Code Review")
	want := "code-review"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "Code Review", got, want)
	}
}

func TestSlugifyChinese(t *testing.T) {
	got := Slugify("代码审查")
	want := "代码审查"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "代码审查", got, want)
	}
}

func TestSlugifyMixed(t *testing.T) {
	got := Slugify("code review 代码")
	want := "code-review-代码"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "code review 代码", got, want)
	}
}

func TestSlugifySpecialChars(t *testing.T) {
	got := Slugify("hello@world!")
	want := "hello-world"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "hello@world!", got, want)
	}
}

func TestSlugifyMultipleSpacesHyphens(t *testing.T) {
	got := Slugify("a--b  c")
	want := "a-b-c"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "a--b  c", got, want)
	}
}

func TestSlugifyEmpty(t *testing.T) {
	got := Slugify("")
	want := "unnamed-skill"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "", got, want)
	}
}

func TestSlugifyUnderscoreAndDot(t *testing.T) {
	got := Slugify("my_skill.name")
	want := "my-skill-name"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "my_skill.name", got, want)
	}
}

func TestSlugifyLeadingTrailingSpecial(t *testing.T) {
	got := Slugify("--hello--")
	want := "hello"
	if got != want {
		t.Errorf("Slugify(%q) = %q, want %q", "--hello--", got, want)
	}
}

func TestSlugifyLongString(t *testing.T) {
	long := "this is a very long skill name that should be truncated because it exceeds sixty four characters in total length"
	got := Slugify(long)
	if len(got) > 64 {
		t.Errorf("Slugify(long) length = %d, want <= 64", len(got))
	}
	if got[len(got)-1] == '-' {
		t.Errorf("Slugify(long) ends with hyphen: %q", got)
	}
}

func TestUniqueSlugNoConflict(t *testing.T) {
	got := UniqueSlug("Code Review", nil)
	want := "code-review"
	if got != want {
		t.Errorf("UniqueSlug(%q, nil) = %q, want %q", "Code Review", got, want)
	}
}

func TestUniqueSlugWithConflict(t *testing.T) {
	got := UniqueSlug("Code Review", []string{"code-review"})
	want := "code-review-2"
	if got != want {
		t.Errorf("UniqueSlug with conflict = %q, want %q", got, want)
	}
}

func TestUniqueSlugWithMultipleConflicts(t *testing.T) {
	got := UniqueSlug("Code Review", []string{"code-review", "code-review-2"})
	want := "code-review-3"
	if got != want {
		t.Errorf("UniqueSlug with multiple conflicts = %q, want %q", got, want)
	}
}

func TestIsValidSlugChar(t *testing.T) {
	cases := []struct {
		r    rune
		want bool
	}{
		{'a', true},
		{'z', true},
		{'0', true},
		{'9', true},
		{'-', true},
		{'中', true},
		{'@', false},
		{' ', false},
		{'_', false},
	}
	for _, tc := range cases {
		got := IsValidSlugChar(tc.r)
		if got != tc.want {
			t.Errorf("IsValidSlugChar(%q) = %v, want %v", tc.r, got, tc.want)
		}
	}
}
