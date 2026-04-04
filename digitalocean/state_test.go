package digitalocean

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tempStatePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state.json")
}

func TestStateManager_LoadAll_Empty(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	states, err := sm.LoadAll()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestStateManager_AddAndGet(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	state := DropletState{Name: "web", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}
	require.NoError(t, sm.Add(state))
	got, err := sm.Get("web")
	require.NoError(t, err)
	assert.Equal(t, "123", got.DropletID)
	assert.Equal(t, "active", got.State)
}

func TestStateManager_Add_Duplicate(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	state := DropletState{Name: "web", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}
	require.NoError(t, sm.Add(state))
	err := sm.Add(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStateManager_Get_NotFound(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	_, err := sm.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateManager_UpdateState(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	state := DropletState{Name: "web", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}
	require.NoError(t, sm.Add(state))
	require.NoError(t, sm.UpdateState("web", "off"))
	got, err := sm.Get("web")
	require.NoError(t, err)
	assert.Equal(t, "off", got.State)
}

func TestStateManager_UpdateState_NotFound(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	err := sm.UpdateState("nonexistent", "off")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateManager_Remove(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	state := DropletState{Name: "web", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}
	require.NoError(t, sm.Add(state))
	require.NoError(t, sm.Remove("web"))
	_, err := sm.Get("web")
	assert.Error(t, err)
}

func TestStateManager_Remove_NotFound(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	err := sm.Remove("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateManager_MultipleDroplets(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	require.NoError(t, sm.Add(DropletState{Name: "web", DropletID: "1", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}))
	require.NoError(t, sm.Add(DropletState{Name: "api", DropletID: "2", State: "active", Provider: "digitalocean", Region: "sfo3", CreatedAt: "2026-03-20T01:00:00Z"}))
	states, err := sm.LoadAll()
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestStateManager_CorruptFile(t *testing.T) {
	path := tempStatePath(t)
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))
	sm := NewStateManagerWithPath(path)
	_, err := sm.LoadAll()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
