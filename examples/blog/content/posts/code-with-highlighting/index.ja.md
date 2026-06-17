---
title: "コードに色を"
date: 2026-06-11T09:00:00+09:00
description: "言語をまたぐフェンス付きコードブロック──ライトとダークの切り替えに追随する構文の色つき。"
tags: ["markdown", "tools"]
crofty:
    tier: full
---

フェンス付きブロックにその言語のタグを付ければ、Hugo のハイライター（Chroma）が色を付けてくれる。
このサイトはクラスベースのハイライトを使っているので、色は
`css/chroma.css` から来ており、読者のライト／ダーク設定とともに切り替わる。

`crofty build` のようなインラインコードはそのまま素のままだ。ブロックには完全な扱いが施される。

```go
package main

import "fmt"

func main() {
    // a tiny program
    for i := 0; i < 3; i++ {
        fmt.Println("hello, crofty", i)
    }
}
```

```javascript
const posts = await fetch("/index.json").then((r) => r.json());
const recent = posts.filter((p) => p.draft === false).slice(0, 5);
console.log(`showing ${recent.length} posts`);
```

```python
from pathlib import Path

def word_count(folder: str) -> int:
    return sum(len(p.read_text().split()) for p in Path(folder).glob("*.md"))

print(word_count("content/posts"))
```

```bash
crofty build       # render Markdown to a static site
crofty deploy      # push it to your own domain
```

```json
{
  "workspace": "01KV1XEQ26RCK07BFWGZW743K1",
  "deploy": { "provider": "cloudflare", "project": "crofty-demo" }
}
```

言語を指定しないフェンスは色が付かないまま残る──シェルのやり取りや素の出力にはこれで十分だ。

```
$ ls content/posts
images-and-figures  embedding-video  code-with-highlighting
```
