package daemon

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sosedoff/gitkit"
)

type GitServer struct {
	service *gitkit.Server
	repoDir string
}

func NewGitServer(repoDir string) (*GitServer, error) {
	// bare repo 디렉토리
	os.MkdirAll(repoDir, 0755)

	service := gitkit.New(gitkit.Config{
		Dir:        repoDir,
		AutoCreate: true,
		AutoHooks:  false,
	})

	if err := service.Setup(); err != nil {
		return nil, fmt.Errorf("gitkit setup: %w", err)
	}

	// hook: push/pull 로그
	service.AuthFunc = func(cred gitkit.Credential, req *gitkit.Request) (bool, error) {
		log.Printf("[git-server] %s by %s", req.RepoName, cred.Username)
		return true, nil // 인증 없이 허용
	}

	return &GitServer{
		service: service,
		repoDir: repoDir,
	}, nil
}

// Handler returns http.Handler for git operations
// Mount at /git/ path
func (gs *GitServer) Handler() http.Handler {
	return gs.service
}

// InitRepo creates a bare git repo for a vault
func (gs *GitServer) InitRepo(name string) (string, error) {
	repoPath := filepath.Join(gs.repoDir, name+".git")
	if _, err := os.Stat(repoPath); err == nil {
		return repoPath, nil // already exists
	}

	// git init --bare
	cmd := exec.Command("git", "init", "--bare", repoPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("init bare: %w", err)
	}

	log.Printf("[git-server] repo created: %s", repoPath)
	return repoPath, nil
}

// ListRepos returns all repos
func (gs *GitServer) ListRepos() []string {
	var repos []string
	entries, _ := os.ReadDir(gs.repoDir)
	for _, e := range entries {
		if e.IsDir() && filepath.Ext(e.Name()) == ".git" {
			repos = append(repos, e.Name())
		}
	}
	return repos
}
