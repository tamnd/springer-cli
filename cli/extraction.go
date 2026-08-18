package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tamnd/springer-cli/spr"
)

func extractionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extraction [field]",
		Short: "Print the extraction table: every field, the rung that answers it, and why",
		Long: "extraction prints the table this tool extracts by. Every field has a row, and the row\n" +
			"says which rung is expected to answer it, which exact tag or region that is, and why it\n" +
			"sits on that rung rather than a lower one.\n\n" +
			"There are four rungs, tried in order, and the first that answers wins. Highwire meta tags\n" +
			"come first because Google Scholar reads them and a publisher who breaks them hears about\n" +
			"it. Then schema.org JSON-LD, then Springer's own data-test and data-title regions, then\n" +
			"css class names, which are presentational and change with a redesign.\n\n" +
			"Authors are the one deliberate exception and come from rung 2. Highwire emits three\n" +
			"parallel arrays and lines them up by position, which breaks silently the moment one\n" +
			"author has two affiliations. JSON-LD binds the three to the right person.\n\n" +
			"Every row names the record it belongs to, because the same field name means different\n" +
			"things on different pages. Title on a work comes from citation_title on rung 1, and title\n" +
			"on a journal comes from an analytics payload on rung 3, because a journal home page ships\n" +
			"no bibliographic meta tags at all.\n\n" +
			"Read this when a field on a record came from somewhere you did not expect, or before\n" +
			"asking whether the page moved or the extractor was always wrong.",
		Args: cobra.MaximumNArgs(1),
		Example: "  spr extraction\n" +
			"  spr extraction authors\n" +
			"  spr extraction journal.title\n" +
			"  spr extraction --record book\n" +
			"  spr extraction --rung highwire\n" +
			"  spr extraction -o json | jq '.[] | select(.rung == 4)'",
	}

	var rung, record string
	cmd.Flags().StringVar(&rung, "rung", "", "only the fields answered by one rung: highwire, linkdata, region or selector")
	cmd.Flags().StringVar(&record, "record", "", "only the fields of one record: "+strings.Join(spr.Records(), ", "))

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		fields := spr.Fields

		if rung != "" {
			l, err := parseLevel(rung)
			if err != nil {
				return exit(CodeUsage, err)
			}
			fields = spr.FieldsAt(l)
		}
		if record != "" {
			fields = keepRecord(fields, record)
			if len(fields) == 0 {
				return exit(CodeUsage, fmt.Errorf("--record %q is not one of %s", record, strings.Join(spr.Records(), ", ")))
			}
		}
		if len(args) > 0 {
			named := spr.FieldsNamed(args[0])
			if len(named) == 0 {
				return exit(CodeUsage, fmt.Errorf("there is no field called %q in the extraction table", args[0]))
			}
			fields = keepNamed(fields, named)
		}
		if len(fields) == 0 {
			return exit(CodeNoData, fmt.Errorf("nothing in the table matches those filters together"))
		}

		out := cmd.OutOrStdout()
		if g.format == "json" {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(fields); err != nil {
				return exit(CodeTransport, err)
			}
			return nil
		}

		// One field asked about by name gets the long form, because the question
		// was about that field and there is room to answer it properly.
		if len(fields) == 1 && len(args) > 0 {
			f := fields[0]
			fmt.Fprintf(out, "field   %s\n", f.Qualified())
			fmt.Fprintf(out, "rung    %d, %s\n", int(f.Rung), f.Rung)
			fmt.Fprintf(out, "source  %s\n", f.Source)
			fmt.Fprintf(out, "why     %s\n", f.Why)
			return nil
		}

		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "RECORD\tFIELD\tRUNG\tSOURCE\tWHY")
		for _, f := range fields {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", f.Record, f.Name, f.Rung, f.Source, f.Why)
		}
		return tw.Flush()
	}
	return cmd
}

// keepRecord narrows a row set to one record.
func keepRecord(in []spr.Field, record string) []spr.Field {
	var out []spr.Field
	for _, f := range in {
		if f.Record == record {
			out = append(out, f)
		}
	}
	return out
}

// keepNamed intersects a row set with the rows matching a name, so that
// --record and a field name asked about together mean both rather than the
// last one given.
func keepNamed(in, named []spr.Field) []spr.Field {
	want := map[string]bool{}
	for _, f := range named {
		want[f.Qualified()] = true
	}
	var out []spr.Field
	for _, f := range in {
		if want[f.Qualified()] {
			out = append(out, f)
		}
	}
	return out
}

// parseLevel reads a rung by the name the envelope writes it under, so that a
// name copied out of a record's via field can be pasted straight back in here.
func parseLevel(s string) (spr.Level, error) {
	for _, l := range []spr.Level{spr.LevelHighwire, spr.LevelLinkedData, spr.LevelRegion, spr.LevelSelector} {
		if strings.EqualFold(s, l.String()) {
			return l, nil
		}
	}
	return spr.LevelNone, fmt.Errorf("--rung %q is not one of highwire, linkdata, region or selector", s)
}
