// pp:client-call
// pp:data-source live
// Novel feature: assemble N provvedimenti on a theme into a folder of clean
// Markdown files plus a CSV manifest (ECLI, tipo, sede, data, url, file).
package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/printing-press-library/library/productivity/giustizia-amministrativa/internal/gaclient"
)

var reUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func newNovelCorpusBuildCmd(flags *rootFlags) *cobra.Command {
	var f searchFlags
	var out string
	var skipExisting, plan, frontMatter bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Assembla N provvedimenti su un tema in Markdown + un CSV manifest.",
		Long: "Esegue una ricerca, scarica il testo integrale di ogni provvedimento in Markdown e scrive\n" +
			"una cartella con un file .md per provvedimento più un manifest.csv (ECLI, tipo, sede, data, url).\n" +
			"Con --skip-existing i provvedimenti il cui .md è già presente in --out non vengono riscaricati\n" +
			"(corpus incrementale), restando comunque elencati nel manifest. Con --plan non scarica nulla:\n" +
			"scrive solo il manifest delle candidate, per rivedere la query prima di impegnare N richieste.\n" +
			"Con --front-matter l'intestazione di ogni .md è il blocco YAML di `get --front-matter`\n" +
			"invece dell'intestazione Markdown predefinita.",
		Example: strings.Trim(`
  giustizia-amministrativa-pp-cli corpus build --testo "soccorso istruttorio" --tipo sentenza --limit 3 --out ./corpus
  giustizia-amministrativa-pp-cli corpus build --all "clausola sociale" --sede roma --limit 20 --out ./clausola-sociale
  giustizia-amministrativa-pp-cli corpus build --all "accesso generalizzato" --plan --out ./corpus
  giustizia-amministrativa-pp-cli corpus build --all "accesso generalizzato" --skip-existing --front-matter --out ./corpus`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if gaSkip(flags) {
				return nil
			}
			if out == "" {
				return fmt.Errorf("specifica la cartella di destinazione con --out")
			}
			opts := f.opts("")
			if !hasAnySearchInput(opts) {
				return fmt.Errorf("specifica almeno un criterio di ricerca (--testo, --all, --tipo, --sede, ...)")
			}
			if opts.Limit == 0 {
				opts.Limit = 25
			}
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			c := gaclient.New()
			res, err := c.Search(cmd.Context(), opts)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			for _, w := range res.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "Attenzione: %s\n", w)
			}
			st, _ := openGAStore(cmd.Context())
			if st != nil {
				defer st.Close()
			}

			manifestPath := filepath.Join(out, "manifest.csv")
			mf, err := os.Create(manifestPath)
			if err != nil {
				return err
			}
			defer mf.Close()
			w := csv.NewWriter(mf)
			_ = w.Write([]string{"ecli", "tipo", "sede", "sezione", "anno", "numero", "nrg", "data_deposito", "url", "file"})

			type built struct {
				Ecli string `json:"ecli"`
				File string `json:"file"`
				URL  string `json:"url"`
			}
			manifestRow := func(p gaclient.Provvedimento, fname string) {
				_ = w.Write([]string{p.Ecli, p.Tipo, p.Sede, p.Sezione, strconv.Itoa(p.Anno), strconv.Itoa(p.Numero), p.Nrg, p.DataDeposito, p.URL, fname})
			}

			var summary []built
			var skipped, planned int
			for _, p := range res.Items {
				fname := sanitizeFilename(provID(p)) + ".md"
				fpath := filepath.Join(out, fname)

				// --plan: list candidates only, no document fetch, no .md written.
				if plan {
					manifestRow(p, fname)
					summary = append(summary, built{Ecli: p.Ecli, File: fname, URL: p.URL})
					planned++
					continue
				}
				// --skip-existing: already-downloaded provvedimenti stay in the
				// manifest (from search metadata) but are not refetched.
				if skipExisting {
					if _, statErr := os.Stat(fpath); statErr == nil {
						manifestRow(p, fname)
						summary = append(summary, built{Ecli: p.Ecli, File: fname, URL: p.URL})
						skipped++
						continue
					}
				}

				docHTML, ferr := c.FullText(cmd.Context(), p)
				if ferr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "salto %s: %v\n", provID(p), ferr)
					continue
				}
				if p.DataDeposito == "" {
					p.DataDeposito = gaclient.ExtractDataDeposito(docHTML)
				}
				md := gaclient.HTMLToMarkdown(docHTML)
				p.FullText = md
				if st != nil {
					persistProvvedimenti(st, []gaclient.Provvedimento{p})
				}
				var header string
				if frontMatter {
					header = gaclient.FrontMatter(p) + "\n"
				} else {
					header = fmt.Sprintf("# %s\n\n- Tipo: %s\n- Sede: %s %s\n- Data deposito: %s\n- NRG: %s\n- URL: %s\n\n---\n\n",
						provID(p), p.Tipo, p.Sede, p.Sezione, p.DataDeposito, p.Nrg, p.URL)
				}
				if werr := os.WriteFile(fpath, []byte(header+md+"\n"), 0o644); werr != nil {
					return werr
				}
				manifestRow(p, fname)
				summary = append(summary, built{Ecli: p.Ecli, File: fname, URL: p.URL})
			}
			w.Flush()
			if err := w.Error(); err != nil {
				return fmt.Errorf("scrittura manifest CSV: %w", err)
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				switch {
				case plan:
					fmt.Fprintf(cmd.ErrOrStderr(), "Piano: %d candidate elencate in %s (nessun download).\n", planned, manifestPath)
				case skipped > 0:
					fmt.Fprintf(cmd.ErrOrStderr(), "Corpus aggiornato in %s: %d provvedimenti (%d già presenti, saltati), manifest %s.\n", out, len(summary), skipped, manifestPath)
				default:
					fmt.Fprintf(cmd.ErrOrStderr(), "Corpus creato in %s: %d provvedimenti, manifest %s.\n", out, len(summary), manifestPath)
				}
			}
			result := map[string]any{
				"out": out, "manifest": manifestPath, "count": len(summary),
				"skipped": skipped, "plan": plan,
				"generated_at": time.Now().UTC().Format(time.RFC3339), "items": summary,
			}
			data, _ := json.Marshal(result)
			return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
		},
	}
	addSearchFlags(cmd, &f)
	cmd.Flags().StringVar(&out, "out", "", "Cartella di destinazione del corpus (richiesto).")
	cmd.Flags().BoolVar(&skipExisting, "skip-existing", false, "Non riscaricare i provvedimenti il cui .md è già presente in --out.")
	cmd.Flags().BoolVar(&plan, "plan", false, "Non scaricare: elenca solo le candidate nel manifest, per revisione.")
	cmd.Flags().BoolVar(&frontMatter, "front-matter", false, "Usa il blocco YAML (come `get --front-matter`) come intestazione dei .md, invece dell'header Markdown predefinito.")
	return cmd
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, ":", "_")
	s = reUnsafe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		s = "provvedimento"
	}
	return s
}
