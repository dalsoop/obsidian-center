package note

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft    Status = "draft"
	StatusReview   Status = "review"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusMerged   Status = "merged"
)

type Note struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Source       string    `json:"source"` // "human" or "ai"
	AIModel      string    `json:"ai_model,omitempty"`
	Author       string    `json:"author"`
	Status       Status    `json:"status"`
	TargetFolder string    `json:"target_folder"`
	Tags         []string  `json:"tags"`
	Sources      []string  `json:"sources,omitempty"`
	Verification string    `json:"verification"` // unverified, partial, verified
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Reviews      []Review  `json:"reviews,omitempty"`
}

type Review struct {
	Reviewer   string    `json:"reviewer"`
	Status     string    `json:"status"` // approved, rejected, changes_requested
	Comments   string    `json:"comments,omitempty"`
	Factcheck  string    `json:"factcheck,omitempty"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// ParseFrontmatter extracts YAML frontmatter from markdown
func ParseFrontmatter(content string) map[string]string {
	fm := make(map[string]string)
	if !strings.HasPrefix(content, "---") {
		return fm
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return fm
	}
	for _, line := range strings.Split(strings.TrimSpace(parts[1]), "\n") {
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, "\"'")
			fm[key] = val
		}
	}
	return fm
}

// Lint checks a note against rules
func Lint(path string) []string {
	var errors []string

	content, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("cannot read: %s", err)}
	}

	text := string(content)
	fm := ParseFrontmatter(text)

	// frontmatter 존재
	if !strings.HasPrefix(text, "---") {
		errors = append(errors, "frontmatter 없음")
	}

	// 필수 필드
	for _, field := range []string{"created", "tags", "source"} {
		if _, ok := fm[field]; !ok {
			errors = append(errors, fmt.Sprintf("'%s' 누락", field))
		}
	}

	// source 값 검증
	if source, ok := fm["source"]; ok {
		if source != "human" && source != "ai" {
			errors = append(errors, fmt.Sprintf("source는 'human' 또는 'ai'여야 함 (현재: '%s')", source))
		}
	}

	// AI 노트면 ai_model 필수
	if fm["source"] == "ai" {
		if _, ok := fm["ai_model"]; !ok {
			errors = append(errors, "AI 노트에 'ai_model' 누락")
		}
	}

	// tags 비어있으면
	if tags, ok := fm["tags"]; ok {
		if tags == "[]" || tags == "" {
			errors = append(errors, "tags 비어있음")
		}
	}

	// 본문 최소 길이
	body := text
	if idx := strings.Index(text[3:], "---"); idx > 0 {
		body = text[idx+6:]
	}
	body = strings.TrimSpace(body)
	if len(body) < 50 {
		errors = append(errors, fmt.Sprintf("본문 너무 짧음 (%d자, 최소 50자)", len(body)))
	}

	return errors
}

// GenerateID creates a note ID from title and timestamp
func GenerateID(title string) string {
	t := time.Now().Format("060102-1504")
	clean := strings.ReplaceAll(title, " ", "-")
	return fmt.Sprintf("%s_%s", t, clean)
}

// DraftPath returns the path for a draft note
func DraftPath(vaultDir, id string) string {
	return filepath.Join(vaultDir, "CLAUDE", "drafts", id+".md")
}

// TargetPath returns the final path for an approved note
func TargetPath(vaultDir, folder, title string) string {
	return filepath.Join(vaultDir, folder, title+".md")
}
