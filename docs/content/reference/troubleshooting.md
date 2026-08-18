---
title: "Troubleshooting"
description: "The handful of things that trip people up, and what each one actually means."
weight: 40
---

Most of these are the site behaving the way it behaves rather than a bug.

## `status challenged`

The Fastly edge in front of `/search` served a client challenge instead of the page. Three things are worth knowing about it, all measured:

- It is triggered by **volume**, at roughly twenty requests, not by the depth of the page or by the query.
- It does **not clear by waiting**, so retrying in a minute gets you another one. spr never retries a challenge for exactly this reason.
- It is scoped to **search only**. Article, journal, book and sitemap urls keep answering normally while search is challenged.

`spr search` already handles this. The feed is its primary path, so a challenged html pass costs the total, the facet counts and the card fields and does not cost the results, and it says so on stderr in the same breath as the output. If both paths are challenged there is nothing to return and the exit code is 2, which is its own code because a challenge is a rate rather than a refusal and waiting is the fix.

Reaching for `/search.rss` by hand with `spr get` works too, and is what `spr search --path rss` does for you.

## `status restricted`

The publisher states `access=No`. This is not a failure and not something to work around: the metadata is real, it was printed, and the body is genuinely not being served to you. The exit code is 4 so a script can tell the difference.

## `status wrong_kind` on a pdf url

The pdf is behind a subscription. The url ran the cookie dance, got redirected across to the chapter page, and served html. A tool that reported this as success would be handing you an html file named `.pdf`.

## Seven redirects

Normal. Every first request runs a three hop cookie dance, and a restricted pdf url runs the whole thing twice on its way to the chapter page. The budget is ten. More than that is a genuine loop and is reported as one.

## Requests seem slow

They are paced on purpose: two seconds between requests to one host, five for search. `--pace` raises it. It does not lower it below one second, and passing a smaller value prints a line saying so.

## A 429, or `X-RateLimit-Remaining` at zero

An upstream is enforcing a budget it told us about in a header. spr reads those headers off live responses rather than assuming a number. Wait for the reset, which the message names, and raise `--pace` if you are going to be running for a while.

## Nothing found for something you expected

The public surface is not the whole site. Check the spelling the site itself uses, and check the same url in a private browser window before concluding it is missing. A missing work answers 404, which spr reports as `not_found` and exit 3, so an empty result and a wrong url are easy to tell apart.

For a search specifically, check the spelling of the facet value against `spr search "<terms>" --facets`, which prints the site's own labels and counts for one request. The four faceted parameters have to reach the site wrapped in double quotes and spr adds them for you, so a taxonomy or discipline that matches nothing is a value the site does not use rather than a value that arrived malformed. A query that matched nothing exits 3 and prints nothing, which is not the same as a run that failed.

## The binary is not on your PATH

`go install` puts the binary in `$(go env GOPATH)/bin`, usually `~/go/bin`, and a release archive leaves it wherever you unpacked it. See [installation](/getting-started/installation/).

## Seeing what spr actually did

`--debug` prints one line per request on stderr: the code, the classification, the byte count, the hop count and the url. That is usually enough to tell a rate limit apart from a genuinely empty result.
