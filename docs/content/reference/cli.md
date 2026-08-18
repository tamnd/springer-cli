---
title: "CLI"
description: "Every command and subcommand, with the flags that matter."
weight: 10
---

```
spr <command> [flags]
```

Run `spr <command> --help` for the full flag list on any command, and see [configuration](/reference/configuration/) for the flags every command shares.

## Commands

| Command | What it does |
|---|---|
| `get` | Fetch one url and report how it was classified |
| `search` | Search link.springer.com, on both of the paths it answers on |
| `work` | Read one article, chapter, protocol or reference work entry |
| `journal` | Read one journal home page, and optionally its volumes and issues |
| `book` | Read one book, proceedings volume or reference work |
| `series` | Read one book series home page |
| `metrics` | Read the accesses, citations and attention a work has drawn |
| `figures` | List a work's figures, or read one at full size |
| `tables` | List a work's tables, or read one in full |
| `sitemap` | Enumerate what the site publishes, from its own sitemaps |
| `graph` | Walk from a seed and write the nodes and edges as a graph |
| `crossref` | Read a work, its deposited reference list or a query from Crossref |
| `openalex` | Read a work or a query from OpenAlex |
| `cited-by` | List the works that cite this one, which this site has no page for |
| `api` | Query the Springer Nature API, which needs a key |
| `extraction` | Print the extraction table: every field, the rung that answers it, and why |
| `verify` | Read the ledger's pages again and say whether they still read the same |
| `cache` | Show or clear the page cache |
| `version` | Print the version, commit and build date |

## Reading identifiers from stdin

Nine commands take one identifier, so all nine take any number of them, and read them one per line from stdin when they are given none: `get`, `work`, `journal`, `book`, `series`, `metrics`, `crossref`, `openalex` and `cited-by`.

```bash
spr sitemap --kind article --since 2026-08-01 | spr work --yes
spr crossref 10.1007/s10994-021-05946-3 --references | spr work --yes
spr cited-by 10.1007/s10994-021-05946-3 -o json | jq -r '.works[].doi' | spr work --yes
spr work --yes < dois.txt
```

Blank lines are skipped and so is any line starting with `#`, which is what a hand maintained list of DOIs collects. Whitespace around a line is trimmed. Arguments always win: a command that was given identifiers does not read the pipe at all, so a stage in the middle of a pipeline that also names a work reads that work.

`figures` and `tables` are the two commands not on the list, because their second positional argument is a figure or table number and a list of works with no numbers attached would be ambiguous.

A run with no arguments and no pipe is refused rather than answered by waiting on the keyboard until you find ctrl-D.

### The bill

More than twenty identifiers stops and says what the run would cost before it makes the first request, because two thousand identifiers at the default pace is an hour and ten minutes and the moment to hear that is before it starts.

```console
$ spr crossref 10.1007/s10994-021-05946-3 --references | spr work
spr: 66 works at 2s pace is 2 minutes
     pass --yes to read them, or pipe in fewer
```

`--yes` is on all nine and means read them anyway. It is the same twenty and the same flag `spr graph` uses.

### What a run of many exits with

One identifier behaves exactly as it did before any of this existed: the exit code is that record's, and the error text is that record's. Everything already scripted against this tool keeps working.

More than one is a question about the run rather than about any one record, so:

| The run | Exit code |
|---|---|
| Every target read | 0 |
| Some read, some restricted or challenged | 0, with the counts on stderr |
| Every target restricted | 4 |
| Every target challenged | 2 |
| Any target failed outright | that failure's code |

One paywalled work in five hundred is not a restricted run, which is why a status has to cover every target before it becomes the run's. A failure part way through does not stop the rest, since a run that dies on the third of five hundred has thrown away twenty minutes of pacing to say something it could have said at the end. Each failure prints as `spr: <target>: <what went wrong>` and the run ends with one summary line, both on stderr:

```console
$ spr crossref 10.1007/s10994-021-05946-3 --references | spr work --yes
crossref: 66 of 122 deposited references carry a doi, and 56 do not
spr: 13 of 66 read, 6 restricted, 53 failed
```

Thirteen of that paper's 66 resolvable references are published by Springer and the other 53 are IEEE, Elsevier, MIT Press and ACM, which this site does not have. The run exits 3, because the first thing that actually failed was a work that is not published here.

## `spr get`

```
spr get <url or path>... [flags]
```

The client with nothing on top of it. Follows the redirect chain, counts the hops, classifies the response on its content, and prints what it found without parsing a single field. Use it when a later command says something surprising and the question is whether the page or the parser changed.

| Flag | Meaning |
|---|---|
| `--body` | Write the response body to stdout instead of the summary |
| `--kind` | Kind expected: `any`, `html`, `pdf` or `xml`. Deciding this up front is what makes `wrong_kind` possible. |
| `--yes` | Read more than 20 urls without being billed first |

```bash
spr get /article/10.1007/s10994-021-05946-3
spr get --body /journal/10994 > journal.html
spr get -o json '/search.rss?query=uncertainty'
```

A bare path is resolved against `https://link.springer.com`. A full url is fetched as given, including urls on other hosts.

## `spr search`

```
spr search [terms] [flags]
```

Runs one query against the two surfaces Springer serves it on and returns one merged answer.

`/search.rss` is the primary path. It pages to the end of the result set, carries the full abstract on every item rather than a truncated card, states the bare DOI in `guid`, and it kept answering while the HTML surface was serving challenges.

`/search` HTML is the enrichment pass. It is the only source of the total, the facet counts, and the per result content type, container and author list. One page of it is fetched for those, and `--enrich` fetches enough pages to cover the whole result set.

The two paths do not agree on what the first twenty results are. Fetched for the same query in the same minute, HTML page 1 and RSS page 1 share 3 of 20, because the HTML honours `sortBy` and the feed ignores it and always answers newest first. So the join is on DOI and never on position, every result says which path it came from, and a result built from both says `rss+html`.

| Flag | Meaning |
|---|---|
| `--type` | Content type, repeatable: `article`, `research`, `review`, `news`, `book`, `chapter`, `conference paper` |
| `--open-access` | Open access only |
| `--from`, `--to` | Earliest and latest publication year |
| `--last` | Relative window instead of a year range: `m3`, `m6`, `m12` or `m24` |
| `--language` | Language code, repeatable: `En`, `De` |
| `--taxonomy` | Taxonomy term, repeatable, quoted for you |
| `--discipline` | Discipline, repeatable, quoted for you |
| `--sub-discipline` | Sub-discipline, repeatable, quoted for you |
| `--sdg` | Sustainable development goal, repeatable, quoted for you |
| `--sort` | `relevance`, `date` or `oldest`, honoured by the HTML path only |
| `--title`, `--contributor`, `--journal` | The field scoped inputs from advanced search |
| `--limit` | How many results to return, 20 per page and fixed by the site |
| `--page` | First page of results |
| `--path` | Force one surface: `rss` or `html`, default is both |
| `--enrich` | Fetch the HTML card fields for every result rather than the first page |
| `--facets` | Print the facet groups and counts instead of the results, one request |
| `--abstract` | Print each result's abstract, which the feed carries in full |
| `--also` | Widen the search with an open index, repeatable: `crossref`, `openalex` |
| `--dry-run` | Print what this query would cost and make no requests |
| `--envelope` | Print the whole envelope: every field, its source, what was missed and what was left unread |

```bash
spr search "aleatoric uncertainty"
spr search "aleatoric uncertainty" --type article --from 2020 --to 2024
spr search "uncertainty" --journal "Machine Learning" --open-access
spr search "graph neural network" --taxonomy "Machine Learning" --sort date --limit 500
spr search --title "uncertainty" --contributor "Hüllermeier"
spr search "climate" --sdg "Climate action" --facets
spr search "aleatoric uncertainty" --also crossref --also openalex
spr search "uncertainty" --limit 500 --dry-run
```

The taxonomy, discipline, sub-discipline and SDG values have to reach the site wrapped in double quotes. Sending them bare is a valid request that answers 200 and matches nothing, so the quotes are added for you and `--taxonomy "Machine Learning"` is what you type.

`--facets` is one request and prints the shape of the result set before you decide to fetch it:

```
$ spr search "aleatoric uncertainty" --type article --from 2020 --to 2024 --facets
557 results

content-type                  Article 557, Research article 482, Review
                              article 57, News article 1

publishing-model              Open access 291

language                      English 555, German 2

taxonomy                      Machine learning 168, Artificial intelligence
                              75, Statistical learning 67, ...
```

`--also` asks an open index the same question and merges its answer into the result set. A search of link.springer.com returns what Springer publishes, and the difference between that and what everybody publishes is a fact about the query that neither set shows on its own:

```
$ spr search "aleatoric uncertainty" --also crossref --also openalex
search: crossref matched 213566 and returned 20, 4 already in the Springer results and 16 new
search: openalex matched 21402 and returned 25, 6 already in the Springer results and 19 new
557 results
...
```

The counts each backend reported go to stderr, and into `notes` in the JSON, where they cannot be mistaken for part of the result set. The join is on the normalized DOI and on nothing else, because titles vary in punctuation and case between the three sources and positions are meaningless across corpora that were sorted differently. A result with no DOI joins to nothing and stays where it is. Every result says which backends answered for it, so one the site and both indexes returned reads `rss+html+crossref+openalex`.

A backend fills only one field, the abstract, and only when the site left it empty, and the envelope records that it did. Overwriting the publisher's own statement with a derived index's version of it would make the record harder to trust rather than fuller.

`--type` is not sent to the backends. Springer's content types are its own words, Crossref and OpenAlex each have a different vocabulary for the same distinction, and no mapping between them was measured. The filter is dropped and stderr says so rather than guessing quietly. `--also` with `--facets` is an error, because facets count the Springer result set and the backends count their own.

`--dry-run` bills the query first. Both search paths share one five second pace bucket, so 26 requests is over two minutes of waiting and it is worth knowing that up front:

```
$ spr search "uncertainty" --limit 500 --dry-run
query         uncertainty
path          /search.rss, 20 per page
requests      25 rss pages + 1 html page for facets and the total
pace          1 request / 5s, which both search paths share
estimate      2 minutes
```

A run over five requests prints the same bill on stderr and then proceeds. It is not a prompt, because prompting breaks every pipeline this tool is meant to sit in.

`--also` adds a line to the bill and a request to the count, and it does not change the estimate. The backends are separate hosts with their own pace buckets, so their requests do not queue behind the search surface's five second one:

```
$ spr search "uncertainty" --also crossref --also openalex --dry-run
query         uncertainty
path          /search.rss, 20 per page
requests      1 rss page + 1 html page for facets and the total + 1 crossref page + 1 openalex page
pace          1 request / 5s, which both search paths share
estimate      5 seconds
also          crossref and openalex, on their own hosts and their own pace, merged on doi
```

When the HTML pass is challenged, the search completes on RSS alone and stderr says so in the same breath rather than leaving a caller to notice that the facets are missing. A query that matched nothing exits 3, so that no results and a failed run are two different things to a script without either being parsed out of the output.

## `spr work`

```
spr work <doi, url or path>... [flags]
```

Reads a single work page and prints the record it produced, along with the envelope that says where each field came from. The four work types share one record: article, chapter, protocol and reference work entry.

A DOI is enough. The registrant prefix says who issued it and nothing about what it is, so the suffix is used to order the paths it could live under and they are tried until one answers, which costs one request per miss and usually costs none.

| Flag | Meaning |
|---|---|
| `--text` | Print the body text of each section as well as the tree |
| `--envelope` | Print the whole envelope: every field, its source, what was missed and what was left unread |
| `--yes` | Read more than 20 works without being billed first |

```bash
spr work 10.1007/s10994-021-05946-3
spr work --envelope /chapter/10.1007/978-3-030-58607-2_1
spr work -o json 10.1007/s10994-021-05946-3 | jq .references
```

A restricted page is read rather than refused. Everything except the body is in the head of a paywalled page, so the record is printed, `body` is named in the envelope with the page's own sentence for why, and the exit code is 4. A container page is not a work, so `spr work /journal/10994` says so and exits 1 rather than printing an empty record.

## `spr journal`

```
spr journal <id, issn, url or path>... [flags]
```

Reads a journal home page. The Springer id, either ISSN, a path or a full url all work, and an id or an ISSN is turned into `/journal/<value>` as given rather than converted between forms.

| Flag | Meaning |
|---|---|
| `--volumes` | Make the second request for the volumes and issues page and print the whole run |
| `--envelope` | Print the whole envelope: every field, its source, what was missed and what was left unread |
| `--yes` | Read more than 20 journals without being billed first |

```bash
spr journal 10994
spr journal 0885-6125
spr journal 10994 --volumes
spr journal -o json 10994 | jq '.metrics[]'
```

Both ISSNs are kept apart, electronic and print, since a citation that gives one and a record that only knows the other do not match and should. Editors keep their role. A metric is only emitted with the year it was measured in, and a yearless one is named in `missed` with the page's own text for it.

Without `--volumes` the last line is a pointer that says `0 held` and where the rest are, which is a different fact from an empty list. With it, the pointer says `348 of 348 held` and the run is printed underneath.

## `spr book`

```
spr book <doi, isbn, url or path>... [flags]
```

Reads a book, proceedings volume or reference work. A book is addressable by DOI and by ISBN and both are used as given, since the site resolves both to the same page.

| Flag | Meaning |
|---|---|
| `--chapters` | Print the table of contents, front and back matter included |
| `--envelope` | Print the whole envelope |
| `--yes` | Read more than 20 books without being billed first |

```bash
spr book 10.1007/978-3-031-28170-9
spr book 978-3-031-28170-9
spr book --chapters 10.1007/978-3-031-28170-9
spr book -o json 978-3-031-28170-9 | jq '.offers[]'
```

The four ISBNs are four fields, not one: electronic, hardcover, softcover and print. They are genuinely different strings for genuinely different editions, and the DOI resolves to the electronic one, which is not the one the page prices most prominently.

The three publication dates are three fields for the same reason. Prices carry the printed string beside the parsed amount, because they are localized by requesting IP, and the kind of each comes from the order form's own field rather than from its printed label.

A book behind a subscription is read rather than refused, and exits 4.

## `spr series`

```
spr series <id, url or path>... [flags]
```

Reads a book series home page. A path with anything after the series id is a subpage rather than the home page and is refused as such.

| Flag | Meaning |
|---|---|
| `--envelope` | Print the whole envelope |
| `--yes` | Read more than 20 series without being billed first |

```bash
spr series 558
spr series --envelope /series/558
spr series -o json 558 | jq '.latest_titles[]'
```

The books listed are the five the page shows out of many thousands, so the field is `latest_titles` and the pointer under it says how to reach the rest. Each card credits either authors or editors and the two are kept apart, read off the card's printed label rather than off its `itemprop`, which says `editor` on both.

## `spr metrics`

```
spr metrics <doi, url or path>... [flags]
```

Reads a work's `/metrics` subpage. A bare DOI goes straight to `/article/<doi>/metrics` rather than through the path search `spr work` does, because this subpage exists for articles: a chapter's `/metrics` answers 404, and one request that says so beats four that say the same thing.

| Flag | Meaning |
|---|---|
| `--envelope` | Print the whole envelope |
| `--yes` | Read more than 20 works without being billed first |

```bash
spr metrics 10.1007/s10994-021-05946-3
spr metrics -o json 10.1007/s10994-021-05946-3 | jq .altmetric.cohorts
spr metrics -o json 10.1007/s10994-021-05946-3 | jq -r '.citations | "\(.count) per \(.source)"'
```

```console
$ spr metrics 10.1007/s10994-021-05946-3
title         Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods
doi           10.1007/s10994-021-05946-3
article       https://link.springer.com/article/10.1007/s10994-021-05946-3
updated       2026-08-18 10:34 UTC
accesses      134k, about 134,000, which the page calls an approximate count
citations     1,906 per Dimensions
attention     52
details       https://link.altmetric.com/details/69076743
  twitter     20 tweeters
  blogs       3 blogs
  news        2 news outlets
  reddit      2 Redditors
  mendeley    1,307 Mendeley
  ranked      22,032nd of 474,090 tracked articles in all journals
              95th percentile
  ranked      1st of 29 tracked articles in Machine Learning
              96th percentile

mentions (5, the named coverage only)
  Medium US
    The importance of uncertainty model estimation in Artificial Intelligence
    for business.
    https://medium.com/@German_Alfaro/the-importance-of-uncertainty-model-estimation-in-artificial-intelligence-for-business-6fb305941327
...
```

Three things in that output are decisions rather than formatting.

The citation count carries its counter. `1,906 per Dimensions` is read off the page's own sentence, and if that sentence changes the record says the source is missing rather than letting the number pass as Springer's own. Crossref reports 1,553 for the same DOI and OpenAlex 1,563, and all three are correct about three different corpora.

The rank comes with two cohorts and not one. The wide one compares this article to every tracked article of a similar age, the narrow one to the tracked articles of a similar age in its own journal, and on this page those are 474,090 articles and 29. Quoting `96th percentile` without the 29 behind it is how a percentile lies, so the size is printed on the same line as the rank.

`mentions` is the named coverage and nothing else. Five cards here against a breakdown counting 1,334 pieces of attention: the 20 tweeters, the 2 Redditors and the 1,307 Mendeley readers are counted and never named. They are separate fields because reading `len(mentions)` as the total is wrong by three orders of magnitude.

The counts are printed as the page prints them. `1 tweeters` is Springer's own text on an article with a single tweet, and correcting the grammar would mean this tool editing the publisher.

## `spr figures`

```
spr figures <doi, url or path> [number] [flags]
```

With no number, lists what the article page already holds: every figure's label, its caption and the address of its own page. That is the same single request `spr work` makes.

With a number, fetches that figure's page. The only thing it buys you is the asset. The article page serves the image at 685 pixels wide and the figure page serves the same asset at 1,177, under a path that differs by one segment, and the path is read rather than built because guessing a CDN's naming scheme works right up until it does not.

| Flag | Meaning |
|---|---|
| `--envelope` | Print the whole envelope |

```bash
spr figures 10.1007/s10994-021-05946-3
spr figures 10.1007/s10994-021-05946-3 1
spr figures -o json 10.1007/s10994-021-05946-3 1 | jq .image
```

```console
$ spr figures 10.1007/s10994-021-05946-3 1
label         Fig. 1
in            Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods
article       https://link.springer.com/article/10.1007/s10994-021-05946-3#Fig1
image         https://media.springernature.com/full/springer-static/image/art%3A10.1007%2Fs10994-021-05946-3/MediaObjects/10994_2021_5946_Fig1_HTML.jpg
size          1177 by 420
webp          https://media.springernature.com/full/springer-static/image/art%3A10.1007%2Fs10994-021-05946-3/MediaObjects/10994_2021_5946_Fig1_HTML.jpg?as=webp
alt           Fig. 1

caption
  Predictions by EfficientNet (Tan and Le 2019) on test images from ImageNet:
  For the left image, the neural network predicts “typewriter keyboard”
  with certainty 83.14 %, for the right image “stone wall” with certainty
  87.63 %

cited in the caption (1)
  Tan, M., & Le, Q. (2019). EfficientNet: Rethinking model scaling for
  convolutional neural networks. In Proceedings of ICML, 36th international
  conference on machine learning, Long Beach, California.
  https://link.springer.com/article/10.1007/s10994-021-05946-3#ref-CR105
```

The caption cites a work, the printed link text is only the year, and the whole reference string sits in the anchor's `title` attribute. Both are kept, so a caption citation is resolvable without a second pass over the reference list.

A number past the end is not a 404. On an article with 17 figures, `/figures/99` answers 200 with 224 KB of page furniture and an empty body, which means the fetch looks healthy and the page is empty. This command says there is no such figure rather than printing a record with nothing in it.

## `spr tables`

```
spr tables <doi, url or path> [number] [flags]
```

With no number, lists the label, caption and link per table, which is genuinely everything the article page knows.

With a number, fetches the table. This is the one subpage in this tool that is not an optimization. The open access capture is 718 KB of HTML, it announces one table, and it contains zero `<table>` elements. The rows are published on `/tables/N` and nowhere else, so a pipeline that reads the article page and expects to find tabular data finds none of it and no error either.

| Flag | Meaning |
|---|---|
| `--envelope` | Print the whole envelope |

```bash
spr tables 10.1007/s10994-021-05946-3
spr tables 10.1007/s10994-021-05946-3 1
spr tables -o json 10.1007/s10994-021-05946-3 1 | jq -r '.rows[] | @tsv'
```

```console
$ spr tables 10.1007/s10994-021-05946-3 1
label         Table 1
in            Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods
article       https://link.springer.com/article/10.1007/s10994-021-05946-3#Tab1

caption
  Notation used throughout the paper

14 rows, 2 columns
Notation	Meaning
\(P\), \(p\)	Probability measure, density or mass function
\({\mathcal{X}}\), \({{\varvec{x}}}\), \({{\varvec{x}}}_i\)	Instance space, instance
...
```

Rows go out tab separated rather than aligned into columns. A cell here holds LaTeX and can be far wider than a terminal, so aligning would either wrap the table into nonsense or truncate the data, and tabs are what the next program in the pipe wants anyway.

Cells keep the LaTeX the publisher wrote. Rendering `\({\mathcal{X}}\)` into anything else would be this tool having an opinion about notation, and anyone who wants it rendered has a renderer.

The heading on this page is one string with the label and the caption run together, `Table 1 Notation used throughout the paper`, where a figure gives them two separate elements. They are split on the label so that a caption is a caption on both records.

## `spr sitemap`

```
spr sitemap [flags]
```

Reads the maps Springer publishes about itself, which are the only complete enumeration of what is on the site. There are twelve of them: one index of 10,408 dated shards, and eight static maps that between them name every journal, series and collection.

With no flags it fetches the index and says what is in it, which is one request. `--static` fetches one of the eight. `--list` prints the child sitemap urls. `--since`, `--until` and `--kind` walk the shards and print the urls in them, one per line.

| Flag | Meaning |
|---|---|
| `--static` | One of the static maps: `journals`, `series`, `collections`, `brands`, `shops`, `subjects` |
| `--list` | Print the url of every child sitemap rather than what is in them |
| `--kind` | Keep only these kinds, repeatable: `article`, `chapter`, `protocol`, `entry`, `referencework`, `book`, `journal`, `series`, `collection`, `brand`, `partner`, `shop` |
| `--since`, `--until` | The bucket window to read: `2026`, `2026-08` or `2026-08-01` |
| `--all` | Walk every shard in the index, which needs `--yes` |
| `--yes` | Proceed with a walk that was billed rather than stopping |
| `--limit` | Stop after this many urls |
| `--resume` | Skip the shards an earlier run of this same selection finished |

```bash
spr sitemap
spr sitemap --list
spr sitemap --static journals
spr sitemap --kind article --since 2026-08-01
spr sitemap --kind book --since 2026-01-01 | spr work --yes
spr sitemap --all --yes --resume > urls.txt
```

```console
$ spr sitemap
index         https://link.springer.com/sitemap-index.xml
children      10,408 child sitemaps
buckets       8,106 named for a day, 2,252 for a month, 50 for a year
span          1850 to 2026-08-18
lastmod       on the 8,106 day shards only, where it restates the bucket
bucket        where a record is filed, and not when it was published
static        journals, series, collections, brands, shops, subjects, read with --static
full walk     10,408 requests at 2s, 5 hours 47 minutes, bounded above by 52,040,000 urls and 5.5 GB
```

The date in a shard's file name is a bucket and not a publication date. The index holds 66 shards named `sitemap_2020-01-01_N`, roughly 330,000 works, because the first of January is where everything known only to its year is filed, and the entries inside the first of them carry 173 distinct `lastmod` values of which none is 2020-01-01. So `--since` and `--until` say which shards to read rather than what was published when, and nothing in this tool turns a bucket into a date.

Urls go to stdout one per line and everything else goes to stderr, which is what makes `spr sitemap --kind book --since 2026 | spr work --yes` a pipeline. With `-o json` the walk emits one object per line rather than an array, because the stream has no end to close an array with.

The bill is computed from the index that was just fetched rather than from a figure compiled in, since the index grows daily. Above ten shards it is printed and the walk proceeds. Above a hundred, and always for `--all`, the walk stops until you pass `--yes`.

`--resume` writes each shard's url to a state file under the cache directory as that shard finishes, keyed on the selection, so resuming a walk of the last three days never inherits the state of a walk of everything. A shard is marked only after every url in it has been printed, and a shard that did not answer is left unmarked and counted, so a resumed run comes back for it. `--resume` with `--no-cache` is a usage error rather than a run that quietly fails to resume.

See the [enumerating the site](/guides/sitemaps/) guide for what the eight static maps hold and what a walk of everything costs.

## `spr graph`

```
spr graph <seed>... [flags]
```

Reads a seed and everything it leads to, and writes the result as nodes and edges rather than as records. A seed is a DOI, a journal number, a path or a url, and more than one may be given.

`--depth 0` reads the seed and stops, which is the default. Each further round follows the edges that lead to a page this tool can read: a work names its container, a journal leads to its volumes and issues page, a book leads to the works in its table of contents, and a series leads to the books it lists. A reference is followed only under `--follow-refs` and only when its DOI carries the `10.1007` prefix, because the others have no page at this publisher.

| Flag | Meaning |
|---|---|
| `--depth` | How many rounds of following to do, and 0 reads the seed and stops |
| `--format` | `json`, `jsonl`, `nt`, `nq`, `ttl`, `jsonld`, `graphml`, `gexf`, `dot`, `csv` |
| `--projection` | `coauthor`, which the gexf writer computes, so it needs `--format gexf` |
| `--dir` | Where the csv pair is written, which needs `--format csv` |
| `--also` | Ask the open indexes about every work read: `crossref`, `openalex` |
| `--cited-by` | Add this many citing works per work, which reads OpenAlex |
| `--include-rails` | Turn the publisher's recommendation strip into `recommends` edges |
| `--follow-refs` | Follow the references that carry this publisher's DOI prefix |
| `--merge-names` | Merge a name keyed person into an ORCID keyed one on an exact name match |
| `--limit` | Stop after this many pages |
| `--dry-run` | Print what this walk would cost and make no requests |
| `--yes` | Proceed with a walk that was billed rather than stopping |
| `--resume` | Skip the pages an earlier run of this same walk finished |
| `--merge` | Merge these graph files into the result, repeatable |

```bash
spr graph 10.1007/s10994-021-05946-3
spr graph 10.1007/s10994-021-05946-3 --also crossref,openalex --format ttl
spr graph 10994 --depth 1 --dry-run
spr graph 10.1007/978-3-030-58607-2 --depth 1 --yes --format graphml > book.graphml
spr graph 10.1007/s10994-021-05946-3 --format gexf --projection coauthor
spr graph --merge one.json --merge two.json > both.json
```

Above twenty requests the bill is printed and nothing is fetched until you pass `--yes`. `--dry-run` prints the same bill and always stops.

```console
$ spr graph 10.1007/s10994-021-05946-3 --depth 1 --dry-run
seed      https://link.springer.com/article/10.1007/s10994-021-05946-3
depth 0   work page, the seed
depth 1   1 journal page
requests  2
pace      1 request / 2s, floor 1 request / 1s
estimate  2 seconds
tier      html only, so no ror, funder or citedBy edges
format    json
```

The bill is per depth because the shape is the point. One work and its journal is two requests, and one proceedings volume at depth 1 is forty. A single total with no breakdown cannot tell those apart until the walk is already running.

The graph goes to stdout and everything else goes to stderr, including what was found:

```console
$ spr graph 10.1007/s10994-021-05946-3 --depth 1 > graph.json
graph: 20 nodes and 24 edges from 2 pages
graph: 1 issue, 1 volume, 1 work, 2 orgs, 2 publishers, 3 journals, 3 persons, 7 subjects
graph: 1 partOf, 1 inVolume, 1 inIssue, 2 authoredBy, 1 editedBy, 2 affiliatedWith, 10 hasSubject, 2 publishedBy, 4 sameAs
graph: 0 of 122 references resolved to an identifier and became edges, and 122 did not and stay in the record as text
```

That last line is the rule this command is built around. The page prints 122 references and none of them carries a DOI, so the html tier alone produces no `cites` edge at all. A reference that did not resolve to an identifier stays in the work record as the text it was and becomes nothing here, because an edge between a work and a guess is worse than no edge.

`--also crossref` is what turns those references into edges, from the deposit rather than from the rendering:

```console
$ spr graph 10.1007/s10994-021-05946-3 --also crossref,openalex
graph: 105 nodes and 107 edges from 1 page
graph: 1 funder, 2 issues, 2 persons, 2 volumes, 21 subjects, 3 journals, 3 publishers, 4 orgs, 67 works
graph: 2 partOf, 2 inVolume, 2 inIssue, 2 authoredBy, 4 affiliatedWith, 66 cites, 1 fundedBy, 21 hasSubject, 3 publishedBy, 4 sameAs
graph: 3 backend requests to crossref and openalex
graph: crossref: 66 of 122 deposited references carry a doi and became edges, and 56 do not and became nothing
```

Sixty six of the same 122 references carry a DOI in the deposit and fifty six still do not, so the count is reported both times rather than rounded up into a total.

### Identity

Every node's uri names the authority that identified it. A person known by an ORCID is `spr:person/orcid/0000-0002-9944-4108` and a person known only by the name a page printed is `spr:person/name/<hash>`, and those are two visibly different nodes rather than one node with a confidence score attached. Nothing in this tool merges them on its own.

`--merge-names` is the explicit opt in. It merges a name keyed person into an ORCID keyed one only when the normalized name matches exactly one ORCID node, refuses when two ORCIDs answer to the same name, and records `mergedFrom` on the surviving node so the guess is visible in the output rather than lost in it.

### Formats

Ten writers, one graph. `json` is the only one that round trips, because it is the only one that holds every field of every node and edge, and it is what `--merge` reads.

| Format | What it is for |
|---|---|
| `json` | The full graph, and the only form `--merge` can read back |
| `jsonl` | One node or edge per line, for streaming into something else |
| `nt`, `nq` | N-Triples and N-Quads, where the fourth term is the tier |
| `ttl` | Turtle, with the eleven prefixes declared once at the top |
| `jsonld` | JSON-LD with the context inline |
| `graphml`, `gexf` | Gephi and yEd, with `--projection coauthor` on the gexf writer |
| `dot` | Graphviz, for a graph small enough to look at |
| `csv` | The Neo4j import pair, written to `--dir` as `nodes.csv` and `edges.csv` |

The RDF forms borrow from schema.org, Dublin Core, PRISM, BIBO, FaBiO, CiTO and FRAPO, and mint exactly four terms of their own: `spr:recommends`, `spr:springerId`, `spr:accessStatement` and `spr:mergedFrom`. The four containment edges all map to `dcterms:isPartOf` and are told apart by the object's own `rdf:type`, and ordered authorship is a plain `rdf:List` under `bibo:authorList`, so neither of them needs a local term.

`--format nq` puts the tier in the fourth position, as `<https://springer-cli.tamnd.com/ns/tier/html>`, so a store can be asked what the html tier alone said. Dropping the fourth column leaves valid N-Triples, which is why the two forms have the same number of lines.

Two runs over the same pages write the same bytes. The writers sort first, GraphML and GEXF ids are derived from the uri rather than counted, and the blank node labels in the author lists come from the work uri, so a graph in version control has a diff worth reading.

### Resuming and merging

`--resume` writes each page's url to a state file under the cache directory as that page finishes, keyed on the seeds, the format and the depth, so a resumed walk of one book never inherits the state of a walk of a journal. A page that did not answer is left unmarked and counted, so a resumed run comes back for it. `--resume` with `--no-cache` is a usage error rather than a run that quietly fails to resume.

`--merge` reads graphs an earlier run wrote and unions them into this one. Merging is on the uri, so the same node from two files is one node, and a merge of a file into itself changes nothing. A walk that fails halfway still writes what it read before the error goes out, because throwing away an hour of reading over one page that timed out would be the wrong way round.

See the [building a graph](/guides/graphs/) guide for the node and edge tables and for what each tier adds.

## `spr crossref`

```
spr crossref [doi...] [flags]
```

Reads the DOI registration agency, which holds what the publisher deposited rather than what the site renders. With a DOI it reads one record. With `--query`, `--title`, `--author` or any filter it searches.

Crossref answers what link.springer.com does not: the deposited abstract in full, the funder list with award numbers, every ORCID with whether the person authenticated it, the licence terms, and the reference list as identifiers rather than as rendered text.

| Flag | Meaning |
|---|---|
| `--query` | Free text across title, container, author and year |
| `--title` | Title contains |
| `--author` | Author name contains |
| `--issn` | Only this journal, by either of its ISSNs |
| `--isbn` | Only this book |
| `--type` | Crossref work type: `journal-article`, `book-chapter`, `proceedings-article` |
| `--funder` | Funder registry id, with or without the resolver prefix |
| `--from`, `--to` | Date range, as a year, a year and month, or a date |
| `--rows` | Results per page, capped by Crossref at 1000 |
| `--cursor` | Deep paging token, `*` for the first page of one |
| `--facet` | Count by group instead of listing, repeatable: `type-name:5` |
| `--sort`, `--order` | `relevance`, `published` or `is-referenced-by-count`, and `asc` or `desc` |
| `--references` | Print the deposited reference list rather than the record |
| `--envelope` | Print the whole envelope |
| `--yes` | Read more than 20 dois without being billed first |

```bash
spr crossref 10.1007/s10994-021-05946-3
spr crossref 10.1007/s10994-021-05946-3 --references
spr crossref --issn 0885-6125 --from 2024 --rows 100
spr crossref --funder 10.13039/501100001659 --type journal-article
spr crossref --query uncertainty --facet type-name:5
spr crossref 10.1007/s10994-021-05946-3 --references | spr crossref --yes
```

This is the one command with two modes and one empty argument list, so the rule is worth stating: DOIs given, or nothing to search with, means read records and take them off stdin when none were typed. Anything else is a search. A search with no DOI and nothing that narrows anything is a usage error. Sort, order, rows and the facet list are all flags, and none of them narrows a corpus of 170 million records into a request anybody meant to make.

`--references` prints the reference DOIs one per line on stdout and the count on stderr, so a pipe still gets a clean list of identifiers while a person watching learns that the list is partial:

```console
$ spr crossref 10.1007/s10994-021-05946-3 --references
10.1007/978-3-642-40994-3_29
10.1613/jair.4192
...
crossref: 66 of 122 deposited references carry a doi, and 56 do not
```

The counts print under their own heading and each one names who counted:

```
counts
  crossref_citations             1,553, deposited citations only
  crossref_references            122 deposited
  crossref_references_with_doi   66 of those resolve to something
```

That first number is what other Crossref members deposited as citing this work. It is not the site's citation count, it is not OpenAlex's, and no command in this tool prints a merged one. See [counts and assets](/guides/counts-and-assets/) for why three counts of the same thing is the correct answer rather than a problem to fix.

## `spr openalex`

```
spr openalex [doi or work id...] [flags]
```

Reads the open index that holds both citation directions, an abstract, an institution graph with ROR ids, and a field normalized impact figure. With a DOI or a `W` work id it reads one record. With `--query`, `--title`, `--author`, `--cites` or `--cited-by` it searches.

| Flag | Meaning |
|---|---|
| `--query` | Full text search, which OpenAlex scores |
| `--title` | Title contains |
| `--author` | Raw author name contains |
| `--issn` | Only this journal, by any of its ISSNs |
| `--from`, `--to` | Date range, as a full date |
| `--cites` | Only works citing this one, by DOI or work id |
| `--cited-by` | Only works this one cites, by DOI or work id |
| `--rows` | Results per page, capped by OpenAlex at 200 |
| `--page` | Which page of results |
| `--envelope` | Print the whole envelope |
| `--yes` | Read more than 20 works without being billed first |

```bash
spr openalex 10.1007/s10994-021-05946-3
spr openalex W3014596384
spr openalex --query "aleatoric uncertainty" --from 2020-01-01 --rows 50
spr openalex --issn 0885-6125 --from 2024-01-01
spr openalex --cited-by W3014596384
spr openalex 10.1007/s10994-021-05946-3 -o json | jq -r '.authors[].institutions[].ror'
spr cited-by W3014596384 -o json | jq -r '.works[].id' | spr openalex --yes
```

Identifiers given, or nothing to search with, means read records and take them off stdin when none were typed, the same rule `spr crossref` follows.

The abstract arrives as an inverted index, a map of word to the positions it appears at, and is put back in reading order before it is printed. Institutions carry ROR ids and country codes, which is the only place in this tool where an affiliation is an identifier rather than a string. Both `concepts` and `topics` are printed, because OpenAlex publishes both classifications and they disagree.

The stored citation count always prints with the date it was stored on:

```
counts
  openalex_citations             1,563, as stored on 2026-08-16T07:02:28.622633
  openalex_references            111 resolved to works in the index
  fwci                           113.99, against the average work of its field, year and type
```

That number is an aggregate rebuilt on its own schedule and the live listing counted 1,554 for the same work in the same minute. The date is what makes the difference explicable rather than alarming, which is why it is never printed without it.

## `spr cited-by`

```
spr cited-by <doi or work id>... [flags]
```

The direction link.springer.com has no page for. A work page lists what a work cites. Nothing on the site lists what cites it, and the metrics page states a total attributed to Dimensions without naming a single citing work.

| Flag | Meaning |
|---|---|
| `--by-year` | Counts grouped by publication year, one request instead of a full listing |
| `--limit` | How many citing works to list, `0` for every one of them at 200 per request |
| `--yes` | Read more than 20 works without being billed first |

```bash
spr cited-by 10.1007/s10994-021-05946-3
spr cited-by 10.1007/s10994-021-05946-3 --by-year
spr cited-by W3014596384 --limit 0
spr cited-by 10.1007/s10994-021-05946-3 -o json | jq -r '.works[].doi' | spr work --yes
```

The total it prints is the count of this listing and not the record's stored `cited_by_count`. The two were 1,554 and 1,563 for the same work in the same minute, because one is the live index and the other is an aggregate. Both are OpenAlex's and the output says which one it is holding.

A DOI costs one extra request, because the citation listing is keyed on the work id and the DOI has to be looked up first. A `W` id costs nothing. `--by-year` gets the whole history in one request rather than the eight a full listing of 1,554 works costs, which is the cheap way to ask when a work was read rather than by whom.

## `spr api`

```
spr api [terms] [flags]
```

The publisher's own API rather than the web site, and the only surface here that needs a credential.

| Flag | Meaning |
|---|---|
| `--endpoint` | Which endpoint: `meta/v2`, `metadata` or `openaccess` |
| `--doi`, `--issn`, `--isbn` | One work, one journal or one book |
| `--title`, `--keyword`, `--subject`, `--type` | The publisher's own query fields, quoted for you when they have a space |
| `--year`, `--from`, `--to` | Publication year, or a date range as `yyyy-mm-dd` |
| `--start` | First record, one based rather than zero based |
| `--rows` | Records per page |
| `--envelope` | Print the whole envelope |

```bash
spr api "aleatoric uncertainty"
spr api --doi 10.1007/s10994-021-05946-3
spr api --issn 0885-6125 --year 2021 --rows 50
spr api --endpoint openaccess --keyword "machine learning"
spr api -o json --doi 10.1007/s10994-021-05946-3 | jq .envelope.unread
```

The key is read from `SPRINGER_API_KEY` or from the config file, and it is never printed, never cached and never written to a record. It travels in the query string because that is the only way these endpoints accept one, and everything downstream of the request sees the url with the key blanked out. A missing key and a wrong key are both a 401 with an identical body, so the error says where to put one rather than guessing which of the two happened.

Every field this command decodes comes from the published schema and not from a measured response, because no key was available to measure one with. The envelope lists every top level key of the answer that the decoder did not read, so the first run with a real key reports the gap rather than hiding it.

## `spr extraction`

```
spr extraction [field] [--rung <name>] [--record <name>]
```

Prints the table this tool extracts by. Every field has a row, and the row says which record it belongs to, which rung is expected to answer it, which exact tag or region that is, and why it sits on that rung rather than a lower one. Naming one field prints the long form for that field alone.

```bash
spr extraction
spr extraction authors
spr extraction journal.title
spr extraction --record book
spr extraction --rung selector
spr extraction -o json | jq '.[] | select(.rung == 4)'
```

The same field name means different things on different pages, so rows are qualified as `record.field`. `title` is answered from rung 1 on a work and rung 3 on a journal, and asking by the bare name prints both rather than picking one and being wrong about the other. `--record` and a name together mean both, not either.

Every container row is rung 3 or rung 4. A work page carries Highwire meta tags because Google Scholar reads them, and a journal, book or series home page carries no bibliographic vocabulary at all, so their fields come from Springer's own `data-test` region names or from a css class and a printed English label.

The rung names are the ones a record's `via` field uses, so a name copied out of a record pastes straight back in here.

## `spr verify`

```
spr verify [--live] [--vocab] [--capture <name>]
```

Reads the fourteen pages the capture ledger was measured from and says whether they still read the same. The ledger is a file in the repository that records, for each of those pages, how many meta tags and JSON-LD blocks it carried, which vocabularies it declared, whether its two access statements agreed, which fields came out set, which were looked for and not found, and how many `data-test` regions were left unread. This command produces that reading again and compares it.

| Flag | What it does |
| --- | --- |
| `--live` | Refetch every page instead of reading the page cache |
| `--vocab` | Print what each vocabulary claims about the facts more than one of them states |
| `--capture` | Only these captures, by name, repeatable |

```bash
spr verify
spr verify --live
spr verify --capture article_oa --capture journal
spr verify --vocab --capture article_oa
spr verify --live -o json | jq '.[] | select(.verdict != "ok")'
```

```
$ spr verify --capture article_oa --capture journal --capture search
source     the page cache at /Users/apple/Library/Caches/spr
ledger     14 captures recorded in the ledger this binary was built with

ok          article_oa.html
ok          journal.html
ok          search.html

3 ok
```

### Which pages it read

The first line of every run says whether the reading came out of the page cache or off the live site, and every finding under it repeats the same thing. That is not padding. A cached page that has gone stale and a page that genuinely changed produce identical findings, and the only way to tell them apart is to know which one was read, at the moment you are reading the finding rather than by scrolling back up.

The default reads the cache and makes no request at all. It also does not fall back: a page that is not in the cache is reported as unread rather than quietly fetched, because a run that says cache in its header and then fetches is a run whose header is a lie. `--no-cache` on its own is refused for the same reason, since it turns off the only thing a default run reads.

`--live` refetches every page and writes what it read back into the cache, so the run after it compares against the bytes this one saw.

### Verdicts

| Verdict | What it means | Exit code |
| --- | --- | --- |
| `ok` | The reading matches the ledger | 0 |
| `drift` | The count of unread regions moved and nothing else did | 0 |
| `improvement` | A field came out set that was not set before | 7 |
| `changed` | A vocabulary appeared or disappeared, or the two access statements stopped agreeing | 7 |
| `regression` | A field that used to come out set no longer does | 7 |
| `unread` | The page could not be read at all | 3 if nothing was readable |

Three of those are worth explaining. An improvement fails, because a field arriving is a change to what this tool claims and somebody should record it deliberately rather than find it in a diff six weeks later. Drift never fails, because Springer shipping a component that this tool does not read is news about the site and not a bug here. And a regression is the only verdict that is definitely this tool's fault.

Exit code 7 is `verify` and nothing else. It exists so that a scheduled job can alert on the site moving without having to tell that apart from a mistyped flag.

### `--vocab`

The other half of the same question. A work page states its title in Highwire and in Dublin Core, its DOI in Highwire and in PRISM, and whether it is free to read in a Highwire `access` tag and again in the JSON-LD `isAccessibleForFree`. Eleven bibliographic facts are stated by more than one vocabulary, the access statement makes a twelfth, and `--vocab` prints what each vocabulary said about each of them.

```
$ spr verify --vocab --capture article_oa
source     the page cache at /Users/apple/Library/Caches/spr

article_oa.html
  agree     access       highwire:access="Yes"  jsonld:isAccessibleForFree="true"
  agree     copyright    dc:dc.copyright="2021 The Author(s)"  prism:prism.copyright="2021 The Author(s)"
  agree     doi          highwire:citation_doi="10.1007/s10994-021-05946-3"  prism:prism.doi="doi:10.1007/s10994-021-05946-3"
  agree     first_page   highwire:citation_firstpage="457"  prism:prism.startingPage="457"
  agree     issn         highwire:citation_issn="1573-0565"  prism:prism.issn="1573-0565"
  agree     issue        highwire:citation_issue="3"  prism:prism.number="3"
  agree     journal      highwire:citation_journal_title="Machine Learning"  prism:prism.publicationName="Machine Learning"
  agree     language     dc:dc.language="En"  highwire:citation_language="en"
  agree     last_page    highwire:citation_lastpage="506"  prism:prism.endingPage="506"
  agree     rights_agent dc:dc.rightsAgent="journalpermissions@springernature.com"  prism:prism.rightsAgent="journalpermissions@springernature.com"
  agree     title        dc:dc.title="Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods"  highwire:citation_title="Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods"
  agree     volume       highwire:citation_volume="110"  prism:prism.volume="110"

12 facts across 1 page, and every one of them agrees
```

Across all fourteen pages that is 75 facts and every one of them agrees, which is the finding. The agreements are printed rather than only the disagreements, because a report that prints nothing looks the same whether every vocabulary agreed or the check never ran.

Two of those rows are comparisons and not string equality. `doi` agrees even though PRISM writes `doi:10.1007/...` and Highwire writes `10.1007/...`, and `language` agrees across `En` and `en`. The normalisation is per fact and it is in the source next to the fact it belongs to. A disagreement exits 7, because a page that contradicts itself is a page somebody has to read.

## `spr cache`

```
spr cache [--clear]
```

Prints the cache directory, how many responses are in it, how much room they take, and the time to live. `--clear` empties it.

## `spr version`

Prints the version, commit and build date stamped in at link time, plus the Go version, the platform, the user agent this build sends, and the pace settings it will hold to.
