---
title: "Reading a container"
description: "Journals, books and series, the three pages that hold works rather than being one."
weight: 20
---

A work is one thing you can read. A container is the thing it was published in, and this site has three of them:

```bash
spr journal 10994                          # a journal, by its Springer id
spr book 978-3-031-28170-9                 # a book, by isbn
spr series 558                             # a series, by its Springer id
```

Each is its own record with its own fields, because a journal has an impact factor and no price, a book has four ISBNs and no volumes, and a series has neither. One record with a kind field would have been mostly empty on every page it was used for.

## The one thing to know first

A work page announces itself. Google Scholar reads the Highwire meta tags on an article, so `citation_title`, `citation_doi` and `citation_author` are there, correct, and quickly fixed when they are not.

A container page announces nothing. There is no `citation_journal_title` on a journal home page, no `citation_isbn` on a book page, and the schema.org block on a series is a breadcrumb list. Every field on these three pages comes from rung 3 or rung 4, which means Springer's own `data-test` region names or, failing that, a css class and a printed English label.

`spr extraction --record journal` prints exactly which, and the `why` column says why each row sits where it does:

```console
$ spr extraction --record series
RECORD  FIELD          RUNG    SOURCE                                    WHY
series  series_id      region  datalayer.Book Series Id                  arrives as a JSON number here and as a string on a journal, so the reader accepts both
series  title          region  datalayer.Book Series Title               the thinnest payload on the site states this and the id and nothing else
series  editors        region  [data-test=editor-links-1]                the same dl per role shape a journal uses, with Series Editor in place of Editor-in-Chief
series  latest_titles  region  [data-test=latest-titles] .c-card         five books out of many thousands, which is why the record names them latest rather than titles
series  indexed_in     region  [data-test=series-abstract-and-index-services-list]  the same claim a journal makes about itself, in the same words
```

Rung 3 is not a worse answer than rung 1. It is a less durable one, and the record says which you got so that a field breaking after a redesign is a thing you can see rather than a thing you discover months later.

## A journal

```console
$ spr journal 10994
title         Machine Learning
id            10994
url           https://link.springer.com/journal/10994
issn          1573-0565, 0885-6125
publisher     Springer, Springer
model         Hybrid Access
publishing    continuous, articles appear before the issue closes
open access   664 articles
metric        Journal Impact Factor 4.9 (2025)
metric        5-year Journal Impact Factor 8.0 (2025)
metric        Downloads 2.4M (2025)
subjects      Machine Learning, Control, Robotics, Automation, Artificial Intelligence, ...

editors (1)
   1  Michelangelo Ceci  Editor-in-Chief

indexed in (29)
  ACM Digital Library, ANVUR, BFI List, Baidu, CLOCKSS, CNKI, CNPIEC, ...

volumes      0 held, more at https://link.springer.com/journal/10994/volumes-and-issues
```

Two ISSNs, electronic first, and they are kept apart rather than folded into one field, because a citation that gives you `0885-6125` and a record that only knows `1573-0565` do not match and should.

Every editor keeps their role. Editor-in-Chief, Associate Editor and Editorial Board Member are different jobs, and a flat list of names throws that away for no gain.

A metric is only emitted with the year it was measured in. The journal prints one line with no year, "Submission to first decision (median), 5 days", and it goes to `missed` with that sentence attached rather than being emitted as a metric that nothing can be compared against:

```console
$ spr journal 10994 --envelope
...
  missed     metrics: the page printed "Submission to first decision (median), 5 days" with no year, and a metric with no year is not comparable with anything
```

`2.4M` is printed as `2.4M`. The record carries `2400000` beside it, but the human form prints the publisher's own number, because that is the one you can go and check.

### The volumes are a separate page

The last line of a journal is a pointer, not a list:

```
volumes      0 held, more at https://link.springer.com/journal/10994/volumes-and-issues
```

`0 held` and a url is a different fact from an empty list. It says the collection exists, this record does not have it, and here is where it is. `--volumes` makes the second request and fills it in:

```console
$ spr journal 10994 --volumes
...
volumes      348 of 348 held

volumes (114), 348 issues
  volume 115   January - August 2026
    Issue 8    2026-08
    Issue 7    2026-07
...
  volume 105   January - December 2016
    Issue 3    2016-12    Special Issue of the ECML PKDD 2016 Journal Track
```

114 volumes and 348 issues, 86 of which carry a themed collection title. The volume year comes from the volume's own printed span rather than from its issues, because volume 115 runs January to August 2026 and a volume that crosses a year boundary would otherwise take the year of whichever issue happened to be listed first.

## A book

A book is addressable two ways and both work as given:

```bash
spr book 10.1007/978-3-031-28170-9      # by doi
spr book 978-3-031-28170-9              # by isbn
```

It is the richest page on the site, and it is read from four sources at once: eight meta names, three JSON-LD blocks, the analytics payload, and the printed bibliographic table at the foot of the page.

```console
$ spr book 978-3-031-28170-9
title         The Economics of Family Taxation. Optimal Tax Issues from a Household Economics Perspective
doi           10.1007/978-3-031-28170-9
kind          book, Monograph
isbn ebook    978-3-031-28170-9
isbn print    978-3-031-28172-3
isbn hard     978-3-031-28169-3
isbn soft     978-3-031-28172-3
publisher     Springer Cham
edition       1
pages         XI, 102
series        Population Economics  1431-6978
published     2023-04-26
hardcover     2023-04-27
softcover     2024-04-28
access        not free to read, access=No
metrics       1169 accesses, 22 citations

prices
  eBook            EUR 85.59  ebook
  Softcover Book   EUR 99.99  book
  Hardcover Book   EUR 99.99  book

chapters (7 of 9 rows, front and back matter included)
```

**Four ISBNs, four fields.** One book has an electronic ISBN, a hardcover ISBN, a softcover ISBN and a print ISBN, and on this book the softcover and the print ISBN happen to be the same string while the hardcover is different. Collapsing them into one `isbn` field picks one edition and silently discards two, and which one you get depends on which source the reader happened to look at first. The doi resolves to the electronic edition, so that is the one JSON-LD names, and it is not the one the page prices most prominently.

**Three publication dates.** The ebook came out 2023-04-26, the hardcover the day after, and the softcover a year later. A book with one `published` field is a book you cannot cite the softcover of.

**The price kind is not the printed label.** `eBook`, `Softcover Book` and `Hardcover Book` are prose that changes with the locale. The `kind` column beside each price is the value the publisher's own order form posts to its cart, which is the stable one. Prices are localized by requesting IP, so what you see depends on where you fetch from, and the printed string is kept beside the parsed amount for that reason.

**Front and back matter are rows.** `--chapters` prints all nine rows and names the two that are not chapters:

```console
$ spr book 10.1007/978-3-031-28170-9 --chapters
...
chapters (7 of 9 rows, front and back matter included)
  i-xi       Front Matter
  1-14       Standard Optimal Taxation with Single Agents: What It Is and What to Use in Its Place
             10.1007/978-3-031-28170-9_1
  15-30      Optimal Taxation in the Presence of Household Production
             10.1007/978-3-031-28170-9_2
...
  99-102     Back Matter
```

Front matter has a page range and no DOI. That is why a chapter is its own type here rather than the generic reference a linked work usually gets, since a reference has no room for a page range and no way to say that a row is deliberately without an identifier.

A book behind a subscription is read rather than refused. Everything above comes off a page with `access=No`, and the exit code is 4 to say so.

## A series

```console
$ spr series 558
title         Lecture Notes in Computer Science
id            558
issn          1611-3349, 0302-9743

editors (4)
   1  Elisa Bertino  Series Editor
   2  Wen Gao  Series Editor
   3  Bernhard Steffen  Series Editor
   4  Moti Yung  Series Editor

latest titles (5)
  2027   Superposition for Higher-Order Logic
  2027   Consolidated Ada 2022 Reference Manual. Volume 1 - Core Language
  ...
  2027   Vibe Coding  open access

titles       5 held, more at https://link.springer.com/series/558/books
```

The five books are the five the page shows, and LNCS has many thousands. The field is called `latest_titles` for that reason, and the pointer under it says `5 held` and where the rest are, in the same shape a journal points at its volumes.

Each card credits either authors or editors, and the two are kept apart. The page's own `itemprop` says `editor` on both, so the printed `Authors:` or `Editors:` label is what this reads instead, because a monograph with one author and a proceedings volume with four editors are different books and the page is careful to say which.

## Where a conference goes

There is no conference page on this site. `/conference/aaai` answers 404 with a zero byte body, so no conference url is ever constructed.

What exists is a conference named inside a proceedings title, and it is parsed out of that title only when the title actually says so:

```
Computer Vision - ECCV 2020: 16th European Conference, Glasgow, UK, August 23-28, 2020, Proceedings, Part I
  name     16th European Conference
  acronym  ECCV
  year     2020
```

The location is never read, even though it is sitting right there in that string, because "Glasgow, UK" and "Lecture Notes in Computer Science, Munich" are the same shape and only one of them is a place the conference happened. A title that names no conference produces no conference at all rather than a guess. A wrong conference year is worse than no conference year, since a reader has no way to tell it was invented.

## The envelope, on every one of them

`--envelope` works the same on all three:

```console
$ spr series 558 --envelope
...
envelope     html, ok, 144018 bytes, 3 redirects, fetched 2026-08-18 10:57:40 UTC
             8 fields answered, 0 missed, 24 regions unread
```

24 regions unread on a series, 43 on a journal and a book. Those are named components this tool saw and did not read, listed rather than dropped, so that the day one of them starts carrying something worth having it is already written down.

## What comes next

Reading a container gives you a pointer to its subpages, and following those is the next command. Until then, the pointer is a url and `spr get --body` will fetch it.
