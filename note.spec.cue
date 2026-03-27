package obsidiancenter

// ===================================================
// note.spec.cue — Obsidian Center 스펙 v1.0.0
//
// 노트 라이프사이클 관리. dalcenter 패턴 기반.
// draft → review → approved → merged
// ===================================================

#NoteStatus: "draft" | "review" | "approved" | "rejected" | "merged"
#Source: "human" | "ai"
#Verification: "unverified" | "partial" | "verified"

// ===== 노트 제출 =====

#NoteSubmission: {
    id!:          string
    title!:       string
    source!:      #Source
    ai_model?:    string
    author!:      string
    content!:     string
    tags!:        [...string] & [_, ...]
    sources?:     [...string]
    target_folder!: string  // 06-Notes, 02-Projects, etc.
    created_at!:  string
}

// ===== 리뷰 =====

#Review: {
    note_id!:    string
    reviewer!:   string
    status!:     "approved" | "rejected" | "changes_requested"
    comments?:   string
    factcheck?:  string
    reviewed_at!: string
}

// ===== 리뷰 규칙 =====

#ReviewRule: {
    // AI 생성 노트는 반드시 사람 리뷰 필요
    ai_requires_review!: bool | *true

    // 필수 frontmatter
    required_fields!: [...string]

    // 최소 리뷰어 수
    min_reviewers!: int | *1

    // 자동 승인 조건 (사람이 쓴 노트 + lint 통과)
    auto_approve_human!: bool | *false

    // lint 규칙
    lint!: {
        frontmatter!:  bool | *true
        min_tags!:     int | *1
        min_length!:   int | *50  // 최소 글자 수
        require_source!: bool | *true
    }
}

// ===== 기본 규칙 =====

default_rules: #ReviewRule & {
    ai_requires_review: true
    required_fields: ["created", "tags", "source"]
    min_reviewers: 1
    auto_approve_human: false
    lint: {
        frontmatter:  true
        min_tags:     1
        min_length:   50
        require_source: true
    }
}

// ===== 폴더별 규칙 =====

folder_rules: {
    "00-Inbox": #ReviewRule & {
        auto_approve_human: true  // Inbox는 사람 노트 자동 승인
        lint: min_length: 0       // 짧아도 됨
    }
    "CLAUDE": #ReviewRule & {
        ai_requires_review: true
        min_reviewers: 1
        required_fields: ["created", "tags", "source", "ai_model"]
    }
    "02-Projects": #ReviewRule & {
        required_fields: ["created", "tags", "source", "project"]
    }
}
