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

## Where every field came from

Reading a work is four rungs, tried in order, and the first one that answers wins:

| rung | source | why it sits there |
| --- | --- | --- |
| 1 | Highwire, Dublin Core and PRISM meta tags | Google Scholar reads them, so a publisher who breaks them hears about it within days |
| 2 | the schema.org JSON-LD block | typed and nested, and nobody outside the publisher checks it |
| 3 | Springer's own `data-test` and `data-title` regions | the site's own names for its own components |
| 4 | css class names | presentational, and one redesign from meaning something else |

Authors are the one deliberate exception and come from rung 2. Highwire emits three parallel arrays for name, institution and email and lines them up by position, which breaks the moment one author has two affiliations, and breaks silently. JSON-LD binds the three to the right person.

A container page is rung 3 and rung 4 work throughout. There is no `citation_journal_title` on a journal home page and no `citation_isbn` on a book page, because nothing outside the publisher reads those pages for bibliographic metadata the way Scholar reads an article. Every field on a journal, book or series therefore comes from Springer's own `data-test` region names or, failing that, a css class and a printed English label, and the record says so rather than letting a fragile field look as solid as a durable one.

`spr extraction` prints the whole table, one row per field, with the reason each row sits where it does. Every record then carries an envelope saying which rung actually answered:

```console
$ spr work 10.1007/s10994-021-05946-3 --envelope
doi           10.1007/s10994-021-05946-3
type          article
title         Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods
published in  Machine Learning 110(3) pp 457-506
access        free to read, access=Yes, world readable declared and empty
...
envelope     html, ok, 718572 bytes, 3 redirects, fetched 2026-08-18 10:12:04 UTC
             40 fields answered, 0 missed, 50 regions unread

via
  authors                linkdata:author[]
  access.world_readable  highwire:citation_fulltext_world_readable (present, empty)
  references             highwire:citation_reference
  ref_links              selector:.c-article-references__links a
  sections               region:section[data-title]
```

Absent means absent. A field the page did not carry is left out rather than emitted as null, a field that was looked for and did not arrive is named in `missed` with the reason, and the regions nobody read are listed rather than quietly dropped, so a record never looks more complete than it is.

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
spr work 10.1007/s10994-021-05946-3             # one article, chapter, protocol or entry
spr work --envelope /chapter/10.1007/978-3-030-58607-2_1
spr work -o json 10.1007/s10994-021-05946-3 | jq .references
spr journal 10994                               # a journal, by id or by either issn
spr journal 10994 --volumes                     # 114 volumes and 348 issues
spr book 978-3-031-28170-9                      # a book, by isbn or by doi
spr book --chapters 10.1007/978-3-031-28170-9
spr series 558                                  # a book series
spr extraction                                  # the field table: rung, source, reason
spr extraction authors
spr extraction journal.title
spr extraction --record book
spr extraction --rung selector
spr get /article/10.1007/s10994-021-05946-3     # fetch and classify one url
spr get --body /journal/10994 > journal.html    # the raw page
spr get -o json '/search.rss?query=uncertainty' # the same, as json
spr cache                                       # what is cached and how much
spr cache --clear
spr version
```

A DOI is enough for `spr work`. The registrant prefix says who issued it and nothing about what it is, so the suffix orders the paths it could live under and they are tried until one answers.

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
spr/          the library: client, pacer, cache, classifier, the extraction ladder
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

Building towards [v0.1.0](https://github.com/tamnd/springer-cli/issues/2). The client, its classifier, the identifiers, the work record and the container records are in; subpages, search, sitemaps, the open indexes and the graph follow.

## License

Apache 2.0
