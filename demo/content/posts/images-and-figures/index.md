---
title: "Images and figures"
date: 2026-06-13T09:00:00+09:00
description: "How pictures, captions, and inline images render from plain Markdown."
tags: ["markdown", "writing"]
crofty:
    tier: full
---

Images live right next to the post that uses them. This post is a *page bundle*
— a folder holding `index.md` and its pictures — so the files travel together
and the links never break.

## A plain Markdown image

The standard syntax is `![alt text](file)`. Here is an SVG that shipped in this
folder:

![Layered hills at dusk](hills.svg)

Alt text is not optional dressing — it is what a screen reader announces and
what shows if the image ever fails to load. Write it like a caption you would
say out loud.

## A figure with a caption

When a picture needs a credit or a caption, raw HTML works too (this site turns
on Hugo's `unsafe` Markdown so your own HTML passes through):

<figure>
  <img src="pipeline.svg" alt="Markdown becomes a static site, then deploys">
  <figcaption>From a folder of Markdown to a deployed site, in one step.</figcaption>
</figure>

## Inline, in a sentence

You can drop a small image ![a dot](/avatar.svg) mid-sentence as well, though a
block image usually reads better.

That is the whole toolkit for pictures: a file in the folder and a line of
Markdown pointing at it.
