#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-full}"

required_files=(
  "AGENTS.md"
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
    anchors="$(cd "$ROOT" && rg -o --no-heading --no-filename '[A-Za-z0-9_./-]+\.go' AGENTS.md skills docs/agent-knowledge || true)"
  else
    anchors="$(cd "$ROOT" && grep -RhoE '[A-Za-z0-9_./-]+\.go' AGENTS.md skills docs/agent-knowledge || true)"
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

recommend_for_diff() {
  local changed_files
  if ! git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "No git repository detected. Skipping diff routing."
    return 0
  fi

  changed_files="$(git -C "$ROOT" diff --name-only --cached 2>/dev/null || true)"
  if [[ -z "$changed_files" ]]; then
    changed_files="$(git -C "$ROOT" diff --name-only 2>/dev/null || true)"
  fi

  if [[ -z "$changed_files" ]]; then
    echo "No changed files detected. Knowledge diff check has nothing to route."
    return 0
  fi

  echo "Changed files:"
  printf '%s\n' "$changed_files" | sed 's/^/  - /'
  echo
  echo "Knowledge files to review:"

  local suggested=0
  while IFS= read -r file; do
    case "$file" in
      api/routes/routes.go)
        echo "  - docs/agent-knowledge/references/routes-map.md"
        suggested=1
        ;;
      models/*.go)
        echo "  - docs/agent-knowledge/references/data-models.md"
        suggested=1
        ;;
      services/memory_service.go|services/ai_memory_service.go|models/compressed_context.go|repositories/redis.go|repositories/mongo/*.go)
        echo "  - skills/ai-meeting-memory/SKILL.md"
        echo "  - docs/agent-knowledge/references/memory-context-flow.md"
        suggested=1
        ;;
      services/agent_service.go|repositories/mysql/agent_conversation_repository.go|repositories/mysql/agent_properties_repository.go|repositories/mysql/agent_file_asset_repository.go|repositories/mongo/agent_message_repository.go|api/handlers/agent_handler.go|dto/agent.go)
        echo "  - skills/ai-meeting-agent/SKILL.md"
        echo "  - docs/agent-knowledge/references/placeholder-risk-register.md"
        suggested=1
        ;;
      services/interview_service.go|repositories/mysql/interview_session_repository.go|repositories/mysql/interview_record_repository.go|repositories/mysql/agent_message_repository.go|api/handlers/interview_handler.go|dto/interview.go)
        echo "  - skills/ai-meeting-interview/SKILL.md"
        echo "  - docs/agent-knowledge/references/placeholder-risk-register.md"
        suggested=1
        ;;
      services/user_service.go|repositories/mysql/user_repository.go|api/handlers/user_handler.go|api/middleware/auth.go|dto/user.go)
        echo "  - skills/ai-meeting-user-auth/SKILL.md"
        echo "  - docs/agent-knowledge/references/placeholder-risk-register.md"
        suggested=1
        ;;
      services/ai_service.go|services/ai_chat_service.go|clients/ai_model_client.go|repositories/mysql/ai_conversation_repository.go|repositories/mysql/ai_properties_repository.go|repositories/mongo/ai_message_repository.go|api/handlers/ai_handler.go|dto/ai.go)
        echo "  - skills/ai-meeting-ai/SKILL.md"
        echo "  - docs/agent-knowledge/references/placeholder-risk-register.md"
        suggested=1
        ;;
    esac
  done <<< "$changed_files"

  if [[ "$suggested" -eq 0 ]]; then
    echo "  - No module-specific Skill matched. Review AGENTS.md if the change affects project workflow."
  fi
}

case "$MODE" in
  full)
    check_go_anchors
    echo "Knowledge full check passed."
    ;;
  diff)
    check_go_anchors
    recommend_for_diff
    echo "Knowledge diff check passed."
    ;;
  *)
    echo "usage: scripts/knowledge-check.sh [full|diff]" >&2
    exit 2
    ;;
esac
