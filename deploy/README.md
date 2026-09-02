# Deploying Workman + Marshal behind a Cloudflare tunnel

One always-on machine, two public hostnames, one tunnel. The board reaches
Marshal through the public hostname (webhooks), GitHub reaches the board the
same way (receipts and merge hooks), and Discord is a plain outgoing webhook.

```
GitHub Actions ──(receipts, close)──▶ https://board.<domain>  ──▶ workman:3456
Workman webhooks ──(task.*, receipt)──▶ https://marshal.<domain> ──▶ marshal:8090 ──▶ Discord
agents (veans / marshal mcp) ────────▶ https://board.<domain>
```

## 1. Cloudflare

1. Zero Trust → Networks → Tunnels → *Create a tunnel* (Cloudflared). Copy the
   token into `CLOUDFLARE_TUNNEL_TOKEN`.
2. Public hostnames on that tunnel:
   - `board.<domain>` → `http://workman:3456`
   - `marshal.<domain>` → `http://marshal:8090`

   Service names resolve inside the compose network, so no ports are published
   on the host.

## 2. Configure

```bash
cd deploy
cp .env.example .env
$EDITOR .env          # hostnames, tunnel token, secrets, REPO_PATH
```

`REPO_PATH` is a clone of the coordinated repository (CapYard) on this
machine. Marshal reads specs and git history from it and fetches
`MARSHAL_SPEC_REV` on every poll; it never writes to it. Give the clone a
read-only deploy key if the repository is private.

## 3. Start the board

```bash
docker compose up -d workman db cloudflared
docker compose exec workman /app/vikunja/vikunja user create -u you -e you@example.com -p '...'
```

Open `https://board.<domain>`, sign in, create the project. Registration is
off by default (`WORKMAN_ENABLE_REGISTRATION`).

## 4. Wire the repository

In a checkout of the coordinated repository, once:

```bash
veans init --server https://board.<domain>        # bot, buckets, .veans.yml, hooks
marshal init --app-root captain-yard-web \
  --prd _bmad-output/prds/prd-captain-yard-networks-2026-09-01/prd.md \
  --spine _bmad-output/architecture/architecture-captain-yard-networks-2026-09-01/ARCHITECTURE-SPINE.md \
  --epics _bmad-output/epics.md \
  --public-url https://marshal.<domain> \
  --ports 3100-3110 --database mongodb://127.0.0.1:27101/cy1 --database mongodb://127.0.0.1:27102/cy2
marshal setup --token <your API token from the board's settings>
```

`marshal setup` creates `bot-marshal-<repo>` and `bot-ci-<repo>`, shares the
project with them, mints their tokens, sets the project's receipt bot and the
kanban view's claim bucket, and registers the signed webhook to
`https://marshal.<domain>/webhooks/workman`. It prints the CI token once:

- put it in the CapYard repository as the GitHub secret `WORKMAN_TOKEN`;
- copy the Marshal token (credential store, account `bot-marshal-<repo>`) into
  `.env` as `MARSHAL_TOKEN`;
- copy `.marshal/webhook-secret` into `.env` as `MARSHAL_WEBHOOK_SECRET`.

Commit `.veans.yml` and `.marshal.yml`; `.marshal/` is gitignored.

## 5. Start Marshal

```bash
docker compose up -d marshal
docker compose logs -f marshal      # "listening on 0.0.0.0:8090", then one tick line per poll
curl https://marshal.<domain>/healthz
```

## 6. Discord

Server settings → Integrations → Webhooks → *New Webhook* on the channel;
copy the URL into `DISCORD_WEBHOOK_URL` and restart Marshal. Then:

```bash
docker compose exec marshal marshal notify test
```

Cards posted: claimed, files released, done, created, deleted, gate receipts
(green/red with every gate's duration, docs:api state, merged flag, run
link), pull request linked, and Marshal's own findings: spec drift, broken
reference, pasted spec text, stale claim, out-of-scope change, board health.

## 7. GitHub

Copy `veans/examples/capyard-gates.yml` into the repository's
`.github/workflows/` (and `pr-hooks.yml` for the scope check on pull
requests). With the receipt bot set, a task closes only when the merge job's
receipt is green; the `Gates` workflow posts a red receipt otherwise and the
task stays in review.

## Updating

```bash
docker compose pull && docker compose up -d      # prebuilt images
docker compose build && docker compose up -d     # from this checkout
```

Migrations run on start. `marshal serve` is stateless apart from
`.marshal/` (ledger, pool registry, dedupe flags), kept in the
`marshal-state` volume.
