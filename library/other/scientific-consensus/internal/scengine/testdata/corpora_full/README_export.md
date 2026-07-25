# corpora_full — untruncated re-export

Generated 2026-07-25 by the script below, from a `scientific-consensus-pp-cli`
binary built at that date from this working tree (`go build ./cmd/scientific-consensus-pp-cli`),
with the `--full-abstracts` flag that had just been added — the sibling
`testdata/corpora/` files were serialized through the 1500-rune `clipAbstract`
cap and are truncated in 108 of 246 studies (43.9%).

Every run is keyless: all five LLM provider API keys are explicitly unset with
`env -u`, so stance classification takes the lexical heuristic path
(`stance_method: "heuristic"`), which is deterministic and free. That is also
what the archived corpora ran on, which is why the two exports agree on every
score, verdict and stance label and differ only in abstract length.

`--limit` per corpus is not arbitrary: it reconstructs the original fetch size
as `study_count + works dropped by the relevance gate + works dropped by the PICO
gate`, read out of each archived corpus's `note` field. Reconstruction was
confirmed correct — all 12 re-exports returned exactly the same number of works
as the archive, DOI-matched with zero drift.

## Script

```bash
#!/usr/bin/env bash
# Re-export the 12 archived corpora with untruncated abstracts.
# Safety: every run writes to a temp file, is validated (parses as JSON,
# stance_method == "heuristic", non-zero study list), and only then moved into
# the target directory. A half-written JSON must never reach corpora_full/.
set -u

SCRATCH="/c/Users/LACI/AppData/Local/Temp/claude/C--Users-LACI-Desktop-printing-press-library/81f33274-440a-40c0-b042-9eba0207f3cb/scratchpad"
BIN="$SCRATCH/sc-pp-cli.exe"
CLIDIR="/c/Users/LACI/Desktop/printing-press-library/library/other/scientific-consensus"
D="$CLIDIR/internal/scengine/testdata/corpora_full"
TMP="$SCRATCH/tmp_export"

mkdir -p "$D" "$TMP" || { echo "FATAL: mkdir failed"; exit 1; }
echo "target dir: $D"

validate() {
  # $1 = file, $2 = corpus name, $3 = expected limit
  python3 - "$1" "$2" "$3" <<'PY'
import json, io, sys
path, name, limit = sys.argv[1], sys.argv[2], int(sys.argv[3])
try:
    d = json.load(io.open(path, encoding='utf-8'))
except Exception as e:
    print(f"INVALID JSON ({name}): {e}"); sys.exit(1)
m = d.get('stance_method')
if m != 'heuristic':
    print(f"WRONG STANCE METHOD ({name}): {m!r}"); sys.exit(1)
studies = d.get('all_studies')
if studies is None:
    print(f"MISSING all_studies ({name})"); sys.exit(1)
abs_lens = [len(s.get('abstract') or '') for s in studies]
at_cap = sum(1 for n in abs_lens if n == 1500)
over   = sum(1 for n in abs_lens if n > 1500)
print(f"OK {name}: studies={len(studies)} relevant={d.get('relevant_count')} "
      f"limit={limit} at_cap_1500={at_cap} over_1500={over} "
      f"max_abstract={max(abs_lens) if abs_lens else 0}")
PY
}

run() {
  local name="$1" claim="$2" limit="$3"
  local tf="$TMP/$name.json"
  echo "--- [$name] limit=$limit"
  env -u ANTHROPIC_API_KEY -u OPENAI_API_KEY -u GEMINI_API_KEY -u GROQ_API_KEY -u MISTRAL_API_KEY \
    "$BIN" consensus "$claim" --full-abstracts --data-source live --limit "$limit" --json > "$tf" 2> "$TMP/$name.err"
  local code=$?
  if [ $code -ne 0 ]; then
    echo "FAIL [$name]: exit=$code"; sed -n '1,10p' "$TMP/$name.err"; exit 1
  fi
  if ! validate "$tf" "$name" "$limit"; then
    echo "FAIL [$name]: validation failed"; exit 1
  fi
  mv "$tf" "$D/$name.json" || { echo "FAIL [$name]: move failed"; exit 1; }
  sleep 2
}

# meditation is reused from the approved smoke run (same command line).
echo "--- [meditation] reused from smoke run (limit=10)"
cp "$SCRATCH/smoke.json" "$TMP/meditation.json" || { echo "FATAL: smoke.json missing"; exit 1; }
validate "$TMP/meditation.json" "meditation" 10 || exit 1
mv "$TMP/meditation.json" "$D/meditation.json" || exit 1

run cellphones   "cell phones cause brain cancer"                                    51
run coffee       "coffee causes heart disease"                                       26
run melatonin    "melatonin supplementation improves sleep quality in shift workers" 19
run omega3       "omega-3 improves cardiovascular health"                            63
run probiotics   "probiotics improve gut health"                                     10
run redmeat_run1 "red meat causes colon cancer"                                      26
run redmeat_run2 "red meat causes colon cancer"                                      26
run saffron      "saffron extract reduces symptoms of mild depression"               12
run sweeteners   "artificial sweeteners cause weight gain"                           15
run vaccines     "vaccines cause autism"                                             51
run vitamind     "vitamin D reduces respiratory infections"                          26

echo
echo "=== ALL 12 EXPORTED ==="
ls -la "$D"
```

## Verified properties of the result

| property | result |
|---|---|
| studies exactly on the old 1500-rune cap | 0 of 246 |
| studies past 1500 runes | 108 |
| longest abstract | 23 377 runes / 3 320 words (omega3) |
| `ReconstructAbstract` 6000-word cap reached | never — closest is 3 320 words (55%) |
| work-set drift vs the archive (DOI-matched) | 0 gone, 0 new, in all 12 corpora |
| score / confidence / verdict / stance-label differences vs the archive | none, in all 12 corpora |
| `redmeat_run1` vs `redmeat_run2` (two separate live runs) | byte-identical |

The last two rows are the load-bearing ones. If the engine had scored the
truncated text, re-running with full abstracts would have produced different
scores. Zero difference across all 12 corpora is direct evidence that
`clipAbstract` runs strictly after scoring — measured, not assumed.
