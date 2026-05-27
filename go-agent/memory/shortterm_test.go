package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileShortTermMemory_Save(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	interaction := Interaction{
		SessionID: sid,
		UserMsg:   "hello",
		AgentMsg:  "hi there",
		Metadata:  map[string]any{"timestamp": time.Now()},
	}

	err := stm.Save(ctx, sid, interaction)
	require.NoError(t, err)

	// Verify file was created.
	_, err = os.Stat(filepath.Join(tmpDir, string(sid)+".json"))
	assert.NoError(t, err)
}

func TestFileShortTermMemory_Load(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	// Save two interactions.
	stm.Save(ctx, sid, Interaction{
		SessionID: sid,
		UserMsg:   "msg1",
		AgentMsg:  "reply1",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})
	stm.Save(ctx, sid, Interaction{
		SessionID: sid,
		UserMsg:   "msg2",
		AgentMsg:  "reply2",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})

	// Load.
	interactions, err := stm.Load(ctx, sid, 0)
	require.NoError(t, err)
	assert.Len(t, interactions, 2)
	assert.Equal(t, "msg1", interactions[0].UserMsg)
	assert.Equal(t, "msg2", interactions[1].UserMsg)
}

func TestFileShortTermMemory_LoadWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	// Save three interactions.
	for i := 0; i < 3; i++ {
		stm.Save(ctx, sid, Interaction{
			SessionID: sid,
			UserMsg:   "msg",
			AgentMsg:  "reply",
			Metadata:  map[string]any{"timestamp": time.Now()},
		})
	}

	// Load with limit of 2.
	interactions, err := stm.Load(ctx, sid, 2)
	require.NoError(t, err)
	assert.Len(t, interactions, 2)
}

func TestFileShortTermMemory_ListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()

	// Create two sessions.
	stm.Save(ctx, SessionID("sess-1"), Interaction{
		SessionID: SessionID("sess-1"),
		UserMsg:   "hello",
		AgentMsg:  "hi",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})
	stm.Save(ctx, SessionID("sess-2"), Interaction{
		SessionID: SessionID("sess-2"),
		UserMsg:   "world",
		AgentMsg:  "earth",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})

	sessions, err := stm.ListSessions(ctx, "")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestFileShortTermMemory_DeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	// Save and delete.
	stm.Save(ctx, sid, Interaction{
		SessionID: sid,
		UserMsg:   "hello",
		AgentMsg:  "hi",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})

	err := stm.DeleteSession(ctx, sid)
	require.NoError(t, err)

	// Verify file was deleted.
	_, err = os.Stat(filepath.Join(tmpDir, string(sid)+".json"))
	assert.True(t, os.IsNotExist(err))
}
