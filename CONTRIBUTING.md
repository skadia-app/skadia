# Contributing to Skadia

Thanks for considering a contribution. This document covers the workflow,
local setup, and conventions specific to this project. If you maintain a
fork or self-hosted instance, the same rules apply.

## Reporting issues

- **Security vulnerabilities:** see [SECURITY.md](SECURITY.md). Do not file
  public issues for security reports.
- **Bugs:** use the bug-report issue template; include reproduction steps,
  expected vs. actual behaviour, your version (commit SHA or release tag),
  and the runtime context (Docker Compose, bare-metal, etc.).
- **Feature requests:** use the feature-request template. Link any relevant
  prior discussion or upstream tracker.
- **Questions:** prefer GitHub Discussions over issues so the bug tracker
  stays focused.

## Pull requests

1. Fork the repository and create a topic branch from `main`.
2. Install the local pre-commit hook before your first commit:
   ```bash
   git config core.hooksPath .githooks
   ```
   The hook scans for personal data, secrets, large binaries, and TypeScript
   regressions. Recommended companion tools (all optional, hook skips when
   absent): `gitleaks`, `actionlint`, `zizmor`. Install via your package
   manager or `go install` / `pip install`.
3. Make your changes. Keep commits focused; one logical change per commit.
4. Push your branch and open a PR. The PR template is mandatory — every
   section is checked by reviewers, including the AI-tooling-disclosure
   section if any AI assistance was involved (see "AI assistance" below).
5. CI runs gitleaks, CodeQL, actionlint, zizmor, the size gate, and the
   pre-commit hook in server-side replay. All must pass before review.
6. Address bot-review findings before requesting human review.

### Branch protection on `main`

`main` is protected. Required status checks, code-owner review, and
linear history are enforced — see `.github/BRANCH_PROTECTION.md` for the
full configuration.

## Local setup (summary)

Full setup notes are in `docs/documentation/dev/`. The short version:

- The project assumes Linux + Podman or Docker. macOS users may need
  workarounds for the bind-mount conventions documented in `setup.sh`.
- Database lives on btrfs with `chattr +C` (see ADR-0024) — `setup.sh`
  applies this automatically on first run.
- Vite dev server (frontend): `cd frontend && npm ci && npm run dev`
  (`npm ci` is the reproducible install path — uses `package-lock.json`
  unmodified. Use `npm install` only when you intentionally want lockfile
  updates.)
- Backend: `distrobox-host-exec podman compose up -d` (or your equivalent)

## Coding conventions

- **Comments in English.** UI strings stay in German until the i18n sprint
  flips them — do not preemptively translate.
- **No comments unless the WHY is non-obvious.** Don't describe what the
  code does (the code does that); describe why a non-obvious choice was
  made, what hidden constraint applies, or which bug a workaround papers
  over.
- **No third-party-platform name-dropping in code/UI/comments** unless an
  ATTRIBUTIONS file, vendor LICENSE, or package metadata in the repo
  already names the source. See the project root `CLAUDE.md` /
  `AGENTS.md` for the full rule.
- **Verify before claiming.** API names, struct field names, config keys —
  read the actual source, never quote from memory or generated docs.
- **Database migrations are append-only.** Never edit a migration that has
  shipped; add a new one that supersedes it.

### Doc-Sync

Every implementation plan ends with a Doc-Sync task — even when no
triggers fire (in which case it is a 30-second no-op, still included).
The Doc-Sync task reads `git log` for the plan's commits and updates
`docs/documentation/` for any of these triggers:

- New migrations → `dev/subsystems/database-migrations.md`
- New env vars → `dev/reference/environment-variables.md`
- REST changes → `dev/reference/rest-api-catalog.md`
- ADRs → `dev/adr/` + `feature-inventory.md`
- User-visible features → `user/features/X.md` + `feature-inventory.md` + `keyword-index.md`

If none of those fire, the task is a no-op — still include it so the
discipline is uniform across plans. Commit separately as `docs: sync for
<feature>` to keep doc updates auditable independently of code commits.

The internal `docs/_internal/WRITING_RULES.md` describes what content
goes where (style, tone, no platform-name-dropping rules, etc.).

### CHANGELOG

The `CHANGELOG.md` file follows a date-as-version format:

- New entries land at the top under a `## YYYY-MM-DD — <feature title>`
  heading. There is no `[Unreleased]` section — releases are continuous.
- Sections within an entry: `### Added`, `### Changed`, `### Fixed`,
  `### Removed`, `### Deprecated`, `### Security`.
- One bullet per user-visible change. Phrase it from the user's
  perspective, not the implementation's. Link the relevant PR.
- Internal-only refactors do not need a CHANGELOG entry.
- User-facing changelog entries take priority over implementation
  jargon — first bullet should name the visible feature, not the
  technical change.


## AI assistance

This project is openly developed with AI agents (Claude Code, Codex, etc.)
contributing alongside humans. AI-authored PRs are welcome under three
conditions:

1. **Disclose the tool used** in the PR template's "AI tooling disclosure"
   section. We do not use this to filter PRs; we use it to interpret the
   diff (e.g. an agent's verbose comments tell us where to push back).
2. **Resolve all bot-review findings before requesting human review.** Do
   not stack human reviewer time on top of unaddressed automation flags.
3. **State the level of human verification.** "Tests pass and I spot-
   checked the diff" is fine; "the agent said it works" is not. Human
   accountability for the change does not transfer to the AI tool.

Project-internal agent rules are in `CLAUDE.md` (root, `frontend/`).
External contributors are not expected to follow them, but reading them
helps interpret existing code patterns.

## License

By contributing, you agree that your contributions are licensed under the
project's [GNU AGPL-3.0](LICENSE).
