#!/usr/bin/env bash
#
# The forward direction of the merge hook: when a PR is opened (or CI fails
# on it), tell the referenced tasks.
#
#   MODE=opened     link the PR on each task and park it In Review
#   MODE=ci-failed  post a comment with the failed run on each task
#
# Idempotent: a task that already carries a comment linking this PR (or run)
# is left alone, so synchronize events and re-runs stay quiet.

# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
MODE="${MODE:-opened}"
RUN_URL="${RUN_URL:-}"

collect_refs
if [[ ${#refs[@]} -eq 0 ]]; then
	log "No Refs: trailers between $BASE_SHA and $HEAD_SHA — nothing to update."
	exit 0
fi
require_server

pr_label="PR"
[[ -n "$PR_NUMBER" ]] && pr_label="PR #$PR_NUMBER"
case "$MODE" in
	opened)
		needle="$PR_URL"
		comment="<p>Opened <a href=\"$(html_escape "$PR_URL")\">$(html_escape "$pr_label")</a>"
		[[ -n "$PR_TITLE" ]] && comment+=": $(html_escape "$PR_TITLE")"
		comment+="${PR_AUTHOR:+ by $(html_escape "$PR_AUTHOR")}</p>"
		;;
	ci-failed)
		needle="$RUN_URL"
		[[ -n "$RUN_URL" ]] || fail "RUN_URL is required for ci-failed"
		comment="<p>CI failed on <a href=\"$(html_escape "$PR_URL")\">$(html_escape "$pr_label")</a>: <a href=\"$(html_escape "$RUN_URL")\">see the run</a>.</p>"
		;;
	*) fail "unknown MODE '$MODE' (opened|ci-failed)" ;;
esac

updated=()
for ref in "${refs[@]}"; do
	task_id="$(resolve_ref "$ref")"
	[[ -n "$task_id" ]] || continue
	task_done="$(jq -r '.done' "$API_BODY")"
	task_title="$(jq -r '.title' "$API_BODY")"
	if [[ "$task_done" == "true" ]]; then
		log "$ref ($task_title) is done — skipped"
		continue
	fi
	if task_has_comment_with "$task_id" "$needle"; then
		log "$ref already links this ${MODE/ci-failed/run} — skipped"
		continue
	fi
	if [[ "$DRY_RUN" == "true" ]]; then
		log "[dry-run] would update $ref ($task_title)"
		updated+=("$ref")
		continue
	fi
	status="$(api POST "/tasks/$task_id/comments" application/json --data "$(jq -cn --arg c "$comment" '{comment:$c}')")"
	[[ "$status" =~ ^2 ]] || warn "$ref: posting the comment failed (HTTP $status)"

	if [[ "$MODE" == "opened" ]]; then
		if [[ "$VIEW_ID" =~ ^[0-9]+$ && "$BUCKET_IN_REVIEW" =~ ^[0-9]+$ ]]; then
			status="$(api PUT "/projects/$PROJECT_ID/views/$VIEW_ID/buckets/$BUCKET_IN_REVIEW/tasks" application/json --data "$(jq -cn --argjson t "$task_id" '{task_id:$t}')")"
			[[ "$status" =~ ^2 ]] || warn "$ref: moving to In Review failed (HTTP $status): $(<"$API_BODY")"
		else
			warn "$ref: view_id / buckets.in_review missing from $WORKMAN_CONFIG — PR linked but not moved"
		fi
	fi
	log "Updated $ref ($task_title)"
	updated+=("$ref")
done

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	{
		echo "## Workman PR hook ($MODE)"
		[[ "$DRY_RUN" == "true" ]] && echo "_dry run — nothing was changed_"
		for r in "${updated[@]:-}"; do [[ -n "$r" ]] && echo "- \`$r\`"; done
		true
	} >> "$GITHUB_STEP_SUMMARY"
fi
exit 0
