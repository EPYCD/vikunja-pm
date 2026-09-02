#!/usr/bin/env bash
#
# Post a gate receipt on every task a change references.
#
#   MODE=receipt   after the gates ran on a PR head: record the results
#   (called from close-merged.sh with MERGED=true before the task is closed)
#
# The receipt is CI's word, under CI's own token: which commit, which gates
# with what result and duration, whether the diff required the API docs to be
# regenerated (derived from the diff, never declared) and whether they were,
# the run URL, and whether the commit is on the merged branch. Red runs post
# too; failures belong on the record.
#
# Inputs (env):
#   GATES            JSON array [{"name":"typecheck","status":"passed","duration_ms":1200},...]
#   COMMIT           sha the gates ran on (default HEAD_SHA)
#   BRANCH           branch name (default GITHUB_HEAD_REF, then GITHUB_REF_NAME)
#   MERGED           "true" when COMMIT is on the merged branch
#   MERGE_SHA        the merge commit when merged
#   RUN_URL          link to the run
#   DOCS_API_PATHS   space-separated globs (relative to APP_ROOT) that make docs:api mandatory
#   DOCS_API_OUTPUT  the generated file whose presence in the diff means docs were regenerated
#   APP_ROOT         sub-directory the globs are relative to ("" for repo root)

# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

post_receipts() {
	local merged="${1:-false}"
	[[ -n "${GATES:-}" ]] || fail "GATES is required: a JSON array of {name,status,duration_ms}"
	jq -e 'type == "array" and length > 0' <<<"$GATES" >/dev/null || fail "GATES must be a non-empty JSON array"

	local commit="${COMMIT:-$HEAD_SHA}"
	local branch="${BRANCH:-${GITHUB_HEAD_REF:-${GITHUB_REF_NAME:-}}}"
	local docs_required=false docs_done=false
	local app_root="${APP_ROOT:-}"
	[[ -n "$app_root" ]] && app_root="${app_root%/}/"
	local docs_output="${DOCS_API_OUTPUT:-docs/API.md}"

	if [[ -n "$BASE_SHA" && -n "$HEAD_SHA" ]]; then
		mapfile -t changed < <(git diff --name-only --diff-filter=ACMRD "$BASE_SHA...$HEAD_SHA" 2>/dev/null || git diff --name-only --diff-filter=ACMRD "$BASE_SHA" "$HEAD_SHA")
		for f in "${changed[@]}"; do
			local rel="${f#"$app_root"}"
			[[ "$rel" == "$docs_output" ]] && docs_done=true
			for g in ${DOCS_API_PATHS:-src/lib/contract.ts src/lib/contract/** src/lib/contract-routes.ts}; do
				# shellcheck disable=SC2053
				if [[ "$rel" == $g ]] || [[ "$g" == *'/**' && "$rel" == "${g%/**}"/* ]]; then
					docs_required=true
				fi
			done
		done
	fi

	local body
	body="$(jq -cn \
		--arg commit "$commit" --arg branch "$branch" --arg run "${RUN_URL:-}" \
		--arg merge "${MERGE_SHA:-}" --argjson gates "$GATES" \
		--argjson merged "$merged" --argjson req "$docs_required" --argjson done "$docs_done" \
		'{commit_sha:$commit, branch:$branch, gates:$gates, docs_api_required:$req, docs_api_regenerated:$done, ci_run_url:$run, merged:$merged, merge_sha:$merge}')"

	posted=()
	for ref in "${refs[@]}"; do
		task_id="$(resolve_ref "$ref")"
		[[ -n "$task_id" ]] || continue
		if [[ "$DRY_RUN" == "true" ]]; then
			log "[dry-run] would post receipt on $ref: $body"
			posted+=("$ref")
			continue
		fi
		status="$(api POST "/tasks/$task_id/receipts" application/json --data "$body")"
		if [[ "$status" =~ ^2 ]]; then
			log "Receipt posted on $ref (passed=$(jq -r '.passed' "$API_BODY"), merged=$merged)"
			posted+=("$ref")
		elif [[ "$status" == "403" ]]; then
			fail "$ref: the token is not the project's receipt bot — run \`marshal setup\` and use the CI token it prints as WORKMAN_TOKEN"
		else
			fail "$ref: posting the receipt failed (HTTP $status): $(<"$API_BODY")"
		fi
	done
	echo "receipts=${posted[*]:-}" >> "${GITHUB_OUTPUT:-/dev/null}"
	echo "docs_api_required=$docs_required" >> "${GITHUB_OUTPUT:-/dev/null}"
	echo "docs_api_regenerated=$docs_done" >> "${GITHUB_OUTPUT:-/dev/null}"
}

# When sourced by close-merged.sh only the function is wanted.
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	collect_refs
	if [[ ${#refs[@]} -eq 0 ]]; then
		log "No Refs: trailers between $BASE_SHA and $HEAD_SHA — no task to receipt."
		echo "receipts=" >> "${GITHUB_OUTPUT:-/dev/null}"
		exit 0
	fi
	require_server
	post_receipts "${MERGED:-false}"
fi
