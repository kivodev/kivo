# Architecture decision records

One short file per significant decision, recording what was decided and why. [ARCHITECTURE.md](../../ARCHITECTURE.md) describes the system as it is now.

Write one when a change sets a direction that would be expensive to reverse or that others will build on: a new dependency at the core of the server, a change to the API's compatibility rules, a new storage format, a new client platform. Bug fixes and ordinary features don't need one.

## Naming

Number records in order, with a short title in lowercase and hyphens: `0001-sqlite-as-the-only-database.md`, `0002-one-ffmpeg-process-per-play.md`. Numbers are never reused.

## Status

A record is **Proposed** while it's being discussed in its pull request, **Accepted** once merged, and **Superseded by 00NN** when a later record replaces it. Don't rewrite an accepted record; write a new one and mark the old one superseded.

## Template

```markdown
# 0001: Short title

Status: Accepted

## Context

What problem or force makes a decision necessary, and what constraints apply.

## Decision

What we decided, stated plainly.

## Consequences

What becomes easier or harder because of it, including the costs we accept.
```
