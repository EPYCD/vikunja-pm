#!/usr/bin/env bash
#
# Close the Workman tasks a merged PR references.
#
# Scans the PR's commit messages for `Refs:` trailers, resolves each
# identifier through /api/v2, marks the task done and leaves a comment that
# links back to the PR. Runs standalone too:
#
#   WORKMAN_TOKEN=tk_... BASE_SHA=... HEAD_SHA=... PR_NUMBER=42 \
#     PR_URL=https://github.com/o/r/pull/42 ./close-merged.sh
#
# Needs git (with base..head reachable — check out with fetch-depth: 0),
# curl and jq. Nothing else.

set -euo pipefail

: "${WORKMAN_TOKEN:?WORKMAN_TOKEN is required}"
: "${BASE_SHA:?BASE_SHA is required}"
: "${HEAD_SHA:?HEAD_SHA is required}"
WORKMAN_CONFIG="${WORKMAN_CONFIG:-.veans.yml}"
DRY_RUN="${DRY_RUN:-false}"
PR_NUMBER="${PR_NUMBER:-}"
PR_URL="${PR_URL:-}"
PR_TITLE="${PR_TITLE:-}"
PR_AUTHOR="${PR_AUTHOR:-}"
MERGE_SHA="${MERGE_SHA:-}"

log()  { printf '%s\n' "$*" >&2; }
warn() { printf '::warning::%s\n' "$*" >&2; }
fail() { printf '::error::%s\n' "$*" >&2; exit 1; }

[[ -f "$WORKMAN_CONFIG" ]] || fail "no $WORKMAN_CONFIG found — run 'veans init' in this repo and commit the file"

# .veans.yml is flat at the top level for the keys we need; a YAML parser
# would be one more thing to install on a runner for two scalar reads.
yml_scalar() {
	sed -n -E "s/^$1:[[:space:]]*//p" "$WORKMAN_CONFIG" | head -n1 | sed -E 's/^["'"'"']?//; s/["'"'"']?[[:space:]]*(#.*)?$//'
}

SERVER="${WORKMAN_SERVER:-$(yml_scalar server)}"
PROJECT_ID="$(yml_scalar project_id)"
PROJECT_IDENTIFIER="$(yml_scalar project_identifier)"
[[ -n "$SERVER" ]] || fail "server is not set in $WORKMAN_CONFIG and no server input was given"
[[ "$PROJECT_ID" =~ ^[0-9]+$ ]] || fail "project_id in $WORKMAN_CONFIG is not numeric: '$PROJECT_ID'"
SERVER="${SERVER%/}"

# ---------------------------------------------------------------------------
# Collect Refs: trailers from every commit on the PR.
# Accepts `Refs: PROJ-12`, `Refs: #12`, `Refs: 12`, and comma/space lists.
# ---------------------------------------------------------------------------
if ! git cat-file -e "$BASE_SHA^{commit}" 2>/dev/null || ! git cat-file -e "$HEAD_SHA^{commit}" 2>/dev/null; then
	fail "base or head commit is not available locally — check out with fetch-depth: 0"
fi

mapfile -t refs < <(
	git log --format=%B "$BASE_SHA".."$HEAD_SHA" \
		| grep -Eio '^[[:space:]]*refs?:[[:space:]]*.*$' \
		| sed -E 's/^[[:space:]]*[Rr][Ee][Ff][Ss]?:[[:space:]]*//' \
		| tr ',' ' ' \
		| tr -s '[:space:]' '\n' \
		| grep -E '^(#?[0-9]+|[A-Za-z][A-Za-z0-9_]*-[0-9]+)$' \
		| sort -u
)

if [[ ${#refs[@]} -eq 0 ]]; then
	log "No Refs: trailers between $BASE_SHA and $HEAD_SHA — nothing to close."
	echo "closed=" >> "${GITHUB_OUTPUT:-/dev/null}"
	exit 0
fi
log "Referenced tasks: ${refs[*]}"

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
status="$(api GET "/projects/$PROJECT_ID")"
[[ "$status" =~ ^2 ]] || fail "cannot reach $SERVER as project $PROJECT_ID (HTTP $status) — check the server URL and WORKMAN_TOKEN"

html_escape() {
	sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' <<<"$1"
}

pr_label="PR"
[[ -n "$PR_NUMBER" ]] && pr_label="PR #$PR_NUMBER"
comment="<p>Closed by merged <a href=\"$(html_escape "$PR_URL")\">$(html_escape "$pr_label")</a>"
[[ -n "$PR_TITLE" ]] && comment+=": $(html_escape "$PR_TITLE")"
comment+="</p>"
[[ -n "$PR_AUTHOR" || -n "$MERGE_SHA" ]] && comment+="<p><code>${MERGE_SHA:0:12}</code>${PR_AUTHOR:+ by $(html_escape "$PR_AUTHOR")}</p>"

closed=()
skipped=()

for ref in "${refs[@]}"; do
	# Split PROJ-12 / #12 / 12 into (prefix, index) and refuse another
	# project's identifiers rather than closing the wrong task by index.
	prefix=""
	index="${ref#\#}"
	if [[ "$ref" == *-* ]]; then
		prefix="${ref%-*}"
		index="${ref##*-}"
	fi
	if [[ -n "$prefix" && -n "$PROJECT_IDENTIFIER" && "${prefix^^}" != "${PROJECT_IDENTIFIER^^}" ]]; then
		warn "$ref belongs to project '$prefix', this repo tracks '$PROJECT_IDENTIFIER' — skipped"
		skipped+=("$ref")
		continue
	fi

	status="$(api GET "/projects/$PROJECT_ID/tasks/by-index/$index")"
	if [[ "$status" == "404" ]]; then
		warn "$ref: no task with index $index in project $PROJECT_ID — skipped"
		skipped+=("$ref")
		continue
	fi
	[[ "$status" =~ ^2 ]] || fail "$ref: resolving task index $index failed (HTTP $status): $(<"$API_BODY")"
	task_id="$(jq -r '.id' "$API_BODY")"
	task_done="$(jq -r '.done' "$API_BODY")"
	task_title="$(jq -r '.title' "$API_BODY")"

	if [[ "$task_done" == "true" ]]; then
		log "$ref ($task_title) is already done — skipped"
		skipped+=("$ref")
		continue
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "[dry-run] would close $ref ($task_title, id $task_id)"
		closed+=("$ref")
		continue
	fi

	# Merge-patch so only `done` is written; the server routes the task into
	# every kanban view's done bucket as a side effect, matching what a human
	# dragging it there would get.
	status="$(api PATCH "/tasks/$task_id" application/merge-patch+json --data '{"done":true}')"
	[[ "$status" =~ ^2 ]] || fail "$ref: marking task $task_id done failed (HTTP $status): $(<"$API_BODY")"
	status="$(api POST "/tasks/$task_id/comments" application/json --data "$(jq -cn --arg c "$comment" '{comment:$c}')")"
	[[ "$status" =~ ^2 ]] || warn "$ref: closed, but posting the PR comment failed (HTTP $status)"

	log "Closed $ref ($task_title)"
	closed+=("$ref")
done

echo "closed=${closed[*]:-}" >> "${GITHUB_OUTPUT:-/dev/null}"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	{
		echo "## Workman merge hook"
		[[ "$DRY_RUN" == "true" ]] && echo "_dry run — nothing was changed_"
		echo
		echo "| Task | Result |"
		echo "|---|---|"
		for r in "${closed[@]:-}"; do [[ -n "$r" ]] && echo "| \`$r\` | closed |"; done
		for r in "${skipped[@]:-}"; do [[ -n "$r" ]] && echo "| \`$r\` | skipped |"; done
	} >> "$GITHUB_STEP_SUMMARY"
fi
