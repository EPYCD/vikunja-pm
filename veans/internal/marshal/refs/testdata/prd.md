# Product Requirements

Some intro paragraph that is not an anchor and mentions FR-161 inline.

## Accounts

**FR-161 — An account can be created by anyone with a verified email address**

Anyone with a verified email address can create an account. The account
owner can change the display name at any time.

> Note: verification links expire after twenty-four hours.

**FR-161a — Account creation requires accepting the terms of service**

The sign-up form blocks submission until the terms checkbox is ticked.

**FR-161b — Account names are unique**

Two accounts can never share the same name, compared case-insensitively.

## Files

**FR-162 — A file belongs to exactly one work item**

Files uploaded to a work item stay with it when the item moves between projects.
Deleting the work item deletes its files after a thirty day grace period.


This paragraph sits after two blank lines and must not be part of FR-162.

**FR-163 — Files are scanned before download**

Every file is scanned by the configured antivirus before the first download
is served.

**NFR-N3 — Scanning finishes within five seconds**

The scan of a file up to one hundred megabytes finishes within five seconds.
