// internal/cli/report.go
package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

// ============================================================
// REPORT TYPES (exported to be used by exportReportData)
// ============================================================

// qualityReportItem represents a single item in a quality report.
type qualityReportItem struct {
	Title           string  `json:"title"`
	Year            int     `json:"year"`
	Design          string  `json:"design"`
	CitedBy         int     `json:"cited_by"`
	EvidenceLevel   string  `json:"evidence_level"`
	Score           int     `json:"quality_score"`
	Grade           string  `json:"grade"`
	TrustScore      float64 `json:"trust_score"`
	CitationContext float64 `json:"citation_context"`
	DesignPoints    float64 `json:"design_points"`
	CitationPoints  float64 `json:"citation_points"`
	VenuePoints     float64 `json:"venue_points"`
	SamplePoints    float64 `json:"sample_points"`
	ContextPoints   float64 `json:"context_points"`
	TrustPoints     float64 `json:"trust_points"`
	DOI             string  `json:"doi,omitempty"`
}

// curateReportItem represents a single item in a curate report.
type curateReportItem struct {
	Rank      int     `json:"rank"`
	Title     string  `json:"title"`
	Year      int     `json:"year"`
	DOI       string  `json:"doi"`
	CitedBy   int     `json:"cited_by"`
	Venue     string  `json:"venue"`
	Relevance float64 `json:"relevance"`
}

// ============================================================
// REPORT COMMAND – root
// ============================================================

func newReportCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report <subcommand> <query>",
		Short: "Generate formatted reports (JSON, CSV, XLSX) from analysis results",
		Long: `Generate formatted reports from analysis commands.
Supports three subcommands:
  - report landmark <query>   : Generate report from landmark analysis
  - report quality <query>    : Generate report from quality analysis
  - report curate <query>     : Generate report from curated reading list

Output formats:
  - JSON  : Machine-readable, structured data
  - CSV   : Spreadsheet format with UTF-8 BOM (Excel compatible)
  - XLSX  : Formatted Excel with column widths, wrapping, frozen header, AutoFilter`,
		Annotations: map[string]string{"mcp:read-only": "true"},
	}

	cmd.AddCommand(newReportLandmarkCmd(flags))
	cmd.AddCommand(newReportQualityCmd(flags))
	cmd.AddCommand(newReportCurateCmd(flags))

	return cmd
}

// ============================================================
// REPORT LANDMARK
// ============================================================

func newReportLandmarkCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var yearFrom int
	var minRelevance float64
	var groupByTopic bool
	var format string
	var outputFile string

	cmd := &cobra.Command{
		Use:   "landmark <query>",
		Short: "Generate report from landmark analysis",
		Long: `Generate a formatted report from landmark analysis.
Includes: title, first author, year, DOI, venue, citations, Trust Score,
Citation Context (ctx), design, evidence level (A–E), topic,
supporting/refuting counts, abstract (200 chars), country, open access, PMID, and type.
Use --group to group results by topic.
Use --format to choose output format (json, csv, xlsx).`,
		Example: `  # Excel report with grouping
  scientific-consensus-pp-cli report landmark "long covid treatment" --limit 20 --group --format xlsx --output landmark.xlsx

  # CSV report
  scientific-consensus-pp-cli report landmark "crispr gene editing" --limit 10 --format csv --output landmark.csv

  # JSON report
  scientific-consensus-pp-cli report landmark "long covid treatment" --limit 50 --format json --output landmark.json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if outputFile == "" {
				return fmt.Errorf("--output is required")
			}
			query := args[0]

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			filter := ""
			if yearFrom > 0 {
				filter = fmt.Sprintf("from_publication_date:%d-01-01", yearFrom)
			}
			fetchLimit := limit * 2
			if fetchLimit > 200 {
				fetchLimit = 200
			}

			works, _, err := fetchWorks(ctx, c, query, filter, "cited_by_count:desc", fetchLimit)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			// Filter out RETRACTED
			filteredWorks := []scWork{}
			for _, w := range works {
				if !isRetracted(w) {
					filteredWorks = append(filteredWorks, w)
				}
			}
			works = filteredWorks

			// Detect citation context
			for i := range works {
				ctxVal, summary, err := fetchWeightedCitationContextWithDetails(ctx, c, works[i], query, 30)
				if err == nil && ctxVal != 0.0 {
					works[i].CitationContext = clampContext(ctxVal)
					works[i].CitationSummary = summary
				} else {
					ctxVal2, supporting2, refuting2 := detectContextAwareWithExamples(works[i], query)
					if ctxVal2 != 0.0 {
						works[i].CitationContext = clampContext(ctxVal2)
						works[i].CitationSummary = CitationSummary{Supporting: supporting2, Refuting: refuting2}
					} else {
						works[i].CitationContext = clampContext(DetectCitationContext(works[i]))
						_, supporting3, refuting3 := detectContextAwareWithExamplesFallback(works[i])
						works[i].CitationSummary = CitationSummary{Supporting: supporting3, Refuting: refuting3}
					}
				}
			}

			// Relevance filtering
			filtered := []scWork{}
			for _, wk := range works {
				score := calculateRelevanceScore(query, wk)
				wk.RelevanceScore = score
				if score >= minRelevance {
					filtered = append(filtered, wk)
				}
			}
			works = filtered

			// Trust Score ranking
			currentYear := 2026
			for i := range works {
				works[i].TrustScore = CalculateTrustScore(works[i], currentYear)
			}
			sort.Slice(works, func(i, j int) bool {
				return works[i].TrustScore > works[j].TrustScore
			})
			if len(works) > limit {
				works = works[:limit]
			}

			// Build data and export
			if groupByTopic {
				groups := make(map[string][]scWork)
				for _, wk := range works {
					topic := wk.Topic
					if topic == "" {
						topic = "Uncategorized"
					}
					groups[topic] = append(groups[topic], wk)
				}
				topicNames := make([]string, 0, len(groups))
				for t := range groups {
					topicNames = append(topicNames, t)
				}
				sort.Strings(topicNames)
				type groupItem struct {
					Topic string   `json:"topic"`
					Works []scWork `json:"works"`
				}
				result := make([]groupItem, 0, len(groups))
				for _, t := range topicNames {
					result = append(result, groupItem{Topic: t, Works: groups[t]})
				}
				return exportReportData(outputFile, format, result, "Landmark")
			}

			return exportReportData(outputFile, format, works, "Landmark")
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "number of papers to return (max 200)")
	cmd.Flags().IntVar(&yearFrom, "year-from", 0, "only include works published from this year onward")
	cmd.Flags().Float64Var(&minRelevance, "min-relevance", 60.0, "minimum relevance score (0-100)")
	cmd.Flags().BoolVar(&groupByTopic, "group", false, "group results by topic")
	cmd.Flags().StringVar(&format, "format", "xlsx", "output format: json, csv, xlsx")
	cmd.Flags().StringVar(&outputFile, "output", "", "output file path (required)")

	return cmd
}

// ============================================================
// REPORT QUALITY
// ============================================================

func newReportQualityCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var currentYear int
	var format string
	var outputFile string
	var enrich bool

	cmd := &cobra.Command{
		Use:   "quality <query>",
		Short: "Generate report from quality analysis",
		Long: `Generate a formatted report from quality analysis.
Includes: title, year, citations, design, quality score (0-100),
grade (A-F), evidence level (A-E), Trust Score, Citation Context (ctx),
and Trust Breakdown (Design, Citation, Venue, Sample, Context points).`,
		Example: `  # Excel report
  scientific-consensus-pp-cli report quality "long covid treatment" --limit 20 --format xlsx --output quality.xlsx

  # CSV report
  scientific-consensus-pp-cli report quality "long covid treatment" --limit 50 --format csv --output quality.csv`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if outputFile == "" {
				return fmt.Errorf("--output is required")
			}
			query := args[0]

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			works, _, err := fetchWorks(ctx, c, query, "", "relevance_score:desc", limit)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if enrich {
				enrichPubTypes(ctx, works, 50)
			}

			year := currentYear
			if year == 0 {
				year = 2026
			}

			items := make([]qualityReportItem, 0, len(works))
			for _, wk := range works {
				cls := scengine.ClassifyDesign(wk.Title, wk.Abstract, wk.Type, wk.PubTypes)
				ctxVal := DetectCitationContext(wk)
				trustScore := CalculateTrustScore(wk, year)
				score, designPts, citePts, venuePts, samplePts, contextPts, trustPts := qualityScoreEnhancedWithBreakdown(
					cls.Design, wk.CitedBy, wk.Venue, wk.Abstract, ctxVal, trustScore)
				g := grade(score)
				level := StudyEvidenceLevel(cls.Design)

				items = append(items, qualityReportItem{
					Title:           wk.Title,
					Year:            wk.Year,
					Design:          string(cls.Design),
					CitedBy:         wk.CitedBy,
					EvidenceLevel:   level,
					Score:           score,
					Grade:           g,
					TrustScore:      trustScore,
					CitationContext: ctxVal,
					DesignPoints:    designPts,
					CitationPoints:  citePts,
					VenuePoints:     venuePts,
					SamplePoints:    samplePts,
					ContextPoints:   contextPts,
					TrustPoints:     trustPts,
					DOI:             wk.DOI,
				})
			}

			sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
			return exportReportData(outputFile, format, items, "Quality")
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "number of works to score (max 200)")
	cmd.Flags().BoolVar(&enrich, "enrich", true, "enrich classification with PubMed publication types")
	cmd.Flags().IntVar(&currentYear, "year", 2026, "current year for recency calculation")
	cmd.Flags().StringVar(&format, "format", "xlsx", "output format: json, csv, xlsx")
	cmd.Flags().StringVar(&outputFile, "output", "", "output file path (required)")

	return cmd
}

// ============================================================
// REPORT CURATE
// ============================================================

func newReportCurateCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var minRelevance float64
	var format string
	var outputFile string

	cmd := &cobra.Command{
		Use:   "curate <query>",
		Short: "Generate report from curated reading list",
		Long: `Generate a formatted report from a curated reading list.
Includes: rank, title, year, DOI, citations, venue, and relevance score.`,
		Example: `  # Excel report
  scientific-consensus-pp-cli report curate "long covid treatment" --limit 20 --format xlsx --output curate.xlsx

  # CSV report
  scientific-consensus-pp-cli report curate "long covid treatment" --limit 50 --format csv --output curate.csv`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if outputFile == "" {
				return fmt.Errorf("--output is required")
			}
			query := args[0]

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			fetchLimit := limit * 2
			if fetchLimit > 200 {
				fetchLimit = 200
			}

			works, _, err := fetchWorks(ctx, c, query, "", "cited_by_count:desc", fetchLimit)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			items := make([]curateReportItem, 0, len(works))

			filtered := []scWork{}
			for _, wk := range works {
				score := calculateRelevanceScore(query, wk)
				wk.RelevanceScore = score
				if score >= minRelevance {
					filtered = append(filtered, wk)
				}
			}
			works = filtered

			seen := map[string]bool{}
			for _, wk := range works {
				key := strings.ToLower(wk.DOI)
				if key != "" && seen[key] {
					continue
				}
				seen[key] = true
				items = append(items, curateReportItem{
					Rank:      len(items) + 1,
					Title:     wk.Title,
					Year:      wk.Year,
					DOI:       wk.DOI,
					CitedBy:   wk.CitedBy,
					Venue:     wk.Venue,
					Relevance: wk.RelevanceScore,
				})
			}
			if len(items) > limit {
				items = items[:limit]
			}

			return exportReportData(outputFile, format, items, "Curate")
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "number of works in the list (max 200)")
	cmd.Flags().Float64Var(&minRelevance, "min-relevance", 60.0, "minimum relevance score (0-100)")
	cmd.Flags().StringVar(&format, "format", "xlsx", "output format: json, csv, xlsx")
	cmd.Flags().StringVar(&outputFile, "output", "", "output file path (required)")

	return cmd
}

// ============================================================
// EXPORT HELPER – directly uses WriteJSON, WriteCSV, WriteXLSX
// ============================================================

func exportReportData(outputFile, format string, data interface{}, sheetName string) error {
	// Determine format from flag or extension
	f := format
	if f == "" {
		ext := strings.ToLower(outputFile[strings.LastIndex(outputFile, "."):])
		switch ext {
		case ".json":
			f = "json"
		case ".csv":
			f = "csv"
		case ".xlsx", ".xls":
			f = "xlsx"
		default:
			f = "xlsx"
		}
	}

	switch f {
	case "json":
		return WriteJSON(outputFile, data)
	case "csv":
		// Convert data to CSV rows based on type
		switch v := data.(type) {
		case []scWork:
			return writeLandmarkCSV(outputFile, v)
		case []qualityReportItem:
			return writeQualityCSV(outputFile, v)
		case []curateReportItem:
			return writeCurateCSV(outputFile, v)
		case []struct {
			Topic string   `json:"topic"`
			Works []scWork `json:"works"`
		}:
			return writeGroupedLandmarkCSV(outputFile, v)
		default:
			return fmt.Errorf("CSV export not implemented for this data type")
		}
	case "xlsx", "xls":
		switch v := data.(type) {
		case []scWork:
			return writeLandmarkXLSX(outputFile, sheetName, v)
		case []qualityReportItem:
			return writeQualityXLSX(outputFile, sheetName, v)
		case []curateReportItem:
			return writeCurateXLSX(outputFile, sheetName, v)
		case []struct {
			Topic string   `json:"topic"`
			Works []scWork `json:"works"`
		}:
			return writeGroupedLandmarkXLSX(outputFile, sheetName, v)
		default:
			return fmt.Errorf("XLSX export not implemented for this data type")
		}
	default:
		return fmt.Errorf("unknown format: %s (use json, csv, or xlsx)", f)
	}
}

// ============================================================
// CSV writers for specific data types
// ============================================================

func writeLandmarkCSV(filename string, works []scWork) error {
	// COMPLETE: All available fields
	headers := []string{
		"Title", "FirstAuthor", "Year", "DOI", "Venue",
		"CitedBy", "TrustScore", "CitationContext",
		"Design", "EvidenceLevel", "Topic",
		"Supporting", "Refuting", "Abstract",
		"Country", "IsOA", "PMID", "Type",
	}
	rows := make([][]string, 0, len(works))
	for _, wk := range works {
		design := scengine.ClassifyDesign(wk.Title, wk.Abstract, wk.Type, wk.PubTypes)
		level := StudyEvidenceLevel(design.Design)
		supporting := len(wk.CitationSummary.Supporting)
		refuting := len(wk.CitationSummary.Refuting)
		abstract := wk.Abstract
		if len(abstract) > 200 {
			abstract = abstract[:200] + "..."
		}
		isOA := "false"
		if wk.IsOA {
			isOA = "true"
		}
		rows = append(rows, []string{
			wk.Title,
			wk.FirstAuthor,
			strconv.Itoa(wk.Year),
			wk.DOI,
			wk.Venue,
			strconv.Itoa(wk.CitedBy),
			fmt.Sprintf("%.1f", wk.TrustScore),
			fmt.Sprintf("%.2f", wk.CitationContext),
			string(design.Design),
			level,
			wk.Topic,
			strconv.Itoa(supporting),
			strconv.Itoa(refuting),
			abstract,
			wk.Country,
			isOA,
			wk.PMID,
			wk.Type,
		})
	}
	return WriteCSV(filename, headers, rows)
}

func writeQualityCSV(filename string, items []qualityReportItem) error {
	headers := []string{"Title", "Year", "Design", "CitedBy", "EvidenceLevel", "Score", "Grade", "TrustScore", "CitationContext", "DesignPoints", "CitationPoints", "VenuePoints", "SamplePoints", "ContextPoints", "TrustPoints", "DOI"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.Title,
			strconv.Itoa(it.Year),
			it.Design,
			strconv.Itoa(it.CitedBy),
			it.EvidenceLevel,
			strconv.Itoa(it.Score),
			it.Grade,
			fmt.Sprintf("%.1f", it.TrustScore),
			fmt.Sprintf("%.2f", it.CitationContext),
			fmt.Sprintf("%.1f", it.DesignPoints),
			fmt.Sprintf("%.1f", it.CitationPoints),
			fmt.Sprintf("%.1f", it.VenuePoints),
			fmt.Sprintf("%.1f", it.SamplePoints),
			fmt.Sprintf("%.1f", it.ContextPoints),
			fmt.Sprintf("%.1f", it.TrustPoints),
			it.DOI,
		})
	}
	return WriteCSV(filename, headers, rows)
}

func writeCurateCSV(filename string, items []curateReportItem) error {
	headers := []string{"Rank", "Title", "Year", "DOI", "CitedBy", "Venue", "Relevance"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			strconv.Itoa(it.Rank),
			it.Title,
			strconv.Itoa(it.Year),
			it.DOI,
			strconv.Itoa(it.CitedBy),
			it.Venue,
			fmt.Sprintf("%.1f", it.Relevance),
		})
	}
	return WriteCSV(filename, headers, rows)
}

func writeGroupedLandmarkCSV(filename string, groups []struct {
	Topic string   `json:"topic"`
	Works []scWork `json:"works"`
}) error {
	// COMPLETE: All available fields
	headers := []string{
		"Topic", "Title", "FirstAuthor", "Year", "DOI", "Venue",
		"CitedBy", "TrustScore", "CitationContext",
		"Design", "EvidenceLevel",
		"Supporting", "Refuting", "Abstract",
		"Country", "IsOA", "PMID", "Type",
	}
	rows := make([][]string, 0)
	for _, g := range groups {
		for _, wk := range g.Works {
			design := scengine.ClassifyDesign(wk.Title, wk.Abstract, wk.Type, wk.PubTypes)
			level := StudyEvidenceLevel(design.Design)
			supporting := len(wk.CitationSummary.Supporting)
			refuting := len(wk.CitationSummary.Refuting)
			abstract := wk.Abstract
			if len(abstract) > 200 {
				abstract = abstract[:200] + "..."
			}
			isOA := "false"
			if wk.IsOA {
				isOA = "true"
			}
			rows = append(rows, []string{
				g.Topic,
				wk.Title,
				wk.FirstAuthor,
				strconv.Itoa(wk.Year),
				wk.DOI,
				wk.Venue,
				strconv.Itoa(wk.CitedBy),
				fmt.Sprintf("%.1f", wk.TrustScore),
				fmt.Sprintf("%.2f", wk.CitationContext),
				string(design.Design),
				level,
				strconv.Itoa(supporting),
				strconv.Itoa(refuting),
				abstract,
				wk.Country,
				isOA,
				wk.PMID,
				wk.Type,
			})
		}
	}
	return WriteCSV(filename, headers, rows)
}

// ============================================================
// XLSX writers for specific data types
// ============================================================

func writeLandmarkXLSX(filename, sheetName string, works []scWork) error {
	// COMPLETE: All available fields
	headers := []string{
		"Title", "FirstAuthor", "Year", "DOI", "Venue",
		"CitedBy", "TrustScore", "CitationContext",
		"Design", "EvidenceLevel", "Topic",
		"Supporting", "Refuting", "Abstract",
		"Country", "IsOA", "PMID", "Type",
	}
	rows := make([][]string, 0, len(works))
	for _, wk := range works {
		design := scengine.ClassifyDesign(wk.Title, wk.Abstract, wk.Type, wk.PubTypes)
		level := StudyEvidenceLevel(design.Design)
		supporting := len(wk.CitationSummary.Supporting)
		refuting := len(wk.CitationSummary.Refuting)
		abstract := wk.Abstract
		if len(abstract) > 200 {
			abstract = abstract[:200] + "..."
		}
		isOA := "false"
		if wk.IsOA {
			isOA = "true"
		}
		rows = append(rows, []string{
			wk.Title,
			wk.FirstAuthor,
			strconv.Itoa(wk.Year),
			wk.DOI,
			wk.Venue,
			strconv.Itoa(wk.CitedBy),
			fmt.Sprintf("%.1f", wk.TrustScore),
			fmt.Sprintf("%.2f", wk.CitationContext),
			string(design.Design),
			level,
			wk.Topic,
			strconv.Itoa(supporting),
			strconv.Itoa(refuting),
			abstract,
			wk.Country,
			isOA,
			wk.PMID,
			wk.Type,
		})
	}
	colWidths := map[int]float64{
		0: 50, // Title
		1: 20, // FirstAuthor
		2: 8,  // Year
		3: 30, // DOI
		4: 25, // Venue
		5: 10, // CitedBy
		6: 12, // TrustScore
		7: 12, // CitationContext
		8: 15, // Design
		9: 8,  // EvidenceLevel
		10: 25, // Topic
		11: 10, // Supporting
		12: 10, // Refuting
		13: 60, // Abstract
		14: 15, // Country
		15: 10, // IsOA
		16: 20, // PMID
		17: 12, // Type
	}
	return WriteXLSX(filename, sheetName, headers, rows, colWidths)
}

func writeQualityXLSX(filename, sheetName string, items []qualityReportItem) error {
	headers := []string{"Title", "Year", "Design", "CitedBy", "EvidenceLevel", "Score", "Grade", "TrustScore", "CitationContext", "DesignPoints", "CitationPoints", "VenuePoints", "SamplePoints", "ContextPoints", "TrustPoints", "DOI"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.Title,
			strconv.Itoa(it.Year),
			it.Design,
			strconv.Itoa(it.CitedBy),
			it.EvidenceLevel,
			strconv.Itoa(it.Score),
			it.Grade,
			fmt.Sprintf("%.1f", it.TrustScore),
			fmt.Sprintf("%.2f", it.CitationContext),
			fmt.Sprintf("%.1f", it.DesignPoints),
			fmt.Sprintf("%.1f", it.CitationPoints),
			fmt.Sprintf("%.1f", it.VenuePoints),
			fmt.Sprintf("%.1f", it.SamplePoints),
			fmt.Sprintf("%.1f", it.ContextPoints),
			fmt.Sprintf("%.1f", it.TrustPoints),
			it.DOI,
		})
	}
	colWidths := map[int]float64{
		0: 60, 1: 8, 2: 15, 3: 10, 4: 8, 5: 8, 6: 6, 7: 12, 8: 12,
		9: 12, 10: 12, 11: 10, 12: 10, 13: 10, 14: 10, 15: 30,
	}
	return WriteXLSX(filename, sheetName, headers, rows, colWidths)
}

func writeCurateXLSX(filename, sheetName string, items []curateReportItem) error {
	headers := []string{"Rank", "Title", "Year", "DOI", "CitedBy", "Venue", "Relevance"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			strconv.Itoa(it.Rank),
			it.Title,
			strconv.Itoa(it.Year),
			it.DOI,
			strconv.Itoa(it.CitedBy),
			it.Venue,
			fmt.Sprintf("%.1f", it.Relevance),
		})
	}
	colWidths := map[int]float64{
		0: 6, 1: 60, 2: 8, 3: 30, 4: 10, 5: 25, 6: 12,
	}
	return WriteXLSX(filename, sheetName, headers, rows, colWidths)
}

func writeGroupedLandmarkXLSX(filename, sheetName string, groups []struct {
	Topic string   `json:"topic"`
	Works []scWork `json:"works"`
}) error {
	// COMPLETE: All available fields
	headers := []string{
		"Topic", "Title", "FirstAuthor", "Year", "DOI", "Venue",
		"CitedBy", "TrustScore", "CitationContext",
		"Design", "EvidenceLevel",
		"Supporting", "Refuting", "Abstract",
		"Country", "IsOA", "PMID", "Type",
	}
	rows := make([][]string, 0)
	for _, g := range groups {
		for _, wk := range g.Works {
			design := scengine.ClassifyDesign(wk.Title, wk.Abstract, wk.Type, wk.PubTypes)
			level := StudyEvidenceLevel(design.Design)
			supporting := len(wk.CitationSummary.Supporting)
			refuting := len(wk.CitationSummary.Refuting)
			abstract := wk.Abstract
			if len(abstract) > 200 {
				abstract = abstract[:200] + "..."
			}
			isOA := "false"
			if wk.IsOA {
				isOA = "true"
			}
			rows = append(rows, []string{
				g.Topic,
				wk.Title,
				wk.FirstAuthor,
				strconv.Itoa(wk.Year),
				wk.DOI,
				wk.Venue,
				strconv.Itoa(wk.CitedBy),
				fmt.Sprintf("%.1f", wk.TrustScore),
				fmt.Sprintf("%.2f", wk.CitationContext),
				string(design.Design),
				level,
				strconv.Itoa(supporting),
				strconv.Itoa(refuting),
				abstract,
				wk.Country,
				isOA,
				wk.PMID,
				wk.Type,
			})
		}
	}
	colWidths := map[int]float64{
		0: 25, // Topic
		1: 50, // Title
		2: 20, // FirstAuthor
		3: 8,  // Year
		4: 30, // DOI
		5: 25, // Venue
		6: 10, // CitedBy
		7: 12, // TrustScore
		8: 12, // CitationContext
		9: 15, // Design
		10: 8, // EvidenceLevel
		11: 10, // Supporting
		12: 10, // Refuting
		13: 60, // Abstract
		14: 15, // Country
		15: 10, // IsOA
		16: 20, // PMID
		17: 12, // Type
	}
	return WriteXLSX(filename, sheetName, headers, rows, colWidths)
}