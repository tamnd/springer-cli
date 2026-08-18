---
title: "Searching"
description: "Two surfaces answer the same query, they disagree, and the disagreement is the useful part."
weight: 5
---

Search is how you find works, and on this site it is two surfaces rather than one:

```bash
spr search "aleatoric uncertainty"
spr search "aleatoric uncertainty" --type article --from 2020 --to 2024 --facets
spr search "graph neural network" --taxonomy "Machine Learning" --limit 500
```

`/search` returns HTML with facet counts, a total and rich cards. `/search.rss` returns the same query as a feed. They are served by one query engine and they take the same parameters, so it would be reasonable to expect them to be two renderings of one answer. They are not, and every design decision in `spr search` comes from that.

## The feed is the primary path

Not the HTML. The feed, for four reasons that were each measured rather than assumed.

It pages to the end. `?page=28` on a 557 result query returns the last 17 items and `?page=29` returns none, so the whole result set is reachable.

It carries the full abstract. The HTML card carries a truncated description that ends in an ellipsis at about 180 characters. The feed's `description` holds the paper's abstract, in full, marked up.

It states the DOI plainly. `guid` is the bare DOI, `10.1007/s10994-021-05946-3`, with nothing to strip and no url to parse it out of.

It kept answering. The HTML search surface serves the edge's client challenge under volume. During the measurements behind this tool, `/search` started returning challenges and `/search.rss` continued to answer the same queries at the same pace.

So `spr search` runs the feed first, and reaches for the HTML for the things only the HTML has.

## The HTML pass is enrichment, and it is not optional

Three things exist on the HTML page and nowhere else.

The total. `Showing 1-20 of 557 results` is the only statement of how many results there are. The feed carries nothing like it, which is why a run that lost the HTML page reports the results it holds and no total at all rather than inferring one by paging to the end.

The facet counts. Eight groups, with a count beside every option.

The card fields. Content type, container, author list, the open access label and the entitlement wording are on the card and are not in the feed.

One HTML page is fetched for those, always, because they cost one request and come from nowhere else. `--enrich` fetches enough pages to carry the card fields across the whole result set.

## The two paths disagree, and that is a feature

HTML page 1 and RSS page 1, same query, same minute, share 3 of 20 results.

The HTML honours `sortBy` and the feed ignores it. Sent with `sortBy` absent, `sortBy=relevance` and `sortBy=newestFirst`, the feed returned the same twenty items in the same order all three times, first `guid` `10.1007/s00193-024-01208-y`, first `pubDate` 28 December 2024. The feed is always newest first.

Two consequences, and `spr` acts on both.

Enrichment joins on DOI and never on position. Merging by index would attach one paper's authors to a different paper, silently, on 17 results out of 20.

A `--sort` that the feed cannot honour is said out loud:

```
$ spr search "uncertainty" --sort relevance --limit 100
search: the feed ignores sortBy and always answers newest first, so --sort relevance applies to the html pass only
```

Every result carries `via`, so a merged set is never ambiguous. `rss` means the feed answered for it, `html` means the card did, and `rss+html` means both and that the record has the feed's abstract and the card's authors.

By default the results the HTML page holds and the feed's ordering does not are left out of a merged run, and the count is stated. They are page 1 of a different ordering, and keeping them would mean a 500 result run carrying the stragglers of the first twenty and none of the next twenty seven pages. `--enrich` reads the rest of the HTML, at which point they are the answer to the same question and they are kept.

## Facets first

`--facets` is one request and no results, which makes it the cheapest way to look at a query before running it:

```
$ spr search "aleatoric uncertainty" --type article --from 2020 --to 2024 --facets
557 results

content-type                  Article 557, Research article 482, Review
                              article 57, News article 1

publishing-model              Open access 291

language                      English 555, German 2

taxonomy                      Machine learning 168, Artificial intelligence
                              75, Statistical learning 67, Bayesian inference
                              64, ...

discipline                    Computer science 142, Engineering 103, Earth
                              sciences 75, ...
```

The date group is the one that behaves differently. It offers four relative windows and a custom range and prints no counts at all, so its options are listed as bare names, and the years the query was sent with come back under `applied`.

## The quoting that fails silently

Four of the facet parameters have to be sent as quoted strings, quotes included:

```
taxonomy="Machine Learning"
facet-discipline="Computer Science"
facet-sub-discipline="Artificial Intelligence"
sustainableDevelopmentGoal="Climate Action"
```

Sending them bare is a syntactically valid request. It answers 200, it returns a well formed page, and it matches nothing. A silent zero is the worst failure a search can have, because it looks exactly like a query with no results.

`spr` adds the quotes in one place, shared by both paths, so it can only be wrong in one place, and there is a test that reads the requirement off a captured page rather than a comment. You type `--taxonomy "Machine Learning"` and never see the detail.

The content type values have a related trap. Three of the sixteen post under a value that is not their printed label: `Research article` posts `Research`, `Review article` posts `Review`, and `News article` posts `News`. A tool that sent the label back would filter to nothing on those three and work perfectly on the other thirteen. `--type` accepts either spelling.

## What a long run costs

20 results per page, fixed by the site. `size`, `results`, `per-page` and `limit` were each tried and each returned 20 items, so there is no parameter to expose and `--limit 500` is 25 feed pages.

Both search paths share one pace bucket of five seconds, separate from the two second bucket everything else uses, because search is the surface that trips and a slow search should not slow down the work fetches after it. So the bill is worth seeing first:

```
$ spr search "uncertainty" --limit 500 --dry-run
query         uncertainty
path          /search.rss, 20 per page
requests      25 rss pages + 1 html page for facets and the total
pace          1 request / 5s, which both search paths share
estimate      2 minutes
```

Any run over five requests prints that same bill on stderr and then gets on with it. It is not a prompt.

## When the HTML is challenged

The search completes. The results are the feed's, they are correct, and what is missing is the total, the facets and the card fields:

```
$ spr search "uncertainty" --limit 200
search: html enrichment failed with "the search surface served a client challenge for the html pass at page 1", so the total and the facets are unavailable for this run
200 results, and no total, because that is stated on the html page only, via rss
```

That line is on stderr, once, in the same breath as the results. Somebody whose facets are missing has a right to know why without turning on a log level.

If both paths are challenged there is nothing to return and the command says so with exit code 2, which is its own code because a challenge is a rate rather than a refusal. Waiting is the fix.

## Three ways to get nothing

A query that matched nothing exits 3 and prints no results. A run that failed exits 2 or 5. Those are three different things and a pipeline can branch on them without reading a word of the output.

The feed has its own version of this distinction, and it is why paging terminates the way it does. Page 29 of a 557 result query returns 190 bytes with a literal `null` in the channel body. Page 200 returns 186 bytes with an empty channel. Both answer 200 OK, so neither the status nor the size says anything useful. A page with no items is the end of the result set, and a short page is the last one, which is how 27 pages of 20 plus one of 17 is known to be exactly the 557 the HTML claims.
