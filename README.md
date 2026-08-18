# spr

A delightful command line for [link.springer.com](https://link.springer.com): works, journals, books, series, references, metrics, and the graph between them.

`spr` is a single pure Go binary. It reads what Springer Nature Link publishes, classifies every response before parsing a byte of it, and records where each field came from. No API key, nothing to run alongside it.

## Why classification comes first

A 200 from this site means very little on its own. All of these answered 200:

```console
$ spr get --kind pdf /content/pdf/10.1007/978-3-031-28170-9_5.pdf
url          https://link.springer.com/content/pdf/10.1007/978-3-031-28170-9_5.pdf
final        https://link.springer.com/chapter/10.1007/978-3-031-28170-9_5?error=cookies_not_supported&code=eefe8166
code         200
status       wrong_kind
redirects    7
bytes        282045
content type text/html; charset=utf-8

The url answered with something other than what was asked for. A pdf url doing this is
the usual case and means the pdf is behind a subscription.
```

A chapter behind a subscription answers 200 with a full set of metadata, `access=No` and no body. The search surface answers 200 with a 3,038 byte Fastly challenge. And a work that does not exist answers an honest 404 with 122,477 bytes of body, so nothing can be guessed from size either.

Every response is therefore sorted into one of five states on its content, before anything is parsed:

| status | what it means | exit |
| --- | --- | --- |
| `ok` | the page came back whole | 0 |
| `restricted` | the publisher states `access=No`: metadata yes, body no | 4 |
| `challenged` | the edge served a client challenge instead of the page | 2 |
| `wrong_kind` | the url answered with something other than what was asked for | 3 |
| `not_found` | there is no such page, however large the error page is | 3 |

## Install

```bash
go install github.com/tamnd/springer-cli/cmd/spr@latest
```

Or take a prebuilt binary from the [releases](https://github.com/tamnd/springer-cli/releases), or run the container image:

```bash
docker run --rm ghcr.io/tamnd/spr:latest --help
```

## Usage

```bash
spr get /article/10.1007/s10994-021-05946-3     # fetch and classify one url
spr get --body /journal/10994 > journal.html    # the raw page
spr get -o json '/search.rss?query=uncertainty' # the same, as json
spr cache                                       # what is cached and how much
spr cache --clear
spr version
```

Every command shares these:

```
--pace       interval between requests to one host, never below 1s
--timeout    per request timeout
--retries    retries on a transport error or a 5xx, never on a challenge
--cache      cache directory
--no-cache   fetch fresh and store nothing
--mailto     contact address for the Crossref and OpenAlex polite pools
--debug      one line per request on stderr
-o, --output text or json
```

## How it behaves

**One request at a time, paced.** Two seconds between requests to a host by default, five for the search surface because that is the one that trips, and one second is a floor that no flag, environment variable or config file can go under.

**A challenge is never retried.** Volume is what causes it, and answering a rate limit with more requests is the reason the limit exists.

**Redirects are followed by hand.** Every first request runs a three hop cookie dance and comes back with `?error=cookies_not_supported&code=<uuid>` appended, and the uuid is different every time. A restricted pdf url runs the whole dance twice, seven hops, and lands on the chapter page. The cache is keyed on the url that was asked for, never the one the chain landed on, or the uuid would reach every key and the hit rate would be silently zero forever.

**Rate limits are read, not assumed.** `X-RateLimit-*` and `Retry-After` come off live responses rather than from a number compiled in off a documentation page.

**There is no user agent flag.** Three user agents were measured against the search challenge, including a current Chrome string, and it treated all three exactly the same. A flag that does nothing is worse than no flag.

## Development

```
cmd/spr/      thin main, wires cli.Root into fang
cli/          the cobra command tree
spr/          the library: client, pacer, cache, classifier
docs/         the tago documentation site
scripts/      drift.sh, the weekly live probe
```

```bash
make build
make test
make fmt
./scripts/drift.sh    # six live probes, the ones the classifier is built on
```

## Status

Building towards [v0.1.0](https://github.com/tamnd/springer-cli/issues/2). The client and its classifier are in; identity, records, subpages, search, sitemaps, the open indexes and the graph follow.

## License

Apache 2.0
