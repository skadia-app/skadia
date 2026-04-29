## Summary

<!-- 1-3 bullet points: what changed and why. -->

## Verification

- [ ] Tests pass locally
- [ ] Manual UI verification (if frontend) — describe what was clicked through
- [ ] Migration dry-run (if SQL) — `EXPLAIN` output attached or noted as N/A
- [ ] No new secrets / large binaries / personal data introduced
- [ ] Pre-commit hook ran without `--no-verify`

## AI tooling disclosure

<!-- Required if any AI assistance was used to write any part of this diff.
Reviewers use this to interpret the diff (verbose comments, scaffolding
patterns, etc.), not to filter PRs. -->

- [ ] No AI assistance used
- [ ] AI agent used (tool: __________)

If you used an AI agent, confirm what YOU verified personally:

- [ ] Ran the relevant test suite locally and read the output
- [ ] Read every line of the diff before opening the PR
- [ ] Manually exercised the changed code path (UI click-through, curl, CLI)
- [ ] Compared behaviour against `main` for any regression-prone area
- [ ] Reviewed bot-review findings and addressed each one

## Reviewer checklist

- [ ] All bot-review comments resolved before requesting human review
- [ ] CHANGELOG entry added (if user-facing)
- [ ] Doc-Sync task completed (if part of an implementation plan)
- [ ] CODEOWNERS for sensitive paths approved (if applicable)
