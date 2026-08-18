---
title: "Output formats"
description: "Text for reading, json for piping, and the exit code that carries the rest."
weight: 30
---

Every command takes `-o`:

```bash
spr get /article/10.1007/s10994-021-05946-3            # text, for reading
spr get -o json /article/10.1007/s10994-021-05946-3    # json, for piping
```

`spr get --body` is a third thing: the response body itself, unformatted, so a page or a pdf goes straight to a file or another tool.

```bash
spr get --body /journal/10994 > journal.html
spr get --body --kind pdf /content/pdf/10.1007/s10994-021-05946-3.pdf > paper.pdf
```

## The exit code is part of the output

A command that fetched a page successfully and a command that fetched a paywalled page both print something worth reading, so the difference between them is the exit code rather than an error message.

| Code | Meaning |
|---|---|
| 0 | It did what it was asked |
| 1 | A flag or argument this tool does not understand. Nothing was fetched. |
| 2 | The search surface answered with a client challenge and there was no fallback left |
| 3 | The page was fetched and understood and there was nothing in it |
| 4 | The publisher states `access=No`. The metadata was printed; only the body is missing. |
| 5 | A network failure, a timeout, or a 5xx that outlived the retries |
| 6 | An upstream said, in a header, that the budget is spent |
| 7 | `spr verify` found that a page no longer reads the way the ledger recorded it |

Seven is the only one of those that is a statement about the site rather than about the run, which is why it is not folded into any of the others. A scheduled job that wants to alert on Springer changing a page should not have to tell that apart from a mistyped flag.

So this works the way you would want it to:

```bash
spr get -o json "$url" || case $? in
  4) echo "paywalled, metadata above" ;;
  2) echo "challenged, try the rss feed" ;;
esac
```

Those are the codes for one identifier. A run given many of them, or reading them off stdin, answers about the run instead: a status has to cover every single target before it becomes the run's code, because one paywalled work in five hundred is not a restricted run. The counts go to stderr either way. See [reading identifiers from stdin](/reference/cli/#reading-identifiers-from-stdin).

## Every record carries an envelope

A record is not just fields. `spr work -o json` puts an `envelope` next to them:

```json
{
  "doi": "10.1007/s10994-021-05946-3",
  "title": "Aleatoric and epistemic uncertainty in machine learning: an introduction to concepts and methods",
  "envelope": {
    "tier": "html",
    "urls": ["https://link.springer.com/article/10.1007/s10994-021-05946-3"],
    "fetched": "2026-08-18T10:12:04Z",
    "status": "ok",
    "redirects": 3,
    "bytes": 718572,
    "via": {
      "authors": "linkdata:author[]",
      "references": "highwire:citation_reference",
      "sections": "region:section[data-title]"
    },
    "unread": ["MPU1-ad", "access-count", "altmetric-score"]
  }
}
```

There is no `missed` key on that record because nothing was missed, which is the same rule the fields follow.

| Field | What it is for |
|---|---|
| `tier` | which surface produced the record: `html`, `rss`, `search`, `sitemap`, `crossref`, `openalex` or `api` |
| `urls` | the requested urls, never the effective ones, because the effective url carries a per request uuid and is not an identifier |
| `via` | which rung and which exact tag or region answered each field |
| `missed` | every field that was looked for and did not arrive, each with the reason |
| `unread` | every region on the page nobody read, so the record never looks more complete than it is |

Absent means absent. A field the page did not carry is left out of the json rather than emitted as `null`, so `.abstract == null` and no `abstract` key are the same answer, and a field in `missed` is the third case: it should have been there and something stopped it.

## Provenance on a graph

`spr graph` is the one command whose output is not a record, and it carries the same two keys in a different place. `via` and `tier` sit on every node and every edge rather than once in an envelope, because a graph walked over forty pages and enriched from two backends is a document where the question is never what produced this file, it is what produced this edge.

```json
{"from": "spr:work/10.1007/s10994-021-05946-3", "to": "spr:person/orcid/0000-0002-9944-4108",
 "type": "authoredBy", "position": 1, "via": "jsonld:author[]", "tier": "html"}
```

The graph's own envelope keeps the seed url and the running byte count and not the url of every page read, since a list of fourteen hundred urls is a log rather than provenance. Under `--format nq` the tier becomes the fourth term of every quad, so the same question can be asked of a triple store.

## Seeing what it did

`--debug` puts one line per request on stderr, which stays out of the pipe:

```console
$ spr get --debug --no-cache /article/10.1007/s10994-021-05946-3 -o json | jq .bytes
spr: 200 ok 718872 bytes 3 redirects https://link.springer.com/article/10.1007/s10994-021-05946-3
718872
```
