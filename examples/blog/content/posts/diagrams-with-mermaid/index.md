---
title: "Diagrams with Mermaid"
date: 2026-06-13T15:00:00+09:00
description: "Flowcharts, sequences, and more — drawn from a fenced code block."
tags: ["markdown", "tools"]
crofty:
    tier: full
---

Mermaid turns a fenced code block into a diagram. You write the relationships in
text; the browser draws them. A project render hook (`render-codeblock-mermaid`)
loads Mermaid only on pages that use it, and the diagrams follow your light/dark
setting.

## A flowchart

```mermaid
graph TD
  A[Write Markdown] --> B{crofty build}
  B -->|ok| C[Static site]
  B -->|error| D[Fix and retry]
  C --> E[Deploy to your domain]
```

## A sequence diagram

```mermaid
sequenceDiagram
  participant You
  participant crofty
  participant Cloudflare
  You->>crofty: crofty deploy
  crofty->>Cloudflare: upload built site
  Cloudflare-->>You: live URL
```

## A pie chart

```mermaid
pie title Where the words live
  "Plain Markdown" : 92
  "Front matter" : 6
  "Everything else" : 2
```

The source stays plain text in your post, so a diagram is as portable — and as
diff-able — as the prose around it.
