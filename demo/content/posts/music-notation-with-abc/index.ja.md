---
title: "abc.js で楽譜を書く"
date: 2026-06-13T19:00:00+09:00
description: "ABC のコードブロックから生まれる、彫刻された譜面とブラウザ内での再生。"
tags: ["markdown", "tools"]
crofty:
    tier: full
---

[ABC notation](https://abcnotation.com/) は音楽を書くための素のテキストの方法だ。
プロジェクトのレンダーフック（`render-codeblock-abc`）が ```abc フェンス付きブロックを
[abc.js](https://www.abcjs.net/) に送り、それが譜面を彫刻して再生ボタンを加える──
すべてブラウザのなかで、あなたの記事のテキストから。

## 簡単な曲

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

## 調と拍子を変えた音階

```abc
X:2
T:C major scale
M:4/4
L:1/4
K:C
C D E F | G A B c | c B A G | F E D C |]
```

どちらの譜面でも再生を押せば音が聞こえる。記法はあなたの Markdown のなかで素の
テキストのまま残るので、差分はきれいに取れ、どのプラットフォームでも生き延びる──
レンダリングと音声は、ファイルに焼き込まれているのではなく、読者のブラウザが加える。
