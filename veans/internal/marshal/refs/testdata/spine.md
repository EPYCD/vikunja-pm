# Architecture Spine

Some context about the spine that is not an anchor.

### AD-14 — A work item owns files

Files are stored under the work item's directory. Moving a work item
moves its files with it.

- Storage backend: local disk or S3.
- Retention: thirty days after deletion.

### AD-15 — Buckets are per view

Each project view carries its own bucket configuration.

#### Consequences

This heading ends the text of AD-15.
