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
