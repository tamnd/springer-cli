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

## `spr api` says there is no key

Set `SPRINGER_API_KEY`, or put `api_key=...` in `~/.config/spr/config`. `spr version` says which of the two answered without printing the key itself.

That check happens before the request, because a missing key and a wrong key both answer 401 with an identical body and spending a request to be told something indistinguishable helps nobody. If the key is set and the run still 401s, the key is wrong or expired.

## Two hosts disagree about how many citations a work has

They are counting different corpora, and none of them is wrong. For DOI `10.1007/s10994-021-05946-3`, Springer's metrics page says 1,906 attributed to Dimensions, Crossref says 1,553 deposited, OpenAlex stores 1,563 and its live listing counted 1,554 in the same minute.

Every count in this tool prints under a name that says who counted, and the OpenAlex stored aggregate prints with the date it was stored on, which is what makes it differing from the live listing explicable rather than alarming. There is no command that merges them, deliberately. See [the open indexes](/guides/open-indexes/).

## `--also` returned fewer new results than expected

The join is on the normalized DOI and on nothing else. A backend result with no DOI joins to nothing and is counted separately in the note on stderr, and `--type` is not sent to the backends at all because Springer's content types have no measured mapping onto the Crossref or OpenAlex vocabularies.

If a backend is missing from the output entirely, it failed and said so on stderr. That is not a failed run, because two hosts answering is still worth having.

## `spr graph` produced no `cites` edges

Expected, on the html tier. The measured article prints 122 references and states a DOI for none of them, so there is nothing on the page to point a citation edge at, and a reference that did not resolve to an identifier deliberately becomes nothing rather than an edge to a guess. The count is printed both ways on stderr so the zero is never silent.

`--also crossref` reads the deposit instead of the rendering and turns 66 of those same 122 into edges. `--cited-by` needs OpenAlex, because no page on this site knows who cites it.

## `spr graph` stopped and asked for `--yes`

The walk was billed above twenty requests. The bill it printed is per depth, so the line that is larger than you expected is the one to change: `--depth 1` on a work is one extra request and on a proceedings volume it is forty. Lower `--depth`, add `--limit`, or pass `--yes` if forty one requests is what you meant. `--dry-run` prints the same bill and never fetches.

## Two person nodes for what is obviously one person

That is the design rather than a deduplication bug. A person with an ORCID and a person known only by a name string printed on one page are identified by two different authorities and are two nodes, and nothing merges them on its own. `--merge-names` merges them when a normalized name matches exactly one ORCID node, refuses when two ORCIDs answer to the same name, and records `mergedFrom` on the survivor so the guess stays visible. The same holds for an institution known by name and the same institution known by ROR.

## `--projection` or `--dir` was refused

`--projection coauthor` is computed by the gexf writer and `--dir` names where the csv pair goes, so each needs its own `--format`. Both are usage errors rather than flags that are quietly ignored.

## A graph file will not merge back in

`--merge` reads `--format json` and only that. It is the one form of the ten that holds every field of every node and edge, where the RDF forms drop the properties that have no standard term and the two xml forms number their nodes, so reading either of those back would return a smaller graph than was written without saying so.

## `spr verify` says every page is unread

It reads the page cache and the cache is empty, which is what a fresh install looks like. `spr verify --live` refetches all fourteen pages and writes them into the cache, so the run after it has something to read. The default deliberately does not fetch on a miss, because a run that says cache in its header and then goes to the network is a run whose header is wrong.

## `spr verify` reports a regression on a page you have not touched

Read the source line first. If it says the page cache, the cached copy may simply be old, and the site itself may not have moved at all. Run the same command with `--live` before changing any code. This is the exact mistake the command is built to make hard, which is why the source is repeated on every finding rather than printed once at the top.

## `spr verify` exits 7 on an improvement

That is deliberate. A field coming out set that was not set before is a change to what this tool claims, and it stays reported until somebody records it in the ledger with `go test ./spr -run TestCaptureLedger -update` and reads the diff. An improvement nobody noticed is how a tool ends up with two versions of what it promises.

## `spr verify --no-cache` is refused

`--no-cache` turns off the only thing a default run reads, so the two together ask for nothing. Use `--live`, which refetches. The two flags together are accepted and mean refetch and store nothing.

## `--vocab` prints nothing for a page

That page states no fact in more than one vocabulary, which is true of the search results page and of every container page. The cross-check needs two claims about one fact to have anything to compare, and a journal home page carries no bibliographic vocabulary at all.

## The binary is not on your PATH

`go install` puts the binary in `$(go env GOPATH)/bin`, usually `~/go/bin`, and a release archive leaves it wherever you unpacked it. See [installation](/getting-started/installation/).

## Seeing what spr actually did

`--debug` prints one line per request on stderr: the code, the classification, the byte count, the hop count and the url. That is usually enough to tell a rate limit apart from a genuinely empty result.
