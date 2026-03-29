# memoria

memoria is a project-aware, LAG-powered memory system for Claude Code.

It automatically captures, structures, and retrieves knowledge from coding sessions across projects, with strong locality and similarity awareness.

## Documentation

- Japanese README: `README.ja.md`
- Detailed design: `docs/specs/SPEC.ja.md`
- CLI design: `docs/specs/CLI.ja.md`
- Hook contract: `docs/specs/HOOKS.ja.md`
- Data model: `docs/specs/SCHEMA.ja.md`
- Workers: `docs/specs/WORKERS.ja.md`
- Retrieval: `docs/specs/RETRIEVAL.ja.md`
- Agent skill: `skills/memoria/SKILL.md`

## Architecture

```text
Claude Code
  -> plugin (hooks + skill)
    -> memoria CLI (Go / Kong)
      -> ingest worker (Go)
      -> embedding worker (Python / uv / UDS)
      -> SQLite (queue + memory + retrieval metadata)
```

## Installation

### Go install

```bash
go install github.com/youyo/memoria@latest
```

### Homebrew (planned)

```bash
brew install youyo/tap/memoria
```

## Setup

### 1. Initialize config

```bash
memoria config init
```

### 2. Print Claude Code hook config

```bash
memoria config print-hook
```

### 3. Install Claude Code plugin

#### From a marketplace

```text
/plugin install memoria
```

#### Manually (local)

```bash
cp -r plugin/memoria ~/.claude/plugins/
```

Then enable in Claude Code with `/plugin`.

## Usage

```bash
memoria memory search "why session end is unreliable"
memoria worker status
memoria doctor
```
