---
title: "貼り付けるだけの外部スニペット"
date: 2026-05-25T09:00:00+09:00
description: "Gist、CodePen、ソーシャル埋め込み、iframe──web が手渡してくるコピペ HTML。"
tags: ["the-web", "tools"]
crofty:
    tier: full
---

web の半分は HTML のかたまりを手渡して「これをあなたのサイトに貼り付けて」と言う。
それらのスニペットはただの生の HTML と `<script>` であり、このサイトが Hugo の
`unsafe` な Markdown を有効にしているので、ここでも動く。よくあるものをいくつか。

## GitHub Gist

Gist は、gist の `.js` URL を指す一つのスクリプトタグで埋め込める。

<script src="https://gist.github.com/octocat/6cad326836d38bd3a7ae.js"></script>

## CodePen

CodePen の埋め込みは `<p>` のプレースホルダーと彼らの `ei.js` で、それがライブの
エディタに差し替わる（これは CodePen 自身のドキュメントにある例の pen だ）。

<p class="codepen" data-height="380" data-default-tab="result" data-slug-hash="XWJPxpZ" data-user="Mamboleoo">
  <span>See <a href="https://codepen.io/Mamboleoo/pen/XWJPxpZ">the Pen on CodePen</a>.</span>
</p>
<script async src="https://cpwebassets.codepen.io/assets/embed/ei.js"></script>

## ソーシャル投稿

Mastodon（や X、Bluesky、Instagram…）は `<iframe>` と、それをリサイズする小さな
スクリプトを手渡してくる。iframe は本物の HTML なので、スクリプトがブロックされても
投稿は表示される──ただ高さを自動で合わせられないだけだ。これは Mastodon 自身の
アカウントである。

<iframe src="https://mastodon.social/@Mastodon/115503016101266241/embed" width="100%" height="420" allowfullscreen sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox" style="border:0"></iframe>
<script src="https://mastodon.social/embed.js" async></script>

## 素の iframe

そして他のすべてが行き詰まったとき、素の `<iframe>` はそれを許すものなら何でも
埋め込む──地図、スライドデッキ、ダッシュボード。

<iframe
  title="A map of central Tokyo"
  width="100%"
  height="320"
  loading="lazy"
  src="https://www.openstreetmap.org/export/embed.html?bbox=139.69,35.67,139.78,35.71&layer=mapnik"></iframe>

上のどの埋め込みも、本物の生きたリソースを指している──だからここで見えるものは
読者が得るものそのものであり、あなたのページをどのプラットフォームも所有しない。
