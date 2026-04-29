# Security policy

Skadia is a self-hosted multi-user fitness platform. A successful attack
on a deployed instance can leak GPS tracks, health metrics, and account
credentials of every user on that instance. We take security reports
seriously and would rather hear about a problem early than late.

## Reporting a vulnerability

**Do not open a public issue for security reports.**

Use GitHub's [Private Vulnerability Reporting][gh-pvr] for this repository
(Security tab → Report a vulnerability). That keeps the discussion
encrypted between you and the maintainers and lets us coordinate a fix
before disclosure.

If for some reason private reporting is unavailable on this repository
(forks may have it disabled), open an issue with title `Security: contact
request` containing **no vulnerability details** — just a way to reach you
back (your GitHub handle alone may not be sufficient because GitHub
private email visibility is account-controlled and unreliable). The
maintainer will respond on the issue thread to coordinate a private
channel, then ask you to delete the public exchange before any details
are shared.

[gh-pvr]: https://docs.github.com/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability

## Response expectations

Best-effort, single-maintainer project — no SLA, no fixed timeline. The
maintainer triages and fixes when they have the time. We will:

- Acknowledge the report when we see it
- Confirm whether it is in or out of scope once we have reproduced it
- Share a fix or coordinated-disclosure plan when one exists
- Publish a public advisory via GitHub Security Advisory after the fix
  is released, crediting the reporter unless they request otherwise

If a report has been quiet for a while and you would like an update, ping
the report thread. Aggressive deadline-setting against a hobbyist project
makes coordination harder, not faster.

## Scope — in

The following are in scope for the Skadia codebase as published in this
repository:

- Authentication / authorization bypass (cookie or JWT forgery, session
  fixation, OAuth-callback redirection abuse)
- Cross-tenant data exposure (one user reading another user's activities,
  routes, profile, segments, fusion edits, or imported third-party data)
- Privilege escalation (regular user → admin, regular user → another
  user's admin-only settings)
- SQL injection, NoSQL injection, ORM-bypass via crafted input
- Cross-site scripting (stored, reflected, DOM-based) in the web UI
- CSRF on state-changing endpoints
- Server-side request forgery via URL-import / OAuth callbacks
- Path traversal in file imports, hotfolder, GDPR archive ingestion
- Remote code execution via parsing of FIT, GPX, TCX, KML, KMZ, image, or
  archive files supplied by users or imported from third-party platforms
- Persistent or transient denial-of-service via crafted upload payloads.
  Skadia has two distinct upload paths with different size envelopes:
    - **Activity uploads** (`POST /api/v1/activities/upload`, hotfolder,
      Garmin/Strava/Komoot importers): bounded by
      `limits.max_upload_bytes` (default 50 MB, operator-raisable to
      256 MB+).
    - **GDPR-archive import** (`POST /api/v1/gdpr/import`): bounded by a
      separate 4 GiB hard limit (intentional — full data exports from
      large platforms run multi-gigabyte).
  Reports should state which path was tested and the size envelope.
  Reports above 4 GiB are out of scope — that exceeds any legitimate
  operator deployment.
- Credential or token leakage via logs, error messages, or
  user-facing API responses
- GDPR-export bypass (data export missing user data, or returning another
  user's data)
- GDPR-delete bypass (data deletion leaving identifiable user data behind)

## Scope — out

The following are **not** considered vulnerabilities of this repository:

- Findings produced by automated scanners without a working
  proof-of-concept that demonstrates impact
- Vulnerabilities in third-party dependencies — please report those
  upstream; we will pick up patches via Dependabot
- Theoretical attacks without a working exploit path
- Issues in self-hosted instances configured against the project's own
  defaults — concretely: exposing internal database/API ports without
  TLS, running with `DEV_AUTH=true` outside development, modifying
  `docker-compose.yml` to bypass the documented hardening switches,
  altering filesystem semantics that the integrity-preserving setup in
  `scripts/setup.sh` depends on
- Social engineering against the project owner or contributors
- Denial-of-service via legitimate but expensive operations (large valid
  FIT files, recompute loops triggered by the user's own data) — these
  are capacity-planning concerns, not security bugs
- Brute-force attacks against authentication endpoints when the operator
  has not configured a rate-limit reverse proxy
- Missing security headers that don't have a concrete exploit chain
  attached to them
- Reports based purely on "this dependency has a CVE" without
  demonstrating that the affected code path is reachable in Skadia
- Browser-version-specific issues in browsers more than two major
  versions behind current Chrome / Firefox / Safari / Edge stable

## Coordinated disclosure

We follow standard coordinated disclosure:

1. Vulnerability reported privately
2. Maintainer reproduces and confirms in/out of scope
3. Patch developed; if the issue affects user data, a remediation note is
   prepared for operators of self-hosted instances
4. Patch released; advisory published via GitHub Security Advisory with
   CVE assignment if applicable
5. Public credit to the reporter (handle, name, and URL of their choice)
   unless they prefer to remain anonymous

We ask reporters to give us a reasonable window before public disclosure.
Aggressive disclosure without prior contact will not get you blocked, but
it makes coordination harder for everyone — including the operators of
deployed instances who need time to upgrade.

## Out-of-band tooling

Repository-level supply-chain hygiene is checked continuously by:

- OpenSSF Scorecard (`.github/workflows/scorecard.yml`) — weekly + per push
- CodeQL (`.github/workflows/codeql.yml`) — weekly + per PR (Go, TS, Python)
- Gitleaks via `pre-commit-guard` — every PR
- TruffleHog (`.github/workflows/secret-sweep-scheduled.yml`) — daily,
  verified-only

Findings from these tools are addressed but are not themselves
vulnerabilities of this repository unless they are paired with a
working proof-of-concept demonstrating impact (see "out of scope" above).

## Bug bounty

There is no bug bounty programme. Reports are welcomed regardless;
public credit and a thank-you in the release notes are what we can offer.
