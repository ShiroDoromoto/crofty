---
title: "Code, highlighted"
date: 2026-06-13T17:00:00+09:00
description: "Fenced code blocks across languages, with syntax colours that follow light and dark mode."
tags: ["markdown", "tools"]
crofty:
    tier: full
---

Tag a fenced block with its language and Hugo's highlighter (Chroma) colours it.
This site uses class-based highlighting, so the colours come from
`css/chroma.css` and switch with the reader's light/dark setting.

Inline code like `crofty build` stays plain. Blocks get the full treatment:

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

A fence with no language stays uncoloured — fine for shell transcripts or plain
output:

```
$ ls content/posts
images-and-figures  embedding-video  code-with-highlighting
```
