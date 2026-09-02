#!/usr/bin/env bash
#
# Check a PR's changed files against the scopes of the tasks it references.
#
# Every file must be inside a referenced task's paths_owned (when any of
# them declares one) and outside every other in-progress task's lease. The
# verdict comes from POST /projects/{id}/scope-check — the same answer
# `veans check` gives an agent locally. Runs standalone too:
#
#   WORKMAN_TOKEN=tk_... BASE_SHA=... HEAD_SHA=... ./check-scope.sh

# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
FAIL_ON_VIOLATION="${FAIL_ON_VIOLATION:-true}"

require_commits
mapfile -t files < <(git diff --name-only --diff-filter=ACMRD "$BASE_SHA"..."$HEAD_SHA")
if [[ ${#files[@]} -eq 0 ]]; then
	log "No files changed between $BASE_SHA and $HEAD_SHA — nothing to check."
	echo "ok=true" >> "${GITHUB_OUTPUT:-/dev/null}"
	exit 0
fi

collect_refs
require_server

task_ids=()
for ref in "${refs[@]:-}"; do
	[[ -n "$ref" ]] || continue
	id="$(resolve_ref "$ref")"
	[[ -n "$id" ]] && task_ids+=("$id")
done
if [[ ${#task_ids[@]} -eq 0 ]]; then
	warn "no Refs: trailers resolve to tasks — only lease collisions can be checked"
fi

request="$(jq -cn \
	--argjson ids "$(printf '%s\n' "${task_ids[@]:-}" | grep -E '^[0-9]+$' | jq -cs 'map(tonumber)')" \
	--argjson files "$(printf '%s\n' "${files[@]}" | jq -R . | jq -cs .)" \
	--arg repo "$REPOSITORY" \
	'{task_ids:$ids, files:$files} + (if $repo != "" then {repository:$repo} else {} end)')"
status="$(api POST "/projects/$PROJECT_ID/scope-check" application/json --data "$request")"
[[ "$status" =~ ^2 ]] || fail "scope check failed (HTTP $status): $(<"$API_BODY")"

ok="$(jq -r '.ok' "$API_BODY")"
enforced="$(jq -r '.enforced' "$API_BODY")"
strays="$(jq -r '.strays' "$API_BODY")"
affected="$(jq -r '.affected' "$API_BODY")"
collisions="$(jq -r '.collisions' "$API_BODY")"
{
	echo "ok=$ok"
	echo "strays=$strays"
	echo "collisions=$collisions"
} >> "${GITHUB_OUTPUT:-/dev/null}"

table="$(jq -r '.files[] | select(.verdict != "owned") | "| `\(.path)` | \(.verdict) | \(if .held_by_task_id then "task \(.held_by_task_id)" else (.task_ids | map(tostring) | join(", ")) end) |"' "$API_BODY")"
summary="## Workman scope check
Tasks: ${refs[*]:-none} · enforced: $enforced · strays: $strays · affected-only: $affected · leased by others: $collisions"
if [[ -n "$table" ]]; then
	summary+="

| File | Verdict | Task |
|---|---|---|
$table"
fi
[[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && printf '%s\n' "$summary" >> "$GITHUB_STEP_SUMMARY"
gh_upsert_comment "<!-- workman-scope-check -->" "$summary"

if [[ "$ok" != "true" ]]; then
	msg="$strays file(s) outside the referenced tasks' paths_owned, $collisions leased by another task"
	if [[ "$FAIL_ON_VIOLATION" == "true" ]]; then
		fail "$msg — widen the task's scope, split the work, or reference the task that owns the file"
	fi
	warn "$msg"
fi
log "Scope check passed (enforced: $enforced)"
