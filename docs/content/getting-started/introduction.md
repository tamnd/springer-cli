---
title: "Introduction"
description: "What spr is, and why it classifies every response before parsing one."
weight: 10
---

spr is a single pure Go binary that reads [link.springer.com](https://link.springer.com). There is nothing to sign up for and nothing to run alongside it. It reads data the site already serves publicly, shapes it into records, and gets out of your way.

## A 200 is not a success

This is the fact the whole tool is built around. Every one of these answered 200:

- a chapter behind a subscription, with a full set of metadata, `access=No`, and no body
- a pdf url without a subscription, which redirects across to the chapter page and serves 282,045 bytes of html
- the search surface under load, which serves a 3,038 byte Fastly client challenge

And a work that does not exist answers an honest 404 with 122,477 bytes of body, so size tells you nothing either.

So every response is sorted into one of five states, on its content, before anything is parsed:

| status | what it means | exit code |
| --- | --- | --- |
| `ok` | the page came back whole | 0 |
| `restricted` | the publisher states `access=No`: metadata yes, body no | 4 |
| `challenged` | the edge served a client challenge instead of the page | 2 |
| `wrong_kind` | the url answered with something other than what was asked for | 3 |
| `not_found` | there is no such page, however large the error page is | 3 |

`restricted` is not an error. The metadata is real and is printed; only the body is missing, and the exit code is there so a script can tell.

## How it is built

- A **library package** (`spr`) holds the HTTP client, the pacer, the cache and the classifier. It follows the redirect chain by hand, paces itself per host, and reads rate limits off live responses.
- A **command tree** (`cli`) wraps the library in subcommands with shared output formats and flags.
- One **`cmd/spr`** entry point ties them together.

## Being a good guest

One request at a time per host. Two seconds between requests by default, five for the search surface because that is the one that trips, and one second is a floor that no flag, environment variable or config file can go under. A challenge is never retried, because volume is what causes it.

Next: [install it](/getting-started/installation/), then take the [quick start](/getting-started/quick-start/).
