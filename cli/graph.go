package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

// spr graph.
//
// This is the command that turns a pile of records into something with edges in
// it, and it is also the one that can spend an afternoon of requests, so it
// bills before it walks and it checkpoints while it does.

func graphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph <seed>...",
		Short: "Walk from a seed and write the nodes and edges as a graph",
		Long: "graph reads a seed and everything it leads to, and writes the result as nodes and\n" +
			"edges in one of ten formats.\n\n" +
			"A seed is a DOI, a journal number, a path or a url. --depth says how many rounds of\n" +
			"following to do, and 0 reads the seed and stops. The walk only follows an edge to a\n" +
			"page this tool can read: a work names its container, a journal leads to its volumes\n" +
			"and issues page, a book leads to the works in its table of contents, and a series\n" +
			"leads to the books it lists. A reference is followed only when its DOI carries this\n" +
			"publisher's prefix, because the others have no page at the other end.\n\n" +
			"Identity comes from identifiers and from nothing else. A person with an ORCID and a\n" +
			"person with only a name are two visibly different nodes, and --merge-names is what\n" +
			"joins them, on an exact name match, recording on the surviving node what it swallowed.\n\n" +
			"Above twenty requests the bill is printed and nothing is fetched until you pass --yes.\n" +
			"Every page that finishes is checkpointed, so --resume picks up a walk that was\n" +
			"interrupted rather than starting it again.",
		// A seed is not required when --merge is: merging two graphs that were
		// already walked is a job with nothing to fetch in it.
		Args: cobra.ArbitraryArgs,
		Example: "  spr graph 10.1007/s10994-021-05946-3\n" +
			"  spr graph 10.1007/s10994-021-05946-3 --also crossref,openalex --format ttl\n" +
			"  spr graph 10994 --depth 1 --dry-run\n" +
			"  spr graph 10.1007/978-3-030-58607-2 --depth 1 --yes --format graphml > book.graphml\n" +
			"  spr graph 10.1007/s10994-021-05946-3 --format gexf --projection coauthor",
	}

	var (
		depth      int
		format     string
		projection string
		dir        string
		also       []string
		citedBy    int
		rails      bool
		followRefs bool
		mergeNames bool
		limit      int
		dryRun     bool
		yes        bool
		resume     bool
		merge      []string
	)

	f := cmd.Flags()
	f.IntVar(&depth, "depth", 0, "how many rounds of following to do, and 0 reads the seed and stops")
	f.StringVar(&format, "format", "json", "output format: "+joinFormats())
	f.StringVar(&projection, "projection", "", "compute a projection on the way out: coauthor, with --format gexf")
	f.StringVar(&dir, "dir", "", "write --format csv as nodes.csv and edges.csv in this directory")
	f.StringSliceVar(&also, "also", nil, "ask the open indexes about every work read: crossref, openalex")
	f.IntVar(&citedBy, "cited-by", 0, "add this many citing works per work, which needs openalex")
	f.BoolVar(&rails, "include-rails", false, "turn the publisher's recommendation strip into recommends edges")
	f.BoolVar(&followRefs, "follow-refs", false, "follow the references that carry this publisher's doi prefix")
	f.BoolVar(&mergeNames, "merge-names", false, "merge a name keyed person into an orcid keyed one on an exact name match")
	f.IntVar(&limit, "limit", 0, "stop after this many pages")
	f.BoolVar(&dryRun, "dry-run", false, "print what this walk would cost and make no requests")
	f.BoolVar(&yes, "yes", false, "proceed with a walk that was billed rather than stopping")
	f.BoolVar(&resume, "resume", false, "skip the pages an earlier run of this same walk finished")
	f.StringSliceVar(&merge, "merge", nil, "merge these graph files into the result, repeatable")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		errw := cmd.ErrOrStderr()
		note := func(s string) { fmt.Fprintln(errw, "graph: "+s) }

		kind, err := spr.ParseGraphFormat(format)
		if err != nil {
			return exit(CodeUsage, err)
		}
		if projection != "" && kind != spr.FormatGEXF {
			return exit(CodeUsage, errors.New("--projection is computed by the gexf writer, so it needs --format gexf"))
		}
		if dir != "" && kind != spr.FormatCSV {
			return exit(CodeUsage, errors.New("--dir names where the csv pair is written, so it needs --format csv"))
		}
		if depth < 0 {
			return exit(CodeUsage, fmt.Errorf("--depth %d asks for nothing to be read", depth))
		}
		if resume && g.noCache {
			return exit(CodeUsage, errors.New("--resume keeps its checkpoint in the cache directory and --no-cache turned that off"))
		}

		backends, err := parseAlso(also)
		if err != nil {
			return err
		}
		opts := spr.WalkOptions{
			Depth:        depth,
			IncludeRails: rails,
			FollowRefs:   followRefs,
			CitedBy:      citedBy,
			Limit:        limit,
			Note:         note,
		}
		for _, b := range backends {
			switch b {
			case spr.AlsoCrossref:
				opts.Crossref = true
			case spr.AlsoOpenAlex:
				opts.OpenAlex = true
			}
		}
		if citedBy < 0 {
			return exit(CodeUsage, fmt.Errorf("--cited-by %d asks for no citing works", citedBy))
		}
		if citedBy > 0 && !opts.OpenAlex {
			// Saying so beats quietly turning it on, because the flag that was
			// not passed is the one that costs somebody else's host a request.
			note("--cited-by reads OpenAlex, so openalex is on for this run")
			opts.OpenAlex = true
		}

		if len(args) == 0 && len(merge) == 0 {
			return exit(CodeUsage, errors.New("a graph needs a seed to walk from, or --merge to read graphs that were already walked"))
		}

		seeds := make([]string, 0, len(args))
		for _, a := range args {
			u, err := spr.GraphSeedURL(a)
			if err != nil {
				return exit(CodeUsage, err)
			}
			seeds = append(seeds, u)
		}

		// The bill, from the first seed's kind, because a bill that averages
		// two different shapes of walk describes neither of them.
		var cost spr.GraphCost
		if len(seeds) > 0 {
			cost = spr.BillGraph(seedKindOf(seeds[0]), opts, effectivePace())
			cost.Requests *= len(seeds)
			cost.Backends *= len(seeds)
		}
		if dryRun {
			printGraphCost(out, seeds, cost, opts, kind)
			return nil
		}
		if !yes && cost.Requests > spr.GraphThreshold {
			printGraphCost(errw, seeds, cost, opts, kind)
			fmt.Fprintln(errw, "         pass --yes to walk it, or lower --depth")
			return exit(CodeUsage, nil)
		}

		graph := spr.NewGraph()
		for _, path := range merge {
			held, err := readGraph(path)
			if err != nil {
				return exit(CodeUsage, err)
			}
			nodes, edges := graph.Merge(held)
			note(fmt.Sprintf("%s: %s nodes and %s edges, of which %s and %s were already held",
				path, group(len(held.Nodes)), group(len(held.Edges)), group(nodes), group(edges)))
		}

		c := client(errw)
		if resume && len(seeds) > 0 {
			key := spr.ResumeKey(append(seeds, format, fmt.Sprint(depth))...)
			state, err := spr.OpenGraphResume(g.cache, key)
			if err != nil {
				return exit(CodeUsage, err)
			}
			defer func() { _ = state.Close() }()
			opts.Resume = state
			if n := state.Count(); n > 0 {
				note(fmt.Sprintf("resuming, %s pages were finished by an earlier run of this walk", group(n)))
			} else {
				note("no earlier run of this walk to resume, so this is all of it")
			}
		}

		var stats *spr.WalkStats
		var walkErr error
		if len(seeds) > 0 {
			stats, walkErr = c.WalkGraph(cmd.Context(), seeds, graph, opts)
		}
		if mergeNames {
			if n := graph.MergeNames(); n > 0 {
				note(fmt.Sprintf("%s name keyed people were merged into an orcid, which is recorded on each surviving node", group(n)))
			} else {
				note("no name matched exactly one orcid, so nothing was merged")
			}
		}

		// A walk that failed halfway still wrote nodes, and they go out before
		// the error does. Throwing away an hour of reading because the last page
		// timed out would be the wrong way round.
		if len(graph.Nodes) > 0 {
			if err := spr.WriteGraph(out, graph, spr.GraphWriteOptions{Format: kind, Projection: projection, Dir: dir}); err != nil {
				return exit(CodeTransport, err)
			}
		}
		if walkErr != nil {
			return exit(CodeTransport, walkErr)
		}
		printGraphStats(errw, graph, stats)
		if len(graph.Nodes) == 0 {
			return exit(CodeNoData, nil)
		}
		return nil
	}
	return cmd
}

// seedKindOf names what the first seed is, for the bill.
func seedKindOf(url string) string {
	if k := spr.SeedKind(url); k != "" {
		return k
	}
	return "work"
}

func joinFormats() string {
	out := make([]string, len(spr.GraphFormats))
	for i, f := range spr.GraphFormats {
		out[i] = string(f)
	}
	return strings.Join(out, ", ")
}

// readGraph reads a graph written by an earlier run, in the json form, which is
// the one format that holds everything a merge needs.
func readGraph(path string) (*spr.Graph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	held, err := spr.ReadGraph(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return held, nil
}

// printGraphCost writes the bill.
//
// The lines are per depth because the shape is the point: one work and its
// journal is two requests, and a book at depth 1 is forty. A single total with
// no breakdown cannot tell those apart until it is too late.
func printGraphCost(out io.Writer, seeds []string, cost spr.GraphCost, opts spr.WalkOptions, format spr.GraphFormat) {
	line := func(label, value string) { fmt.Fprintf(out, "%-9s %s\n", label, value) }

	if len(seeds) == 0 {
		line("seed", "none, this run only merges graphs that were already walked")
	} else {
		line("seed", strings.Join(seeds, ", "))
	}
	for _, l := range cost.Lines {
		line(fmt.Sprintf("depth %d", l.Depth), l.What)
	}
	line("requests", group(cost.Requests))
	if cost.Backends > 0 {
		line("backends", fmt.Sprintf("%s to crossref and openalex, on their own hosts and their own pace", group(cost.Backends)))
	}
	line("pace", fmt.Sprintf("1 request / %s, floor 1 request / %s", effectivePace(), spr.PaceFloor))
	line("estimate", estimate(cost.Duration))

	var tier string
	switch {
	case opts.Crossref && opts.OpenAlex:
		tier = "html, crossref and openalex, so funders, ror ids and citedBy edges are all in"
	case opts.Crossref:
		tier = "html and crossref, so funders are in and ror ids and citedBy edges are not"
	case opts.OpenAlex:
		tier = "html and openalex, so ror ids are in and funders are not"
	default:
		tier = "html only, so no ror, funder or citedBy edges"
	}
	line("tier", tier)
	line("format", string(format))
}

// printGraphStats says what the walk found, on stderr, so that the graph itself
// is the only thing on stdout.
func printGraphStats(out io.Writer, graph *spr.Graph, stats *spr.WalkStats) {
	if stats == nil {
		return
	}
	nodes, edges := graph.Counts()
	fmt.Fprintf(out, "graph: %s %s and %s %s from %s %s\n",
		group(len(graph.Nodes)), plural(len(graph.Nodes), "node"),
		group(len(graph.Edges)), plural(len(graph.Edges), "edge"),
		group(stats.Pages), plural(stats.Pages, "page"))

	fmt.Fprintf(out, "graph: %s\n", countLine(nodeCounts(nodes)))
	fmt.Fprintf(out, "graph: %s\n", countLine(edgeCounts(edges)))
	if stats.Backends > 0 {
		fmt.Fprintf(out, "graph: %s backend %s to crossref and openalex\n", group(stats.Backends), plural(stats.Backends, "request"))
	}
	if stats.Skipped > 0 {
		fmt.Fprintf(out, "graph: %s pages were skipped because an earlier run had them\n", group(stats.Skipped))
	}
	if stats.Failed > 0 {
		fmt.Fprintf(out, "graph: %s pages did not answer and are left unread, so --resume comes back for them\n", group(stats.Failed))
	}
	for _, n := range graph.Notes {
		fmt.Fprintln(out, "graph: "+n)
	}
}

func nodeCounts(m map[spr.NodeKind]int) []string {
	out := make([]string, 0, len(m))
	for k, n := range m {
		out = append(out, fmt.Sprintf("%s %s", group(n), plural(n, string(k))))
	}
	sort.Strings(out)
	return out
}

func edgeCounts(m map[spr.EdgeKind]int) []string {
	var out []string
	// In the reference table's order rather than alphabetically, because the
	// order says which edges are containment and which are citation.
	for _, k := range spr.EdgeKinds {
		if n := m[k]; n > 0 {
			out = append(out, fmt.Sprintf("%s %s", group(n), string(k)))
		}
	}
	return out
}

func countLine(parts []string) string {
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}
