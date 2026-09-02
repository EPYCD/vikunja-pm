# veans

A beans-shaped CLI for Vikunja. Drop it into a repo, run `veans init`, paste a
hook snippet into your coding agent's settings, and the agent immediately
knows to track its work in Vikunja instead of in `TodoWrite` or `.beans/`.

veans is a thin Go binary that wraps Vikunja's REST API with an opinionated
agent-friendly surface and emits a system prompt teaching agents the workflow
(claim → work → in-review → human closes). The agent prompt is re-emitted on
every `SessionStart` and `PreCompact`, so context never goes stale.

## Quick start

```sh
# 1. Build (or download) the binary
cd veans && mage build && sudo install ./veans /usr/local/bin/

# 2. In a repo with a Vikunja instance reachable
veans init --server https://vikunja.example.com

# 3. Wire it into Claude Code (.claude/settings.json):
{
  "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "veans prime" }] }],
    "PreCompact":   [{ "hooks": [{ "type": "command", "command": "veans prime" }] }]
  }
}

# OpenCode (.opencode/plugin/veans-prime.ts):
export const VeansPrime = {
  event: ["session.start", "compact.before"],
  handler: async ({ exec }) => exec("veans prime"),
}
```

`veans prime` exits silently with status 0 when no `.veans.yml` is reachable
upward from cwd, so the hook is safe to install in a global `~/.claude/`
without breaking sessions in unrelated repos.

## What `veans init` does

1. Authenticates as you. Default is OAuth 2.0 Authorization Code + PKCE
   against Vikunja's built-in authorization server (Vikunja 2.3+ — no
   client registration needed). veans prints an authorize URL; you open
   it in your browser, sign in, and paste the resulting
   `vikunja-veans-cli://callback?code=...` URL back into the CLI. The
   browser will fail to open the custom scheme — that's expected; the
   address bar still has what we need.

   Alternative auth modes:
   - `--token <jwt-or-personal-api-token>` — paste-in, useful for SSO/OIDC
   - `--use-password` — fall back to `POST /login` (local accounts only)
   - `--username` + `--password` (non-interactive; implies `--use-password`)
2. Asks you to pick a project and a Kanban view.
3. Bootstraps the canonical buckets if missing: `Todo`, `In Progress`,
   `In Review`, `Done`, `Scrapped`.
4. Creates a `bot-<repo-name>` user (Vikunja bot user — no password, no
   email, can't log in interactively).
5. Shares the project with the bot at read+write.
6. Mints a long-lived API token for the bot via `PUT /tokens` with
   `owner_id`, scoped to the discovered route groups (tasks, comments,
   labels, relations, assignees, etc.) the server actually exposes.
7. Stores the token in your OS keychain (or
   `~/.config/veans/credentials.yml` if no keychain is available).
8. Writes `.veans.yml` to the repo root.

The token stored is the bot's, not yours. The human's transient session is
discarded as soon as init finishes — rotate or revoke the bot independently
without affecting your own session.

## Commands

```
veans init                     OAuth/login → create bot → mint token → write .veans.yml
veans prime                    emit system prompt for agents (silent if no .veans.yml)
veans list                     filtered list (--ready, --mine, --branch, --filter, --status); emits JSON
veans show <id>                view a task (JSON)
veans create "title"           --description, --label, --status, --priority, --parent, --blocked-by
veans update <id>              --status, --title, --priority, --label-add/remove,
                               --description, --description-replace-old/new, --description-append,
                               --comment, --reason, --if-unchanged-since
veans claim <id>               assign the bot, move to In Progress, tag with current branch label, lease paths_owned
veans ready                    ready queue with reasons (assigned / blocked / lease_conflict)
veans scope <id> [flags]       show or set the task's scope (paths owned/affected, endpoints, notes)
veans leases                   list the paths in-progress tasks are editing right now
veans release <id>             drop a task's leases without changing its status
veans heartbeat <id>           mark a task's leases active (long silent work)
veans check [--staged]         changed files vs the referenced tasks' scopes and others' leases
veans plan [--dry-run] <file>  lint a decomposition and create all of it in one transaction
veans plan --export            the open board in plan shape, keyed by task identifier
veans agents                   who is working in the project; bots carry their owner
veans watch                    follow the ready queue and leases; one JSON line per change (--exec, --once)
veans hooks install|uninstall  git pre-commit hook that runs `veans check --staged`
veans api METHOD PATH          raw REST passthrough — escape hatch for endpoints not wrapped here
veans login                    re-mint the bot's token (rotation)
veans version
```

Task IDs accept `PROJ-NN` (when the project has an identifier), `#NN`
(when it doesn't), or a bare integer.

## `.veans.yml`

Committed to the repo root. The numeric IDs are the source of truth; cached
identifiers and bot username are for human-readable output.

```yaml
server: https://vikunja.example.com
project_id: 42
project_identifier: PROJ        # may be "" — task IDs render as #NN then
view_id: 7
buckets:
  todo: 11
  in_progress: 12
  in_review: 13
  done: 14
  scrapped: 15
bot:
  username: bot-myrepo
  user_id: 99
```

## Credentials

Resolved in order on every command:

1. **OS keychain** (macOS Keychain, Windows Credential Manager,
   libsecret/gnome-keyring on Linux), via `github.com/zalando/go-keyring`.
2. **`VEANS_TOKEN`** env var (read-only). Optionally pin to a server with
   `VEANS_SERVER`. Intended for CI / containers.
3. **`~/.config/veans/credentials.yml`** (mode 0600) — automatic fallback
   when the keychain is unavailable. Honors `XDG_CONFIG_HOME`.

## Mage targets

```
mage build              # go build -o ./veans ./cmd/veans
mage test               # unit tests across the module
mage test:filter EXPR   # go test -run EXPR ./...
mage test:e2e           # e2e suite (needs VEANS_E2E_API_URL)
mage lint / lint:fix    # golangci-lint
mage fmt                # go fmt ./...
mage clean              # remove built binary
```

## End-to-end tests

The suite in `e2e/` assumes a running Vikunja API. Locally, point it at any
dev instance:

```sh
export VEANS_E2E_API_URL=http://localhost:3456
export VEANS_E2E_ADMIN_USER=user1
export VEANS_E2E_ADMIN_PASS=12345678   # canonical fixture password
mage test:e2e
```

CI spins Vikunja up the same way the frontend Playwright suite does — see
`.github/workflows/veans-e2e.yml`. The workflow builds the parent API
binary, starts it with `VIKUNJA_DATABASE_TYPE=sqlite`,
`VIKUNJA_DATABASE_PATH=memory`, fixtures from `pkg/db/fixtures/`, and runs
`mage test:e2e` from this directory.

E2E tests never touch the developer's keychain — they override `HOME` and
`XDG_CONFIG_HOME` per test, which forces the credential store to fall
through to its file backend.

## Status model

| Status        | Bucket name    | Done flag | Who moves there?                         |
| ------------- | -------------- | --------- | ---------------------------------------- |
| `todo`        | Todo           | false     | created here by default                  |
| `in-progress` | In Progress    | false     | `veans claim` / `update -s in-progress`  |
| `in-review`   | In Review      | false     | the agent, when work is finished         |
| `completed`   | Done           | true      | the merge hook when the PR lands, or a human |
| `scrapped`    | Scrapped       | true      | the agent, with `--reason`               |

The agent never moves tasks to `completed` itself — it parks them in
`In Review`, and merging the PR closes them (see *Merge hook* below).

## Claiming is atomic

`veans claim` calls `POST /api/v2/tasks/{id}/claim`, which checks, moves
and assigns in one server-side transaction (the current bucket row is read
under `SELECT … FOR UPDATE` on Postgres and MySQL). Two agents racing for
the same task get exactly one winner; the loser exits non-zero with
`CONFLICT` and must pick another task. The claim also refuses a task that
has left the Todo bucket since it was listed, so acting on a stale
`list --ready` is safe. `--force` lifts the Todo guard (for picking up a
task a human parked In Review); nothing overrides another user's claim.
Re-claiming a task you already hold is a no-op.


## Scope and path leases

Every task can carry a scope — `paths_owned` (the files it will edit),
`paths_affected` (files it reads or depends on), `endpoints` and free-form
`notes` — written by whoever decomposes the work:

```
veans create "atomic claim" --paths-owned pkg/models/task_claim.go --paths-owned pkg/routes/api/v2/task_claim.go \
  --paths-affected pkg/models/tasks.go --endpoint "POST /api/v2/tasks/{id}/claim"
veans scope PROJ-12 --paths-owned frontend/src/components/tasks/**
```

Only `paths_owned` is enforced. `veans claim` leases those globs for the
project (`POST /tasks/{id}/claim` does it in the same transaction as the
assignment); a claim whose owned paths overlap a lease held by another
in-progress task is refused with `CONFLICT`, as is widening a claimed task's
scope onto a leased path. Leases go away when the task is done, scrapped,
deleted, or explicitly released (`veans release`). Overlap is judged on
patterns, conservatively: `pkg/models/**` collides with `pkg/models/tasks.go`
and with `pkg/**/*.go`.

Paths are repository-relative. A project that spans several repositories
namespaces them with a `repo:` prefix (`api:pkg/models/**`, `web:src/**`);
patterns in different repositories never collide. Set `repository: api` in
a checkout's `.veans.yml` and `veans create`/`veans scope` prefix bare paths
for you.

Bots set up before scopes existed hold tokens without the new
permissions; `veans login` mints a fresh token with everything the server
offers.

## Staying inside the lines

`veans check` lists the files changed since the main branch (or staged with
`--staged`), reads the `Refs:` trailers from the commits and asks
`POST /projects/{id}/scope-check` for a verdict per file: `owned`,
`affected` (declared read-only, yet changed), `unscoped`, or
`leased_by_other`. It exits `CONFLICT` on strays (when the referenced tasks
declare `paths_owned`) and on collisions. The `check` mode of the
`workman-merge-hook` action runs the same query on every pull request and
posts the verdict as a PR comment (see `examples/pr-hooks.yml`); the
`opened` mode links the PR on its tasks and parks them In Review, and
`ci-failed` (`examples/ci-failed-hook.yml`) comments failed runs.

## Planning a decomposition

`veans plan plan.json` sends a whole breakdown — tasks with plan-local keys,
`parent_key`, `blocked_by`, `follows` and scopes — to
`POST /projects/{id}/plan`, which lints it as a set and creates everything
in one transaction. Errors (duplicate or unknown keys, cycles, invalid
paths) stop creation; warnings (missing `paths_owned`, overlapping paths
without an ordering, overlap with an open task on the board) are reported
alongside the created tasks. `--dry-run` only lints. See `veans prime` for
the plan shape.

Re-planning is incremental: `veans plan --export` prints the open board in
the same shape keyed by identifier (`PROJ-12`, `#12`), and a plan may
reference those keys in `parent_key`, `blocked_by` and `follows` without
redefining them — the server resolves them against the board.

The ready queue is ordered: `veans list --ready` returns tasks in the
board's drag order and `--first` picks the top one, so the owner
prioritises by dragging cards in the Todo column.

## Stale leases

Leases record the holder's last activity — a claim, task update, comment or
`veans heartbeat`. After `service.leasestaleafter` (default 4h) without any,
`veans leases`, `veans ready` and the board flag them `stale`. They still
block; the flag tells a human they can release safely.

A cron on the server polices them every five minutes. The first time a lease
crosses the threshold, the holder is notified once — for a bot, the human
who owns it, since bots receive nothing. With `service.leaseautoreleaseafter`
set (default `0`, off; e.g. `24h`), leases silent for that long are released
outright: the task keeps its status and assignee, a comment on it says the
paths were freed, and the `task.leases.released` webhook fires so an
orchestrator can hand the work on.

## Who is working here

`veans agents` (`GET /api/v2/projects/{id}/agents`) lists every user assigned
to an open task or holding a lease, bots first with the `owner` behind each,
plus their open-task and lease counts. The board's "By assignee" lanes and
the Leases panel read the same answer to show `bot-alice · for Alice`.

## Orchestrating

`veans watch` polls the ready queue and the leases and prints one JSON line
per change: `task.ready` when a task becomes claimable (also for everything
already ready when the watcher starts), `task.unready` when a ready task is
claimed, blocked or removed, and `lease.stale` when a holder goes quiet.
`--exec CMD` runs a shell command per event with the JSON on stdin and in
`$VEANS_EVENT`; `--once` prints the current ready tasks and exits, for cron.
Transient server errors are emitted as `error` events and polling continues.

## Pre-commit hook

`veans hooks install` writes `.git/hooks/pre-commit` so every commit runs
`veans check --staged`; a stray or a collision aborts the commit with the
verdicts on stdout. Until a commit on the branch carries a `Refs:` trailer,
the check judges the index against the tasks the bot has claimed. `veans init --install-git-hook` does it during setup.
The hook is a no-op when veans is not on PATH or nothing is staged, and
`VEANS_SKIP_CHECK=1 git commit` bypasses it once. A pre-commit hook veans
did not write is never touched (`--force` replaces it); `veans hooks
uninstall` removes only the veans one.

`veans ready` is the server's ready queue (`GET
/projects/{id}/views/{view}/readiness`): every Todo task with `ready` and
the reasons it is not — `assigned`, `blocked` (with `blocked_by`) or
`lease_conflict` (with `lease_conflicts` naming the holder). `veans list
--ready` is that queue reduced to the claimable tasks, so agents and the
board never disagree about what can be picked up.
## Merge hook

`.github/actions/workman-merge-hook` in the parent repo closes the tasks a
merged PR references. It scans every commit on the PR for a `Refs:`
trailer (`Refs: PROJ-12`, `Refs: #12, #13`), marks each task done through
`/api/v2` and leaves a comment linking back to the PR. Already-done tasks
are skipped, so re-runs are safe. It is plain curl + jq — no veans binary
on the runner, and it works against a self-hosted Workman that GitHub
cannot reach inbound.

Copy `examples/merge-hook.yml` to `.github/workflows/` in a repo with a
committed `.veans.yml` and add the bot token as the `WORKMAN_TOKEN` secret.
The PR merge is the human gate: nobody has to touch the board.

## Marshal

`marshal` (built from `cmd/marshal`, same module) is the repo-aware layer over
the board: spec references that resolve from the repository at read time,
CODEOWNERS chokepoint queues, branch-vs-claim reconciliation, the ownership
lock on the board's In Progress column, CI gate receipts that gate "done",
per-worker worktrees with a database and port from a pool, a hash-chained
ledger, Discord cards for every pipeline event, and an MCP server for agents.
See `docs/marshal.md` for the design and `deploy/README.md` for running it
next to Workman behind a Cloudflare tunnel.

```
marshal init                   write .marshal.yml (spec files, CODEOWNERS, pool, service, Discord)
marshal setup --token <admin>  create the Marshal + CI bots, set receipt bot and claim bucket, register the webhook
marshal refs resolve|check     references with provenance; broken refs, pastes, unlinked tasks
marshal health                 the graph invariants
marshal chokepoints            CODEOWNERS queues
marshal open <path>            is anything open on this path
marshal reconcile              branches vs claims, stale branches
marshal worktree <task>        worktree command + database + port
marshal claims import          owns: labels → paths_owned
marshal receipt <task> ...     post a gate receipt as CI
marshal serve                  webhook receiver, panels API, watcher
marshal mcp                    agent tools over stdio
```

## Out of scope (for now)

- OAuth 2.0 device flow (RFC 8628) — would let SSH'd / headless setups
  authenticate without a browser-on-the-same-machine; not implemented
  upstream yet.
- Project-scoped API tokens — Vikunja doesn't ship them yet. The
  credential schema's `scope` field is forward-compatible for when it does.
- Auto-installing hook snippets. We print them; you paste them.
