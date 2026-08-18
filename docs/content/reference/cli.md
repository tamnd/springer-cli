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
| `extraction` | Print the extraction table: every field, the rung that answers it, and why |
| `cache` | Show or clear the page cache |
| `version` | Print the version, commit and build date |

## `spr get`

```
spr get <url or path> [flags]
```

The client with nothing on top of it. Follows the redirect chain, counts the hops, classifies the response on its content, and prints what it found without parsing a single field. Use it when a later command says something surprising and the question is whether the page or the parser changed.

| Flag | Meaning |
|---|---|
| `--body` | Write the response body to stdout instead of the summary |
| `--kind` | Kind expected: `any`, `html`, `pdf` or `xml`. Deciding this up front is what makes `wrong_kind` possible. |

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
| `--dry-run` | Print what this query would cost and make no requests |
| `--envelope` | Print the whole envelope: every field, its source, what was missed and what was left unread |

```bash
spr search "aleatoric uncertainty"
spr search "aleatoric uncertainty" --type article --from 2020 --to 2024
spr search "uncertainty" --journal "Machine Learning" --open-access
spr search "graph neural network" --taxonomy "Machine Learning" --sort date --limit 500
spr search --title "uncertainty" --contributor "Hüllermeier"
spr search "climate" --sdg "Climate action" --facets
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

When the HTML pass is challenged, the search completes on RSS alone and stderr says so in the same breath rather than leaving a caller to notice that the facets are missing. A query that matched nothing exits 3, so that no results and a failed run are two different things to a script without either being parsed out of the output.

## `spr work`

```
spr work <doi, url or path> [flags]
```

Reads a single work page and prints the record it produced, along with the envelope that says where each field came from. The four work types share one record: article, chapter, protocol and reference work entry.

A DOI is enough. The registrant prefix says who issued it and nothing about what it is, so the suffix is used to order the paths it could live under and they are tried until one answers, which costs one request per miss and usually costs none.

| Flag | Meaning |
|---|---|
| `--text` | Print the body text of each section as well as the tree |
| `--envelope` | Print the whole envelope: every field, its source, what was missed and what was left unread |

```bash
spr work 10.1007/s10994-021-05946-3
spr work --envelope /chapter/10.1007/978-3-030-58607-2_1
spr work -o json 10.1007/s10994-021-05946-3 | jq .references
```

A restricted page is read rather than refused. Everything except the body is in the head of a paywalled page, so the record is printed, `body` is named in the envelope with the page's own sentence for why, and the exit code is 4. A container page is not a work, so `spr work /journal/10994` says so and exits 1 rather than printing an empty record.

## `spr journal`

```
spr journal <id, issn, url or path> [flags]
```

Reads a journal home page. The Springer id, either ISSN, a path or a full url all work, and an id or an ISSN is turned into `/journal/<value>` as given rather than converted between forms.

| Flag | Meaning |
|---|---|
| `--volumes` | Make the second request for the volumes and issues page and print the whole run |
| `--envelope` | Print the whole envelope: every field, its source, what was missed and what was left unread |

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
spr book <doi, isbn, url or path> [flags]
```

Reads a book, proceedings volume or reference work. A book is addressable by DOI and by ISBN and both are used as given, since the site resolves both to the same page.

| Flag | Meaning |
|---|---|
| `--chapters` | Print the table of contents, front and back matter included |
| `--envelope` | Print the whole envelope |

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
spr series <id, url or path> [flags]
```

Reads a book series home page. A path with anything after the series id is a subpage rather than the home page and is refused as such.

| Flag | Meaning |
|---|---|
| `--envelope` | Print the whole envelope |

```bash
spr series 558
spr series --envelope /series/558
spr series -o json 558 | jq '.latest_titles[]'
```

The books listed are the five the page shows out of many thousands, so the field is `latest_titles` and the pointer under it says how to reach the rest. Each card credits either authors or editors and the two are kept apart, read off the card's printed label rather than off its `itemprop`, which says `editor` on both.

## `spr metrics`

```
spr metrics <doi, url or path> [flags]
```

Reads a work's `/metrics` subpage. A bare DOI goes straight to `/article/<doi>/metrics` rather than through the path search `spr work` does, because this subpage exists for articles: a chapter's `/metrics` answers 404, and one request that says so beats four that say the same thing.

| Flag | Meaning |
|---|---|
| `--envelope` | Print the whole envelope |

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
spr sitemap --kind book --since 2026-01-01 | spr work
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

Urls go to stdout one per line and everything else goes to stderr, which is what makes `spr sitemap --kind book --since 2026 | spr work` a pipeline. With `-o json` the walk emits one object per line rather than an array, because the stream has no end to close an array with.

The bill is computed from the index that was just fetched rather than from a figure compiled in, since the index grows daily. Above ten shards it is printed and the walk proceeds. Above a hundred, and always for `--all`, the walk stops until you pass `--yes`.

`--resume` writes each shard's url to a state file under the cache directory as that shard finishes, keyed on the selection, so resuming a walk of the last three days never inherits the state of a walk of everything. A shard is marked only after every url in it has been printed, and a shard that did not answer is left unmarked and counted, so a resumed run comes back for it. `--resume` with `--no-cache` is a usage error rather than a run that quietly fails to resume.

See the [enumerating the site](/guides/sitemaps/) guide for what the eight static maps hold and what a walk of everything costs.

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

## `spr cache`

```
spr cache [--clear]
```

Prints the cache directory, how many responses are in it, how much room they take, and the time to live. `--clear` empties it.

## `spr version`

Prints the version, commit and build date stamped in at link time, plus the Go version, the platform, the user agent this build sends, and the pace settings it will hold to.
