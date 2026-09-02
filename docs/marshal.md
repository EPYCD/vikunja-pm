# Marshal — the repo-aware layer over a Workman board

Marshal keeps the board honest against the repository it coordinates. It
runs beside Workman (`marshal serve`), as a CLI, and as an MCP server for
agents (`marshal mcp`). Two guards live inside Workman because a
one-second window there is the failure itself; everything else observes,
computes and reports from outside.

The requirements Marshal implements are the M-1 … M-5 series of the Marshal
brief; the working rules it enforces are the four rules in CapYard's
`docs/PARALLEL-WORK.md`. Neither is restated here.

## Inside Workman

| Guard | Where | Behaviour |
|---|---|---|
| Entering the **claim bucket** acquires the lock | `ProjectView.claim_bucket_id`; `updateTaskBucket` → `claimOnBucketEntry` | Any move into that bucket (drag, v1/v2 bucket move, project transfer) is refused with the same 409s as `POST /claim` while a blocker is open or an owned path is leased elsewhere; on success the task's `paths_owned` are leased to its sole assignee, else to the mover, who is assigned. `POST /claim` is unchanged. |
| **Done** needs a receipt | `Project.receipt_bot_id`; `checkDoneAllowed` in task update and done-bucket entry | With a receipt bot set, a task cannot be marked done without a merged, passing `TaskReceipt` (409/4039), and the user recorded as having moved it into its current bucket cannot close it (409/4040). CI (the receipt bot) is exempt from the second rule. Parent auto-completion is not gated. |
| **Receipts** | `POST/GET /api/v2/tasks/{id}/receipts` | Only the receipt bot's token may post (403 otherwise). Append-only. `passed` is computed: every gate passed and, when `docs_api_required`, the docs were regenerated. Fires `task.receipt.created`. |
| **Blocked cycles** | `TaskRelation.Create` | `blocked`/`blocking` and `follows`/`precedes` get the cycle check the hierarchy already had (409/4023). |
| **Who holds what** | `ErrPathLeaseConflict` | A refusal now names the holder's username, branch (`veans:branch:` label) and when the lease was taken. |
| `task_buckets.moved_by_id` / `moved_at` | every bucket move | The reviewer rule's evidence. |

Config: `service.marshalurl` (`VIKUNJA_SERVICE_MARSHALURL`) is exposed on
`/api/v1/info` as `marshal_url`; the frontend shows the reference,
health and worker panels when it is set.

## Outside: the marshal binary

`.marshal.yml` (next to `.veans.yml`, written by `marshal init`) says where
the spec lives and how the pool is shaped:

```yaml
references:                                  # anchors resolve from these
  - {prefix: FR-,  file: _bmad-output/prds/.../prd.md}
  - {prefix: NFR-, file: _bmad-output/prds/.../prd.md}
  - {prefix: AD-,  file: _bmad-output/architecture/.../ARCHITECTURE-SPINE.md}
  - {prefix: D-,   file: _bmad-output/epics.md}
paste_min_words: 12
codeowners: .github/CODEOWNERS
app_root: captain-yard-web                   # board paths are relative to this
docs_api_paths: [src/lib/contract.ts, src/lib/contract/**, src/lib/contract-routes.ts]
docs_api_output: docs/API.md
pool:
  databases: [mongodb://127.0.0.1:27101/cy1, mongodb://127.0.0.1:27102/cy2]
  ports: 3100-3110
worktree:
  branch: "{{.Story}}-{{.Slug}}"             # e5.3-scheduler
  dir: "../{{.Repo}}-{{.Story}}"             # ../capyard-e5.3
stale_after: 72h
serve:
  listen: 127.0.0.1:8090
  public_url: https://marshal.example.com
  poll: 60s
discord:
  username: Marshal
```

Secrets never live in the file: `MARSHAL_TOKEN`, `MARSHAL_WEBHOOK_SECRET`
(or `.marshal/webhook-secret`), `MARSHAL_DISCORD_WEBHOOK`.

### Commands

| Command | Does |
|---|---|
| `marshal init` | write `.marshal.yml`, ignore `.marshal/` |
| `marshal setup --token …` | create the Marshal and CI bots, share, mint tokens, set the receipt bot and the claim bucket, register the signed webhook |
| `marshal refs resolve FR-161 AD-14` | the anchor text with `file@sha` provenance; `NOT_FOUND` when an anchor vanished |
| `marshal refs check [task]` | broken references, verbatim pastes and unlinked tasks, board-wide or for one task; exit `CONFLICT` when any |
| `marshal refs index` | every anchor the sources define |
| `marshal health` | the invariants: every story claims files, no unordered overlap, no blocked cycle, unblocked roots; exit `CONFLICT` when violated |
| `marshal chokepoints` | the CODEOWNERS chokepoints and the queue on each |
| `marshal open <path>` | is anything open on this path |
| `marshal reconcile [--branch b]` | each claimed task's branch diffed against its claim (strays, collisions) and its last commit (stale) |
| `marshal worktree <task>` | the `git worktree add` command, branch name from the story id, a database and port from the pool, the checkout recorded |
| `marshal pool list|release` | allocations, worktrees, agents, checkout conflicts |
| `marshal claims import [--apply] [--drop-labels]` | `owns:` labels → `paths_owned`; non-path labels are reported, never guessed |
| `marshal receipt <task> --commit … --gate name=status:ms …` | post a receipt as CI from any CI system |
| `marshal ledger tail|verify` | the hash-chained record |
| `marshal notify test|replay` | Discord |
| `marshal serve [--once]` | the service |
| `marshal mcp` | the agent tools over stdio |

### The service

`marshal serve` listens on `serve.listen`:

- `POST /webhooks/workman` — the board's deliveries, HMAC-SHA256 verified
  (`X-Vikunja-Signature`), written to the ledger and relayed to Discord.
- `GET /api/tasks/{id}/references`, `/api/references`, `/api/health`,
  `/api/chokepoints`, `/api/workers`, `/api/open?path=`, `/api/reconcile`,
  `/api/ledger` — the panels' JSON, authenticated by forwarding the caller's
  Workman bearer token to the board.
- Every `serve.poll`: `git fetch`, rebuild the anchor index at `--spec-rev`,
  diff it against the previous index (drift → comment, `marshal:drift`
  label, Discord card to the assignee), re-check broken references and
  pastes, diff every claimed branch against its claim and its last commit,
  recompute the invariants. Each finding is announced once per state
  (`.marshal/flags.json`) and cleared when fixed.

### MCP tools

`list_startable_tasks`, `claim_task`, `report_status`, `attach_receipt`,
`release_task`, `open_on_path`, `resolve_references`, `check_scope`,
`worktree_plan`, `board_health`. The identity is the token the server runs
with (`VEANS_TOKEN` per agent, or `MARSHAL_TOKEN`); refusals are the board's
own, so an agent and a human get the same answer. Claude Code:

```json
{"mcpServers": {"marshal": {"command": "marshal", "args": ["mcp"], "env": {"VEANS_TOKEN": "tk_…"}}}}
```

## Traceability

| Req | Where |
|---|---|
| M3.1–M3.6 | `refs` package, `marshal refs`, watcher drift/broken/paste loops, task references panel; Marshal never writes to the repository |
| M1.1 | `marshal claims import` |
| M1.2, M1.11 | claim bucket guard in Workman; merge hook closes on merge |
| M1.3, M1.6 | path patterns (prefix covers subtree, `**`) |
| M1.4 | `ErrPathLeaseConflict` with holder username, branch, since |
| M1.5 | claim refuses blocked tasks; plan linter and invariants treat ordering as resolving overlap |
| M1.7 | `codeowners` package, `marshal chokepoints`, panel |
| M1.8, M1.9 | `marshal reconcile`, watcher stray/stale loops, `stale_after` |
| M1.10 | `invariants` package, `marshal health`, panel, watcher |
| M1.12 | ledger (hash-chained), board webhooks into it |
| M2.1–M2.6 | `TaskReceipt`, receipt bot, done guard, action `receipt`/`close` modes, `docs_api_required` derived from the diff |
| M2.7 | `moved_by_id` + `ErrTaskDoneBySubmitter` |
| M4.1–M4.4 | `worktree` and `pool` packages, `marshal worktree`, `marshal pool`, workers panel, checkout conflicts |
| M5.1–M5.4 | `mcptools`, per-agent tokens from env/credential store |
