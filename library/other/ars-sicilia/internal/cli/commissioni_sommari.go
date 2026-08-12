// pp:client-call
package cli

import "github.com/spf13/cobra"

func newCommissioniSommariCmd(flags *rootFlags) *cobra.Command {
	var (
		flagLegisl   int
		flagAnno     int
		flagCodcom   string
		flagCommis   string
		flagData     string
		flagPresid   string
		flagArgom    string
		flagTesto    string
		flagFrase    string
		flagISIS     string
		flagLimit    int
		flagMaxPages int
	)
	cmd := &cobra.Command{
		Use:         "sommari",
		Args:        rejectPositionalArgs,
		Short:       "Sommari dei lavori delle Commissioni.",
		Example:     "  ars-sicilia-pp-cli commissioni sommari --legisl 18 --commissione \"Bilancio\" --json",
		Annotations: map[string]string{"pp:endpoint": "commissioni.sommari", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			if flagLegisl != 0 {
				params["legisl"] = itoa(flagLegisl)
			}
			if flagAnno != 0 {
				params["anno"] = itoa(flagAnno)
			}
			if flagCodcom != "" {
				params["codcom"] = flagCodcom
			}
			if flagCommis != "" {
				params["commissione"] = flagCommis
			}
			if flagData != "" {
				params["data"] = flagData
			}
			if flagPresid != "" {
				params["presidente"] = flagPresid
			}
			if flagArgom != "" {
				params["testo"] = flagArgom
			}
			if flagTesto != "" {
				params["testo"] = flagTesto
			}
			if flagFrase != "" {
				params["frase"] = flagFrase
			}
			return runCerca(cmd, flags, "sommari", cercaParams{
				Params: params, ISISRaw: flagISIS,
				Limit: flagLimit, MaxPages: flagMaxPages,
			})
		},
	}
	cmd.Flags().IntVar(&flagLegisl, "legisl", 0, "Legislatura.")
	cmd.Flags().IntVar(&flagAnno, "anno", 0, "Anno della seduta (es. 2026). Filtro nativo del backend /bd/.")
	cmd.Flags().StringVar(&flagCodcom, "codcom", "", "Codice commissione 1-6 (PRIMA..SESTA); in alternativa usa --commissione.")
	cmd.Flags().StringVar(&flagCommis, "commissione", "", "Nome commissione.")
	cmd.Flags().StringVar(&flagData, "data", "", "Data seduta (YYYY-MM-DD; range con YYYY-MM-DD:YYYY-MM-DD).")
	cmd.Flags().StringVar(&flagPresid, "presidente", "", "Nome del presidente di seduta.")
	cmd.Flags().StringVar(&flagArgom, "argomento", "", "Argomento (free-text).")
	cmd.Flags().StringVar(&flagTesto, "testo", "", "Ricerca testuale.")
	cmd.Flags().StringVar(&flagFrase, "frase", "", "Cerca le parole come locuzione, adiacenti e nell'ordine dato (ISIS adj). Piu' preciso di --testo, che combina le parole in AND sull'intero documento: --testo \"aree idonee\" aggancia anche chi ha le due parole in articoli diversi.")
	cmd.Flags().StringVar(&flagISIS, "isis-query", "", "Espressione ISIS grezza (escape hatch).")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Max risultati da scaricare.")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", 0, "Pagine massime (0 = auto).")
	cmd.Flags().String("escludi", "", "Escludi i documenti che contengono questo termine (ISIS NOT).")
	return cmd
}
