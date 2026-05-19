package skills

import (
	"testing"
)

func TestDetectCreateSkillIntent_Chinese(t *testing.T) {
	result := DetectCreateSkillIntent("把这个流程保存成skill")
	if !result.Detected {
		t.Error("expected Detected=true for Chinese phrase")
	}
}

func TestDetectCreateSkillIntent_English(t *testing.T) {
	result := DetectCreateSkillIntent("save this as a skill")
	if !result.Detected {
		t.Error("expected Detected=true for English phrase")
	}
}

func TestDetectCreateSkillIntent_WithName(t *testing.T) {
	result := DetectCreateSkillIntent("创建skill叫code-review")
	if !result.Detected {
		t.Error("expected Detected=true")
	}
	if result.SkillName != "code-review" {
		t.Errorf("expected SkillName='code-review', got '%s'", result.SkillName)
	}
}

func TestDetectCreateSkillIntent_WithName_English(t *testing.T) {
	result := DetectCreateSkillIntent("create a skill called my-helper")
	if !result.Detected {
		t.Error("expected Detected=true")
	}
	if result.SkillName != "my-helper" {
		t.Errorf("expected SkillName='my-helper', got '%s'", result.SkillName)
	}
}

func TestDetectCreateSkillIntent_NotDetected(t *testing.T) {
	result := DetectCreateSkillIntent("tell me about skills")
	if result.Detected {
		t.Error("expected Detected=false for non-creation phrase")
	}
}

func TestDetectCreateSkillIntent_NotDetected2(t *testing.T) {
	result := DetectCreateSkillIntent("what skills do you have")
	if result.Detected {
		t.Error("expected Detected=false for non-creation phrase")
	}
}

func TestDetectCreateSkillIntent_WithKeywords(t *testing.T) {
	result := DetectCreateSkillIntent("创建skill，标签：review,code")
	if !result.Detected {
		t.Error("expected Detected=true")
	}
	if len(result.Keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d: %v", len(result.Keywords), result.Keywords)
	}
	if result.Keywords[0] != "review" || result.Keywords[1] != "code" {
		t.Errorf("expected keywords ['review','code'], got %v", result.Keywords)
	}
}

func TestDetectCreateSkillIntent_Regex(t *testing.T) {
	result := DetectCreateSkillIntent("can you make this a skill")
	if !result.Detected {
		t.Error("expected Detected=true for regex match")
	}
}

func TestDetectCreateSkillIntent_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		detected  bool
		skillName string
		keywords  []string
	}{
		{
			name:     "Chinese: 沉淀为技能",
			input:    "把这个沉淀为技能吧",
			detected: true,
		},
		{
			name:     "Chinese: 生成skill",
			input:    "帮我生成skill",
			detected: true,
		},
		{
			name:     "Chinese: 新建技能",
			input:    "新建技能",
			detected: true,
		},
		{
			name:     "Chinese: 添加skill",
			input:    "添加skill到列表",
			detected: true,
		},
		{
			name:     "English: new skill",
			input:    "I want a new skill",
			detected: true,
		},
		{
			name:     "English: add skill",
			input:    "add skill for deployment",
			detected: true,
		},
		{
			name:     "Regex: build a skill",
			input:    "build a skill for this",
			detected: true,
		},
		{
			name:     "Regex: generate this as skill",
			input:    "generate this as skill",
			detected: true,
		},
		{
			name:     "Regex: save this skill (case insensitive)",
			input:    "Save This As A Skill",
			detected: true,
		},
		{
			name:     "Not detected: just mentions skill",
			input:    "how do I use this skill",
			detected: false,
		},
		{
			name:     "Not detected: list skills",
			input:    "list all skills",
			detected: false,
		},
		{
			name:     "Not detected: run skill",
			input:    "run the skill now",
			detected: false,
		},
		{
			name:     "Not detected: empty input",
			input:    "",
			detected: false,
		},
		{
			name:      "Name with 命名为",
			input:     "创建skill命名为my-deploy",
			detected:  true,
			skillName: "my-deploy",
		},
		{
			name:      "Name with 名字是",
			input:     "保存成skill，名字是quick-fix",
			detected:  true,
			skillName: "quick-fix",
		},
		{
			name:      "Name with named",
			input:     "create a skill named auto-test",
			detected:  true,
			skillName: "auto-test",
		},
		{
			name:      "Name with name it",
			input:     "save as skill, name it deploy-helper",
			detected:  true,
			skillName: "deploy-helper",
		},
		{
			name:     "Keywords with English tags",
			input:    "create skill, tags: deploy, ci, test",
			detected: true,
			keywords: []string{"deploy", "ci", "test"},
		},
		{
			name:     "Keywords with 关键词",
			input:    "创建skill，关键词：部署,测试",
			detected: true,
			keywords: []string{"部署", "测试"},
		},
		{
			name:      "Name and keywords together",
			input:     "创建skill叫my-tool，标签：review,lint",
			detected:  true,
			skillName: "my-tool",
			keywords:  []string{"review", "lint"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCreateSkillIntent(tt.input)
			if result.Detected != tt.detected {
				t.Errorf("Detected: got %v, want %v", result.Detected, tt.detected)
			}
			if tt.skillName != "" && result.SkillName != tt.skillName {
				t.Errorf("SkillName: got '%s', want '%s'", result.SkillName, tt.skillName)
			}
			if tt.keywords != nil {
				if len(result.Keywords) != len(tt.keywords) {
					t.Fatalf("Keywords length: got %d, want %d (%v)", len(result.Keywords), len(tt.keywords), result.Keywords)
				}
				for i, k := range tt.keywords {
					if result.Keywords[i] != k {
						t.Errorf("Keywords[%d]: got '%s', want '%s'", i, result.Keywords[i], k)
					}
				}
			}
		})
	}
}
