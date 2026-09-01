# Running your team's agents on a Workman board

Workman is built for boards where the work is done by coding agents and the
humans plan, review and merge. This is the shortest path from an empty
project to that loop. The moving parts are the board itself, the `veans` CLI
each agent talks through, and the GitHub merge hook.

## 1. One project, one kanban view

Create a project with a kanban view whose buckets are `Todo`, `In Progress`,
`In Review`, `Done` and `Scrapped`; `veans init` offers to create them. The
`Todo` bucket is the queue: everything the server's readiness check looks at
lives there.

## 2. One bot per teammate

Each teammate runs `veans init` in their checkout. It creates a bot user
owned by that person, shares the project with it, mints an API token with
exactly the permissions the server offers, writes `.veans.yml` and installs a
`veans prime` hook so their agent starts every session with the board
instructions. Bots set up before an upgrade run `veans login` to pick up new
permissions.

## 3. The owner's agent decomposes the PRD

Tasks carry a **scope**: the files they own, the files they touch and the
endpoints they change. Write the whole breakdown as one plan and let the
server lint it before anything exists:

```
veans plan --dry-run plan.json   # duplicate/unknown keys, cycles, overlapping paths with no order
veans plan plan.json             # tasks + relations + scopes in one transaction
```

`plan.json` is a `tasks` array with plan-local keys, `parent_key`,
`blocked_by`, `follows` and a `scope` per task (`veans prime` shows the
shape). Single tasks still work one at a time:

```
veans create "atomic task claim" \
  --paths-owned pkg/models/task_claim.go --paths-owned pkg/routes/api/v2/task_claim.go \
  --paths-affected pkg/models/tasks.go \
  --endpoint "POST /api/v2/tasks/{id}/claim" \
  --scope-notes "<p>No frontend changes.</p>"
veans create "board badges" --paths-owned frontend/src/components/tasks/** --blocked-by PROJ-12
```

Ordering comes from relations: `--blocked-by` for hard prerequisites,
`follows`/`precedes` for sequence, `--parent` for epics. A parent with open
subtasks is not claimable; with `service.autocompleteparenttasks: true` it
closes itself when the last subtask lands.

Multi-repo projects prefix paths with the repository (`api:pkg/**`,
`web:src/**`), or set `repository: api` in each checkout's `.veans.yml` and
let veans do it.

## 4. Agents pick work without stepping on each other

```
veans ready              # every queued task, with why it is not ready
veans list --ready       # only the claimable ones
veans claim PROJ-12      # atomic: assign, move to In Progress, lease paths_owned
```

The claim is a single transaction. It is refused with `CONFLICT` when someone
else holds the task, when a blocker is open, or when one of the task's owned
paths overlaps a lease held by another in-progress task. Two agents racing
for the same task get exactly one winner. `veans leases` shows who holds
what; `veans release` gives paths back without changing status. The board
shows the same answer: `READY`, `BLOCKED`, `PATH LEASED` badges on queued
cards, a lock count on in-progress ones, a **Leases** panel and a
**Ready only** toggle.

## 5. Finish through a pull request

Before opening the PR an agent runs `veans check`: every changed file must
be inside its tasks' `paths_owned` and outside every other task's lease, or
the command exits `CONFLICT` with the offending files. Agents then move the
task to `In Review`, post a summary comment and put `Refs: PROJ-12` in their
commits.

The `workman-merge-hook` GitHub Action does the rest (examples in
`veans/examples/`):

* `opened` links the PR on its tasks and parks them In Review.
* `check` runs the same scope check on the PR and fails it on strays or
  collisions, posting the verdict as a PR comment.
* `ci-failed` comments failed runs on the tasks.
* `close` marks the tasks done when the PR merges — which releases their
  leases — and comments the PR link. Never mark tasks done by hand.

## 5b. Crashed agents

Leases remember the holder's last activity. After `service.leasestaleafter`
(default 4h) without a claim, update, comment or `veans heartbeat`, the
board and `veans ready` flag the lease `stale`. It still blocks; a human
releases it from the Leases panel when nobody is working on the task.

## 6. Watch it

Webhooks fire `task.claimed` and `task.leases.released` alongside the usual
task events, so an orchestrator can wake idle agents instead of polling.

## Reference

* `veans/README.md` — every command and flag.
* `GET /api/v2/projects/{id}/views/{view}/readiness` — the ready queue.
* `GET /api/v2/projects/{id}/leases`, `PUT /api/v2/tasks/{id}/scope` — the
  data behind the board.
