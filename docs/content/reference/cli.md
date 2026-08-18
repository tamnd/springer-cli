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
| `journal` | Read one journal home page, and optionally its volumes and issues |
| `book` | Read one book, proceedings volume or reference work |
| `series` | Read one book series home page |
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
