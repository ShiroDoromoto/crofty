---
title: "Mermaid で図を描く"
date: 2026-06-07T09:00:00+09:00
description: "フローチャート、シーケンス、その他──フェンス付きコードブロックから描かれる。"
tags: ["markdown", "tools"]
crofty:
    tier: full
---

Mermaid はフェンス付きコードブロックを図に変える。あなたは関係をテキストで書き、
ブラウザがそれを描く。プロジェクトのレンダーフック（`render-codeblock-mermaid`）が
それを使うページにだけ Mermaid を読み込み、図はあなたのライト／ダーク
設定に追随する。

## フローチャート

```mermaid
graph TD
  A[Write Markdown] --> B{crofty build}
  B -->|ok| C[Static site]
  B -->|error| D[Fix and retry]
  C --> E[Deploy to your domain]
```

## シーケンス図

```mermaid
sequenceDiagram
  participant You
  participant crofty
  participant Cloudflare
  You->>crofty: crofty deploy
  crofty->>Cloudflare: upload built site
  Cloudflare-->>You: live URL
```

## 円グラフ

```mermaid
pie title Where the words live
  "Plain Markdown" : 92
  "Front matter" : 6
  "Everything else" : 2
```

ソースは記事のなかで素のテキストのままなので、図はその周りの散文と同じくらい
持ち運びやすく──そして差分も取りやすい。
