---
name: search
description: Search memoria's memory database for past decisions, constraints, failures, patterns, and knowledge from previous sessions. Use when recalling previous work, checking if a problem was solved before, or looking up project history.
argument-hint: <query>
allowed-tools: Bash(memoria *)
---

Search memoria's long-term memory for relevant past knowledge.

## Execution

Run the following command and present the results to the user:

```bash
memoria memory search "$ARGUMENTS" --limit 10
```

If results are found, summarize them with relevance to the current context.
If no results, suggest alternative search terms or inform that no matching memories exist.

## When to use

- Recalling previous design decisions
- Checking if a similar problem was solved before
- Looking up project constraints or failure patterns
- Finding technical patterns from other projects
