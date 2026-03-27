package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dalsoop/obsidian-center/internal/note"
	"github.com/dalsoop/obsidian-center/internal/review"
	"github.com/dalsoop/obsidian-center/internal/store"
)

type Daemon struct {
	store     *store.Store
	git       *review.GitOps
	gitServer *GitServer
	vaultDir  string
	addr      string
}

func New(vaultDir, dataDir, addr string) *Daemon {
	gitOps, err := review.NewGitOps(vaultDir)
	if err != nil {
		fmt.Printf("warning: git not available: %s\n", err)
	}

	gitSrv, err2 := StartSoftServe(context.Background(), softServeDataPath(), "23231")
	if err2 != nil {
		fmt.Printf("warning: soft-serve not available: %s\n", err2)
	}

	return &Daemon{
		store:     store.New(dataDir),
		git:       gitOps,
		gitServer: gitSrv,
		vaultDir:  vaultDir,
		addr:      addr,
	}
}

func (d *Daemon) Serve() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", d.handleStatus)
	mux.HandleFunc("GET /api/notes", d.handleList)
	mux.HandleFunc("GET /api/notes/{id}", d.handleGet)
	mux.HandleFunc("POST /api/submit", d.handleSubmit)
	mux.HandleFunc("POST /api/review/{id}", d.handleRequestReview)
	mux.HandleFunc("POST /api/approve/{id}", d.handleApprove)
	mux.HandleFunc("POST /api/reject/{id}", d.handleReject)
	mux.HandleFunc("POST /api/merge/{id}", d.handleMerge)
	mux.HandleFunc("POST /api/lint/{id}", d.handleLint)
	mux.HandleFunc("GET /api/doctor", d.handleDoctor)
	mux.HandleFunc("GET /api/repos", d.handleListRepos)
	mux.HandleFunc("POST /api/repos/{name}", d.handleCreateRepo)

	fmt.Printf("obsidian-center serving on %s\n", d.addr)
	fmt.Printf("vault: %s\n", d.vaultDir)
	return http.ListenAndServe(d.addr, mux)
}

func (d *Daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	drafts := d.store.List(note.StatusDraft)
	reviews := d.store.List(note.StatusReview)
	approved := d.store.List(note.StatusApproved)
	merged := d.store.List(note.StatusMerged)

	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"vault":    d.vaultDir,
		"drafts":   len(drafts),
		"reviews":  len(reviews),
		"approved": len(approved),
		"merged":   len(merged),
		"total":    len(d.store.List("")),
	})
}

func (d *Daemon) handleList(w http.ResponseWriter, r *http.Request) {
	status := note.Status(r.URL.Query().Get("status"))
	notes := d.store.List(status)
	json.NewEncoder(w).Encode(map[string]any{"notes": notes})
}

func (d *Daemon) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := d.store.Get(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	json.NewEncoder(w).Encode(n)
}

type submitRequest struct {
	Title        string   `json:"title"`
	Source       string   `json:"source"`
	AIModel      string   `json:"ai_model,omitempty"`
	Author       string   `json:"author"`
	Content      string   `json:"content"`
	Tags         []string `json:"tags"`
	Sources      []string `json:"sources,omitempty"`
	TargetFolder string   `json:"target_folder"`
}

func (d *Daemon) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %s", err), 400)
		return
	}
	fmt.Printf("[submit] body: %d bytes\n", len(body))
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %s", err), 400)
		return
	}

	if req.Title == "" || req.Source == "" || req.Author == "" || req.Content == "" {
		http.Error(w, "title, source, author, content required", 400)
		return
	}

	id := note.GenerateID(req.Title)
	n := &note.Note{
		ID:           id,
		Title:        req.Title,
		Source:       req.Source,
		AIModel:      req.AIModel,
		Author:       req.Author,
		Status:       note.StatusDraft,
		TargetFolder: req.TargetFolder,
		Tags:         req.Tags,
		Sources:      req.Sources,
		Verification: "unverified",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// draft 파일 생성
	draftPath := note.DraftPath(d.vaultDir, id)
	os.MkdirAll(filepath.Dir(draftPath), 0755)
	os.WriteFile(draftPath, []byte(req.Content), 0644)

	if err := d.store.Submit(n); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// git: create branch + commit draft
	if d.git != nil {
		if err := d.git.CreateBranch(id); err != nil {
			fmt.Printf("[git] branch create failed: %s\n", err)
		} else {
			d.git.CommitDraft(id, req.Title, req.Author)
			fmt.Printf("[git] branch created: drafts/%s\n", id)
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"id":     id,
		"status": "draft",
		"path":   draftPath,
		"branch": fmt.Sprintf("drafts/%s", id),
	})
}

func (d *Daemon) handleRequestReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// lint 먼저
	draftPath := note.DraftPath(d.vaultDir, id)
	errors := note.Lint(draftPath)
	if len(errors) > 0 {
		json.NewEncoder(w).Encode(map[string]any{
			"status": "lint_failed",
			"errors": errors,
		})
		return
	}

	if err := d.store.RequestReview(id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"status": "review", "id": id})
}

func (d *Daemon) handleApprove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Reviewer  string `json:"reviewer"`
		Comments  string `json:"comments"`
		Factcheck string `json:"factcheck"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)

	review := note.Review{
		Reviewer:   req.Reviewer,
		Status:     "approved",
		Comments:   req.Comments,
		Factcheck:  req.Factcheck,
		ReviewedAt: time.Now(),
	}

	if err := d.store.AddReview(id, review); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"status": "approved", "id": id})
}

func (d *Daemon) handleReject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Reviewer string `json:"reviewer"`
		Comments string `json:"comments"`
	}
	body, _ := io.ReadAll(r.Body)
	json.Unmarshal(body, &req)

	review := note.Review{
		Reviewer:   req.Reviewer,
		Status:     "rejected",
		Comments:   req.Comments,
		ReviewedAt: time.Now(),
	}

	if err := d.store.AddReview(id, review); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"status": "rejected", "id": id})
}

func (d *Daemon) handleMerge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	n, err := d.store.Get(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	// draft → target 폴더로 이동
	src := note.DraftPath(d.vaultDir, id)
	dst := note.TargetPath(d.vaultDir, n.TargetFolder, n.Title)
	os.MkdirAll(filepath.Dir(dst), 0755)

	if err := os.Rename(src, dst); err != nil {
		http.Error(w, fmt.Sprintf("move failed: %s", err), 500)
		return
	}

	if err := d.store.Merge(id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// git: merge branch + cleanup
	if d.git != nil {
		if err := d.git.MergeBranch(id); err != nil {
			fmt.Printf("[git] merge failed: %s\n", err)
		} else {
			d.git.DeleteBranch(id)
			fmt.Printf("[git] merged + branch deleted: drafts/%s\n", id)
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status": "merged",
		"id":     id,
		"path":   dst,
	})
}

func (d *Daemon) handleLint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	draftPath := note.DraftPath(d.vaultDir, id)

	errors := note.Lint(draftPath)
	status := "pass"
	if len(errors) > 0 {
		status = "fail"
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"errors": errors,
	})
}

func (d *Daemon) handleListRepos(w http.ResponseWriter, r *http.Request) {
	if d.gitServer == nil {
		http.Error(w, "git server not available", 500)
		return
	}
	repos := d.gitServer.ListRepos()
	json.NewEncoder(w).Encode(map[string]any{"repos": repos})
}

func (d *Daemon) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if d.gitServer == nil {
		http.Error(w, "git server not available", 500)
		return
	}
	cloneURL, err := d.gitServer.EnsureRepo(name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"repo":      name,
		"clone_url": cloneURL,
	})
}
