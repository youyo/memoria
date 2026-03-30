---
name: timeline
description: Show a chronological timeline of memoria's stored memories. Use when reviewing what happened in recent sessions, understanding project history over time, or getting an overview of stored knowledge. Supports filtering by kind and project.
argument-hint: [--kind <kind>] [--limit <n>]
allowed-tools: Bash(memoria *)
---

Show a chronological timeline of memories stored in memoria.

## Execution

Run the appropriate command based on arguments:

Default (recent 20 entries):

```bash
memoria memory list --limit 20
```

With kind filter:

```bash
memoria memory list --kind $ARGUMENTS[0] --limit 20
```

With limit:

```bash
memoria memory list --limit $ARGUMENTS[0]
```

Parse $ARGUMENTS flexibly:
- If a number is given, use it as --limit
- If a kind name is given (decision/constraint/todo/failure/fact/preference/pattern), use it as --kind
- If both are given, apply both
- If "all" is given, use --limit 100

Present results as a formatted timeline with timestamps, kinds, and summaries.
For each entry, show the chunk_id prefix (first 8 chars) so the user can drill down with `/memoria:search` or `memoria memory get`.

## When to use

- Reviewing what happened in recent sessions
- Getting an overview of stored decisions and constraints
- Understanding the chronological flow of a project
- Checking what memories exist before searching for specific ones

## Output format

Present as a clean timeline:

```
[2026-03-30] decision  abc12345  stakeholder と議論し requirement を decided
[2026-03-30] pattern   def67890  func handleError() で panic: を workaround した
[2026-03-29] fact      ghi24680  SQLite の FTS5 を使った全文検索の実装
```
