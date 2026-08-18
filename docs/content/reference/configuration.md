---
title: "Configuration"
description: "The global flags every command shares, and what they do."
weight: 20
---

spr needs no configuration to run. Everything below has a working default.

## Global flags

| Flag | Default | Meaning |
|---|---|---|
| `--pace` | `2s` | Interval between requests to one host. Never goes below 1s, whatever you pass. |
| `--timeout` | `30s` | Per request timeout |
| `--retries` | `2` | Retries on a transport error or a 5xx. Never on a challenge. |
| `--cache` | platform cache dir | Where responses are stored |
| `--no-cache` | off | Fetch fresh and store nothing |
| `--mailto` | none | Contact address for the Crossref and OpenAlex polite pools |
| `--debug` | off | One line per request on stderr |
| `-o`, `--output` | `text` | `text` or `json` |
| `--help` | | Help for any command |
| `--version` | | Print the version |

## The pace floor

`--pace` sets the gap between requests to a single host, and one second is a floor. Passing `--pace 100ms` gets you one second and a line on stderr saying so. There is no flag, environment variable or config file that lowers it, and that is deliberate: it is the one setting where the person running the tool and the site serving it want different things.

The search surface gets its own five second bucket, because it is the one that trips. Raising `--pace` above five seconds raises search with it.

## The cache

Responses are cached for 24 hours, keyed on the url you asked for. `spr cache` shows the directory, how many responses are in it and how much room they take; `spr cache --clear` empties it. `XDG_CACHE_HOME` moves the default location.

A challenge and a 404 are both cached. A work that does not exist still does not exist five minutes later, and asking again for a challenge is how you get another one.

## There is no user agent flag

Three user agents were measured against the search challenge, including a current Chrome string, and it treated all three exactly the same. A flag that does nothing is worse than no flag.

## The polite pools

Crossref and OpenAlex both run a separate queue for callers who identify themselves. Set `--mailto` and requests to both hosts carry it, which is a faster queue and a nicer neighbour. Without it they go to the common pool, which works and is slower. `--debug` names the pool each request went to.

Each host also has its own pace bucket. A limit link.springer.com sets says nothing about what Crossref will serve, so `spr search --also crossref` adds a request to the bill and nothing to the estimate.

## The API key

`spr api` is the only command that needs a credential. Everything else reads pages and open indexes that need none.

The key is read from `SPRINGER_API_KEY` first and from the config file second, and `spr version` names which of the two answered without naming the key. It is never printed, never cached and never written to a record. It travels in the query string because that is the only way the endpoints accept one, and every url that reaches a log line, a cache key, an envelope or a record has it blanked out first, including the `citation_springer_api_url` the work page itself publishes with a key in it.

The config file is `$XDG_CONFIG_HOME/spr/config`, or the platform config directory when that is unset. It is `key=value`, one per line, with `#` for a comment, and `api_key` is the only key it holds:

```
# ~/.config/spr/config
api_key=your key here
```

That is deliberately the smallest format that can hold a credential. A file this tool reads a secret out of is not the place for an expressive syntax with an edge case in it.

A missing key and a wrong key both answer 401 with an identical body, so a missing one is caught before the request is made rather than spending a request to be told something that cannot be told apart.

## Environment variables

| Variable | Meaning |
|---|---|
| `XDG_CACHE_HOME` | Moves the default cache directory |
| `XDG_CONFIG_HOME` | Moves the default config directory |
| `SPRINGER_API_KEY` | The key for `spr api`, read before the config file |
