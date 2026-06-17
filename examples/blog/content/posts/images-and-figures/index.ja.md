---
title: "画像と図版"
date: 2026-06-04T09:00:00+09:00
description: "写真、キャプション、インライン画像が素の Markdown からどうレンダリングされるか。"
tags: ["markdown", "writing"]
crofty:
    tier: full
---

画像は、それを使う記事のすぐ隣に置かれる。この記事は *ページバンドル*
──`index.md` とその写真を収めたフォルダ──なので、ファイルは一緒に旅をし、
リンクが壊れることはない。

## 素の Markdown 画像

標準の記法は `![alt text](file)` だ。これはこのフォルダに同梱された SVG である。

![夕暮れに重なる丘](hills.svg)

代替テキストは省略可能な飾りではない──それはスクリーンリーダーが読み上げるものであり、
画像が読み込みに失敗したときに表示されるものだ。声に出して言うキャプションのように
書こう。

## キャプション付きの図版

写真にクレジットやキャプションが必要なとき、生の HTML も使える（このサイトは
Hugo の `unsafe` な Markdown を有効にしているので、あなた自身の HTML がそのまま通る）。

<figure>
  <img src="/posts/images-and-figures/pipeline.svg" alt="Markdown becomes a static site, then deploys">
  <figcaption>Markdown のフォルダから、デプロイ済みのサイトへ、ひと手間で。</figcaption>
</figure>

## 文中に、インラインで

画像は文中にも、周囲のテキストの大きさで収まれる──文の途中に落とした
小さなアイコン <img src="/avatar.svg" alt="サイトのアイコン" style="height:1.1em;vertical-align:-0.15em;border-radius:3px"> に便利だ。（これは高さを
指定するのに少しだけ生の HTML を使っている。素の `![]()` 画像はつねにそれ自身の
ブロックとしてフルサイズでレンダリングされ、本物の写真にはたいていそのほうが読みやすい。）

これが写真のための道具一式のすべてだ。フォルダのなかのファイルと、それを指す
一行の Markdown。
