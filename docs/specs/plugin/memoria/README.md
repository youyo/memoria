# memoria Claude Code Plugin

This directory is the Claude Code plugin payload for memoria.

It is intended to distribute:

- hook integration
- memory-aware behavior
- skill guidance

## Hook commands

- `memoria hook session-start`
- `memoria hook user-prompt`
- `memoria hook stop`
- `memoria hook session-end`

## Install locally

```bash
cp -r plugin/memoria ~/.claude/plugins/
```

Then enable it inside Claude Code with `/plugin`.
