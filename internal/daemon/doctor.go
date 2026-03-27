package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dalsoop/obsidian-center/internal/note"
)

type DoctorResult struct {
	Checks  []Check `json:"checks"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Fixed   int     `json:"fixed"`
	Warned  int     `json:"warned"`
}

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, fail, warn, fixed
	Detail string `json:"detail,omitempty"`
}

func (d *Daemon) handleDoctor(w http.ResponseWriter, r *http.Request) {
	result := d.runDoctor()
	json.NewEncoder(w).Encode(result)
}

func (d *Daemon) runDoctor() DoctorResult {
	var checks []Check

	// 1. vault 경로
	checks = append(checks, checkVaultPath(d.vaultDir))

	// 2. .obsidian 설정
	checks = append(checks, checkObsidianConfig(d.vaultDir))

	// 3. obsidian-git 플러그인
	checks = append(checks, checkPlugin(d.vaultDir, "obsidian-git"))

	// 4. 폴더 구조
	for _, folder := range []string{"00-Inbox", "01-Daily", "02-Projects", "03-Areas", "04-Tasks", "05-Collections", "06-Notes", "07-System", "CLAUDE", "CLAUDE/drafts", "CLAUDE/generated"} {
		checks = append(checks, checkFolder(d.vaultDir, folder))
	}

	// 5. 템플릿
	for _, tmpl := range []string{"Daily Note.md", "Note.md", "AI Note.md", "Project.md", "Task.md"} {
		checks = append(checks, checkTemplate(d.vaultDir, tmpl))
	}

	// 6. 노트 lint
	checks = append(checks, lintAllNotes(d.vaultDir)...)

	// 7. draft 상태
	checks = append(checks, checkDrafts(d.vaultDir, d.store))

	// 8. git 상태
	checks = append(checks, checkGitStatus(d.vaultDir))

	// 9. soft-serve
	if d.gitServer != nil {
		checks = append(checks, Check{Name: "soft-serve", Status: "pass", Detail: "실행 중"})
	} else {
		checks = append(checks, Check{Name: "soft-serve", Status: "warn", Detail: "미실행 (soft 바이너리 확인)"})
	}

	// 10. Dashboard
	checks = append(checks, checkFile(d.vaultDir, "Dashboard.md"))

	// 집계
	result := DoctorResult{Checks: checks}
	for _, c := range checks {
		switch c.Status {
		case "pass":
			result.Passed++
		case "fail":
			result.Failed++
		case "fixed":
			result.Fixed++
		case "warn":
			result.Warned++
		}
	}
	return result
}

func checkVaultPath(vaultDir string) Check {
	if _, err := os.Stat(vaultDir); err != nil {
		return Check{Name: "vault 경로", Status: "fail", Detail: fmt.Sprintf("%s 없음", vaultDir)}
	}
	return Check{Name: "vault 경로", Status: "pass", Detail: vaultDir}
}

func checkObsidianConfig(vaultDir string) Check {
	configDir := filepath.Join(vaultDir, ".obsidian")
	if _, err := os.Stat(configDir); err != nil {
		return Check{Name: ".obsidian", Status: "fail", Detail: "설정 디렉토리 없음"}
	}
	// community-plugins.json
	cpPath := filepath.Join(configDir, "community-plugins.json")
	if data, err := os.ReadFile(cpPath); err == nil {
		var plugins []string
		json.Unmarshal(data, &plugins)
		return Check{Name: ".obsidian", Status: "pass", Detail: fmt.Sprintf("플러그인 %d개 등록", len(plugins))}
	}
	return Check{Name: ".obsidian", Status: "warn", Detail: "community-plugins.json 없음"}
}

func checkPlugin(vaultDir, pluginName string) Check {
	pluginDir := filepath.Join(vaultDir, ".obsidian", "plugins", pluginName)
	mainJS := filepath.Join(pluginDir, "main.js")
	if _, err := os.Stat(mainJS); err != nil {
		return Check{Name: pluginName, Status: "fail", Detail: "미설치"}
	}
	return Check{Name: pluginName, Status: "pass", Detail: "설치됨"}
}

func checkFolder(vaultDir, folder string) Check {
	path := filepath.Join(vaultDir, folder)
	if _, err := os.Stat(path); err != nil {
		// 자동 생성
		os.MkdirAll(path, 0755)
		return Check{Name: folder, Status: "fixed", Detail: "폴더 생성됨"}
	}
	return Check{Name: folder, Status: "pass"}
}

func checkTemplate(vaultDir, tmpl string) Check {
	path := filepath.Join(vaultDir, "07-System", "templates", tmpl)
	if _, err := os.Stat(path); err != nil {
		return Check{Name: "템플릿/" + tmpl, Status: "warn", Detail: "없음"}
	}
	return Check{Name: "템플릿/" + tmpl, Status: "pass"}
}

func checkFile(vaultDir, filename string) Check {
	path := filepath.Join(vaultDir, filename)
	if _, err := os.Stat(path); err != nil {
		return Check{Name: filename, Status: "warn", Detail: "없음"}
	}
	return Check{Name: filename, Status: "pass"}
}

func lintAllNotes(vaultDir string) []Check {
	var checks []Check
	var totalErrors int
	var totalNotes int

	folders := []string{"00-Inbox", "01-Daily", "02-Projects", "03-Areas", "04-Tasks", "05-Collections", "06-Notes", "CLAUDE"}

	for _, folder := range folders {
		dir := filepath.Join(vaultDir, folder)
		notes := findMDFiles(dir)
		for _, n := range notes {
			totalNotes++
			errors := note.Lint(n)
			if len(errors) > 0 {
				totalErrors += len(errors)
				rel, _ := filepath.Rel(vaultDir, n)

				// 자동 수정 시도 — frontmatter 없으면 추가
				content, _ := os.ReadFile(n)
				text := string(content)
				if !strings.HasPrefix(text, "---") {
					fixed := fmt.Sprintf("---\ncreated: 2026-03-27\ntags: []\nsource: human\n---\n\n%s", text)
					os.WriteFile(n, []byte(fixed), 0644)
					checks = append(checks, Check{
						Name:   "lint/" + rel,
						Status: "fixed",
						Detail: "frontmatter 자동 추가",
					})
					continue
				}

				checks = append(checks, Check{
					Name:   "lint/" + rel,
					Status: "fail",
					Detail: strings.Join(errors, ", "),
				})
			}
		}
	}

	checks = append(checks, Check{
		Name:   "lint 전체",
		Status: func() string { if totalErrors > 0 { return "warn" }; return "pass" }(),
		Detail: fmt.Sprintf("%d개 노트 검사, %d개 에러", totalNotes, totalErrors),
	})

	return checks
}

func checkDrafts(vaultDir string, store interface{ List(s note.Status) []*note.Note }) Check {
	drafts := store.List(note.StatusDraft)
	reviews := store.List(note.StatusReview)

	if len(drafts) == 0 && len(reviews) == 0 {
		return Check{Name: "대기 노트", Status: "pass", Detail: "없음"}
	}
	return Check{
		Name:   "대기 노트",
		Status: "warn",
		Detail: fmt.Sprintf("draft %d개, review %d개", len(drafts), len(reviews)),
	}
}

func checkGitStatus(vaultDir string) Check {
	cmd := exec.Command("git", "-C", vaultDir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return Check{Name: "git", Status: "warn", Detail: "git 아님"}
	}
	changes := len(strings.Split(strings.TrimSpace(string(out)), "\n"))
	if string(out) == "" {
		changes = 0
	}
	if changes > 0 {
		return Check{Name: "git", Status: "warn", Detail: fmt.Sprintf("%d개 미커밋 변경", changes)}
	}
	return Check{Name: "git", Status: "pass", Detail: "clean"}
}

func findMDFiles(dir string) []string {
	var files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files
}
