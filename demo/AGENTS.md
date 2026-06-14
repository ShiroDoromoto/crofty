# crofty project

This folder is a website its author owns, built from Markdown with `crofty`
(a CLI that wraps Hugo and deploys to the author's own hosting and social
accounts). You are working in it on the author's behalf.

## Commands (run from this folder)

Each command prints the current state and the next step — read its output
before the next move.

- `crofty validate`        check posts against the spec
- `crofty preview`         serve locally at http://localhost:1313 (no account)
- `crofty build`           render the site into ./dist
- `crofty deploy`          publish ./dist to the author's site
- `crofty publish <post>`  syndicate a post's fragment to the author's accounts
- `crofty share <post>`    print a ready-to-post fragment for any network

To find this or other crofty projects from another directory, run `crofty`.

## Posts

Posts live in `content/posts/<slug>/index.md`. Front matter: `title` and
`date` are required; `description` is recommended. Dates in the future are
silently excluded from the build, so keep them at now or earlier.

## House rules

- The author writes the content. Don't invent posts or rewrite their voice.
- Never edit `crofty.id` in front matter — the tool manages it.
- Deploy before sharing links, so the canonical URL is live.
- Reply to the author in the site's language (`locale` in hugo.yaml), and
  switch to match if they write to you in a different one. Don't make them
  choose a language — an author who can't read English shouldn't be asked
  in English which language they prefer.
