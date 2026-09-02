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

# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
MERGE_SHA="${MERGE_SHA:-}"

collect_refs
if [[ ${#refs[@]} -eq 0 ]]; then
	log "No Refs: trailers between $BASE_SHA and $HEAD_SHA — nothing to close."
	echo "closed=" >> "${GITHUB_OUTPUT:-/dev/null}"
	exit 0
fi
log "Referenced tasks: ${refs[*]}"
require_server

pr_label="PR"
[[ -n "$PR_NUMBER" ]] && pr_label="PR #$PR_NUMBER"
comment="<p>Closed by merged <a href=\"$(html_escape "$PR_URL")\">$(html_escape "$pr_label")</a>"
[[ -n "$PR_TITLE" ]] && comment+=": $(html_escape "$PR_TITLE")"
comment+="</p>"
[[ -n "$PR_AUTHOR" || -n "$MERGE_SHA" ]] && comment+="<p><code>${MERGE_SHA:0:12}</code>${PR_AUTHOR:+ by $(html_escape "$PR_AUTHOR")}</p>"

closed=()
skipped=()

for ref in "${refs[@]}"; do
	task_id="$(resolve_ref "$ref")"
	if [[ -z "$task_id" ]]; then
		skipped+=("$ref")
		continue
	fi
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
		true
	} >> "$GITHUB_STEP_SUMMARY"
fi
exit 0
