package digitalocean

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeExecCommand(exitCode int, stdout string) func(string, ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", exitCode),
			fmt.Sprintf("GO_HELPER_STDOUT=%s", stdout),
		)
		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	exitCode := 0
	if code := os.Getenv("GO_HELPER_EXIT_CODE"); code != "" {
		fmt.Sscanf(code, "%d", &exitCode)
	}
	stdout := os.Getenv("GO_HELPER_STDOUT")
	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}
	os.Exit(exitCode)
}

func TestUp_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()

	doctlResponse := `[{"id":12345678}]`
	ExecCommand = fakeExecCommand(0, doctlResponse)

	sm := NewStateManagerWithPath(tempStatePath(t))
	mgr := NewWithState(sm)

	config := &DropletConfig{
		Name:   "test-droplet",
		Image:  "ubuntu-22-04-x64",
		Region: "nyc1",
		Size:   "s-1vcpu-1gb",
	}

	state, err := mgr.Up(config)
	require.NoError(t, err)
	assert.Equal(t, "test-droplet", state.Name)
	assert.Equal(t, "12345678", state.DropletID)
	assert.Equal(t, "active", state.State)
	assert.Equal(t, "digitalocean", state.Provider)
	assert.Equal(t, "nyc1", state.Region)
}

func TestUp_MissingName(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	mgr := NewWithState(sm)
	config := &DropletConfig{Image: "ubuntu-22-04-x64"}
	_, err := mgr.Up(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "droplet name is required")
}

func TestUp_MissingImage(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	mgr := NewWithState(sm)
	config := &DropletConfig{Name: "test"}
	_, err := mgr.Up(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image is required")
}

func TestUp_Defaults(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, `[{"id":1}]`)

	sm := NewStateManagerWithPath(tempStatePath(t))
	mgr := NewWithState(sm)
	config := &DropletConfig{Name: "test", Image: "ubuntu-22-04-x64"}
	state, err := mgr.Up(config)
	require.NoError(t, err)
	assert.Equal(t, "nyc1", state.Region)
}

func TestHalt_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, `[{"id":1,"status":"completed"}]`)

	sm := NewStateManagerWithPath(tempStatePath(t))
	require.NoError(t, sm.Add(DropletState{Name: "test", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}))

	mgr := NewWithState(sm)
	err := mgr.Halt("test")
	require.NoError(t, err)

	got, err := sm.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "off", got.State)
}

func TestDestroy_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, "")

	sm := NewStateManagerWithPath(tempStatePath(t))
	require.NoError(t, sm.Add(DropletState{Name: "test", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}))

	mgr := NewWithState(sm)
	err := mgr.Destroy("test")
	require.NoError(t, err)

	_, err = sm.Get("test")
	assert.Error(t, err)
}

func TestStatus_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, "active\n")

	sm := NewStateManagerWithPath(tempStatePath(t))
	require.NoError(t, sm.Add(DropletState{Name: "test", DropletID: "123", State: "active", Provider: "digitalocean", Region: "nyc1", CreatedAt: "2026-03-20T00:00:00Z"}))

	mgr := NewWithState(sm)
	state, err := mgr.Status("test")
	require.NoError(t, err)
	assert.Equal(t, "test", state.Name)
}

func TestStatusAll_Empty(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	mgr := NewWithState(sm)
	states, err := mgr.StatusAll()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestParseDropletID_Success(t *testing.T) {
	output := []byte(`[{"id":12345678}]`)
	id, err := parseDropletID(output)
	require.NoError(t, err)
	assert.Equal(t, "12345678", id)
}

func TestParseDropletID_NoDroplets(t *testing.T) {
	output := []byte(`[]`)
	_, err := parseDropletID(output)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no droplets")
}

func TestParseDropletID_InvalidJSON(t *testing.T) {
	_, err := parseDropletID([]byte(`not json`))
	assert.Error(t, err)
}

func TestBuildCreateArgs(t *testing.T) {
	mgr := &manager{}
	config := &DropletConfig{
		Name:    "test",
		Region:  "nyc1",
		Size:    "s-1vcpu-1gb",
		Image:   "ubuntu-22-04-x64",
		SSHKeys: []string{"key1", "key2"},
		VpcUUID: "vpc-123",
		Tags:    []string{"web", "prod"},
	}

	args := mgr.buildCreateArgs(config)
	assert.Contains(t, args, "test")
	assert.Contains(t, args, "--region")
	assert.Contains(t, args, "nyc1")
	assert.Contains(t, args, "--size")
	assert.Contains(t, args, "s-1vcpu-1gb")
	assert.Contains(t, args, "--image")
	assert.Contains(t, args, "ubuntu-22-04-x64")
	assert.Contains(t, args, "--ssh-keys")
	assert.Contains(t, args, "key1,key2")
	assert.Contains(t, args, "--vpc-uuid")
	assert.Contains(t, args, "vpc-123")
	assert.Contains(t, args, "--tag-names")
	assert.Contains(t, args, "web,prod")
}

func TestBuildCreateArgs_Minimal(t *testing.T) {
	mgr := &manager{}
	config := &DropletConfig{
		Name:   "test",
		Region: "nyc1",
		Size:   "s-1vcpu-1gb",
		Image:  "ubuntu-22-04-x64",
	}
	args := mgr.buildCreateArgs(config)
	assert.NotContains(t, args, "--ssh-keys")
	assert.NotContains(t, args, "--vpc-uuid")
	assert.NotContains(t, args, "--tag-names")
}
