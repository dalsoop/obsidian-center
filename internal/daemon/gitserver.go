package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type GitServer struct {
	cmd     *exec.Cmd
	dataDir string
	sshPort string
}

// StartSoftServe starts soft-serve as a child process
func StartSoftServe(ctx context.Context, dataDir, sshPort string) (*GitServer, error) {
	softBin, err := exec.LookPath("soft")
	if err != nil {
		return nil, nil // not installed, skip
	}

	os.MkdirAll(dataDir, 0755)

	cmd := exec.CommandContext(ctx, softBin, "serve")
	cmd.Env = append(os.Environ(),
		"SOFT_SERVE_DATA_PATH="+dataDir,
		"SOFT_SERVE_SSH_LISTEN_ADDR=:"+sshPort,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start soft-serve: %w", err)
	}

	time.Sleep(2 * time.Second)
	log.Printf("[soft-serve] started on ssh://localhost:%s data=%s", sshPort, dataDir)

	return &GitServer{cmd: cmd, dataDir: dataDir, sshPort: sshPort}, nil
}

// EnsureRepo creates a repository if it doesn't exist
func (gs *GitServer) EnsureRepo(name string) (string, error) {
	cmd := exec.Command("ssh", "-p", gs.sshPort, "-o", "StrictHostKeyChecking=no",
		"localhost", "repo", "create", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[soft-serve] repo create %s: %s", name, string(out))
	}
	cloneURL := fmt.Sprintf("ssh://localhost:%s/%s", gs.sshPort, name)
	return cloneURL, nil
}

// ListRepos returns all repos via SSH
func (gs *GitServer) ListRepos() []string {
	cmd := exec.Command("ssh", "-p", gs.sshPort, "-o", "StrictHostKeyChecking=no",
		"localhost", "repo", "list")
	out, _ := cmd.Output()
	var repos []string
	for _, line := range splitLines(string(out)) {
		if line != "" {
			repos = append(repos, line)
		}
	}
	return repos
}

// CloneURL returns the SSH clone URL for a repo
func (gs *GitServer) CloneURL(name string) string {
	return fmt.Sprintf("ssh://localhost:%s/%s", gs.sshPort, name)
}

// DataDir returns the soft-serve data directory
func (gs *GitServer) DataDir() string {
	return gs.dataDir
}

// Stop stops soft-serve
func (gs *GitServer) Stop() {
	if gs.cmd != nil && gs.cmd.Process != nil {
		gs.cmd.Process.Kill()
	}
}

func softServeDataPath() string {
	if p := os.Getenv("SOFT_SERVE_DATA_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "문서", "시스템", "obsidian-center", "soft-serve")
}

func splitLines(s string) []string {
	var lines []string
	for _, l := range []byte(s) {
		if l == '\n' {
			lines = append(lines, "")
		}
	}
	// simple split
	result := make([]string, 0)
	start := 0
	for i, c := range s {
		if c == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
