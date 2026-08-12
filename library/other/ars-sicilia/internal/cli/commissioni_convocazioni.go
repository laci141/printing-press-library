// pp:client-call
package cli

import "github.com/spf13/cobra"

func newCommissioniConvocazioniCmd(flags *rootFlags) *cobra.Command {
	var (
		flagLegisl   int
		flagAnno     int
		flagCodcom   string
		flagCommis   string
		flagData     string
		flagISIS     string
		flagLimit    int
		flagMaxPages int
	)
	cmd := &cobra.Command{
		Use:         "convocazioni",
		Args:        rejectPositionalArgs,
		Short:       "Convocazioni delle Commissioni.",
		Example:     "  ars-sicilia-pp-cli commissioni convocazioni --legisl 18 --codcom 5 --json",
		Annotations: map[string]string{"pp:endpoint": "commissioni.convocazioni", "mcp:read-only": "true"},
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
			return runCerca(cmd, flags, "convocazioni", cercaParams{
				Params: params, ISISRaw: flagISIS,
				Limit: flagLimit, MaxPages: flagMaxPages,
			})
		},
	}
	cmd.Flags().IntVar(&flagLegisl, "legisl", 0, "Legislatura.")
	cmd.Flags().IntVar(&flagAnno, "anno", 0, "Anno della convocazione (es. 2026). Filtro nativo del backend /bd/.")
	cmd.Flags().StringVar(&flagCodcom, "codcom", "", "Codice commissione 1-6 (I..VI); in alternativa usa --commissione. Usare con --legisl (gli id commissione sono per-legislatura).")
	cmd.Flags().StringVar(&flagCommis, "commissione", "", "Nome commissione.")
	cmd.Flags().StringVar(&flagData, "data", "", "Data seduta (YYYY-MM-DD; range con YYYY-MM-DD:YYYY-MM-DD).")
	cmd.Flags().StringVar(&flagISIS, "isis-query", "", "Espressione ISIS grezza (escape hatch).")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Max risultati da scaricare.")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", 0, "Pagine massime (0 = auto).")
	cmd.Flags().String("escludi", "", "Escludi i documenti che contengono questo termine (ISIS NOT).")
	return cmd
}
