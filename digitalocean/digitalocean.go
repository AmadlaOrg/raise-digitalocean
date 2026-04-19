package digitalocean

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var ExecCommand = exec.Command

// DropletConfig holds the configuration for a DigitalOcean Droplet.
type DropletConfig struct {
	Name          string   `json:"name" yaml:"name"`
	Region        string   `json:"region" yaml:"region"`
	Size          string   `json:"size" yaml:"size"`
	Image         string   `json:"image" yaml:"image"`
	SSHKeys       []string `json:"ssh_keys" yaml:"ssh_keys"`
	VpcUUID       string   `json:"vpc_uuid" yaml:"vpc_uuid"`
	Tags          []string `json:"tags" yaml:"tags"`
	SSHUser       string   `json:"ssh_user" yaml:"ssh_user"`
	SSHPort       int      `json:"ssh_port" yaml:"ssh_port"`
	SSHPrivateKey string   `json:"ssh_private_key" yaml:"ssh_private_key"`
}

// DropletState represents the current state of a managed Droplet.
type DropletState struct {
	Name      string `json:"name"`
	DropletID string `json:"droplet_id"`
	State     string `json:"state"`
	Provider  string `json:"provider"`
	Region    string `json:"region"`
	PublicIP  string `json:"public_ip,omitempty"`
	CreatedAt string `json:"created_at"`
}

// Manager defines the interface for managing DigitalOcean Droplets.
type Manager interface {
	Up(config *DropletConfig) (*DropletState, error)
	Halt(name string) error
	Destroy(name string) error
	SSH(name string, user string, port int, keyFile string) error
	Status(name string) (*DropletState, error)
	StatusAll() ([]DropletState, error)
}

type manager struct {
	stateManager StateManager
}

func New() Manager {
	return &manager{stateManager: NewStateManager()}
}

func NewWithState(sm StateManager) Manager {
	return &manager{stateManager: sm}
}

func (m *manager) Up(config *DropletConfig) (*DropletState, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("droplet name is required")
	}
	if config.Image == "" {
		return nil, fmt.Errorf("image is required")
	}

	if config.Region == "" {
		config.Region = "nyc1"
	}
	if config.Size == "" {
		config.Size = "s-1vcpu-1gb"
	}
	if config.SSHUser == "" {
		config.SSHUser = "root"
	}
	if config.SSHPort <= 0 {
		config.SSHPort = 22
	}

	args := m.buildCreateArgs(config)

	cmd := ExecCommand("doctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("doctl compute droplet create failed: %s: %w", string(output), err)
	}

	dropletID, err := parseDropletID(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse droplet ID: %w", err)
	}

	state := &DropletState{
		Name:      config.Name,
		DropletID: dropletID,
		State:     "active",
		Provider:  "digitalocean",
		Region:    config.Region,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	ip, _ := m.getDropletIP(dropletID)
	state.PublicIP = ip

	if err := m.stateManager.Add(*state); err != nil {
		return nil, fmt.Errorf("droplet created but failed to save state: %w", err)
	}

	return state, nil
}

func (m *manager) Halt(name string) error {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return fmt.Errorf("droplet %q not found in state: %w", name, err)
	}

	cmd := ExecCommand("doctl", "compute", "droplet-action", "power-off", stored.DropletID, "--output", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("doctl power-off failed: %s: %w", string(output), err)
	}

	if err := m.stateManager.UpdateState(name, "off"); err != nil {
		return fmt.Errorf("droplet powered off but failed to update state: %w", err)
	}

	return nil
}

func (m *manager) Destroy(name string) error {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return fmt.Errorf("droplet %q not found in state: %w", name, err)
	}

	cmd := ExecCommand("doctl", "compute", "droplet", "delete", stored.DropletID, "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("doctl delete failed: %s: %w", string(output), err)
	}

	if err := m.stateManager.Remove(name); err != nil {
		return fmt.Errorf("droplet destroyed but failed to update state: %w", err)
	}

	return nil
}

func (m *manager) SSH(name string, user string, port int, keyFile string) error {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return fmt.Errorf("droplet %q not found in state: %w", name, err)
	}

	ip := stored.PublicIP
	if ip == "" {
		refreshedIP, err := m.getDropletIP(stored.DropletID)
		if err != nil || refreshedIP == "" {
			return fmt.Errorf("droplet %q has no public IP address", name)
		}
		ip = refreshedIP
	}

	if user == "" {
		user = "root"
	}
	if port <= 0 {
		port = 22
	}

	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=" + os.DevNull,
		"-p", fmt.Sprintf("%d", port),
	}
	if keyFile != "" {
		sshArgs = append(sshArgs, "-i", keyFile)
	}
	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", user, ip))

	cmd := ExecCommand("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *manager) Status(name string) (*DropletState, error) {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return nil, fmt.Errorf("droplet %q not found in state: %w", name, err)
	}

	state, err := m.describeDroplet(stored.DropletID)
	if err != nil {
		return stored, nil
	}

	stored.State = state
	ip, _ := m.getDropletIP(stored.DropletID)
	stored.PublicIP = ip
	_ = m.stateManager.UpdateState(name, state)

	return stored, nil
}

func (m *manager) StatusAll() ([]DropletState, error) {
	states, err := m.stateManager.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}
	if len(states) == 0 {
		return []DropletState{}, nil
	}
	return states, nil
}

func (m *manager) buildCreateArgs(config *DropletConfig) []string {
	args := []string{
		"compute", "droplet", "create", config.Name,
		"--region", config.Region,
		"--size", config.Size,
		"--image", config.Image,
		"--output", "json",
		"--wait",
	}

	if len(config.SSHKeys) > 0 {
		args = append(args, "--ssh-keys", strings.Join(config.SSHKeys, ","))
	}

	if config.VpcUUID != "" {
		args = append(args, "--vpc-uuid", config.VpcUUID)
	}

	if len(config.Tags) > 0 {
		args = append(args, "--tag-names", strings.Join(config.Tags, ","))
	}

	return args
}

func (m *manager) getDropletIP(dropletID string) (string, error) {
	cmd := ExecCommand("doctl", "compute", "droplet", "get", dropletID, "--format", "PublicIPv4", "--no-header")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(output))
	if ip == "" {
		return "", nil
	}
	return ip, nil
}

func (m *manager) describeDroplet(dropletID string) (string, error) {
	cmd := ExecCommand("doctl", "compute", "droplet", "get", dropletID, "--format", "Status", "--no-header")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// parseDropletID extracts the droplet ID from doctl JSON output.
func parseDropletID(output []byte) (string, error) {
	var droplets []struct {
		ID int `json:"id"`
	}

	if err := json.Unmarshal(output, &droplets); err != nil {
		return "", fmt.Errorf("failed to parse doctl response: %w", err)
	}

	if len(droplets) == 0 {
		return "", fmt.Errorf("no droplets in doctl response")
	}

	return fmt.Sprintf("%d", droplets[0].ID), nil
}

func OutputJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
