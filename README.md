<h1 align="center">Workman</h1>

<p align="center"><strong>The work console you actually own.</strong></p>

[![Build Status](https://github.com/EPYCD/workman/actions/workflows/ci.yml/badge.svg)](https://github.com/EPYCD/workman/actions/workflows/ci.yml)
[![License: AGPL-3.0-or-later](https://img.shields.io/badge/License-AGPL--3.0--or--later-blue.svg)](LICENSE)

Workman is a self-hosted work management console: projects, tasks, time and
teams, in a dense, keyboard-first interface built for people who live in it all
day. It runs as a single Go binary with a Vue 3 web client, a desktop app,
CalDAV sync and a full REST API.

> [!NOTE]
> Workman powers [KAOXHQ](https://kaoxhq.tech).
>
> Workman is a derivative work of Vikunja, free software licensed under
> AGPL-3.0-or-later, and is distributed under that same licence. Upstream
> copyright notices are retained throughout the source, as the licence requires.

> [!NOTE]
> For the development of this project, we're using LLM-assisted coding tools in
> various parts of the codebase.

## Table of contents

- [Security Reports](#security-reports)
- [Features](#features)
- [Design](#design)
- [Docs](#docs)
- [Contributing](#contributing)
- [License](#license)
	- [Unsplash Images](#unsplash-images)

## Security Reports

If you find any security-related issues you don't want to disclose publicly,
please open a private security advisory on
[this repository](https://github.com/EPYCD/workman/security/advisories/new).

## Features

- **Projects** — nestable, shareable, archivable, with per-project task
  identifiers (`PROJ-12`) and custom backgrounds.
- **Four views per project** — List, Table, Kanban and Gantt, each with its own
  saved positions, filters and buckets.
- **Tasks** — rich-text descriptions, due/start/end dates, reminders, repeating
  schedules, priorities, assignees, labels, attachments, relations, comments and
  reactions.
- **Saved filters** — a query language over every task field, usable as
  pseudo-projects and as filter-driven Kanban buckets.
- **Teams and sharing** — user, team and password-protected link shares at
  read / write / admin.
- **Auth** — local accounts, TOTP 2FA, LDAP, OpenID Connect, scoped API tokens,
  bot users, and a built-in OAuth 2.0 + PKCE authorization server.
- **Sync and integrations** — CalDAV, webhooks, Atom feeds, and importers for
  Todoist, Trello, Microsoft To Do, TickTick, Wekan, Planka and CSV.
- **Ops** — Prometheus metrics, S3 or local file storage, rate limiting, autoTLS,
  structured logging, and a plugin system.
- **Licensed extras** — admin panel, time tracking and audit logs.

## Design

The interface is a tech-first operations console: a near-black ground, hairline
structure, monospace data, 45° chamfered corners and a single crimson accent.
The design system is documented in
[`frontend/src/styles/WORKMAN-DESIGN.md`](frontend/src/styles/WORKMAN-DESIGN.md)
— read it before touching any styling.

## Docs

* [Running your team's agents on a board](docs/agent-workflow.md)
* [Marshal, the repo-aware layer](docs/marshal.md)
* [Deploying behind a Cloudflare tunnel](deploy/README.md)

The build and deployment mechanics are unchanged from upstream, so its
documentation still applies until ours replaces it: [installing](https://vikunja.io/docs/installing/),
[building from source](https://vikunja.io/docs/build-from-sources/),
[development setup](https://vikunja.io/docs/development/),
[magefile](https://vikunja.io/docs/magefile/) and [testing](https://vikunja.io/docs/testing/).

## Contributing

See [AGENTS.md](AGENTS.md) for the conventions this repository follows —
API versioning, permissions, migrations and code style.

The frontend typecheck is a ratchet: `pnpm typecheck:gate` fails only when a
file gains type errors against `frontend/typecheck-baseline.json`. After
fixing errors, `pnpm typecheck:baseline` records the lower count.

## License

Most of this repository is licensed under [AGPL‑3.0‑or‑later](LICENSE).
The contents of [`desktop/`](desktop/) are licensed under
[GPL‑3.0‑or‑later](desktop/LICENSE).

### Unsplash Images

Background images from Unsplash are distributed under the [Unsplash License](https://unsplash.com/license). The license requires giving credit to the photographer and Unsplash. See [Unsplash’s terms](https://unsplash.com/terms) for more information.
