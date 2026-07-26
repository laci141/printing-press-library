package scengine

import "testing"

// retractedFixture is one work that a live production run actually pulled into
// the scored corpus, together with the values that run measured for it.
//
// Provenance: /tmp/sc.exe consensus <claim> --limit <n> --full-abstracts
// --data-source live --json, keyless (stance_method: heuristic), 2026-07-26.
// Retraction status cross-checked per work; see wantRetracted comments.
type retractedFixture struct {
	name string
	// claim is the claim the production run was scoring.
	claim string
	// title and abstract are verbatim from that run's all_studies entry.
	title    string
	abstract string
	// doi, citedBy and design are the same run's measured values.
	doi     string
	citedBy int
	design  Design
	// titlePrefix is whether the publisher's retraction marker survives in the
	// title OpenAlex serves. openAlexFlag is that work's is_retracted value,
	// fetched separately because the production select list omits the field.
	titlePrefix  bool
	openAlexFlag bool
	// wantStance and wantConf are what THIS engine produced. Recording them is
	// the point: they are the harm, not the goal.
	wantStance Stance
	wantConf   float64
}

var retractedFixtures = []retractedFixture{
	{
		name:  "vitaminC_metaanalysis_no_title_marker",
		claim: "vitamin C prevents the common cold",
		title: "Extra Dose of Vitamin C Based on a Daily Supplementation Shortens the Common Cold: " +
			"A Meta-Analysis of 9 Randomized Controlled Trials",
		abstract: "AIM: To investigate whether vitamin C is effective in the treatment of the common cold. " +
			"METHOD: After systematically searching the National Library of Medicine (PubMed), Cochrane Library, " +
			"Elsevier, China National Knowledge Infrastructure (CNKI), VIP databases, and WANFANG databases, " +
			"9 randomized placebo-controlled trials were included in our meta-analysis in RevMan 5.3 software, " +
			"all of which were in English. RESULTS: In the evaluation of vitamin C, administration of extra " +
			"therapeutic doses at the onset of cold despite routine supplementation was found to help reduce its " +
			"duration (mean difference (MD) = -0.56, 95% confidence interval (CI) [-1.03, -0.10], and P = 0.02), " +
			"shorten the time of confinement indoors (MD = -0.41, 95% CI [-0.62, -0.19], and P = 0.0002), and " +
			"relieve the symptoms associated with it, including chest pain (MD = -0.40, 95% CI [-0.77, -0.03], " +
			"and P = 0.03), fever (MD = -0.45, 95% CI [-0.78, -0.11], and P = 0.009), and chills (MD = -0.36, " +
			"95% CI [-0.65, -0.07], and P = 0.01). CONCLUSIONS: Extra doses of vitamin C could benefit some " +
			"patients who contract the common cold despite taking daily vitamin C supplements.",
		// Retracted 2023-04-10, notice DOI 10.1155/2023/9848057: the placebo arm
		// was double-counted in multi-arm trials, and correcting it removes
		// statistical significance in at least some outcomes. The retraction is
		// about the effect not being real — exactly what this corpus scored.
		doi:          "https://doi.org/10.1155/2018/1837634",
		citedBy:      100,
		design:       DesignMetaAnalysis,
		titlePrefix:  false,
		openAlexFlag: true,
		wantStance:   StanceSupporting,
		wantConf:     0.8999999999999999,
	},
	{
		name:  "covid_greenfoods_prefix_and_flag",
		claim: "vitamin C prevents the common cold",
		title: "RETRACTED: Coronavirus disease (COVID\u201019) and immunity booster green foods: A mini review",
		abstract: "This review focused on the use of plant-based foods for enhancing the immunity of all aged " +
			"groups against COVID-19. In humans, coronaviruses are included in the spectrum of viruses that cause " +
			"the common cold and, recently, severe acute respiratory syndrome (SARS). Emerging infectious diseases, " +
			"such as SARS present a major threat to public health. The novel coronavirus has spread rapidly to " +
			"multiple countries and has been declared a pandemic by the World Health Organization. COVID-19 is " +
			"usually caused a virus to which most probably the people with low immunity response are being affected. " +
			"Plant-based foods increased the intestinal beneficial bacteria which are helpful and make up of 85% of " +
			"the immune system. By the use of plenty of water, minerals like magnesium and Zinc, micronutrients, " +
			"herbs, food rich in vitamins C, D and E, and better life style one can promote the health and can " +
			"overcome this infection. Various studies investigated that a powerful antioxidant glutathione and a " +
			"bioflavonoid quercetin may prevent various infections including COVID-19. In conclusion, the " +
			"plant-based foods play a vital role to enhance the immunity of people to control of COVID-19.",
		doi:          "https://doi.org/10.1002/fsn3.1719",
		citedBy:      154,
		design:       DesignUnknown,
		titlePrefix:  true,
		openAlexFlag: true,
		wantStance:   StanceSupporting,
		wantConf:     0.95,
	},
	{
		name:  "fasting_natcomm_prefix_and_flag",
		claim: "intermittent fasting extends lifespan",
		title: "RETRACTED ARTICLE: Fasting inhibits aerobic glycolysis and proliferation in colorectal cancer " +
			"via the Fdft1-mediated AKT/mTOR/HIF1\u03b1 pathway suppression",
		abstract: "Evidence suggests that fasting exerts extensive antitumor effects in various cancers, " +
			"including colorectal cancer (CRC). However, the mechanism behind this response is unclear. We " +
			"investigate the effect of fasting on glucose metabolism and malignancy in CRC. We find that fasting " +
			"upregulates the expression of a cholesterogenic gene, Farnesyl-Diphosphate Farnesyltransferase 1 " +
			"(FDFT1), during the inhibition of CRC cell aerobic glycolysis and proliferation. In addition, the " +
			"downregulation of FDFT1 is correlated with malignant progression and poor prognosis in CRC. Moreover, " +
			"FDFT1 acts as a critical tumor suppressor in CRC. Mechanistically, FDFT1 performs its tumor-inhibitory " +
			"function by negatively regulating AKT/mTOR/HIF1\u03b1 signaling. Furthermore, mTOR inhibitor can " +
			"synergize with fasting in inhibiting the proliferation of CRC. These results indicate that FDFT1 is a " +
			"key downstream target of the fasting response and may be involved in CRC cell glucose metabolism. Our " +
			"results suggest therapeutic implications in CRC and potential crosstalk between a cholesterogenic gene " +
			"and glycolysis.",
		doi:          "https://doi.org/10.1038/s41467-020-15795-8",
		citedBy:      253,
		design:       DesignUnknown,
		titlePrefix:  true,
		openAlexFlag: true,
		wantStance:   StanceInconclusive,
		wantConf:     0.2,
	},
}

// TestRetractedFixturesReproduceCurrentBehaviour pins what this engine does
// TODAY with three works that a live run really scored. It is a positive
// control for the fixture, not a statement that the behaviour is correct: if
// this test ever fails, the fixture stopped reproducing the bug and every
// measurement built on it is void.
func TestRetractedFixturesReproduceCurrentBehaviour(t *testing.T) {
	for _, f := range retractedFixtures {
		t.Run(f.name, func(t *testing.T) {
			got, conf := ClassifyStance(f.title, f.abstract, f.claim)
			if got != f.wantStance {
				t.Errorf("stance = %q, a mert futas %q volt — a fixture elavult", got, f.wantStance)
			}
			if conf != f.wantConf {
				t.Errorf("confidence = %v, a mert futas %v volt", conf, f.wantConf)
			}
		})
	}
}

// TestTitleMarkerAloneMissesRetractions is the measurement that decides the
// design. Two of the three works carry the publisher's marker in the title
// OpenAlex serves; the third — the only meta-analysis, i.e. the highest
// evidence tier in the corpus — carries none. A title-only rule would score it
// as supporting evidence.
func TestTitleMarkerAloneMissesRetractions(t *testing.T) {
	var withMarker, flaggedOnly int
	for _, f := range retractedFixtures {
		if !f.openAlexFlag {
			t.Fatalf("%s: fixture hiba — minden fixture visszavont kell legyen", f.name)
		}
		if f.titlePrefix {
			withMarker++
		} else {
			flaggedOnly++
		}
	}
	if flaggedOnly == 0 {
		t.Fatalf("a fixture nem tartalmaz cim-jelolo nelkuli visszavont muvet, "+
			"pedig a mert futas tartalmazott (%d/%d)", 1, len(retractedFixtures))
	}
	t.Logf("MERT: cim-jelolovel %d, csak is_retracted flaggel %d (osszesen %d)",
		withMarker, flaggedOnly, len(retractedFixtures))
}

// TestRetractedWorkMovesTheScore quantifies the damage on the real numbers: the
// retracted meta-analysis is dropped from a small but faithful corpus and the
// verdict is recomputed. Whatever moves here is what a reader of
// corpova.pubvera.com is being shown on the strength of a retracted paper.
func TestRetractedWorkMovesTheScore(t *testing.T) {
	// Three real non-retracted works from the same run (the negative-control
	// block of the fixture report), plus the retracted meta-analysis.
	clean := []ScoredWork{
		{Stance: StanceSupporting, StanceConf: 0.8, Design: DesignUnknown, CitedBy: 4731},
		{Stance: StanceSupporting, StanceConf: 0.95, Design: DesignNarrativeReview, CitedBy: 2006},
		{Stance: StanceSupporting, StanceConf: 0.95, Design: DesignNarrativeReview, CitedBy: 1999},
	}
	var retracted ScoredWork
	for _, f := range retractedFixtures {
		if f.name == "vitaminC_metaanalysis_no_title_marker" {
			retracted = ScoredWork{
				Stance: f.wantStance, StanceConf: f.wantConf,
				Design: f.design, CitedBy: f.citedBy,
			}
		}
	}
	withIt := Consensus(append(append([]ScoredWork{}, clean...), retracted))
	without := Consensus(clean)

	t.Logf("MERT  visszavonttal: apex=%s strength=%s studies=%d citations=%d score=%.2f conf=%.2f",
		withIt.ApexDesign, withIt.EvidenceStrength, withIt.StudyCount,
		withIt.TotalCitations, withIt.ConsensusScore, withIt.Confidence)
	t.Logf("MERT  nelkule:       apex=%s strength=%s studies=%d citations=%d score=%.2f conf=%.2f",
		without.ApexDesign, without.EvidenceStrength, without.StudyCount,
		without.TotalCitations, without.ConsensusScore, without.Confidence)

	if withIt.ApexDesign == without.ApexDesign &&
		withIt.EvidenceStrength == without.EvidenceStrength &&
		withIt.StudyCount == without.StudyCount {
		t.Errorf("a visszavont mu semmit nem mozdit — akkor nincs mit javitani, " +
			"a fixture vagy a mero rossz")
	}
}
