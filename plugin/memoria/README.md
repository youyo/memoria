# memoria Claude Code Plugin

memoria is a project-aware local RAG memory system for Claude Code. It automatically captures decisions, constraints, failures, and insights from coding sessions and retrieves them in future sessions.

## Features

- **SessionStart**: Injects relevant memories from previous sessions as context
- **UserPromptSubmit**: Retrieves prompt-relevant memories via FTS + vector search
- **Stop**: Captures important decisions as checkpoints
- **SessionEnd**: Queues full session transcript for ingestion

## Prerequisites

Install the `memoria` CLI:

```bash
go install github.com/youyo/memoria@latest
```

Initialize config:

```bash
memoria config init
```

## Installation

### From a plugin marketplace

```text
/plugin install memoria
```

### Manually (local)

```bash
cp -r plugin/memoria ~/.claude/plugins/
```

Then enable in Claude Code with `/plugin`.

## Hook commands

- `memoria hook session-start`
- `memoria hook user-prompt`
- `memoria hook stop`
- `memoria hook session-end`

## Manual hook setup (without plugin)

```bash
memoria config print-hook
```

Paste the output into your `.claude/settings.json`.

## Troubleshooting

```bash
memoria doctor
memoria worker status
```
