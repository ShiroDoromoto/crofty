---
title: "External snippets that paste in"
date: 2026-06-13T13:00:00+09:00
description: "Gists, CodePen, social embeds, and iframes — the copy-paste HTML the web hands you."
tags: ["the-web", "tools"]
crofty:
    tier: full
---

Half the web hands you a block of HTML and says "paste this on your site." Those
snippets are just raw HTML and a `<script>`, so they work here because this site
turns on Hugo's `unsafe` Markdown. A few of the common ones:

## A GitHub Gist

Gists embed with a single script tag pointed at the gist's `.js` URL:

<script src="https://gist.github.com/octocat/6cad326836d38bd3a7ae.js"></script>

## A CodePen

CodePen's embed is a `<p>` placeholder plus their `ei.js`, which swaps in a live
editor (this is the example pen from CodePen's own docs):

<p class="codepen" data-height="380" data-default-tab="result" data-slug-hash="XWJPxpZ" data-user="Mamboleoo">
  <span>See <a href="https://codepen.io/Mamboleoo/pen/XWJPxpZ">the Pen on CodePen</a>.</span>
</p>
<script async src="https://cpwebassets.codepen.io/assets/embed/ei.js"></script>

## A social post

Mastodon (and X, Bluesky, Instagram…) hand you an `<iframe>` plus a small script
that resizes it. Because the iframe is real HTML, the post shows even if the
script is blocked — it just won't auto-fit its height. Here is Mastodon's own
account:

<iframe src="https://mastodon.social/@Mastodon/115503016101266241/embed" width="100%" height="420" allowfullscreen sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox" style="border:0"></iframe>
<script src="https://mastodon.social/embed.js" async></script>

## A bare iframe

And when all else fails, a plain `<iframe>` embeds anything that allows it — a
map, a slide deck, a dashboard:

<iframe
  title="A map of central Tokyo"
  width="100%"
  height="320"
  loading="lazy"
  src="https://www.openstreetmap.org/export/embed.html?bbox=139.69,35.67,139.78,35.71&layer=mapnik"></iframe>

Every embed above points at a real, live resource — so what you see here is what
a reader gets, with no platform owning your page.
