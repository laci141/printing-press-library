// pp:client-call
// Replaces generator-emitted stub: real implementation in internal/icaroclient.

package cli

import "github.com/spf13/cobra"

func newResocontiCercaCmd(flags *rootFlags) *cobra.Command {
	var (
		flagLegisl    int
		flagAnno      int
		flagData      string
		flagNumero    int
		flagOratore   string
		flagArgomento string
		flagTesto     string
		flagFrase     string
		flagISIS      string
		flagLimit     int
		flagMaxPages  int
	)

	cmd := &cobra.Command{
		Use:     "cerca",
		Args:    rejectPositionalArgs,
		Short:   "Cerca resoconti delle sedute d'aula per data, oratore o argomento.",
		Example: "  ars-sicilia-pp-cli resoconti cerca --legisl 18 --json",
		Annotations: map[string]string{
			"pp:endpoint":   "resoconti.cerca",
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]string{}
			if flagLegisl != 0 {
				params["legisl"] = itoa(flagLegisl)
			}
			if flagAnno != 0 {
				params["anno"] = itoa(flagAnno)
			}
			if flagData != "" {
				params["data"] = flagData
			}
			if flagNumero != 0 {
				params["numero"] = itoa(flagNumero)
			}
			if flagOratore != "" {
				params["oratore"] = flagOratore
			}
			if flagArgomento != "" {
				params["argomento"] = flagArgomento
			}
			if flagTesto != "" {
				params["testo"] = flagTesto
			}
			if flagFrase != "" {
				params["frase"] = flagFrase
			}
			return runCerca(cmd, flags, "resoconti", cercaParams{
				Params: params, ISISRaw: flagISIS,
				Limit: flagLimit, MaxPages: flagMaxPages,
			})
		},
	}
	cmd.Flags().IntVar(&flagLegisl, "legisl", 0, "Legislatura.")
	cmd.Flags().IntVar(&flagAnno, "anno", 0, "Anno della seduta.")
	cmd.Flags().StringVar(&flagData, "data", "", "Data seduta (YYYY-MM-DD; range con YYYY-MM-DD:YYYY-MM-DD).")
	cmd.Flags().IntVar(&flagNumero, "numero", 0, "Numero seduta.")
	cmd.Flags().StringVar(&flagOratore, "oratore", "", "Cognome/nome dell'oratore: filtra le sedute in cui è intervenuto (risolto sull'anagrafica del portale; se combinato con --legisl considera solo chi vi è attivo).")
	cmd.Flags().StringVar(&flagArgomento, "argomento", "", "Argomento.")
	cmd.Flags().StringVar(&flagTesto, "testo", "", "Ricerca testuale.")
	cmd.Flags().StringVar(&flagFrase, "frase", "", "Cerca le parole come locuzione, adiacenti e nell'ordine dato (ISIS adj). Piu' preciso di --testo, che combina le parole in AND sull'intero documento: --testo \"aree idonee\" aggancia anche chi ha le due parole in articoli diversi.")
	cmd.Flags().StringVar(&flagISIS, "isis-query", "", "Espressione ISIS grezza (escape hatch).")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Max risultati da scaricare.")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", 0, "Pagine massime da scaricare (0 = auto da --limit).")
	cmd.Flags().String("escludi", "", "Escludi i documenti che contengono questo termine (ISIS NOT).")
	return cmd
}
