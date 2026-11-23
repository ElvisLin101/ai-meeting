#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-full}"

required_files=(
  "AGENTS.md"
  "README.md"
  "docs/agent-knowledge/README.md"
  "docs/agent-knowledge/rules/go-backend.md"
  "skills/ai-meeting-agent/SKILL.md"
  "skills/ai-meeting-memory/SKILL.md"
  "skills/ai-meeting-interview/SKILL.md"
  "skills/ai-meeting-user-auth/SKILL.md"
  "skills/ai-meeting-ai/SKILL.md"
  "docs/agent-knowledge/references/routes-map.md"
  "docs/agent-knowledge/references/data-models.md"
  "docs/agent-knowledge/references/memory-context-flow.md"
  "docs/agent-knowledge/references/placeholder-risk-register.md"
  "docs/agent-knowledge/references/ai-memory-evolution.md"
  "docs/agent-knowledge/references/interview-runtime-governance.md"
  "docs/agent-knowledge/references/reference-project-gap-summary.md"
)

missing=0
for file in "${required_files[@]}"; do
  if [[ ! -f "$ROOT/$file" ]]; then
    echo "missing required knowledge file: $file"
    missing=1
  fi
done

if [[ "$missing" -ne 0 ]]; then
  exit 1
fi

check_go_anchors() {
  local anchors
  if command -v rg >/dev/null 2>&1; then
    anchors="$(cd "$ROOT" && rg -o --no-heading --no-filename '[A-Za-z0-9_./-]+\.go' AGENTS.md README.md skills docs/agent-knowledge || true)"
  else
    anchors="$(cd "$ROOT" && grep -RhoE '[A-Za-z0-9_./-]+\.go' AGENTS.md README.md skills docs/agent-knowledge || true)"
  fi

  local bad=0
  while IFS= read -r anchor; do
    [[ -z "$anchor" ]] && continue
    anchor="${anchor#./}"
    if [[ ! -f "$ROOT/$anchor" ]]; then
      echo "stale Go anchor: $anchor"
      bad=1
    fi
  done < <(printf '%s\n' "$anchors" | sort -u)

  return "$bad"
}

route_file() {
  local file="$1"
  case "$file" in
    api/routes/*)
      echo "docs/agent-knowledge/references/routes-map.md"
      ;;
    models/*)
      echo "docs/agent-knowledge/references/data-models.md"
      ;;
  esac
  case "$file" in
    services/agent/*|api/handlers/agent*|api/handlers/profile*|dto/agent*|dto/profile*|models/agent*|repositories/mysql/agent*|static/*)
      echo "skills/ai-meeting-agent/SKILL.md"
      echo "docs/agent-knowledge/references/placeholder-risk-register.md"
      ;;
  esac
  case "$file" in
    services/ai/*|api/handlers/ai*|dto/ai*|models/ai*|repositories/mysql/ai*|clients/ai*|clients/deepseek*)
      echo "skills/ai-meeting-ai/SKILL.md"
      echo "docs/agent-knowledge/references/placeholder-risk-register.md"
      ;;
  esac
  case "$file" in
    services/agent/context/*|services/agent/run/*|api/handlers/profile*|dto/profile*|services/ai/*memory*|services/ai/*chat*|models/agent_runtime.go|models/conversation.go|repositories/redis*|repositories/mysql/message*|repositories/mysql/conversation*|repositories/mysql/sequence*|repositories/mysql/compressed_context*|repositories/mysql/*profile*|repositories/mysql/*identity*|pkg/singleflight/*)
      echo "skills/ai-meeting-memory/SKILL.md"
      echo "docs/agent-knowledge/references/memory-context-flow.md"
      echo "docs/agent-knowledge/references/placeholder-risk-register.md"
      ;;
  esac
  case "$file" in
    services/interview/*|services/agent/tools/interview/*|api/handlers/interview*|dto/interview*|models/interview*|repositories/mysql/interview*|clients/resume*|pkg/lock/*)
      echo "skills/ai-meeting-interview/SKILL.md"
      echo "docs/agent-knowledge/references/interview-runtime-governance.md"
      echo "docs/agent-knowledge/references/placeholder-risk-register.md"
      ;;
  esac
  case "$file" in
    services/user/*|api/handlers/user*|api/middleware/auth*|dto/user*|models/user*|repositories/mysql/user*)
      echo "skills/ai-meeting-user-auth/SKILL.md"
      echo "docs/agent-knowledge/references/placeholder-risk-register.md"
      ;;
  esac
  case "$file" in
    repositories/mysql/conversation*|repositories/mysql/message*|models/conversation.go)
      echo "skills/ai-meeting-agent/SKILL.md"
      echo "skills/ai-meeting-ai/SKILL.md"
      echo "skills/ai-meeting-interview/SKILL.md"
      ;;
  esac
  case "$file" in
    main.go|config/*|clients/*|repositories/mysql/database*|services/metric/*|api/resp/*|pkg/ecode/*|api/middleware/*|go.mod|go.sum|Dockerfile|docker-compose*|.github/workflows/*)
      echo "AGENTS.md"
      echo "docs/agent-knowledge/rules/go-backend.md"
      echo "docs/agent-knowledge/references/reference-project-gap-summary.md"
      ;;
  esac
  case "$file" in
    AGENTS.md|README.md|skills/*|docs/agent-knowledge/*|scripts/knowledge-check.sh)
      echo "$file"
      ;;
  esac
}

recommend_for_diff() {
  if ! git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "No git repository detected. Skipping diff routing."
    return 0
  fi

  # NUL 分隔保留含空格的路径；数组去重兼容 macOS Bash 3.2。
  local changed_files=()
  local file existing duplicate
  while IFS= read -r -d '' file; do
    duplicate=0
    if [[ "${#changed_files[@]}" -gt 0 ]]; then
      for existing in "${changed_files[@]}"; do
        if [[ "$existing" == "$file" ]]; then
          duplicate=1
          break
        fi
      done
    fi
    [[ "$duplicate" -eq 1 ]] || changed_files+=("$file")
  done < <(
    git -C "$ROOT" diff --name-only -z --cached
    git -C "$ROOT" diff --name-only -z
    git -C "$ROOT" ls-files --others --exclude-standard -z
  )

  if [[ "${#changed_files[@]}" -eq 0 ]]; then
    echo "No changed files detected. Knowledge diff check has nothing to route."
    return 0
  fi

  echo "Changed files:"
  printf '  - %s\n' "${changed_files[@]}"
  echo
  echo "Knowledge files to review:"

  local suggestions
  suggestions="$(for file in "${changed_files[@]}"; do route_file "$file"; done | sort -u)"
  if [[ -n "$suggestions" ]]; then
    while IFS= read -r file; do printf '  - %s\n' "$file"; done <<< "$suggestions"
  else
    echo "  - No module-specific Skill matched. Review AGENTS.md if the change affects project workflow."
  fi
}

case "$MODE" in
  full)
    check_go_anchors
    echo "Knowledge full check passed (file anchors only; verify behavior against code)."
    ;;
  diff)
    check_go_anchors
    recommend_for_diff
    echo "Knowledge diff check passed (routing suggestions only; verify behavior against code)."
    ;;
  *)
    echo "usage: scripts/knowledge-check.sh [full|diff]" >&2
    exit 2
    ;;
esac
