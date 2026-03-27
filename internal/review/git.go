package review

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type GitOps struct {
	repo     *git.Repository
	vaultDir string
}

func NewGitOps(vaultDir string) (*GitOps, error) {
	repo, err := git.PlainOpen(vaultDir)
	if err != nil {
		return nil, fmt.Errorf("git open: %w", err)
	}
	return &GitOps{repo: repo, vaultDir: vaultDir}, nil
}

// CreateBranch creates a new branch for a draft note
// drafts/YYMMDD-title
func (g *GitOps) CreateBranch(noteID string) error {
	branchName := fmt.Sprintf("drafts/%s", noteID)

	head, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("get HEAD: %w", err)
	}

	ref := plumbing.NewBranchReferenceName(branchName)
	newRef := plumbing.NewHashReference(ref, head.Hash())

	return g.repo.Storer.SetReference(newRef)
}

// Checkout switches to a branch
func (g *GitOps) Checkout(branchName string) error {
	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}

	return w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
}

// CommitDraft commits the draft note on its branch
func (g *GitOps) CommitDraft(noteID, title, author string) error {
	branchName := fmt.Sprintf("drafts/%s", noteID)

	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}

	// Checkout draft branch
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: false,
	})
	if err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	// Stage all
	_, err = w.Add(".")
	if err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Commit
	_, err = w.Commit(fmt.Sprintf("draft: %s", title), &git.CommitOptions{
		Author: &object.Signature{
			Name:  author,
			Email: fmt.Sprintf("%s@obsidian-center", author),
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// MergeBranch merges a draft branch into main
func (g *GitOps) MergeBranch(noteID string) error {
	branchName := fmt.Sprintf("drafts/%s", noteID)

	// Checkout main
	w, err := g.repo.Worktree()
	if err != nil {
		return err
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	})
	if err != nil {
		return fmt.Errorf("checkout main: %w", err)
	}

	// Get branch commit
	branchRef, err := g.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return fmt.Errorf("get branch ref: %w", err)
	}

	// Merge (fast-forward)
	head, err := g.repo.Head()
	if err != nil {
		return err
	}

	// Create merge commit
	branchCommit, err := g.repo.CommitObject(branchRef.Hash())
	if err != nil {
		return err
	}

	_, err = w.Commit(fmt.Sprintf("merge: %s (approved)", noteID), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "obsidian-center",
			Email: "obsidian-center@local",
			When:  time.Now(),
		},
		Parents: []plumbing.Hash{head.Hash(), branchCommit.Hash},
	})

	return err
}

// DeleteBranch removes a draft branch after merge
func (g *GitOps) DeleteBranch(noteID string) error {
	branchName := fmt.Sprintf("drafts/%s", noteID)
	return g.repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(branchName))
}

// ListBranches returns all draft branches
func (g *GitOps) ListBranches() ([]string, error) {
	var branches []string

	refs, err := g.repo.References()
	if err != nil {
		return nil, err
	}

	refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if ref.Name().IsBranch() {
			branches = append(branches, name)
		}
		return nil
	})

	return branches, nil
}

// Push pushes to origin
func (g *GitOps) Push() error {
	return g.repo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{"refs/heads/*:refs/heads/*"},
	})
}
