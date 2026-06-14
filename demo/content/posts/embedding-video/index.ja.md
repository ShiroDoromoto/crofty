---
title: "動画を埋め込む"
date: 2026-06-13T11:00:00+09:00
description: "自前ホスティングの動画とプラットフォームの埋め込み、どちらも Markdown から。"
tags: ["markdown", "the-web"]
crofty:
    tier: full
---

自分の所有するページに動画を載せる誠実なやり方は二つある。ファイルを自分で
ホストするか、どこか別の場所からプレイヤーを埋め込むか。ここではどちらも動く。

## 自前ホスティングの動画

ファイルを手元に置けば、主導権も手元に残る。HTML の `<video>` 要素には第三者も
JavaScript も要らない。下のクリップはこの記事のフォルダに同梱されているので、
音を出してほしい──音声がある。

<video controls preload="metadata">
  <source src="bunny.mp4" type="video/mp4">
  Your browser does not support the video tag.
</video>

`.mp4` を記事のフォルダに置き、画像と同じやり方で `src` をそこに向ければいい。
（このクリップは *Big Buck Bunny*、© Blender Foundation, CC-BY 3.0。）

## YouTube の埋め込み

動画がすでにプラットフォーム上にあるなら、Hugo の組み込みショートコードを使う──
生の HTML は要らず、動画 id だけでいい。

{{< youtube aqz-KE-bpKQ >}}

このショートコードはレスポンシブでプライバシーに配慮した iframe をレンダリングする。`{{</* vimeo id */>}}`
は Vimeo に対して同じように働く。プラットフォームの到達力がほしいときは埋め込みに、
ファイルがプラットフォームより長生きしてほしいときは `<video>` に手を伸ばそう。
