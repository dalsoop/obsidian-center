package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/dalsoop/obsidian-center/internal/daemon"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		serve()
	case "status":
		apiGet("/api/status")
	case "submit":
		submit()
	case "list":
		list()
	case "review":
		review()
	case "approve":
		approve()
	case "reject":
		reject()
	case "merge":
		merge()
	case "lint":
		lint()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`obsidian-center — note lifecycle manager

Usage:
  obsidian-center serve                      Start daemon
  obsidian-center status                     Show status
  obsidian-center submit --title "..." ...   Submit a draft note
  obsidian-center list [--status draft]      List notes
  obsidian-center review <id>                Request review (lint gate)
  obsidian-center approve <id>               Approve a note
  obsidian-center reject <id> --comment "..."  Reject a note
  obsidian-center merge <id>                 Merge approved note to vault
  obsidian-center lint <id>                  Lint check a draft

Flow:
  submit → review (lint gate) → approve/reject → merge`)
}

func serve() {
	vault := envOr("OBSIDIAN_VAULT", os.Getenv("HOME")+"/문서/프로젝트/mac-host-commands/vault")
	data := envOr("OBSIDIAN_DATA", os.Getenv("HOME")+"/문서/시스템/obsidian-center")
	addr := envOr("OBSIDIAN_ADDR", ":8910")

	// parse flags
	for i, arg := range os.Args {
		switch arg {
		case "--vault":
			if i+1 < len(os.Args) {
				vault = os.Args[i+1]
			}
		case "--addr":
			if i+1 < len(os.Args) {
				addr = os.Args[i+1]
			}
		}
	}

	d := daemon.New(vault, data, addr)
	if err := d.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func submit() {
	var title, source, author, content, target, aiModel string
	var tags, sources []string

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--title":
			i++; title = os.Args[i]
		case "--source":
			i++; source = os.Args[i]
		case "--author":
			i++; author = os.Args[i]
		case "--content":
			i++; content = os.Args[i]
		case "--file":
			i++
			data, _ := os.ReadFile(os.Args[i])
			content = string(data)
		case "--target":
			i++; target = os.Args[i]
		case "--ai-model":
			i++; aiModel = os.Args[i]
		case "--tags":
			i++; tags = strings.Split(os.Args[i], ",")
		case "--sources":
			i++; sources = strings.Split(os.Args[i], ",")
		}
	}

	if title == "" || source == "" {
		fmt.Println("--title and --source required")
		os.Exit(1)
	}
	if author == "" {
		author = os.Getenv("USER")
	}
	if target == "" {
		if source == "ai" {
			target = "CLAUDE/generated"
		} else {
			target = "06-Notes"
		}
	}

	body := map[string]any{
		"title": title, "source": source, "author": author,
		"content": content, "target_folder": target,
		"tags": tags, "sources": sources, "ai_model": aiModel,
	}
	data, _ := json.Marshal(body)
	apiPost("/api/submit", string(data))
}

func list() {
	status := ""
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--status" && i+1 < len(os.Args) {
			status = os.Args[i+1]
		}
	}
	path := "/api/notes"
	if status != "" {
		path += "?status=" + status
	}
	apiGet(path)
}

func review() {
	if len(os.Args) < 3 {
		fmt.Println("usage: obsidian-center review <id>")
		os.Exit(1)
	}
	apiPost("/api/review/"+os.Args[2], "")
}

func approve() {
	if len(os.Args) < 3 {
		fmt.Println("usage: obsidian-center approve <id>")
		os.Exit(1)
	}
	reviewer := os.Getenv("USER")
	comments := ""
	for i := 3; i < len(os.Args); i++ {
		if os.Args[i] == "--comment" && i+1 < len(os.Args) {
			comments = os.Args[i+1]
		}
	}
	body := fmt.Sprintf(`{"reviewer":"%s","comments":"%s"}`, reviewer, comments)
	apiPost("/api/approve/"+os.Args[2], body)
}

func reject() {
	if len(os.Args) < 3 {
		fmt.Println("usage: obsidian-center reject <id>")
		os.Exit(1)
	}
	reviewer := os.Getenv("USER")
	comments := ""
	for i := 3; i < len(os.Args); i++ {
		if os.Args[i] == "--comment" && i+1 < len(os.Args) {
			comments = os.Args[i+1]
		}
	}
	body := fmt.Sprintf(`{"reviewer":"%s","comments":"%s"}`, reviewer, comments)
	apiPost("/api/reject/"+os.Args[2], body)
}

func merge() {
	if len(os.Args) < 3 {
		fmt.Println("usage: obsidian-center merge <id>")
		os.Exit(1)
	}
	apiPost("/api/merge/"+os.Args[2], "")
}

func lint() {
	if len(os.Args) < 3 {
		fmt.Println("usage: obsidian-center lint <id>")
		os.Exit(1)
	}
	apiPost("/api/lint/"+os.Args[2], "")
}

func apiGet(path string) {
	url := envOr("OBSIDIAN_CENTER_URL", "http://localhost:8910") + path
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func apiPost(path, body string) {
	url := envOr("OBSIDIAN_CENTER_URL", "http://localhost:8910") + path
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
