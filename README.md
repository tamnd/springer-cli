# spr

A delightful command line for [link.springer.com](https://link.springer.com): works, journals, books, series, references, metrics, and the graph between them.

`spr` is a single pure Go binary. It reads what Springer Nature Link publishes, classifies every response before parsing a byte of it, and records where each field came from. Nothing to run alongside it, and no API key for anything on the site itself.

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
envelope     html, ok, 718371 bytes, 3 redirects, fetched 2026-08-18 14:43:08 UTC
             41 fields answered, 0 missed, 46 regions unread

via
  authors                linkdata:author[]
  access.world_readable  highwire:citation_fulltext_world_readable (present, empty)
  references             highwire:citation_reference
  ref_links              selector:.c-article-references__links a
  sections               region:section[data-title]
```

Absent means absent. A field the page did not carry is left out rather than emitted as null, a field that was looked for and did not arrive is named in `missed` with the reason, and the regions nobody read are listed rather than quietly dropped, so a record never looks more complete than it is.

## A number without its counter is not a fact

`spr metrics` reads the one page that says how often a work was read and cited, and it never hands back a bare integer:

```console
$ spr metrics 10.1007/s10994-021-05946-3
accesses      134k, about 134,000, which the page calls an approximate count
citations     1,906 per Dimensions
attention     52
  ranked      22,032nd of 474,090 tracked articles in all journals
              95th percentile
  ranked      1st of 29 tracked articles in Machine Learning
              96th percentile
```

Springer says 1,906 citations for that DOI, Crossref says 1,553 and OpenAlex says 1,563. All three are right about three different corpora, so `Dimensions` is read off the page's own sentence and travels with the number. The rank arrives with two cohorts and both are kept, because 96th percentile of 29 tracked articles and 95th percentile of 474,090 are not the same claim and the sizes are the only thing that says so.

The same page counts 1,334 pieces of attention and names five of them. Those are two fields, `breakdown` and `mentions`, because reading `len(mentions)` as the total would be wrong by three orders of magnitude.

## The rows of a table are not on the article page

`spr figures` and `spr tables` look like a matched pair and are not.

The article page carries every figure inline, so listing them costs nothing and asking for one buys resolution: 685 pixels wide on the article, 1177 on `/figures/1`, and the tool reads the second url off the page rather than rewriting the first.

The article page carries no tables at all. The open access capture is 718 KB of html which announces one table, links to it, and contains zero `<table>` elements. So `spr work -o json` gives you a `tables` array of captions and links with no rows in it, deliberately, and `spr tables <doi> 1` is the only way to read one.

## A date in a sitemap file name is not a date

The sitemaps are the only complete enumeration of this site, and the first thing to know about them is what their dates mean.

The index holds 10,408 child sitemaps. 66 of them are named `sitemap_2020-01-01_N`, which is roughly 330,000 works filed under one nominal day, because the first of January is where everything known only to its year ends up. Read the first of those shards and its 5,000 urls carry 173 distinct `lastmod` values running from 2020-01-23 to 2026-08-17, and not one of them is 2020-01-01.

So the field is called `bucket` everywhere in this tool, it is parsed from the file name, and no flag turns it into a published date. `--since` and `--until` say which shards to read, and a window compares against the span a bucket covers rather than its first instant, so `--since 1850-06-01` still keeps a shard filed under 1850.

The same care applies to the bill. A walk of everything is 10,408 requests and five and three quarter hours, and the estimate for it is computed from the index that was just fetched rather than from a number compiled in here, because the index grows every day.

## Two search paths that disagree

`/search` and `/search.rss` are one query engine and two answers. Fetched for the same query in the same minute, HTML page 1 and RSS page 1 share 3 results out of 20.

The feed is the primary path, because it pages to the end of the result set, carries the full abstract where the card carries 180 characters and an ellipsis, states the bare DOI in `guid`, and kept answering while the HTML surface was serving challenges. The HTML is the enrichment pass, because the total, the facet counts and the per result content type, container and author list exist there and nowhere else.

They disagree because the HTML honours `sortBy` and the feed ignores it and always answers newest first. So the two are joined on DOI and never on position, joining them by index would have attached the wrong authors to 17 results in 20, and `spr search --sort relevance` says on stderr that the sort reached one path and not the other.

Four of the facet parameters have to arrive quoted, `taxonomy="Machine Learning"` and three others. Unquoted is a valid request that answers 200 and matches nothing, which is the worst failure a search can have because it is indistinguishable from a query with no results. The quotes are added in one shared place and a test reads the requirement off a captured page.

## The site has no page for who cites a work

A work page lists what a work cites. Nothing on link.springer.com lists what cites it, and the metrics page states a total attributed to Dimensions without naming a single citing work. So `spr cited-by` asks OpenAlex, which publishes the edges themselves, and `spr crossref --references` gets the other direction as identifiers rather than as rendered text:

```console
$ spr crossref 10.1007/s10994-021-05946-3 --references > refs.txt
crossref: 66 of 122 deposited references carry a doi, and 56 do not
```

The identifiers go to stdout, one per line, and the count goes to stderr, so a pipe gets a clean list and a person watching still learns the list is partial.

Fifty six of those 122 entries carry no DOI at all. They are unresolvable rather than missing, and a graph built from this list should say so rather than quietly having 66 edges where the paper has 122 references.

Three hosts, four numbers, one work. Springer's metrics page says 1,906 citations, Crossref says 1,553 deposited, OpenAlex stores 1,563 and its live listing counted 1,554 in the same minute. Every one of them prints under a name that says who counted and, for the stored aggregate, the date it was stored on. No command here prints a merged count: averaging them would invent a number no host published and picking one would hide the two that disagree.

`spr search --also crossref --also openalex` asks the indexes the same question the site was asked and merges the answers on the normalized DOI, which is the only key the three sources agree on. 557 results from Springer against 213,566 matches at Crossref is a fact about the query that neither set shows on its own, so the backend totals go to stderr where they cannot be mistaken for a count of what came back.

## A name is not an identifier

`spr graph` turns those records into nodes and edges, and the only hard question in it is who is who. Scholarly publishing solved naming in the 1990s and Springer prints the results in the page head, so this tool mints no identifier for anything that already has one. The problem is the other half: four authorities are in play and the job is keeping them from being quietly conflated.

Every node's uri names the authority that identified it. The measured article has two authors and one of them registered an ORCID:

```
spr:person/orcid/0000-0002-9944-4108                    Eyke Hüllermeier    via orcid
spr:person/name/d58cff045d1d56f7215ff4a315b4fb70ee0…    Willem Waegeman     via name
```

One of those can be joined to any other graph in the world. The other is a hash of a name string on one page, is worth exactly what a name string is worth, and this tool will not join it to anything on its own. There is no third case and no confidence score. `--merge-names` merges a name into an ORCID when the name matches exactly one, refuses when two ORCIDs answer to the same name, and writes `mergedFrom` on the survivor so the guess is a fact in the output rather than a decision buried in it.

The same rule kills citation edges. The article page prints 122 references and states a DOI for none of them, so the html tier produces zero `cites` edges, not weak ones. `--also crossref` reads the deposit and turns 66 of the same 122 into edges, and both counts are printed, because a graph that quietly has 66 edges where the paper has 122 references is a graph that lies by omission.

Ten output formats, of which four are RDF, and the vocabulary is borrowed from schema.org, Dublin Core, PRISM, BIBO, FaBiO, CiTO and FRAPO down to exactly four local terms. Holding that budget is why the four containment edges share `dcterms:isPartOf` and are told apart by the object's `rdf:type`, and why ordered authorship is a plain `rdf:List` rather than a reified position. Under `--format nq` the fourth term is the tier, so a store can be asked what the html alone said, and dropping that column leaves valid N-Triples.

## A parser that quietly reads two fields fewer looks exactly like one that is fine

So there is a ledger. `spr/testdata/capture.txt` records, for each of the fourteen captured pages, how many meta names and JSON-LD blocks it carried, which vocabularies it declared, whether its two access statements agreed, which fields came out set, which were looked for and missed, and how many `data-test` regions nobody read. It is a text file that a diff explains itself in.

`go test ./spr` reads it against the frozen bytes, which proves the extractor did not change. That is only half the question, and it is the easier half: bytes in a repository cannot tell you the site moved. So the same code ships in the binary, and `spr verify` produces the same reading from pages fetched today.

```console
$ spr verify --live
source     a live refetch
ledger     14 captures recorded in the ledger this binary was built with

ok          article_oa.html
ok          article_subscription.html
...
14 ok
```

Fewer fields set is a regression and is this tool's fault. More fields set is an improvement, and it is reported until somebody records it, because an improvement nobody noticed is how a tool ends up with two versions of what it promises. A vocabulary appearing or disappearing is the site restating a fact and needs a person. A change in unread regions is drift, is Springer shipping a component, and never fails. All of those exit 7, which is `verify` and nothing else, so a scheduled job can alert on the site moving without having to tell it apart from a mistyped flag.

Every line says whether it was read from the cache or from the site, and repeats it on every finding rather than printing it once at the top. That is the one lesson here that was paid for: a cached page that had gone stale reported a regression that did not exist, and it took a live refetch to prove nothing had changed.

`spr verify --vocab` asks the other half of the question. Eleven bibliographic facts are stated by more than one vocabulary and the access statement makes a twelfth, so a work page says its own title in Highwire and in Dublin Core and its own DOI in Highwire and in PRISM. Across the fourteen pages that is 75 comparisons and every one of them agrees, which is exactly why a disagreement would be worth printing.

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
spr search "aleatoric uncertainty"              # both search paths, one merged answer
spr search "uncertainty" --type article --from 2020 --to 2024
spr search "climate" --sdg "Climate action" --facets
spr search "uncertainty" --limit 500 --dry-run  # what it costs before it costs it
spr work 10.1007/s10994-021-05946-3             # one article, chapter, protocol or entry
spr work --envelope /chapter/10.1007/978-3-030-58607-2_1
spr work -o json 10.1007/s10994-021-05946-3 | jq .references
spr journal 10994                               # a journal, by id or by either issn
spr journal 10994 --volumes                     # 114 volumes and 348 issues
spr book 978-3-031-28170-9                      # a book, by isbn or by doi
spr book --chapters 10.1007/978-3-031-28170-9
spr series 558                                  # a book series
spr metrics 10.1007/s10994-021-05946-3          # accesses, citations and attention
spr figures 10.1007/s10994-021-05946-3          # the figure list, off the article page
spr figures 10.1007/s10994-021-05946-3 1        # one figure at full resolution
spr tables 10.1007/s10994-021-05946-3 1         # one table, rows and all
spr sitemap                                     # the shape of the whole site, one request
spr sitemap --static journals                   # every journal there is, three requests
spr sitemap --kind article --since 2026-08-01   # urls, one per line, ready to pipe
spr sitemap --all --yes --resume > urls.txt     # 10,408 shards, resumable
spr graph 10.1007/s10994-021-05946-3            # nodes and edges instead of records
spr graph 10.1007/s10994-021-05946-3 --also crossref --format ttl
spr graph 10994 --depth 1 --dry-run             # billed per depth before it walks
spr graph --merge monday.json --merge tuesday.json > both.json
spr crossref 10.1007/s10994-021-05946-3         # what the publisher deposited
spr crossref 10.1007/s10994-021-05946-3 --references
spr crossref --issn 0885-6125 --from 2024 --rows 100
spr openalex 10.1007/s10994-021-05946-3         # the open index, both directions
spr cited-by 10.1007/s10994-021-05946-3         # who cites it, which the site never says
spr cited-by 10.1007/s10994-021-05946-3 --by-year
spr search "uncertainty" --also crossref --also openalex
spr api --doi 10.1007/s10994-021-05946-3        # the publisher's own api, with a key
spr extraction                                  # the field table: rung, source, reason
spr extraction authors
spr extraction journal.title
spr extraction --record book
spr extraction --rung selector
spr verify                                      # do the ledger's pages still read the same
spr verify --live                               # ask the site rather than the cache
spr verify --vocab --capture article_oa         # what each vocabulary claims about one fact
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
./scripts/drift.sh    # seven live probes, the ones the classifier is built on
spr verify --live     # the fourteen ledger pages, read again off the site
```

`scripts/drift.sh` and `spr verify --live` are the two halves of the weekly job in `.github/workflows/drift.yml`. It reports and does not fail: a red weekly job is a weekly job everybody learns to scroll past, and none of this is a broken build. It opens one issue, comments on it the next week if the drift is still there, and closes it when the site comes back.

## Status

Building towards [v0.1.0](https://github.com/tamnd/springer-cli/issues/2). The client, its classifier, the identifiers, the work record, the container records, the subpages, search, the sitemaps, the open indexes and the graph are in; the docs, the captures and the release follow.

## License

Apache 2.0
