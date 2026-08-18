---
title: "spr"
description: "A delightful command line for link.springer.com"
heroTitle: "link.springer.com, from the command line"
heroLead: "Works, journals, books, series, references, metrics, and the graph between them. One pure Go binary, no API key, output that pipes into the rest of your tools."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

`spr` reads what Springer Nature Link publishes, classifies every response before parsing a byte of it, and records where each field came from.

```bash
spr get /article/10.1007/s10994-021-05946-3
spr get --body /journal/10994 > journal.html
spr cache
```

## Why classification comes first

A 200 from this site means very little on its own. A chapter behind a subscription answers 200 with a full set of metadata and no body. A pdf url without a subscription answers 200 with 282,045 bytes of html. The search surface answers 200 with a 3,038 byte client challenge. And a work that does not exist answers an honest 404 with 122,477 bytes of body, so nothing can be guessed from size either.

Every response is therefore sorted into one of five states on its content, before anything is parsed. See the [introduction](/getting-started/introduction/) for what each one means.

## Where to go next

- New here? Read the [introduction](/getting-started/introduction/), then the [quick start](/getting-started/quick-start/).
- Installing? See [installation](/getting-started/installation/).
- Need every flag? The [CLI reference](/reference/cli/) is the full surface.
