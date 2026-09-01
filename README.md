<h1 align="center">Workman</h1>

<p align="center"><strong>The work console you actually own.</strong></p>

[![Build Status](https://github.com/go-vikunja/vikunja/actions/workflows/ci.yml/badge.svg)](https://github.com/go-vikunja/vikunja/actions/workflows/ci.yml)
[![License: AGPL-3.0-or-later](https://img.shields.io/badge/License-AGPL--3.0--or--later-blue.svg)](LICENSE)
[![OpenAPI Docs](https://img.shields.io/badge/swagger-docs-brightgreen.svg)](https://try.vikunja.io/api/v2/docs)

Workman is a self-hosted work management console: projects, tasks, time and
teams, in a dense, keyboard-first interface built for people who live in it all
day. It runs as a single Go binary with a Vue 3 web client, a desktop app,
CalDAV sync and a full REST API.

> [!NOTE]
> Workman is built on [Vikunja](https://vikunja.io), which is free software
> licensed under AGPL-3.0-or-later. The upstream project does the heavy lifting;
> Workman is a rebranded, redesigned distribution of it. Please consider
> [supporting Vikunja](https://vikunja.io/support/) if you find this useful.

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
please use [the contact information on the upstream website](https://vikunja.io/contact/#security).

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

* [Installing](https://vikunja.io/docs/installing/)
* [Build from source](https://vikunja.io/docs/build-from-sources/)
* [Development setup](https://vikunja.io/docs/development/)
* [Magefile](https://vikunja.io/docs/magefile/)
* [Testing](https://vikunja.io/docs/testing/)

## Contributing

See [AGENTS.md](AGENTS.md) for the conventions this repository follows —
API versioning, permissions, migrations and code style.

## License

Most of this repository is licensed under [AGPL‑3.0‑or‑later](LICENSE).
The contents of [`desktop/`](desktop/) are licensed under
[GPL‑3.0‑or‑later](desktop/LICENSE).

### Unsplash Images

Background images from Unsplash are distributed under the [Unsplash License](https://unsplash.com/license). The license requires giving credit to the photographer and Unsplash. See [Unsplash’s terms](https://unsplash.com/terms) for more information.
