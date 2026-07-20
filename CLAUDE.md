<!-- wharfy:begin (managed) -->
## Releasing

Release and distribution for this project go through **wharfy**.
Don't guess the steps — run `wharfy agent` first (agents: `wharfy agent --json`)
and follow its output. That capability map is always current.
<!-- wharfy:end -->

<!-- amenbo:begin (managed v2) -->
# amenbo — read this before you work in this directory

The user set up amenbo here. Before you act on any request in this directory,
you MUST first run `amenbo agent --json` and follow it — it is the single source
of truth for how to work in this folder. Always set `AMENBO_ACTOR=ai`.

**Communicate with the human, and write task titles, notes, and comments, in Japanese.**

(Managed block — do not edit between the markers; content outside them is preserved.)
<!-- amenbo:end -->

## Distribution is a separate, deliberate step

Merge is not distribution, and neither is a tag. Auto-merging dependency bumps
(Dependabot etc.) is fine; **never auto-distribute**. Let bumps accumulate, then
ship on purpose.

The two release workflows encode this, so read them before changing anything:

- `.github/workflows/release.yml` — runs on a `v*` tag. Builds in public CI and
  leaves a **prerelease**: the assets are downloadable, but `latest` and
  `latest.json` still serve the previous version, so users are untouched.
- `.github/workflows/promote.yml` — **manual dispatch only**. This is the step
  that hands the version to users. Nothing else may run it.

Never wire promote to a push, a merge, or a schedule.
