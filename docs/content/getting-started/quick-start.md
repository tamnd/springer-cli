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

## Read one work

`spr work` is the first command that parses anything. A DOI is enough, and the four work types share one record.

```console
$ spr work 10.1007/s10994-021-05946-3
doi           10.1007/s10994-021-05946-3
type          article
title         Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods
published in  Machine Learning 110(3) pp 457-506
access        free to read, access=Yes, world readable declared and empty
...
envelope      html, ok, 718572 bytes, 3 redirects, fetched 2026-08-18 10:12:04 UTC
              40 fields answered, 0 missed, 50 regions unread
```

Add `--envelope` to see which rung answered each field, and run `spr extraction` for the table of which rung is expected to answer what, and why. There is a longer walkthrough in [reading one work](/guides/reading-one-work/).

## Find something to read

`spr search` runs one query against the two surfaces Springer serves it on, `/search` and `/search.rss`, and returns one merged answer.

```console
$ spr search "aleatoric uncertainty" --type article --from 2020 --to 2024 --limit 5
search: html enrichment matched 3 of its 20 results to the feed's by doi
search: the other 17 are the two orderings disagreeing, and they are left out because only page 1 of the html was read, so pass --enrich to read the rest and keep them
557 results, showing 5, via rss+html

  1  An empirical method for modelling the secondary shock from high
     explosives in the far-field
     2024-12-28
     10.1007/s00193-024-01208-y
     https://link.springer.com/article/10.1007/s00193-024-01208-y
...
```

Three of twenty is not a bug in either surface. The HTML honours `sortBy` and the feed ignores it and always answers newest first, so the two orderings are genuinely different and the results are joined on DOI rather than on position.

`--facets` is one request and prints the shape of a result set before you spend anything on fetching it:

```console
$ spr search "aleatoric uncertainty" --type article --from 2020 --to 2024 --facets
557 results

content-type                  Article 557, Research article 482, Review
                              article 57, News article 1

publishing-model              Open access 291

language                      English 555, German 2
```

`--dry-run` bills a long run first. Both search paths share a five second pace bucket of their own, so `--limit 500` is 26 requests and a little over two minutes. [Searching](/guides/searching/) covers why the feed is the primary path and which four parameters fail silently without their quotes.

## Read the thing it was published in

```bash
spr journal 10994                    # by Springer id, or by either issn
spr journal 10994 --volumes          # 114 volumes and 348 issues, one more request
spr book 978-3-031-28170-9           # by isbn, or by doi
spr book --chapters 10.1007/978-3-031-28170-9
spr series 558
```

Three pages, three records, because a journal has an impact factor and no price and a book has four ISBNs and no volumes. [Reading a container](/guides/reading-a-container/) covers what each one carries and where it comes from.

## Read how often it was read

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

The counter travels with the count, because Crossref reports 1,553 citations for the same DOI and OpenAlex 1,563, and all three are correct about three different corpora. The rank comes with the size of the cohort it was ranked in, for the same reason: 96th percentile of 29 tracked articles is one article out of twenty-nine.

## Read a figure or a table

```bash
spr figures 10.1007/s10994-021-05946-3       # 17 figures, off the article page, no extra request
spr figures 10.1007/s10994-021-05946-3 1     # one figure at 1177 wide instead of 685
spr tables 10.1007/s10994-021-05946-3        # the captions, which is all the article page has
spr tables 10.1007/s10994-021-05946-3 1      # the rows
```

That last one is not a convenience. The article page announces its tables and contains zero `<table>` elements, so the rows are published on the subpage and nowhere else. [Counts and assets](/guides/counts-and-assets/) has the measurements.

## List what is there, rather than searching it

```bash
spr sitemap                                  # one request, the shape of the whole site
spr sitemap --static journals                # three requests, every journal there is
spr sitemap --kind article --since 2026-08-01 | spr work
```

Urls go to stdout one per line and everything else goes to stderr, so that last line is a pipeline. The date in a shard's file name is a bucket and not a publication date, which is the one thing worth reading [the guide](/guides/sitemaps/) for before walking any of it.

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
