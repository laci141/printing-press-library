package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/health/grants/internal/sources"
)

func cmdNIH(args []string) int {
	fs := flag.NewFlagSet("nih", flag.ContinueOnError)
	minAmount := fs.Int64("min-amount", 0, "min. award USD")
	year := fs.Int("year", 0, "fiscal year, e.g. 2025")
	rows := fs.Int("rows", 15, "number of results")
	asJSON := fs.Bool("json", false, "JSON output")
	pos, err := parseFlexible(fs, args)
	if err != nil {
		return 2
	}
	keyword := strings.Join(pos, " ")
	if keyword == "" {
		fmt.Fprintln(os.Stderr, "a keyword is required: grants-pp-cli nih <keyword>")
		return 2
	}

	projects, total, err := sources.SearchNIH(keyword, *year, *rows)
	if err != nil {
		return fail(err)
	}
	if *minAmount > 0 {
		var kept []sources.NIHProject
		for _, p := range projects {
			if int64(p.AwardAmount) >= *minAmount {
				kept = append(kept, p)
			}
		}
		projects = kept
	}

	if *asJSON {
		return printJSON(map[string]any{"keyword": keyword, "total": total, "shown": len(projects), "projects": projects})
	}

	fmt.Printf("🏥 NIH RePORTER %q — %d awarded projects total, %d shown (sorted by award, descending)\n", keyword, total, len(projects))
	for _, p := range projects {
		fmt.Printf("  %-16s %12s  FY%-5d %-28s %s\n",
			p.ProjectNum, FormatMoney(int64(p.AwardAmount)), p.FiscalYear,
			truncate(p.Org.Name, 28), truncate(p.Title, 55))
	}
	if len(projects) == 0 {
		fmt.Println("  (no results)")
	}
	return 0
}
