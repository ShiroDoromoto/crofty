---
title: "Embedding video"
date: 2026-06-13T11:00:00+09:00
description: "Self-hosted video and platform embeds, both from Markdown."
tags: ["markdown", "the-web"]
crofty:
    tier: full
---

There are two honest ways to put video on a page you own: host the file
yourself, or embed a player from somewhere else. Both work here.

## A self-hosted video

If you keep the file, you keep control. The HTML `<video>` element needs no
third party and no JavaScript:

<video controls preload="metadata" poster="">
  <source src="https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4" type="video/mp4">
  Your browser does not support the video tag.
</video>

Drop the `.mp4` in the post's folder and point `src` at it the same way you
would an image. (The example above streams a public-domain clip so the demo
stays small.)

## A YouTube embed

When the video already lives on a platform, use Hugo's built-in shortcode —
no raw HTML, just the video id:

{{< youtube 7e90gvfFa78 >}}

The shortcode renders a responsive, privacy-aware iframe. `{{</* vimeo id */>}}`
works the same way for Vimeo. Reach for an embed when you want the platform's
reach; reach for `<video>` when you want the file to outlive the platform.
