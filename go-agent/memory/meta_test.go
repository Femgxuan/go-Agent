package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileMetaMemory_Record(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewFileMetaMemory(tmpDir)

	ctx := context.Background()
	reflection := Reflection{
		ID:        "ref-1",
		Content:   "用户喜欢简洁的回答",
		Trigger:   "user_feedback",
		CreatedAt: time.Now(),
	}

	err := mm.Record(ctx, reflection)
	require.NoError(t, err)

	// 验证可以检索到
	refs, err := mm.Recent(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, "用户喜欢简洁的回答", refs[0].Content)
}

func TestFileMetaMemory_Recent(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewFileMetaMemory(tmpDir)

	ctx := context.Background()

	// 添加多条反思
	for i := 0; i < 5; i++ {
		mm.Record(ctx, Reflection{
			ID:        "ref-" + string(rune('0'+i)),
			Content:   "反思内容",
			Trigger:   "test",
			CreatedAt: time.Now(),
		})
	}

	// 限制返回3条
	refs, err := mm.Recent(ctx, 3)
	require.NoError(t, err)
	assert.Len(t, refs, 3)
}

func TestFileMetaMemory_Search(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewFileMetaMemory(tmpDir)

	ctx := context.Background()

	mm.Record(ctx, Reflection{
		ID:        "ref-1",
		Content:   "用户喜欢Go语言",
		Trigger:   "user_feedback",
		CreatedAt: time.Now(),
	})
	mm.Record(ctx, Reflection{
		ID:        "ref-2",
		Content:   "用户不喜欢Python",
		Trigger:   "user_feedback",
		CreatedAt: time.Now(),
	})

	// 搜索包含"Go"的反思
	refs, err := mm.Search(ctx, "Go", 10)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, "用户喜欢Go语言", refs[0].Content)
}
