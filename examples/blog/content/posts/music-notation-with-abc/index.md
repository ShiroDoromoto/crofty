---
title: "Music notation with abc.js"
date: 2026-06-13T19:00:00+09:00
description: "Engraved sheet music and in-browser playback from an ABC code block."
tags: ["markdown", "tools"]
crofty:
    tier: full
---

[ABC notation](https://abcnotation.com/) is a plain-text way to write music. A
project render hook (`render-codeblock-abc`) feeds an `abc` fenced code block to
[abc.js](https://www.abcjs.net/), which engraves the score and adds a play
button — all in the browser, from text in your post.

## A simple tune

```abc
X:1
T:Cooley's
M:4/4
L:1/8
R:reel
K:Emin
D2|EBBA B2 EB|B2 AB dBAG|FDAD BDAD|FDAD dAFD|
EBBA B2 EB|B2 AB defg|afe^c dBAF|DEFD E2:|
```

## A scale, with a different key and metre

```abc
X:2
T:C major scale
M:4/4
L:1/4
K:C
C D E F | G A B c | c B A G | F E D C |]
```

Press play on either score to hear it. The notation stays plain text in your
Markdown, so it diffs cleanly and survives any platform — the rendering and the
audio are added by the reader's browser, not baked into a file.
