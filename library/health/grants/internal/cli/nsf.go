package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/health/grants/internal/sources"
)

func cmdNSF(args []string) int {
	fs := flag.NewFlagSet("nsf", flag.ContinueOnError)
	minAmount := fs.Int64("min-amount", 0, "min. amount USD")
	rows := fs.Int("rows", 15, "number of results (max 25)")
	asJSON := fs.Bool("json", false, "JSON output")
	pos, err := parseFlexible(fs, args)
	if err != nil {
		return 2
	}
	keyword := strings.Join(pos, " ")
	if keyword == "" {
		fmt.Fprintln(os.Stderr, "a keyword is required: grants-pp-cli nsf <keyword>")
		return 2
	}

	awards, stats, err := sources.SearchNSF(keyword, *rows)
	if err != nil {
		return fail(err)
	}
	shownBeforeAmount := len(awards)
	if *minAmount > 0 {
		var kept []sources.NSFAward
		for _, a := range awards {
			if sources.ParseMoney(a.FundsObligated) >= *minAmount {
				kept = append(kept, a)
			}
		}
		awards = kept
		// The amount filter runs on the rows we are about to show, not on the
		// whole candidate pool. When those rows filled the requested page,
		// matching awards below the cut almost certainly exist.
		if shownBeforeAmount == *rows && stats.Matched > *rows {
			fmt.Fprintf(os.Stderr,
				"  (warn: --min-amount applies only to the top %d ranked NSF results of %d matches; increase --rows to fetch more)\n",
				*rows, stats.Matched)
		}
	}

	if *asJSON {
		return printJSON(map[string]any{
			"keyword":   keyword,
			"shown":     len(awards),
			"awards":    awards,
			"relevance": stats,
		})
	}

	fmt.Printf("🔬 NSF %q — %d awarded grants shown\n", keyword, len(awards))
	for _, a := range awards {
		fmt.Printf("  %-9s %12s  %-10s→%-10s %-26s %s\n",
			a.ID, FormatMoney(sources.ParseMoney(a.FundsObligated)),
			a.StartDate, a.ExpDate, truncate(a.Awardee, 26), truncate(a.Title, 48))
	}
	if len(awards) == 0 {
		fmt.Println("  (no results)")
	}
	fmt.Printf("  searched %d recent NSF awards; %d mention every word of your query, %d in the title\n",
		stats.Examined, stats.Matched, stats.TitleMatched)
	if stats.Matched < 10 {
		fmt.Println("  few awards use all of these words together — try fewer or broader words (e.g. one topic word instead of two)")
	}
	return 0
}
