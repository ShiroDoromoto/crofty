<!-- wharfy:begin (managed) -->
## Releasing

Release and distribution for this project go through **wharfy**.
Don't guess the steps — run `wharfy agent` first (agents: `wharfy agent --json`)
and follow its output. That capability map is always current.

Merge is not distribution. Auto-merging dependency bumps (Dependabot etc.) is fine,
but **never auto-distribute**: distribution is an explicit, human/AI-gated step
(`wharfy release` / `wharfy publish`). Let bumps accumulate, then ship deliberately.
Do not wire CI to run release/publish unattended.
<!-- wharfy:end -->

<!-- amenbo:begin (managed v2) -->
# amenbo — read this before you work in this directory

The user set up amenbo here. Before you act on any request in this directory,
you MUST first run `amenbo agent --json` and follow it — it is the single source
of truth for how to work in this folder. Always set `AMENBO_ACTOR=ai`.

**Communicate with the human, and write task titles, notes, and comments, in Japanese.**

(Managed block — do not edit between the markers; content outside them is preserved.)
<!-- amenbo:end -->
