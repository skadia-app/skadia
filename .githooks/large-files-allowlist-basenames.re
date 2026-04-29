# Large-file size-gate basename regex (anchored).
#
# Source-of-truth shared by .githooks/pre-commit (Check 1) and
# .github/workflows/repo-hygiene.yml (large-files job).
#
# Pure data file — NEVER executed as bash. The first non-empty, non-comment
# line is read as an extended regex anchored against the file's basename.
# All other lines are ignored.
#
# Trust-boundary note: CI loads this file from origin/main, not from PR
# head, so a malicious PR cannot widen the allowlist for the gate that
# protects main.
^(package-lock\.json|pnpm-lock\.yaml|yarn\.lock|go\.sum|Cargo\.lock|poetry\.lock|Gemfile\.lock|composer\.lock|uv\.lock|bun\.lock|bun\.lockb|Pipfile\.lock|mix\.lock|flake\.lock|deno\.lock|pdm\.lock|Berksfile\.lock|Podfile\.lock|conan\.lock|CHANGELOG(\.md)?|HISTORY(\.md)?|AUTHORS|CONTRIBUTORS)$
