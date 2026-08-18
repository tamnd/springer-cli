---
title: "The open indexes"
description: "Crossref, OpenAlex and the publisher's own API: what each one answers that link.springer.com does not."
weight: 50
---

Three of the four commands in this guide go somewhere other than link.springer.com, and the fourth goes to the publisher through a door the web site does not use. Each one exists because the site has a gap that host fills:

```bash
spr crossref 10.1007/s10994-021-05946-3    # what the publisher deposited
spr openalex 10.1007/s10994-021-05946-3    # what an open index derived
spr cited-by 10.1007/s10994-021-05946-3    # who cites it, which the site never says
spr api --doi 10.1007/s10994-021-05946-3   # the publisher's own API, with a key
```

None of them replaces `spr work`. The site is still the publisher's own statement about its own work, and everything here is a second opinion, a derived record or a direction the site does not publish in.

## Crossref holds what was deposited

A rendered page shows what a publisher chose to display. Crossref holds what the same publisher deposited as a member of the registration agency, and the two are not the same document.

For DOI `10.1007/s10994-021-05946-3`, the deposit carries 1,061 characters of JATS abstract where the search card carries a truncated snippet, both ISSNs of the journal with the medium of each stated, five separate dates rather than one, the funder with its award numbers, an ORCID with whether the person authenticated it, and the licence terms with the day each starts.

```console
$ spr crossref 10.1007/s10994-021-05946-3
doi           10.1007/s10994-021-05946-3
type          journal-article
title         Aleatoric and epistemic uncertainty in machine learning: an
              introduction to concepts and methods
published in  Machine Learning 110(3) pp 457-506
publisher     Springer Science and Business Media LLC
issn          0885-6125 print
issn          1573-0565 electronic
issued        2021-03
online        2021-03-08
```

Two of those lines are the reason the dates are printed apart rather than reconciled. Four of the five deposited dates were a month and one was a day, and folding them into a single `published` would have thrown away the only one that names a day.

The affiliation arrays on this record were empty on every author, which is a deposit that does not carry the information rather than a parser that missed it. That is what the envelope's `missed` list is for, and it is why an empty column here does not mean the same thing as an empty column on the work page.

### The reference list as identifiers

The work page renders a reference list as text. Crossref holds it as identifiers, which is the difference between a citation you can read and one you can follow:

```console
$ spr crossref 10.1007/s10994-021-05946-3 --references | head -3
10.1007/978-3-642-40994-3_29
10.1613/jair.4192
10.1023/A:1007607513941
crossref: 66 of 122 deposited references carry a doi, and 56 do not
```

The identifiers go to stdout and the count goes to stderr, so the list pipes cleanly into another command while a person watching still learns it is partial:

```bash
spr crossref 10.1007/s10994-021-05946-3 --references | spr work
```

Fifty six of the 122 entries carry no DOI at all. Those are not lost, they are unresolvable, and a graph built from this list should say so rather than quietly having 66 edges where the paper has 122 references.

## OpenAlex holds both directions

Crossref holds what a work cites. OpenAlex holds that and what cites it, along with an institution graph and a field normalized impact figure:

```console
$ spr openalex 10.1007/s10994-021-05946-3
id            W3014596384
access        open, hybrid

authors (2)
   1  Eyke Hüllermeier  first  https://orcid.org/0000-0002-9944-4108
      Paderborn University  https://ror.org/058kzsd48  DE

counts
  openalex_citations             1,563, as stored on 2026-08-16T07:02:28.622633
  openalex_references            111 resolved to works in the index
  fwci                           113.99, against the average work of its field, year and type
  percentile                     0.999703, in the top 1 percent
```

The ROR id is the one place in this tool where an affiliation is an identifier rather than a string, which is what makes an institution joinable across works. The abstract arrives as an inverted index, a map of each word to the positions it occupies, and is put back in reading order before it is printed.

Both `concepts` and `topics` are printed, because OpenAlex publishes both classifications and they disagree with each other. Picking one and hiding the other would present a settled answer where the source has two.

## Who cites this

Nothing on link.springer.com lists the works that cite a work. The metrics page states a total attributed to Dimensions and names none of them. `spr cited-by` is the direction the site has no page for:

```console
$ spr cited-by 10.1007/s10994-021-05946-3 --by-year
W3014596384 cited by 1,554 works, as OpenAlex counts them today
  2026  188
  2025  412
  2024  386
  2023  268
```

That total is 1,554 where the record two sections up says 1,563, measured in the same minute. One is a live count of the listing and the other is a stored aggregate rebuilt on its own schedule. Neither is wrong, both are OpenAlex's, and each command says which of the two it is holding. This is why the stored number never prints without the date it was stored on.

`--by-year` is one request. A full listing of 1,554 works is eight, at 200 per page, so ask by year when the question is when a work was read and ask for the listing when the question is by whom:

```bash
spr cited-by 10.1007/s10994-021-05946-3 -o json | jq -r '.works[].doi' | spr work
```

A DOI costs one extra request, because the listing is keyed on the OpenAlex work id and the DOI has to be resolved first. Passing `W3014596384` skips that.

## The publisher's own API

`spr api` is the one surface here that needs a credential:

```bash
export SPRINGER_API_KEY=...
spr api --doi 10.1007/s10994-021-05946-3
```

The key is read from `SPRINGER_API_KEY` or from the config file. It is never printed, never cached and never written to a record. It travels in the query string because that is the only way these endpoints accept one, and everything downstream of the request sees the url with the key blanked out. That includes `citation_springer_api_url`, which the work page itself publishes with a key in it.

A missing key and a wrong key both answer 401 with an identical body, so there is nothing to tell them apart with and the error says where to put a key rather than guessing which of the two happened.

Every field this command decodes comes from the published schema rather than from a measured response, because no key was available to measure one with. The envelope lists every top level key the decoder did not read, so the first run with a real key reports the gap instead of hiding it:

```bash
spr api -o json --doi 10.1007/s10994-021-05946-3 | jq .envelope.unread
```

## Widening a search

`--also` asks an index the same question the site was asked and merges the answer:

```console
$ spr search "aleatoric uncertainty" --also crossref --also openalex
search: crossref matched 213566 and returned 20, 4 already in the Springer results and 16 new
search: openalex matched 21402 and returned 25, 6 already in the Springer results and 19 new
557 results
```

The gap between 557 and 213,566 is the point. A search of link.springer.com returns what Springer publishes, and neither set shows that difference on its own.

The join is on the normalized DOI and on nothing else. Titles vary in punctuation and case between the three sources, positions are meaningless across corpora that were sorted differently, and a merge on either would silently fuse two different works. A result with no DOI joins to nothing and stays where it is, and the note says how many of those there were.

Every result records which backends answered for it, so one all three returned reads `rss+html+crossref+openalex`. A backend fills exactly one field, the abstract, and only when the site left it empty, and the envelope records that it did.

The backend counts go to stderr and into `notes` in the JSON. They never enter the result set, where a total of 213,566 sitting next to twenty returned results would read as a count of what you have.

Two things `--also` does not do. It does not send `--type`, because Springer's content types are its own words and no mapping onto the Crossref or OpenAlex vocabularies was measured, and stderr says so rather than guessing quietly. It does not work with `--facets`, which counts the Springer result set while the backends count their own.

## Three counts, and no fourth

The same work has three citation counts, and every one of them is printed under a name that says who counted:

| Who counted | Citations |
|---|---|
| Springer, on its own metrics page | 1,906 |
| Crossref, deposited citations only | 1,553 |
| OpenAlex, stored aggregate | 1,563 |
| OpenAlex, live listing | 1,554 |

No command in this tool prints a merged citation count. Averaging them would invent a number no host published, and picking one would hide the two that disagree. See [counts and assets](/guides/counts-and-assets/) for the same argument from the site's side.

## What each host costs

Each host has its own pace bucket, because a limit one publisher sets says nothing about what another will serve. A request to Crossref does not queue behind the five seconds the search surface waits, which is why `--also` adds requests to the bill and nothing to the estimate.

Crossref and OpenAlex both run a polite pool for callers who identify themselves. Set `--mailto` or put a mail address in the config and both hosts route the request to it, which is a faster queue and a nicer neighbour. `--debug` says which pool a request went to.

OpenAlex meters a query in dollars rather than in requests, so a page of `spr openalex` results says what it cost of the metered budget. On that host the money binds before the request count does.
