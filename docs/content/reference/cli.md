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
| `work` | Read one article, chapter, protocol or reference work entry |
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

## `spr extraction`

```
spr extraction [field] [--rung <name>]
```

Prints the table this tool extracts by. Every field has a row, and the row says which rung is expected to answer it, which exact tag or region that is, and why it sits on that rung rather than a lower one. Naming one field prints the long form for that field alone.

```bash
spr extraction
spr extraction authors
spr extraction --rung selector
spr extraction -o json | jq '.[] | select(.rung == 4)'
```

The rung names are the ones a record's `via` field uses, so a name copied out of a record pastes straight back in here.

## `spr cache`

```
spr cache [--clear]
```

Prints the cache directory, how many responses are in it, how much room they take, and the time to live. `--clear` empties it.

## `spr version`

Prints the version, commit and build date stamped in at link time, plus the Go version, the platform, the user agent this build sends, and the pace settings it will hold to.
