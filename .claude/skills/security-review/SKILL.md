---
name: security-review
description: Security review of the pending changes on the current branch (diff against origin/HEAD, including uncommitted work). Use when asked for a security review, to check a branch or PR for vulnerabilities, or before merging changes that touch SQL, filters, DSN parsing, or connection handling.
argument-hint: "[base-ref | baseline]"
allowed-tools: Bash(git diff:*), Bash(git status:*), Bash(git log:*), Bash(git show:*), Bash(git merge-base:*), Bash(git rev-parse:*), Read, Glob, Grep, Agent
---

Claude Code wrapper for the tool-agnostic workflow in `.ai/security-review.md`. Read that file now and follow it exactly. The threat model, method, and output format all live there.

Claude-specific notes:
- The change set for step 1 is already collected below against `origin/HEAD`. If the argument (`$ARGUMENTS`) is `baseline`, or the diff below is empty, run a baseline review of the whole repository. If it is a base ref, re-run those git commands against it instead.
- Step 6 (record): write the report to `local/security-review/<date>/report.md` and update the register and review history as `.ai/security-review.md` describes.
- Step 2 (find candidates): launch one `general-purpose` Agent. Give it the full contents of `.ai/security-review.md` plus the diff.
- Step 3 (verify): launch one Agent per candidate, in parallel. Tell each to read `.ai/security-review-false-positives.md` and the cited code, then return a confidence score.

GIT STATUS:

```
!`git status --short --branch`
```

FILES MODIFIED:

```
!`git diff --name-only --merge-base origin/HEAD`
```

COMMITS:

```
!`git log --no-decorate --oneline origin/HEAD..HEAD`
```

DIFF:

```
!`git diff --merge-base origin/HEAD`
```
