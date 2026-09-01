#!/usr/bin/env bash
#
# Shared plumbing for the Workman PR hooks: reads .veans.yml, talks to
# /api/v2 with curl, collects Refs: trailers and resolves them to tasks.
# Sourced by close-merged.sh, check-scope.sh and pr-opened.sh.

set -euo pipefail

: "${WORKMAN_TOKEN:?WORKMAN_TOKEN is required}"
WORKMAN_CONFIG="${WORKMAN_CONFIG:-.veans.yml}"
DRY_RUN="${DRY_RUN:-false}"
PR_NUMBER="${PR_NUMBER:-}"
PR_URL="${PR_URL:-}"
PR_TITLE="${PR_TITLE:-}"
PR_AUTHOR="${PR_AUTHOR:-}"

log()  { printf '%s\n' "$*" >&2; }
warn() { printf '::warning::%s\n' "$*" >&2; }
fail() { printf '::error::%s\n' "$*" >&2; exit 1; }

[[ -f "$WORKMAN_CONFIG" ]] || fail "no $WORKMAN_CONFIG found — run 'veans init' in this repo and commit the file"

# .veans.yml is flat at the top level for the keys we need; a YAML parser
# would be one more thing to install on a runner for a few scalar reads.
yml_scalar() {
	sed -n -E "s/^$1:[[:space:]]*//p" "$WORKMAN_CONFIG" | head -n1 | sed -E 's/^["'"'"']?//; s/["'"'"']?[[:space:]]*(#.*)?$//'
}

# yml_nested buckets in_review → the value of in_review under the buckets: map.
yml_nested() {
	awk -v section="$1:" -v key="$2:" '
		$0 ~ "^" section { inside = 1; next }
		inside && /^[^[:space:]]/ { inside = 0 }
		inside && $1 == key { sub(/[[:space:]]*#.*$/, "", $2); print $2; exit }
	' "$WORKMAN_CONFIG"
}

SERVER="${WORKMAN_SERVER:-$(yml_scalar server)}"
PROJECT_ID="$(yml_scalar project_id)"
PROJECT_IDENTIFIER="$(yml_scalar project_identifier)"
VIEW_ID="$(yml_scalar view_id)"
REPOSITORY="${WORKMAN_REPOSITORY:-$(yml_scalar repository)}"
BUCKET_IN_REVIEW="$(yml_nested buckets in_review)"
[[ -n "$SERVER" ]] || fail "server is not set in $WORKMAN_CONFIG and no server input was given"
[[ "$PROJECT_ID" =~ ^[0-9]+$ ]] || fail "project_id in $WORKMAN_CONFIG is not numeric: '$PROJECT_ID'"
SERVER="${SERVER%/}"

# ---------------------------------------------------------------------------
# API helpers.
#
# `api` prints the HTTP status (000 when the server was unreachable) and leaves
# the response body in $API_BODY, so a caller can tell a genuine 404 apart from
# an outage or a bad token — those must fail the job, never quietly skip a task.
# The status comes back as output rather than a variable because callers run
# it in a command substitution, where assignments would not survive.
# --retry covers connection failures and 5xx only; a 4xx is an answer.
# ---------------------------------------------------------------------------
API_BODY="$(mktemp)"
trap 'rm -f "$API_BODY"' EXIT

api() {
	local method="$1" path="$2" ctype="${3:-application/json}"
	shift 3 || shift $#
	curl --silent --show-error --retry 3 --retry-connrefused \
		--output "$API_BODY" --write-out '%{http_code}' \
		-X "$method" \
		-H "Authorization: Bearer $WORKMAN_TOKEN" \
		-H "Accept: application/json" \
		-H "Content-Type: $ctype" \
		"$SERVER/api/v2$path" "$@" || true # curl prints 000 itself when it cannot connect
}

# One cheap authenticated call up front, so a dead server or a revoked token
# fails loudly before any task is touched.
require_server() {
	local status
	status="$(api GET "/projects/$PROJECT_ID")"
	[[ "$status" =~ ^2 ]] || fail "cannot reach $SERVER as project $PROJECT_ID (HTTP $status) — check the server URL and WORKMAN_TOKEN"
}

html_escape() {
	sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' <<<"$1"
}

require_commits() {
	: "${BASE_SHA:?BASE_SHA is required}"
	: "${HEAD_SHA:?HEAD_SHA is required}"
	if ! git cat-file -e "$BASE_SHA^{commit}" 2>/dev/null || ! git cat-file -e "$HEAD_SHA^{commit}" 2>/dev/null; then
		fail "base or head commit is not available locally — check out with fetch-depth: 0"
	fi
}

# collect_refs fills the global `refs` array with the Refs: trailers of every
# commit on the PR. Accepts `Refs: PROJ-12`, `Refs: #12`, `Refs: 12`, and
# comma/space lists.
collect_refs() {
	require_commits
	mapfile -t refs < <(
		git log --format=%B "$BASE_SHA".."$HEAD_SHA" \
			| grep -Eio '^[[:space:]]*refs?:[[:space:]]*.*$' \
			| sed -E 's/^[[:space:]]*[Rr][Ee][Ff][Ss]?:[[:space:]]*//' \
			| tr ',' ' ' \
			| tr -s '[:space:]' '\n' \
			| grep -E '^(#?[0-9]+|[A-Za-z][A-Za-z0-9_]*-[0-9]+)$' \
			| sort -u
	)
}

# resolve_ref PROJ-12 → prints the task id and leaves the task JSON in
# $API_BODY. Prints nothing (exit 0) for a ref that should be skipped, and
# fails the job on anything that is not a clean answer.
resolve_ref() {
	local ref="$1" prefix="" index
	index="${ref#\#}"
	if [[ "$ref" == *-* ]]; then
		prefix="${ref%-*}"
		index="${ref##*-}"
	fi
	if [[ -n "$prefix" && -n "$PROJECT_IDENTIFIER" && "${prefix^^}" != "${PROJECT_IDENTIFIER^^}" ]]; then
		warn "$ref belongs to project '$prefix', this repo tracks '$PROJECT_IDENTIFIER' — skipped"
		return 0
	fi
	local status
	status="$(api GET "/projects/$PROJECT_ID/tasks/by-index/$index")"
	if [[ "$status" == "404" ]]; then
		warn "$ref: no task with index $index in project $PROJECT_ID — skipped"
		return 0
	fi
	[[ "$status" =~ ^2 ]] || fail "$ref: resolving task index $index failed (HTTP $status): $(<"$API_BODY")"
	jq -r '.id' "$API_BODY"
}

# task_has_comment_with 42 "https://…/pull/7" → 0 when a comment already
# links the PR, so re-runs stay quiet.
task_has_comment_with() {
	local task_id="$1" needle="$2" status
	status="$(api GET "/tasks/$task_id/comments?per_page=50")"
	[[ "$status" =~ ^2 ]] || return 1
	jq -e --arg n "$needle" '[.items[]?.comment // empty] | any(contains($n))' "$API_BODY" >/dev/null
}

# gh_upsert_comment "<!-- marker -->" "body" — posts or updates one PR comment
# when a GitHub token is available; silently does nothing otherwise.
gh_upsert_comment() {
	local marker="$1" body="$2"
	[[ -n "${GITHUB_TOKEN:-}" && -n "$PR_NUMBER" && -n "${GITHUB_REPOSITORY:-}" ]] || return 0
	local api="${GITHUB_API_URL:-https://api.github.com}/repos/$GITHUB_REPOSITORY/issues/$PR_NUMBER/comments"
	local existing
	existing="$(curl --silent --show-error -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" "$api?per_page=100" \
		| jq -r --arg m "$marker" '[.[] | select(.body | contains($m))][0].id // empty')" || existing=""
	local payload
	payload="$(jq -cn --arg b "$marker"$'\n'"$body" '{body:$b}')"
	if [[ -n "$existing" ]]; then
		curl --silent --show-error --output /dev/null -X PATCH -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
			"${GITHUB_API_URL:-https://api.github.com}/repos/$GITHUB_REPOSITORY/issues/comments/$existing" --data "$payload" || warn "could not update the PR comment"
	else
		curl --silent --show-error --output /dev/null -X POST -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
			"$api" --data "$payload" || warn "could not post the PR comment"
	fi
}
