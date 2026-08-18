---
title: "Reading one work"
description: "One article, chapter, protocol or reference work entry, and how to tell where every field came from."
weight: 10
---

`spr work` reads a single work page. Four content types share one record, and the type is a field rather than a different command:

```bash
spr work 10.1007/s10994-021-05946-3                      # an article
spr work 10.1007/978-3-030-58607-2_1                     # a chapter
spr work /protocol/10.1007/978-1-0716-1170-8_1           # a protocol
spr work /referenceworkentry/10.1007/978-3-642-27737-5_100-2
```

A DOI is enough. The registrant prefix says who issued it and nothing about what it is, so the suffix orders the paths it could live under and they are tried until one answers.

## What comes back

```console
$ spr work 10.1007/s10994-021-05946-3
doi           10.1007/s10994-021-05946-3
type          article
title         Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods
url           https://link.springer.com/article/10.1007/s10994-021-05946-3
language      en
published in  Machine Learning 110(3) pp 457-506
publisher     Springer US
issn          1573-0565, 0885-6125
published     2021-03
online        2021-03-08
cover date    2021-03-01
modified      2021-03-08
access        free to read, access=Yes, world readable declared and empty
license       http://creativecommons.org/licenses/by/4.0/
copyright     2021 The Author(s)

authors (2)
   1  Eyke Hüllermeier  0000-0002-9944-4108  eyke@upb.de
      Paderborn University, Heinz Nixdorf Institute and Department of Computer Science, Paderborn, Germany
   2  Willem Waegeman
      Ghent University, Department of Mathematical Modelling, Statistics and Bioinformatics, Ghent, Belgium

sections (14)
  Abstract
  1 Introduction
  2 Sources of uncertainty in supervised learning
  ...

body          17 figures, 66 equations, 24 footnotes, 122 references, 68 with resolver links, 3 recommendations

envelope      html, ok, 718572 bytes, 3 redirects, fetched 2026-08-18 10:12:04 UTC
              40 fields answered, 0 missed, 50 regions unread
```

## Four dates, kept apart

The page states four and they are four different facts: when the issue is dated, when it went online, what the cover says, and when the record last changed. Flattening them into one is the single most common cause of a wrong sort in a bibliographic tool, so this record keeps all four and states the precision of each. `published 2021-03` is a month because the page said `2021/03` and nothing more.

## A paywalled work is read, not refused

Everything except the body is in the head of a restricted page, so the record comes out whole and the envelope says what was withheld and why, in the page's own words:

```console
$ spr work 10.1007/978-3-642-27737-5_100-2
access        not free to read, access=No, This is a preview of subscription content
...
envelope     html, restricted, 230058 bytes, 4 redirects, fetched 2026-08-18 10:12:17 UTC
             23 fields answered, 3 missed, 42 regions unread
  missed     abstract: dc.description was present and empty, and so was the schema.org description
  missed     body: the page says "This is a preview of subscription content"
  missed     keywords: the page declared keywords and the value was empty
$ echo $?
4
```

Three different kinds of nothing are on display there. `abstract` was declared and empty, `body` was withheld, and `keywords` was declared with an empty value. None of them is the same as a field the page never carried, and a field the page never carried is simply not in the record at all.

## Where each field came from

`--envelope` prints the whole ladder:

```console
$ spr work 10.1007/s10994-021-05946-3 --envelope
via
  authors                linkdata:author[]
  access.world_readable  highwire:citation_fulltext_world_readable (present, empty)
  keywords               linkdata:keywords
  references             highwire:citation_reference
  ref_links              selector:.c-article-references__links a
  sections               region:section[data-title]
  title                  highwire:citation_title
```

Four rungs, tried in order, first one that answers wins. `spr extraction` prints the table of which rung is expected to answer each field, and the reason it sits there:

```console
$ spr extraction authors
field   authors
rung    2, linkdata
source  author[]
why     the only source binding orcid, affiliation and email to the right person
```

That reason is the point. When a field starts coming back empty, the first question is always whether the page moved or the extractor was always wrong, and a row that states its reasoning answers it without an archaeology session in the git history.

## References in two shapes

122 references on that article, and 68 of them parse into structured pairs while 54 are free text. Both are kept:

```bash
spr work -o json 10.1007/s10994-021-05946-3 | jq '.references[] | select(.structured) | .doi'
```

An unstructured reference is a normal outcome and not a failure. A reference nobody can identify produces no citation edge, which is the correct result rather than a missing one, and the published string is kept either way because the pairs are lossy.

The resolver links Springer prints next to a reference carry their kind, so a Google Scholar search link and a MathSciNet record are distinguishable rather than both being "a link":

```bash
spr work -o json 10.1007/s10994-021-05946-3 | jq -r '.references[].links[]?.kind' | sort | uniq -c
```

## What was left unread

The envelope lists every region on the page nobody read. It is the other half of the same honesty: rather than letting a record look complete, the tool says out loud what it is leaving on the table, and a new component appearing in that list is news about the site rather than a bug.
