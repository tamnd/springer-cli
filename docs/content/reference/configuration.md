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

## Environment variables

| Variable | Meaning |
|---|---|
| `XDG_CACHE_HOME` | Moves the default cache directory |
