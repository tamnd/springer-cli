---
title: "Building a graph"
description: "Nodes and edges instead of records, where every node's uri names the authority that identified it and nothing is merged on a guess."
weight: 60
---

Every other command in this tool answers a question about one thing. `spr graph` answers a question about the relations between things, and it writes nodes and edges rather than records.

```bash
spr graph 10.1007/s10994-021-05946-3                        # one page, one graph
spr graph 10.1007/s10994-021-05946-3 --also crossref        # the same work, with its references as edges
spr graph 10994 --depth 1 --dry-run                         # what would this cost
spr graph --merge monday.json --merge tuesday.json          # union two earlier walks
```

## One page is already a graph

```console
$ spr graph 10.1007/s10994-021-05946-3 --depth 1 > graph.json
graph: 20 nodes and 24 edges from 2 pages
graph: 1 issue, 1 volume, 1 work, 2 orgs, 2 publishers, 3 journals, 3 persons, 7 subjects
graph: 1 partOf, 1 inVolume, 1 inIssue, 2 authoredBy, 1 editedBy, 2 affiliatedWith, 10 hasSubject, 2 publishedBy, 4 sameAs
graph: 0 of 122 references resolved to an identifier and became edges, and 122 did not and stay in the record as text
```

Two requests, twenty nodes. Three journal nodes for one journal, because the page names it three ways, by print ISSN, by electronic ISSN and by the publisher's own number 10994, and those are three identifiers joined by four `sameAs` edges rather than one node the tool picked a favourite for.

The graph goes to stdout and every line above goes to stderr, so `> graph.json` gets the graph and nothing else.

## Nothing is merged on a guess

Look at the two authors of that article.

```json
{
  "uri": "spr:person/orcid/0000-0002-9944-4108",
  "kind": "person",
  "label": "Eyke Hüllermeier",
  "props": { "name": "Eyke Hüllermeier", "orcid": "0000-0002-9944-4108" },
  "via": "orcid",
  "tier": "html"
}
{
  "uri": "spr:person/name/d58cff045d1d56f7215ff4a315b4fb70ee0520abf62eec6fd7fdce29402884bb",
  "kind": "person",
  "label": "Willem Waegeman",
  "props": { "name": "Willem Waegeman" },
  "via": "name",
  "tier": "html"
}
```

One of them registered an ORCID and one of them did not, and the uri says which. The first is identified by ORCID and can be joined to any other graph in the world that uses ORCIDs. The second is identified by a hash of a name string on one page, is worth exactly what a name string is worth, and cannot be joined to anything.

This is the whole design. There is no third case, no confidence score, and no threshold to tune. Every uri names the authority that identified the thing, so a consumer can filter on the authority rather than trusting a number this tool made up.

The same applies to institutions. Add OpenAlex and the two universities appear twice:

```
spr:org/name/54dc5fdb…  Ghent University       via name                            html
spr:org/name/efea5253…  Paderborn University   via name                            html
spr:org/ror/00cv9y106   Ghent University       via openalex:institutions[].ror     openalex
spr:org/ror/058kzsd48   Paderborn University   via openalex:institutions[].ror     openalex
```

Four nodes for two universities, and they stay four. A matching label is not identifier equivalence, and `sameAs` here means identifier equivalence and nothing else.

## Merging names, when you ask for it

`--merge-names` is the explicit opt in, and it is deliberately timid.

```console
$ spr graph 10.1007/s10994-021-05946-3 --also crossref,openalex --merge-names
graph: no name matched exactly one orcid, so nothing was merged
```

It merges a name keyed person into an ORCID keyed one only when the normalized name matches exactly one ORCID node in the graph. Two ORCIDs answering to the same name is the case that matters, and there it refuses and leaves all three nodes standing rather than picking one. When it does merge, it writes `mergedFrom` on the surviving node and adds a note to the graph, so the guess is a fact in the output that anyone can undo.

## A reference that did not resolve is not an edge

The measured article prints 122 references and the html tier produces no `cites` edge at all. Not a weak one, none. The page renders reference text and resolver links and states a DOI for none of them, so there is nothing on it to point a citation edge at.

`--also crossref` reads the deposit instead of the rendering:

```console
$ spr graph 10.1007/s10994-021-05946-3 --also crossref,openalex
graph: 105 nodes and 107 edges from 1 page
graph: 1 funder, 2 issues, 2 persons, 2 volumes, 21 subjects, 3 journals, 3 publishers, 4 orgs, 67 works
graph: 2 partOf, 2 inVolume, 2 inIssue, 2 authoredBy, 4 affiliatedWith, 66 cites, 1 fundedBy, 21 hasSubject, 3 publishedBy, 4 sameAs
graph: 3 backend requests to crossref and openalex
graph: crossref: 66 of 122 deposited references carry a doi and became edges, and 56 do not and became nothing
```

Sixty six references became edges and fifty six still did not, and both halves are reported. The fifty six are not lost, they are in the work record as the text they always were, they just do not get an edge, because an edge from a work to a guess is worse than no edge.

## What each tier adds

| Tier | What it brings that the others do not |
|---|---|
| html | Containment, authors in order, affiliations as names, subjects, the publisher |
| crossref | References as DOIs, funders with award numbers, authenticated ORCIDs |
| openalex | ROR ids for institutions, topics, and `citedBy` with `--cited-by` |

`citedBy` is the one edge direction no page on this site can produce, because a work does not know who will cite it later. It comes from OpenAlex or it does not come at all.

## The bill

The walk is billed per depth before it runs, and above twenty requests it stops until you pass `--yes`.

```console
$ spr graph 10.1007/978-3-030-58607-2 --depth 1 --dry-run
seed      https://link.springer.com/book/10.1007/978-3-030-58607-2
depth 0   book page, the seed
depth 1   40 work pages
requests  41
pace      1 request / 2s, floor 1 request / 1s
estimate  1 minute
tier      html only, so no ror, funder or citedBy edges
format    json
```

Per depth rather than as one total, because the shape is the point. A work and its journal is two requests. The same `--depth 1` on a proceedings volume is forty one, and on a series it is ten books whose own chapters are another depth below that. One number with no breakdown cannot tell those apart until the walk is already running.

The frontier only ever contains pages this tool can read. A work leads to its container, a journal to its volumes page, a book to its chapters, a series to the books it lists, and a volumes page to nothing. Under `--follow-refs` a reference joins the frontier only if its DOI starts with `10.1007`, because a reference to another publisher has no page at this one.

## Ten ways out

```bash
spr graph 10.1007/s10994-021-05946-3 --format ttl
spr graph 10.1007/s10994-021-05946-3 --format gexf --projection coauthor > coauthors.gexf
spr graph 10.1007/s10994-021-05946-3 --format csv --dir ./import
```

```console
$ spr graph 10.1007/s10994-021-05946-3 --format ttl | head -20
@prefix spr: <https://springer-cli.tamnd.com/ns/> .
@prefix schema: <http://schema.org/> .
@prefix dcterms: <http://purl.org/dc/terms/> .
@prefix prism: <http://prismstandard.org/namespaces/basic/2.0/> .
@prefix bibo: <http://purl.org/ontology/bibo/> .
@prefix fabio: <http://purl.org/spar/fabio/> .
@prefix cito: <http://purl.org/spar/cito/> .
@prefix frapo: <http://purl.org/cerif/frapo/> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

spr:journal/0885-6125
    a schema:Periodical, fabio:Journal ;
    rdfs:label "Machine Learning" ;
    prism:issn "0885-6125" ;
    owl:sameAs spr:journal/1573-0565 .
```

Eleven prefixes and four local terms. The vocabulary is borrowed almost entirely, from schema.org, Dublin Core, PRISM, BIBO, FaBiO, CiTO and FRAPO, and the only terms minted here are `spr:recommends`, `spr:springerId`, `spr:accessStatement` and `spr:mergedFrom`, which are the four things nobody else has a term for. Holding that budget cost two design decisions worth knowing about: the four containment edges all map to `dcterms:isPartOf` and are told apart by the object's own `rdf:type`, and ordered authorship is a plain `rdf:List` under `bibo:authorList` rather than a reified position property.

`--format nq` puts the tier in the fourth term:

```console
$ spr graph 10.1007/s10994-021-05946-3 --format nq | head -1
<https://springer-cli.tamnd.com/ns/journal/0885-6125> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://schema.org/Periodical> <https://springer-cli.tamnd.com/ns/tier/html> .
```

So a store can be asked what the html tier alone said, or what only arrived with Crossref. Drop the fourth column and it is valid N-Triples, which is why the `nt` and `nq` forms of the same graph have the same number of lines.

For a graph database, `--format csv --dir` writes the pair with the import headers already on them:

```console
$ head -2 import/nodes.csv
uri:ID,:LABEL,label,props,via,tier,fetched:datetime
spr:journal/0885-6125,journal,Machine Learning,"{""issn"":""0885-6125""}",highwire:citation_issn,html,2026-08-18T10:12:04Z

$ head -2 import/edges.csv
:START_ID,:END_ID,:TYPE,position:int,sequence,role,via,tier,fetched:datetime
spr:journal/0885-6125,spr:journal/1573-0565,sameAs,,,,identifier equivalence,html,2026-08-18T10:12:04Z
```

Of the ten, `json` is the only one that round trips. It is the only form that holds every field of every node and edge, and it is the one `--merge` reads.

## Two runs, one graph

```bash
spr graph 10.1007/978-3-030-58607-2 --depth 1 --yes --resume > proceedings.json
spr graph 10994 --depth 1 > journal.json
spr graph --merge proceedings.json --merge journal.json > both.json
```

`--merge` unions on the uri, so a work that appears in both files is one node in the result, and merging a file into itself changes nothing. That is what makes a graph something you can build up over several days rather than in one long run.

`--resume` checkpoints each page as it finishes, keyed on the seeds, the format and the depth, so a resumed walk of one book never inherits the state of a walk of a journal. A page that did not answer is left unmarked, so the next run comes back for it. And a walk that fails halfway still writes what it read before it reports the error, because throwing away forty pages over the forty first that timed out would be the wrong way round.

## Where next

The [reference for `spr graph`](/reference/cli/#spr-graph) has the full flag list and the format table. [The open indexes](/guides/open-indexes/) covers what Crossref and OpenAlex answer on their own, which is where the reference DOIs and the ROR ids in this guide came from.
