package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// driftCmd represents the drift command
var driftCmd = &cobra.Command{
	Use:   "drift [journal-slug] [window1] [window2]",
	Short: "Compare a journal's topic distribution between two year windows",
	Long: `Drift compares how a journal's topic distribution shifts between two time windows.

This command analyzes the shift in topic distribution between two specified time windows,
helping identify emerging and fading research areas within a journal.

Examples:
  # Compare topic distribution between 2020-2021 and 2022-2023
  thelancet-pp-cli drift flagship 2020-2021 2022-2023

  # Compare two single years
  thelancet-pp-cli drift flagship 2020 2021

  # Output as JSON
  thelancet-pp-cli drift flagship 2020-2021 2022-2023 --json
`,
	Args: cobra.ExactArgs(3),
	RunE: runDrift,
}

func init() {
	rootCmd.AddCommand(driftCmd)
	driftCmd.Flags().Int("limit", 20, "Maximum number of topics to display")
}

func runDrift(cmd *cobra.Command, args []string) error {
	journalSlug := args[0]
	window1 := args[1]
	window2 := args[2]

	// Parse and validate year windows
	w1s, w1e, err := parseYearWindow(window1)
	if err != nil {
		_ = cmd.Usage()
		return fmt.Errorf("invalid window1: %w", err)
	}
	w2s, w2e, err := parseYearWindow(window2)
	if err != nil {
		_ = cmd.Usage()
		return fmt.Errorf("invalid window2: %w", err)
	}

	// CHECK: window2 must start after window1 ends (no overlap)
	if w2s <= w1e {
		return fmt.Errorf("window2 must start after window1 ends: window1 ends %d, window2 starts %d", w1e, w2s)
	}

	// Get journal ISSN from slug
	issn, err := getJournalISSN(journalSlug)
	if err != nil {
		return fmt.Errorf("get journal: %w", err)
	}

	// Ensure store exists and has data
	store, err := ensureLancetStore()
	if err != nil {
		return fmt.Errorf("ensure store: %w", err)
	}
	defer store.Close()

	limit, _ := cmd.Flags().GetInt("limit")

	// Query drift data
	drift, err := store.TopicDrift(cmd.Context(), issn, w1s, w1e, w2s, w2e, limit)
	if err != nil {
		return fmt.Errorf("query drift: %w", err)
	}

	// Output results
	if len(drift) == 0 {
		fmt.Println("No topic drift data found for the specified windows.")
		return nil
	}

	output, err := formatOutput(cmd, drift)
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}
	fmt.Print(output)

	return nil
}

// yearLowerBound returns the starting year for a given number of years back from current.
func yearLowerBound(years int) int {
	currentYear := time.Now().Year()
	return currentYear - years + 1
}

// getJournalISSN returns the ISSN for a given journal slug.
func getJournalISSN(slug string) (string, error) {
	// This is a simplified version - in real implementation, this would look up from journals registry
	journalMap := map[string]string{
		"flagship": "0140-6736",    // The Lancet
		"oncology": "1470-2045",    // Lancet Oncology
		"neurology": "1474-4422",   // Lancet Neurology
		"infectious": "1473-3099",  // Lancet Infectious Diseases
		"respiratory": "2213-2600", // Lancet Respiratory Medicine
		"psychiatry": "2215-0366",  // Lancet Psychiatry
		"child": "2352-4642",       // Lancet Child & Adolescent Health
		"healthy": "2468-2667",     // Lancet Healthy Longevity
		"microbe": "2666-5247",     // Lancet Microbe
		"rheumatology": "2665-9913", // Lancet Rheumatology
		"regional": "2666-5352",     // Lancet Regional Health
		"digital": "2589-7500",      // Lancet Digital Health
		"planetary": "2542-5196",    // Lancet Planetary Health
	}
	
	if issn, ok := journalMap[slug]; ok {
		return issn, nil
	}
	return "", fmt.Errorf("unknown journal slug: %s", slug)
}