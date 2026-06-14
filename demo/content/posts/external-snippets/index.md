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

<script src="https://gist.github.com/anonymous/0a1b2c3d4e5f60718293a4b5c6d7e8f9.js"></script>

## A CodePen

CodePen's embed is a `<p>` placeholder plus their `embed.js`, which swaps in a
live editor:

<p class="codepen" data-height="300" data-default-tab="result" data-slug-hash="ExrEY" data-user="chriscoyier">
  <span>See the Pen on CodePen.</span>
</p>
<script async src="https://cpwebassets.codepen.io/assets/embed/ei.js"></script>

## A social post

X / Twitter (and Mastodon, Bluesky, Instagram…) all use the same shape: a
`<blockquote>` that their `widgets.js` upgrades into a rendered card. If the
script is blocked, the blockquote degrades to a plain link — which is the point.

<blockquote class="twitter-tweet">
  <p lang="en" dir="ltr">A folder of Markdown is a surprisingly durable thing.</p>
  &mdash; crofty (@crofty) <a href="https://twitter.com/crofty/status/1">link</a>
</blockquote>
<script async src="https://platform.twitter.com/widgets.js"></script>

## A bare iframe

And when all else fails, an `<iframe>` embeds anything that allows it — a map,
a slide deck, a dashboard:

<iframe
  title="A map"
  width="100%"
  height="320"
  loading="lazy"
  src="https://www.openstreetmap.org/export/embed.html?bbox=139.69,35.67,139.78,35.71&layer=mapnik"></iframe>

These are placeholder ids, so some cards will show their fallback rather than
live content — that fallback behaviour is exactly what you are seeing tested.
