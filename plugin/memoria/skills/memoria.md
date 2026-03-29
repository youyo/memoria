# memoria SKILL

## Overview

memoria is a long-term memory system for Claude Code.

It stores structured knowledge extracted from coding sessions and retrieves relevant memories later, prioritizing:

1. Same project
2. Similar projects
3. Global knowledge

## When to use memoria

Use memoria when:

- recalling previous design decisions
- avoiding repeated mistakes
- recovering context after long sessions
- reusing knowledge across similar projects

## How memoria works

memoria automatically:

- captures important decisions at `Stop`
- ingests full sessions at `SessionEnd`
- injects recent memories at `SessionStart`
- retrieves prompt-relevant memories at `UserPromptSubmit`

## Best practices

- state decisions clearly
- describe constraints explicitly
- write actionable next steps
- make failures and lessons concrete

## Troubleshooting

```bash
memoria doctor
memoria worker status
```
