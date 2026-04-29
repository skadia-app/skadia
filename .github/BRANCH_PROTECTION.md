# Branch protection rules

GitHub branch-protection rules live in repo settings, not in code. This
document captures the recommended config so it can be re-applied if the
repo is recreated, forked into a new org, or the settings drift.

## `main` — recommended rules

Settings → Branches → Branch protection rule → Branch name pattern: `main`.

### Required status checks

Toggle "Require status checks to pass before merging". Add the checks below
**after the workflows have run at least once** so their names appear in the
search box:

- `pre-commit guard / guard`
- `pre-commit guard / gitleaks`
- `Repo Hygiene / actionlint (GHA YAML lint)`
- `Repo Hygiene / zizmor (GHA security audit)`
- `Repo Hygiene / hadolint (Dockerfile lint)`
- `Repo Hygiene / shellcheck (shell-script lint)`
- `Repo Hygiene / Block files > 500 KB outside whitelist`
- `CodeQL / Analyze (go)`
- `CodeQL / Analyze (javascript-typescript)`

`CodeQL / Analyze (python)` runs as part of the matrix but is intentionally
**not** required: services/ml/ is small relative to the Go/TS surface and
Python CodeQL alerts on test fixtures (which deliberately exercise error
paths) would block PRs without security value. The job still appears in the
Security tab when it finds something — treat findings as informational.

`Scorecard supply-chain audit / Scorecard analysis` is **not** a required
status check. The Scorecard workflow only triggers on `push`,
`branch_protection_rule`, and `schedule` — never on `pull_request`. Adding
it to the required list would mean every PR waits forever for a check that
never runs. Scorecard's value is the long-running supply-chain hygiene
score visible in the Security tab and on scorecard.dev; treat it as a
weekly health metric, not a per-PR gate.

Toggle "Require branches to be up to date before merging" — required so a
reviewer's approval doesn't certify code that was never co-evaluated with
the latest `main`.

### Required reviews

- "Require a pull request before merging" — on
- "Require approvals" — 1 minimum (CODEOWNERS will additionally require
  code-owner approval when touching protected paths)
- "Require review from Code Owners" — on
- "Dismiss stale pull request approvals when new commits are pushed" — on
- "Require approval of the most recent reviewable push" — on

### Other

- "Require signed commits" — recommended once GPG/SSH-signing is set up
  for the project owner
- "Restrict who can push to matching branches" — limit to the repo owner;
  all other commits go through PR
- "Allow force pushes" — off
- "Allow deletions" — off

### Empirical-test verification before relying on the gates

Two of the required checks above need an empirical confirmation that they
actually fail when they should fail. Do this BEFORE adding them to the
required-checks list, otherwise a passing-by-default check blocks nothing.

1. **`pre-commit guard / gitleaks`** — gitleaks-action's exit-code
   semantics on the Free Starter license (used here, no license key) are
   version-dependent. Verify by opening a draft PR that adds a fake but
   well-shaped credential to a non-allowlisted file (e.g.
   `frontend/src/lib/test.ts` with a `sk-ant-api03-` + 60 random
   alphanumerics). Expected: workflow goes red. If green: switch to a
   manually-driven `gitleaks git --staged` invocation in the workflow.
2. **`CodeQL / Analyze (go)`** with `build-mode: none` — buildless mode
   can silently produce zero alerts on monorepos whose extractor cannot
   resolve types across vendor boundaries. After the first run, open
   `Security tab → Code scanning` and verify alerts > 0 (Skadia has
   ~145k lines of Go, expect several informational findings on the
   security-and-quality query suite). Zero alerts on first run = extractor
   problem; switch to `build-mode: autobuild` and re-test.

After both verifications pass, add the checks to the required list.

## `Verified-Secret Sweep` — separate handling

The TruffleHog scheduled workflow does **not** run on PRs and therefore is
not added to required status checks. A red sweep is signalled via the
notification email GitHub sends to the repo owner when a scheduled
workflow fails — rotate the credential immediately, then track remediation
as a regular issue/PR.

## CodeQL setup

CodeQL needs `Settings → Code security → Code scanning` enabled. Default
on for public repositories since 2024; if disabled, enable and trigger
once manually via `Actions → CodeQL → Run workflow` so the analysis names
become available for the required-status-checks list above.
