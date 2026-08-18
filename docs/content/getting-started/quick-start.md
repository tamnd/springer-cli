---
title: "Quick start"
description: "Fetch your first page and read what came back."
weight: 30
---

Once `spr` is on your `PATH`:

```bash
spr --help       # the command tree
spr version      # build info
```

## Fetch one page

`spr get` is the client with nothing on top of it. It follows the redirect chain, counts the hops, classifies the response, and prints what it found without parsing a single field.

```console
$ spr get /article/10.1007/s10994-021-05946-3
url          https://link.springer.com/article/10.1007/s10994-021-05946-3
final        https://link.springer.com/article/10.1007/s10994-021-05946-3?error=cookies_not_supported&code=fbffe6f2
code         200
status       ok
redirects    3
bytes        718872
content type text/html; charset=utf-8

The page came back whole.
```

Three redirects on a first request is normal, not a warning. Every first request runs a cookie dance and comes back with `?error=cookies_not_supported&code=<uuid>` appended, and the uuid is different every time.

## Watch it refuse to lie about a pdf

```console
$ spr get --kind pdf /content/pdf/10.1007/978-3-031-28170-9_5.pdf
status       wrong_kind
redirects    7
bytes        282045
content type text/html; charset=utf-8
```

Seven hops, a 200, and no pdf. The url ran the cookie dance, got sent across to the chapter page, and ran it again. Telling you `ok` here would be the tool lying to you.

## Take the raw page, or json

```bash
spr get --body /journal/10994 > journal.html
spr get -o json /article/10.1007/s10994-021-05946-3 | jq .status
```

## See what is cached

```bash
spr cache
spr cache --clear
```

The cache is keyed on the url you asked for, never the one the redirect chain landed on, because that one carries a per request uuid and no key would ever be seen twice.
