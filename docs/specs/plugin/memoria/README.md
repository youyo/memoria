# memoria Claude Code Plugin — Design Reference

This directory contains design reference documents for the memoria plugin.

The actual plugin files are at the repository root:

```
repo-root/
├── .claude-plugin/
│   ├── marketplace.json
│   └── plugin.json
├── hooks/
│   └── hooks.json
└── skills/
    └── memoria/
        └── SKILL.md
```

## Hook commands

- `memoria hook session-start`
- `memoria hook user-prompt`
- `memoria hook stop`
- `memoria hook session-end`

## Install

In Claude Code, run `/plugin` and add marketplace: `youyo/memoria`.
