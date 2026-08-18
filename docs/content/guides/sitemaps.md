---
title: "Enumerating the site"
description: "The sitemaps are the only complete list of what Springer publishes, and the date in them is not a publication date."
weight: 40
---

Search answers questions. The sitemaps answer a different one: what is there at all.

```bash
spr sitemap                       # one request, the shape of everything
spr sitemap --static journals     # three requests, every journal there is
spr sitemap --kind book --since 2026-01-01 | spr work
```

There are twelve maps in total. One index of dated shards, and eight static maps that between them name every journal, series and collection Springer has. The eight are the best value per request anywhere in this tool. The index is 10,408 shards and close to six hours, so it is billed before it is walked.

## One request for the shape of it

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

envelope     sitemap, ok, 1267777 bytes, 0 redirects, fetched 2026-08-18 12:53:02 UTC
             2 fields answered, 1 missed, 0 regions unread
  missed     lastmod: absent on 2302 of 10408 children, which is every month and year shard, because the site knows the bucket those are filed under and not a day
```

Every number there was read out of the index that was just fetched. The bill at the bottom is computed the same way, which is the point: the index grows daily, and a cost estimate compiled into the tool would be right on the day it was written and quietly wrong for the rest of its life.

## The date in a file name is not a date

This is the one thing worth knowing before you use any of it.

The index holds 66 shards named `sitemap_2020-01-01_N`, which is roughly 330,000 works filed under a single nominal day. The first of January is where everything known only to its year ends up. Read the first of those shards and you get 5,000 urls carrying 173 distinct `lastmod` values running from 2020-01-23 to 2026-08-17, and not one of them is 2020-01-01.

So the field is called `bucket` throughout, it is parsed from the file name, and there is no flag anywhere in this tool that turns it into a published date. `--since` and `--until` filter on it, and what they mean is which shards to read rather than what was published when. If you want publication dates, they are on the records, and `spr work` reads them.

The bucket has a precision, because the file name has one:

| Name | Precision | How many |
|---|---|---|
| `sitemap_1850_1.xml` | year | 50 |
| `sitemap_1900-01_1.xml` | month | 2,252 |
| `sitemap_2026-08-18_1.xml` | day | 8,106 |

A window compares against the span a bucket covers and not against its first instant, so `--since 1850-06-01` still keeps `sitemap_1850_1.xml`. Half of what is in it is after that date and the site has not said which half.

`lastmod` is present on the 8,106 day shards and absent on all 2,302 others, where it would be a claim the site is not making. The envelope says so rather than reporting a hole.

## The eight maps that are worth eight requests

```console
$ spr sitemap --static journals
sitemap: 3557 urls across 3 maps are 3405 distinct ones, because the imprint maps list 152 of the same titles
https://link.springer.com/journal/11137
https://link.springer.com/journal/11439
https://link.springer.com/journal/402
...
sitemap: journals: 3,405 urls from 3 maps
```

`journals` is three maps and not one. Springer, BMC with SpringerOpen, and Palgrave are published separately by imprint, they hold 3,043, 468 and 46 entries, and they overlap. Fetching any one of them and calling it the journal list is short by hundreds either way, so all three are fetched, the union is taken, and the count of what was listed twice is stated rather than quietly dropped.

| Name | Maps | Urls | What it is |
|---|---|---|---|
| `journals` | 3 | 3,405 | Every journal, across all three imprints |
| `series` | 1 | 9,578 | Every book series |
| `collections` | 1 | 26,133 | Every collection |
| `brands` | 1 | 79 | 75 brands and 4 partners, which is why it is not only brands |
| `shops` | 1 | 65 | One url per line of plain text, at a `.txt` path among `.xml` siblings |
| `subjects` | 1 | 10 | An index of ten more sitemaps rather than a list of pages |

`subjects` says so when you fetch it, because a list of ten sitemap urls and a list of ten pages look identical once they are on stdout:

```console
$ spr sitemap --static subjects
sitemap: sitemap-subjects-index.xml is an index rather than a list, so these 10 urls are sitemaps and not pages
```

Feed any of these straight into the container commands:

```bash
spr sitemap --static journals | head -20 | xargs -n1 spr journal
```

## Walking the shards

Enumeration starts when you give `--since`, `--until`, `--kind` or `--all`. Urls go to stdout one per line and everything else goes to stderr, so the pipe stays clean:

```console
$ spr sitemap --kind article --since 2026-08-18
https://link.springer.com/article/10.1007/s00431-026-07333-3
https://link.springer.com/article/10.1007/s11126-026-10306-2
...
sitemap: 1 of 1 shards read, 1,012 urls, 112.8 KB, 0 seconds
```

Above ten shards the bill is printed and the walk proceeds. Above a hundred it stops:

```console
$ spr sitemap --kind book --since 2026-06
sitemap: 87 child sitemaps at 2s pace is 3 minutes
         a full shard is 5,000 urls and 559.0 KB, so this is bounded above by 435,000 urls and 47.5 MB
         narrow it with --since and --until, or pass --yes to proceed
sitemap: 50 of 87 shards, 0 urls
sitemap: 87 of 87 shards read, 149,513 urls, 0 kept, 16.3 MB, 3 minutes
```

That run is also a demonstration of `--kind`. It read 149,513 urls published across the last three months and kept none of them, because books are not filed in those shards. The filter is on the url's own first path segment, so it costs nothing and it happens before anything is printed.

`--all` is the whole index and it always stops for `--yes`, because the difference between meaning the last three days and meaning everything since 1850 is one flag and the tool should not guess which one you meant.

## Interruptions

A walk of everything is a five hour job on a laptop that will close, so `--resume` writes each shard's url to a state file as that shard finishes:

```bash
spr sitemap --all --yes --resume > urls.txt
# a laptop closes, or ctrl-c happens
spr sitemap --all --yes --resume >> urls.txt
```

The second run skips what the first one finished and says how many that was. Three details matter and all three are deliberate:

A shard is marked only after every url in it has been printed, so an interruption mid-shard re-reads that shard rather than losing part of it. The state file is keyed on the selection, so resuming a walk of the last three days never inherits the state of a walk of everything. And a shard that did not answer is left unmarked and counted, so `--resume` comes back for it and the exit code says the walk has a hole in it.

The state lives under the cache directory, which is what `spr cache --clear` empties and what `--no-cache` turns off. `--resume` with `--no-cache` is a usage error rather than a run that quietly fails to resume.

## What this is for

The sitemaps are how you get a corpus rather than a search result:

```bash
# every article published on one day, read in full
spr sitemap --kind article --since 2026-08-18 | spr work -o json > day.jsonl

# every journal, as records
spr sitemap --static journals | xargs -n1 -P1 spr journal -o json > journals.jsonl
```

Both of those are paced at the same one request per two seconds as everything else in this tool, and the second one is 3,405 requests, which is just under two hours. The bill is arithmetic you can do before you start, and `spr sitemap` prints it for the case where you would rather not.
