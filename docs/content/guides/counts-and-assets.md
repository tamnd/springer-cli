---
title: "Counts and assets"
description: "The three pages that hang off a work: how often it was read, and the figures and tables it published."
weight: 30
---

A work has three subpages of its own, and each one exists for a different reason:

```bash
spr metrics 10.1007/s10994-021-05946-3     # how often it was read and cited
spr figures 10.1007/s10994-021-05946-3     # what it illustrates
spr tables  10.1007/s10994-021-05946-3 1   # the actual table
```

They are not a matched set. `/metrics` is the only place on this site that states a readership or a citation count. `/figures/N` gives you a larger copy of an image you already have. `/tables/N` gives you data that exists nowhere else on the site, which makes it the only one of the three you cannot skip.

## A count is a number and a counter

The whole reason `spr metrics` prints the way it does is one measurement. For DOI `10.1007/s10994-021-05946-3`:

| Who counted | Citations |
|---|---|
| Springer, on its own metrics page | 1,906 |
| Crossref | 1,553 |
| OpenAlex | 1,563 |

Three numbers, three corpora, and none of them is wrong. So the record never carries a bare integer:

```console
$ spr metrics 10.1007/s10994-021-05946-3
...
citations     1,906 per Dimensions
```

`Dimensions` is read off the page's own sentence, `Citation counts are provided by Dimensions and depend on their data availability`, and not compiled in. If Springer switches counter tomorrow the record says so. If the sentence disappears, the record says the source is missing rather than letting the number pass as Springer's own.

In JSON that is two fields and no ambiguity:

```console
$ spr metrics -o json 10.1007/s10994-021-05946-3 | jq .citations
{
  "count": 1906,
  "source": "Dimensions"
}
```

The accesses count gets the same treatment for a different reason. The page prints `134k`, which is a rounding, and it says as much in prose beside it:

```
accesses      134k, about 134,000, which the page calls an approximate count
```

`134k` is what was published, `134000` is this tool's reading of it, and the caveat is the page's, read rather than assumed. A second article on the same site prints `1069` with no abbreviation and the same caveat, so the caveat is not a side effect of rounding and the two facts are kept apart.

## A percentile without its cohort is not a fact

The attention score comes with a sentence that states two comparisons and looks like one:

> This article is in the 95th percentile (ranked 22,032nd) of the 474,090 tracked articles of a similar age in all journals and the 96th percentile (ranked 1st) of the 29 tracked articles of a similar age in Machine Learning.

96th percentile of 29 tracked articles is one article out of twenty-nine. 95th percentile of 474,090 is a different claim entirely, and the two are not comparable. The record keeps both, with the size that qualifies each:

```console
$ spr metrics -o json 10.1007/s10994-021-05946-3 | jq .altmetric.cohorts
[
  {
    "scope": "all journals",
    "percentile": 95,
    "rank": 22032,
    "size": 474090
  },
  {
    "scope": "Machine Learning",
    "percentile": 96,
    "rank": 1,
    "size": 29
  }
]
```

The text output prints the size on the same line as the rank for the same reason.

That sentence has no class, no `data-test` and no markup inside it. It is rung 4, read out of prose with a regular expression, and the extraction table says so:

```console
$ spr extraction --record metrics
RECORD   FIELD                RUNG      SOURCE                                                  WHY
metrics  doi                  highwire  citation_doi                                            the subpage carries the parent article's whole head, so the work counted is named at rung 1
...
metrics  altmetric_cohorts    selector  the donut caption's prose                               two comparisons in one sentence, all journals and this journal, and the sizes are what make them non comparable
```

## Counted and named are different lists

The attention breakdown counts five kinds of mention, and the page separately shows a card for each piece of coverage it can name:

```
  twitter     20 tweeters
  blogs       3 blogs
  news        2 news outlets
  reddit      2 Redditors
  mendeley    1,307 Mendeley

mentions (5, the named coverage only)
```

1,334 pieces of attention counted, five of them named. The 20 tweeters, the 2 Redditors and the 1,307 Mendeley readers are numbers with nobody behind them, on this page or anywhere reachable from it. `breakdown` and `mentions` are therefore separate fields, because a consumer reading `len(mentions)` as the total would be wrong by three orders of magnitude.

Two small things the page does that this tool passes through rather than tidying. An article with a single tweet still reads `1 tweeters`, because that is Springer's own string and correcting the grammar would mean editing the publisher. And a blog that posted twice appears twice in the cards under two urls, because the page shows five cards and deduplicating them would be this tool deciding what counts as one mention.

## `/metrics` is for articles

A chapter's `/metrics` answers 404 with 122 KB of it. So `spr metrics` with a bare DOI goes straight to `/article/<doi>/metrics` rather than trying the four work paths the way `spr work` does. One request that says no beats four that say the same thing.

## Figures: the same image, twice the size

`spr figures` with no number costs one request and answers from the article page, which already holds the labels, the captions and the links:

```console
$ spr figures 10.1007/s10994-021-05946-3
Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods

figures (17)

  Fig. 1
  Predictions by EfficientNet (Tan and Le 2019) on test images from
  ImageNet: For the left image, the neural network predicts “typewriter
  keyboard” with certainty 83.14 %, for the right image “stone wall”
  with certainty 87.63 %
  https://link.springer.com/article/10.1007/s10994-021-05946-3/figures/1
...

The images above are the inline rendition. Ask for a number to get the full one.
```

Adding a number costs a second request and buys exactly one thing:

| Page | Image path | Size |
|---|---|---|
| Article | `//media.springernature.com/lw685/...` | 685 by 244 |
| `/figures/1` | `//media.springernature.com/full/...` | 1177 by 420 |

One path segment apart. This tool reads the second one off the page rather than rewriting the first, because guessing a CDN's naming scheme works until the day it does not, and by then the guess is in somebody's archive.

The figure page also gives you the caption's own citations, which the article page renders as a bare year:

```console
$ spr figures -o json 10.1007/s10994-021-05946-3 1 | jq '.refs[0]'
{
  "text": "2019",
  "url": "https://link.springer.com/article/10.1007/s10994-021-05946-3#ref-CR105",
  "citation": "Tan, M., & Le, Q. (2019). EfficientNet: Rethinking model scaling for convolutional neural networks. In Proceedings of ICML, 36th international conference on machine learning, Long Beach, California."
}
```

The printed link text is `2019` and the whole reference sits in the anchor's `title` attribute. Both are kept, so a caption citation resolves without a second pass over the reference list.

There is one thing the checklist for this work expected and the site does not publish: a figure source or credit line. It is not on the article page and not on the figure page, on any capture measured. The caption's citations are what exists instead.

## Tables: the rows are not on the article page

This is the finding worth taking away from the whole subpage story.

The open access capture is 718 KB of HTML. It announces one table, with a caption and a link. It contains zero `<table>` elements.

```console
$ spr tables 10.1007/s10994-021-05946-3
Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods

tables (1)

  Table 1
  Notation used throughout the paper
  https://link.springer.com/article/10.1007/s10994-021-05946-3/tables/1

The article page carries no rows for any of these. Ask for a number to read one.
```

A pipeline that scrapes the article page for tabular data finds none and gets no error, which is the worst shape a missing thing can take. So `spr work -o json` gives you a `tables` array with captions and links and no rows, deliberately, and the rows come from one more request:

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

Cells keep the LaTeX. Rendering `\({\mathcal{X}}\)` into anything else would be this tool having an opinion about notation, and anyone who wants it rendered has a renderer. Rows go out tab separated rather than aligned, because a cell here can be far wider than a terminal and aligning would either wrap the table into nonsense or truncate the data.

## The two subpages were written years apart

They look like a pair and the markup says otherwise. The figures page names five regions: `figure`, `top-caption`, `bottom-caption`, `subtitle` and `back-link`, so reading it is rung 3 throughout. The tables page names exactly one, `subtitle`, and everything else on it is read off a css class.

The heading follows the same split. A figure gives you the label and the caption as two elements. A table runs them into one string, `Table 1 Notation used throughout the paper`, which this tool splits on the label so that a caption is a caption on both records.

`spr extraction --record table` prints the consequence: four rows and three of them are rung 4.

## A 200 is still not a success

`/figures/99` on an article with 17 figures answers 200 with 224 KB of page furniture and an empty body. Nothing about the response says it failed: the status is fine, the size is large, the classifier reports `ok`. Only the extractor can tell.

```console
$ spr figures 10.1007/s10994-021-05946-3 99
spr: 10.1007/s10994-021-05946-3 has no figure 99, and the site answered 200 for it anyway
$ echo $?
3
```

Exit code 3 is `no_data`: the page was fetched and understood and had nothing in it. That is the same rule the rest of this tool runs on, turning up in a new place.

## One thing that surprised us

A subpage identifies its parent better than a container page identifies itself.

`/metrics`, `/figures/N` and `/tables/N` all carry the article's entire bibliographic head, all 66 meta names in Highwire, Dublin Core and PRISM. So the work being counted or illustrated is named at rung 1, from `citation_title` and `citation_doi`, while a journal's own home page cannot manage rung 1 for its own title and falls back to an analytics payload.

That is convenient and it is also a trap. Extracting a `/metrics` page as a work succeeds, because the head is all there, and hands back an article record with no body, no sections and no references. Nothing in the page says which of the two it is. The url is the only thing that does, so that is where the check lives.

## What comes next

Reading the works and the things they hang off is now covered. Finding them without knowing a DOI first is the search commands, and following a reference list from one work to another is the graph.
