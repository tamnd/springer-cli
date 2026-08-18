---
title: "Release notes"
linkTitle: "Release notes"
description: "What changed in each spr release, newest first."
weight: 40
---

What shipped in each release, newest first. Every tagged version builds the same set of artifacts: archives for Linux, macOS, Windows and FreeBSD, Linux packages (deb, rpm, apk), a multi-arch container image on GHCR, and entries for the package managers. Binaries are pure Go, so there is nothing to install alongside them.

## v0.2.0

The first release of `spr`, and a complete rewrite. Everything below is new.

**The binary is called `spr` now.** v0.1.0 shipped a binary called `springer` and nothing in this release answers to that name. Remove the old one before installing this, because two versions of the same tool under two names on one `PATH` is a problem that surfaces weeks later in somebody's cron job.

### Every response is classified before it is parsed

A 200 from this site means very little on its own. A chapter behind a subscription answers 200 with a full set of metadata and no body, the search surface answers 200 with a 3,038 byte Fastly challenge, and a work that does not exist answers 404 with 122,477 bytes of body. So every response is sorted into `ok`, `restricted`, `challenged`, `wrong_kind` or `not_found` on its content, before a byte is parsed, and the classification is carried out to the exit code.

### Every field says where it came from

Reading a work is four rungs tried in order, and the record's envelope names which one answered for each field, what was looked for and not found, and what was left unread. A field the page did not carry is absent from the json rather than emitted as `null`, and a field that should have been there and was stopped is in `missed` with the reason. A record never looks more complete than it is.

### The commands

Twenty of them, flat, with mode flags rather than nested subcommands.

`get` classifies one url. `work`, `journal`, `book` and `series` read the four record types. `metrics`, `figures` and `tables` read the subpages. `search` runs both of the paths this site answers searches on and joins them on DOI. `sitemap` enumerates the site from its own sitemaps. `crossref`, `openalex` and `cited-by` ask the three hosts this site is not. `graph` walks from a seed and writes ten formats. `api` queries the Springer Nature API, which is the one thing here that needs a key. `verify`, `extraction`, `cache` and `version` explain what the tool is doing and whether the site still reads the way it did.

### Nine commands take a list, and read it from stdin

`get`, `work`, `journal`, `book`, `series`, `metrics`, `crossref`, `openalex` and `cited-by` take any number of identifiers, and read them one per line from stdin when none are given. That is what makes the pipelines in these docs real rather than aspirational:

```console
$ spr crossref 10.1007/s10994-021-05946-3 --references | spr work --yes
crossref: 66 of 122 deposited references carry a doi, and 56 do not
spr: 13 of 66 read, 6 restricted, 53 failed
```

Blank lines and `#` comments are skipped. More than twenty targets is billed before the first request, with `--yes` to go ahead. A status becomes the run's exit code only when it covers every target, and a failure part way through does not stop the rest.

### The counts are never merged

Springer's metrics page says 1,906 citations attributed to Dimensions, Crossref says 1,553 deposited, and OpenAlex stores 1,563 while its live listing counted 1,554 in the same minute. Every count prints under a name that says who counted it, and no command adds them up. They are counting different corpora and a single number would be a fiction.

### The capture ledger

`spr verify` reads fourteen pages again and reports whether they still yield what they yielded on the day they were measured. Fewer fields fails. More fields also fails, until somebody records the improvement, because an improvement nobody noticed is how a tool ends up with two versions of what it promises. A weekly workflow runs it live and opens one issue rather than turning the build red.

### Built and verified

Go 1.26.6, three direct dependencies, 285 tests. Archives for ten platform pairs, deb, rpm and apk, a multi-arch image on GHCR, an SBOM next to every archive, and `checksums.txt` signed with keyless cosign. See [installation](/getting-started/installation/) for the verify recipe.

## v0.1.0

The `springer` binary: a Crossref member 297 client with `recent` and `search`. Superseded entirely by v0.2.0 and left published so its download urls keep working. Nothing in it shares code with what shipped above.
