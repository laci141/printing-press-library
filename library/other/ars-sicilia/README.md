# ARS Sicilia CLI

**L'unica CLI per il portale dell'Assemblea Regionale Siciliana: cerca, sincronizza in locale e interroga tutti i 12 archivi documentali con SQL, FTS e MCP.**

Sostituisce le 12 maschere JSP del portale ufficiale con una CLI agent-native. Sync in SQLite locale per query SQL, ricerca full-text cross-archivio, e novel commands come `ddl iter` (timeline completa di un disegno di legge) e `deputato profilo` (tutta l'attività di un parlamentare in un'unica chiamata).

Learn more at [ARS Sicilia](https://dati.ars.sicilia.it).

Printed by [@aborruso](https://github.com/aborruso) (aborruso).

## Install

The recommended path installs both the `ars-sicilia-pp-cli` binary and the `pp-ars-sicilia` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install ars-sicilia
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install ars-sicilia --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install ars-sicilia --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install ars-sicilia --agent claude-code
npx -y @mvanhorn/printing-press-library install ars-sicilia --agent claude-code --agent codex
```

### Without Node

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.5 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/other/ars-sicilia/cmd/ars-sicilia-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ars-sicilia-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-ars-sicilia --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-ars-sicilia --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-ars-sicilia skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-ars-sicilia. The skill defines how its required CLI can be installed.
```

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ars-sicilia-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ars-sicilia": {
      "command": "ars-sicilia-pp-mcp"
    }
  }
}
```

</details>

## Authentication

Nessuna credenziale richiesta: il portale ARS è pubblico. La sessione `JSESSIONID` per la ricerca è gestita automaticamente in modo trasparente dal client.

## Quick Start

```bash
# Verifica raggiungibilità del portale e stato del database locale.
ars-sicilia-pp-cli doctor

# Sincronizza in locale leggi e DDL degli ultimi 30 giorni in SQLite.
ars-sicilia-pp-cli sync --resources leggi,ddl --max-pages 0

# Cerca i DDL della XVIII legislatura presentati nel 2024.
ars-sicilia-pp-cli ddl cerca --anno 2024 --legisl 18 --json

# Ricerca full-text cross-archivio sui documenti già sincronizzati.
ars-sicilia-pp-cli search "bilancio sanitario" --limit 20

# Timeline completa del DDL 1153 della XVIII legislatura.
ars-sicilia-pp-cli ddl iter 18 1153 --json

# Tutta l'attività parlamentare di un deputato in un'unica chiamata.
ars-sicilia-pp-cli deputato profilo "Abbate Ignazio" --json --select tipo,data,titolo

```

## Known Gaps

- **HTTP error exit codes**: Non-429 HTTP errors from the Icaro portal (404, 5xx) exit with code 1 rather than typed exit codes (e.g. exit 3 for not-found, exit 5 for server error). Rate-limit responses (HTTP 429) correctly return exit 7. Scripts that branch on specific exit codes should use `ars-sicilia-pp-cli doctor` to check connectivity first.
- **Result `url` and `doc_id` are session-scoped, not citable** ⚠️: on the Icaro archives `icaDocId` is the row's position in the *current session's* short list, not the document's identity. Run a different query and the same `icaDocId` opens a different act (`(18.LEGISL E 738.NUMDDL)` → `icaDocId=1` is `docno(9037)`, not the one you saw before); outside a session the URL answers 302. Do not store or cite them. `get` also returns `docno` — the portal's own stable document number — and `permalink`, which reopens exactly that document in a fresh session: those are the ones to keep. The short list does not carry `docno` (its markup only has `showDoc(N)`), so search rows cannot expose it without one detail fetch per row.
- **One number can match more than one document** ℹ️: the portal keeps distinct documents under the same legislature+number, usually successive versions of the same file. Ddl 6030 has two — `docno(9513)` with the bill text and the iter up to 4 Aug 2026, and `docno(9390)`, scheda-only and stuck at 14 Jul — identical in every list field, title and date included. `cerca` flags it with a hint and `get` says which one it opened (with its `docno`) instead of silently taking the first. The `cerca` hint counts inside the window it downloaded, so a tight `--limit` can hide the second row and silence it — that case is covered by `troncato`/the truncation hint, which always speaks. Neither hint fires on `leggi` (indexed per article) or `resoconti` (indexed per agenda item), where repeated numbers are the norm.
- **`legge cronologia` needs `--anno` when the number repeats** ⚠️: the same law number recurs in different years of one legislature — the XVIII has two L.R. 26 (7 Oct 2024, 10 Jun 2025). The archive returns one row and the command takes it, so without `--anno` you can get a perfectly coherent timeline for the wrong act. A stderr hint names the law it picked (`uso la L.R. 26 promulgata il 7.10.2024`): check that date, or pin `--anno`.
- **`legge cronologia` date filtering**: The sommari search finds committee meetings that mention the law number in free text without a date ceiling. A committee meeting held after the law's promulgation date may appear in the timeline if it references the same number. Filter results by the `data` field when you need only pre-promulgation events.
- **`--csv` on empty results**: when a command (e.g. `analytics --csv`) produces an empty result set, the CSV output is the JSON literal `[]` instead of an empty/header-only CSV. Piping that to a `.csv` file yields malformed content. Use `--json` for empty/unsynced data until this is fixed upstream.
- **`search` JSON shape**: `search --json` returns an object `{ "meta": {...}, "results": [...] }`, not a top-level array. When piping to `jq`, select `.results` (e.g. `search "x" --json | jq '.results[]'`). Status lines ("no search endpoint…", sync hints) go to stderr, so `2>/dev/null` keeps stdout clean.
- **Local-store commands need a sync first**: `search`, `analytics` and `sync stale` read the local SQLite store. On a fresh install run `ars-sicilia-pp-cli sync --full` first; until then they return empty results (with a sync hint on stderr). All other commands (`*/cerca`, `*/get`, `ddl iter`, `deputato profilo`, `legge cronologia`, `commissione dossier`) query the portal live and need no sync.
- **`analytics --group-by cofirmatari` and `ddl drift` need a deep sync** ⚠️: a plain sync stores only the list-page fields (`data`, `numero`, `title`, `url`, …). The signatories (`firmatari`) and per-bill iter status (`iter`) live only inside each document `body`, so run **`ars-sicilia-pp-cli sync --resources ddl --deep`** to extract them into the store — then `analytics --group-by cofirmatari --type ddl` works, and `ddl drift` compares the iter state across two deep syncs. The deep pass fetches one detail page per ddl (~1 extra request each), so a full legislature takes a few minutes; a normal sync stays fast.
- **`analytics --group-by oratore` runs live (no sync)** ℹ️: the speaker ranking is built by querying the `/bd/resoconti` backend once per speaker of the legislature (≈90 requests, ~1 min), so it needs `--legisl` (without it the ~1000 all-time speakers would be too many requests) and does not read the local store. Example: `analytics --type resoconti --group-by oratore --legisl 18`.
- **ISIS-only filters are rejected on the migrated archives** ⚠️: `sommari`, `resoconti` and `convocazioni` are served by the portal's `/bd/` backend, which has a fixed form instead of an ISIS query string. `--isis-query` and `--escludi` (plus `--argomento` on `resoconti` and `--presidente` on `commissioni sommari`) have no equivalent there, so those commands fail with an explicit error rather than silently returning a wider result set. Every other filter (`--legisl`, `--anno`, `--data`, `--numero`, `--testo`, `--oratore` on resoconti, `--commissione`/`--codcom` on sommari and convocazioni) works. ISIS filters remain available on all the other archives (`ddl`, `leggi`, `interrogazioni`, …).
- **Six-digit `AAMMGG` dates resolve their century from the archive, and stop at 2046** ℹ️: `--data` also accepts the portal's native six-digit form, which carries no century. On the `/bd/` archives `47`–`99` reads as 1900s and `00`–`46` as 2000s, because the oldest document those archives serve is the inaugural sitting of **25 May 1947** (nothing exists for 1946 — the ARS begins there). So `--data 510412` is 12 April 1951; it used to be read as 2051 and returned `[]` on a sitting that exists, and with it every date between 1947 and 1999 written this way. Dates from 2047 on cannot be expressed in six digits — write them in full, which is never ambiguous.
- **A malformed `--data` is an error, and only on the `/bd/` archives** ⚠️: on `sommari`, `resoconti` and `convocazioni`, a value the date parser cannot read (`2025-01-01:garbage`, `garbage`) or a date that does not exist (`2025-13-45`, `2025-02-30`) now fails with exit 2 and a message naming the accepted forms. It used to drop the filter entirely and return the archive from the beginning — `resoconti cerca --data 2025-01-01:garbage` answered with sittings from **1951** — presented as a valid answer. On the Icaro archives the same input is passed through to ISIS and simply matches nothing (`mozioni cerca --data garbage` → `[]`), so the wrong-results failure was `/bd/`-only.
- **`--data` across calendar years costs one request per year** ℹ️: the `/bd/` form filters by a single year, so `--data 2024-11-01:2026-02-28` queries 2026, 2025 and 2024 in turn (most recent first) and filters the exact days client-side. With a small `--limit` you get the most recent records of the range and `troncato`/truncation flags report that earlier years were left unread.
- **`leggi cerca` returns one row per law, not per article** ℹ️: archive 201 is indexed **per article**, so a twenty-article law used to fill twenty rows — and `--limit 10` was spent on the articles of the first law, answering "which laws passed in 2025?" with a single law while there were dozens. The command now aggregates by law (`articoli_trovati` counts the articles this search matched, not the law's total) and `--limit` counts laws. Use `--articoli` for the raw per-article rows, which is what you want with `--testo` when you need to know *which* article mentions a term. Pagination stops on the unit you asked for: it keeps reading pages until the requested laws are collected, rather than guessing a row budget up front. That guess (10 rows per law) is what made `leggi cerca --legisl 18 --anno 2025` answer with 4 laws out of the year's 31 — the first laws of a year are the budget ones, ~25 article-rows each, and they ate the window. Short laws now cost fewer requests than before, long ones cost more (the portal allows 2 requests/second, so a full default page of 10 laws takes ~20 s on a budget-heavy year). A safety ceiling still caps the rows read; when it cuts in before the requested laws are collected, the CLI says so on stderr instead of returning a short list silently.
- **Look an act up by its number with `--numero`, never with `--testo`** ⚠️: `--numero` is field-qualified (`NUMORD`/`NUMDDL`/`LEGNUM`) and returns the act itself; it is available on every `cerca` (`ddl`, `leggi`, `odg`, `interrogazioni`, `interpellanze`, `mozioni`, `risoluzioni`). Passing the number as free text instead matches every document that *mentions* it, newest first, so the act you want can sink past the default `--limit`: `mozioni cerca --testo "143"` buries mozione 143 at position 17 of 19, while `--numero 143` returns it alone.
- **`--testo` is AND over the whole document; `--frase` matches a phrase** ℹ️: `--testo "aree idonee"` builds `(aree E idonee)`, so both words must appear *somewhere* in the document — on a text as long as a bill that also matches acts with one word in article 3 and the other in article 40 (peschicoltura, coworking). `--frase "aree idonee"` builds `(aree adj idonee)`: adjacent, in the given order, which surfaces the acts that actually legislate on the topic (ddl 803, 726). A single word passes through unchanged, and a value already containing operators or parentheses is left verbatim. Not available on `resoconti`, `sommari` and `convocazioni` (the `/bd/` backend takes no ISIS expression) — there the command fails with an explicit error instead of dropping the filter.
- **Full-text results are not ranked by relevance, and a short list is not proof of absence** ⚠️: the portal returns matches in its own order (roughly newest first), not by how well they match `--testo`. The CLI now pulls the rows whose **title** contains every search term to the front, which is usually enough: `ddl cerca --legisl 17 --testo "gestione rifiuti" --limit 100` moves ddl 290 ("Riforma degli ambiti territoriali ottimali e nuove disposizioni per la gestione integrata dei rifiuti") from row 75 to row 2. **That reordering only sorts the window you already downloaded** — it cannot surface a document that sits past `--limit`. So when none of the rows shown has the terms in its title, a stderr hint says exactly that: read it as "the act you want is probably further down", raise `--limit`, or use `--frase` for the exact phrase. On `resoconti`, `sommari` and `convocazioni` the hint stops at `--limit`, since `--frase` does not exist on the `/bd/` backend and suggesting it would only lead to an error. Whenever the result set is cut short, `*/cerca` says so too — treat both hints as "widen or narrow before concluding it does not exist".
- **The portal cuts list titles at 256 characters, so a missing term in the title proves nothing** ⚠️: long-titled acts — `Schema di progetto di legge costituzionale…`, `Disegno di legge voto…` — are exactly the ones whose subject sits past the cut. Sicily's XVII-legislature bill 199 is titled "…riconoscimento degli svantaggi derivanti dalla condizione di insularità" but the list shows "…svantaggi deriva", so a title match on `insularità` is impossible to see. Rows whose title hits the cap and does not match are therefore ranked **between** the proven matches and the off-topic rows, not lumped with the latter, and the "no relevant title" hint says how many titles were cut and tells you to open the document for the full one. `ddl cerca --legisl 17 --testo "insularità"` moves bill 199 from row 9 to row 1 this way.
- **`ddl iter` event `url` now points at the sitting record** ⚠️ *(behaviour change)*: on Aula events the event `url` is the resoconto scheda for that sitting, not the bill's own page — it used to repeat the bill page on every event, which is where the events are parsed from but not where they happened. The bill page moved to the report root (`url`, next to `legisl`/`numero`/`titolo`), which also gains it in `legge cronologia`. On events carrying a resoconto link, `archive_id`/`doc_id` are omitted: they identified the bill document and would be inconsistent beside a URL pointing elsewhere. **Aula and committee sittings are numbered independently**, so the link appears only where the portal marks the sitting as Aula — `Esitato per Aula (epa) Seduta n. 260 0400 Commissione QUARTA` is an Aula-phase event citing a *committee* sitting, and linking it would land on the unrelated Aula sitting 260. **The link is also dropped when the source contradicts itself** ⚠️: the Assembly holds one sitting per date, so two Aula events of the same iter giving that date different sitting numbers cannot both be right, and the portal does not say which is. `ddl iter 17 199` reports the final vote as "19 feb 2020 — Approvato dall'Assemblea — Seduta n. 179", but sitting 179 is 26 February — the vote is in 178, as its own record spells out. On such a date both events keep the bill's page as their `url` and a stderr hint tells you to resolve the real sitting from the date (`resoconti cerca --legisl 17 --data 2020-02-19`).
- **`--envelope` puts the truncation signal inside the JSON** ℹ️: by default `*/cerca` prints a bare array and the "results truncated" / "no relevant title" hints go to **stderr**, where a JSON consumer never sees them — that is how a truncated window gets read as "the document does not exist". Add `--envelope` and the output becomes `{"risultati": [...], "troncato": true, "hint": "..."}`. Opt-in, so existing pipelines that do `... --agent | jq '.[]'` keep working. `--select` filters **inside** `risultati` (the envelope keys always stay), and `--csv` ignores the flag since a wrapper makes no sense around a table. Recommended for agents: `resoconti cerca --legisl 17 --data 2019-10-01:2019-12-31 --agent --envelope --limit 10`. **The MCP tools handle this for you**: they pass `--envelope`, and every `hint:`/`warning:` line the CLI writes to stderr comes back inside the payload as an `avvisi` field — on every command, not just searches. Without that, the MCP transport would discard them all, since it reads stderr only when a command fails.
- **`--data` on the acts archives filters the *presentation* date, not the approval one** ℹ️: on `interrogazioni`, `interpellanze`, `mozioni`, `odg` and `risoluzioni`, `--data` is qualified on `DATPRE`, the date the act was filed. Press coverage usually reports the date an act was *approved in the chamber*, which is later — a motion approved on 2020-02-04 may well have been presented on 2020-01-28, and searching the approval date returns nothing. When a known date yields no result, widen the range backwards (`--data 2020-01-01:2020-02-04`) or combine it with `--firmatario`. The approval step itself is in the act's iter, visible with `get`.
- **`--data` is not a native flag on `ddl cerca`** ℹ️: the other acts archives expose it, but `ddl cerca` still needs `--isis-query "(18.LEGISL E 250701/250807.DATPRE)"` for an arbitrary date range. For a whole year use `--anno`, which the CLI already expands into a `DATPRE` Jan-1..Dec-31 range.
- **`doctor`'s cache-staleness threshold (6h) and `sync stale`'s default (7d) are intentionally different, not a bug**: `doctor`'s cache section is generated framework code with a fixed, conservative 6h default and no flag to change it — it's a generic "is this stale enough to worry about" signal. `sync stale --max-age` defaults to 7d because ARS parliamentary data (sedute, commissioni, interrogazioni) doesn't turn over hourly; a weekly sync is normally plenty. An agent that automates sync should not rely on `sync stale`'s default alone as the sole freshness signal — it can report `stale: false` on a store `doctor` already flags as stale. Check `doctor`'s `cache.status` (or pass an explicit `sync stale --max-age` matching your own freshness needs) rather than assuming the two agree.

## Unique Features

These capabilities aren't available in any other tool for this API.

### Vista cronologica cross-archivio
- **`ddl iter`** — Ricostruisce la cronologia completa di un disegno di legge: presentazione, passaggio in commissione, lavori d'aula, eventuale promulgazione come legge regionale.

  _Quando un agente deve raccontare 'a che punto sta il DDL X', questa è l'unica chiamata che restituisce la timeline completa senza incollare 5 ricerche manuali._

  Ogni evento porta il numero di **`seduta`**, quando il portale lo dichiara, e per le sedute d'Aula il campo **`url`** punta alla scheda del resoconto (la scheda dell'atto sta nel campo `url` della radice). Serve a rispondere a «in quale seduta l'hanno votato?» e, soprattutto, a non scambiare la data della notizia con la data della seduta — la stampa scrive quasi sempre il giorno dopo. Attenzione: sedute d'Aula e di commissione hanno numerazioni **indipendenti**, e il link compare solo dove il portale marca la seduta come d'Aula.

  ```bash
  ars-sicilia-pp-cli ddl iter 18 1153 --json
  ars-sicilia-pp-cli ddl iter 17 290 --json --select data,fase,seduta,url
  ```
- **`ddl stralci`** — Elenca i disegni di legge ricavati per stralcio da un ddl base. Il verso opposto è nel campo `stralcio` di `ddl get` e `ddl iter`, che dice da quale ddl lo stralcio proviene.

  _Durante la sessione di bilancio la finanziaria viene spacchettata in stralci che proseguono da soli: senza questo comando bisogna indovinarne i numeri, e non c'è una regola da indovinare (gli stralci del ddl 1030 sono 3030…8030, quelli del 738 sono una ventina fra 7381 e 73864)._

  ```bash
  ars-sicilia-pp-cli ddl stralci 18 1030 --json
  ```

  Il legame è **dichiarato dal portale**, non calcolato: ogni stralcio porta con sé il riferimento al ddl base (`ddl n. 1030/A Stralcio IV`). Due casi che l'output rende espliciti invece di nascondere: uno stralcio può nascere da **più ddl abbinati** (`di` con due voci), e per una parte degli atti della XVII legislatura il portale scrive l'id interno al posto del numero base — lì `base_dichiarata` è `false` e `di` resta vuoto, perché dedurre la base dalla numerazione sarebbe un'invenzione.
- **`deputato profilo`** — Aggrega in un'unica vista tutti gli atti firmati o pronunciati da un deputato: DDL, interrogazioni, interpellanze, mozioni, ordini del giorno, risoluzioni e interventi in resoconti d'aula. `--data` (range `YYYY-MM-DD:YYYY-MM-DD`) filtra per data di presentazione/seduta su tutti i sotto-archivi, per query storiche mirate senza dover alzare `--limit`.

  _Sostituisce un workflow di 7 click manuali con un'unica chiamata strutturata: pensata per agenti che rispondono a 'che ha fatto il deputato X?'._

  ```bash
  ars-sicilia-pp-cli deputato profilo "Abbate Ignazio" --legisl 18 --json --select tipo,data,titolo
  ars-sicilia-pp-cli deputato profilo "Safina" --legisl 18 --data 2024-07-01:2024-07-31 --json
  ```
- **`commissione dossier`** — Vista completa su una commissione: convocazioni in calendario, sommari lavori, DDL assegnati e pareri richiesti al Governo regionale. Accetta il codice `1`-`6`, l'ordinale (`PRIMA`..`SESTA`) o un frammento della denominazione d'archivio. Le **commissioni speciali** (Antimafia, Statuto, Unione Europea) non hanno un codice e si raggiungono solo per denominazione, che non coincide con l'etichetta d'uso corrente: `"Antimafia"` non corrisponde a nulla, la denominazione è *«Commissione d'inchiesta e vigilanza sul fenomeno della mafia e della corruzione in Sicilia»*. Un termine che non aggancia nessuna commissione non produce un dossier vuoto: l'errore elenca le denominazioni della legislatura.

  _Quando segui i lavori di una commissione specifica, questa è l'unica chiamata che dà il quadro completo invece di 3 ricerche separate._

  ```bash
  ars-sicilia-pp-cli commissione dossier "SESTA" --legisl 18 --json
  ars-sicilia-pp-cli commissione dossier "inchiesta e vigilanza" --legisl 18 --json
  ```
- **`legge cronologia`** — Partendo da una legge regionale promulgata (archivio 201), risale al DDL originario, agli emendamenti citati nei resoconti d'aula e ai pareri di commissione: l'inverso temporale di ddl iter.

  _Per ricercatori e giornalisti che partono dalla legge promulgata e vogliono raccontare come ci si è arrivati._

  ```bash
  ars-sicilia-pp-cli legge cronologia 18 26 --anno 2025 --json
  ```

### Analytics su campi strutturati
- **`analytics --group-by anno`** — Distribuzione dei documenti per anno in un archivio (aggregazione locale sul DB sincronizzato).

  ```bash
  ars-sicilia-pp-cli analytics --type ddl --group-by anno --limit 50 --json
  ```
- **`analytics --group-by cofirmatari`** — Mappa le alleanze legislative (coppie di co-firmatari di DDL). Richiede una **deep sync** che estragga i firmatari dalle schede di dettaglio: `ars-sicilia-pp-cli sync --resources ddl --deep`. Dopo, funziona su `--type ddl` (vedi **Known Gaps** per i costi).
- **`analytics --group-by oratore`** — Classifica gli oratori più attivi in Aula per numero di sedute in cui sono intervenuti. Gira **in diretta** sul backend `/bd/resoconti` (una richiesta per oratore della legislatura), quindi richiede `--legisl` e impiega ~1 minuto. Es: `analytics --type resoconti --group-by oratore --legisl 18`.

### Stato e monitoraggio
- **`ddl drift`** — Confronta lo stato dell'iter dei DDL tra due sync e segnala quelli "mossi" (da commissione ad aula, approvati, ritirati). Richiede due **deep sync** a distanza di tempo (`sync --resources ddl --deep`), perché il campo `iter` viene scritto solo dalla deep sync (vedi **Known Gaps**). Per la cronologia di un singolo DDL usa `ddl iter <legisl> <numero>`, che la legge in diretta dal documento.
- **`sync stale`** — Mostra per ognuno dei 12 archivi ARS: timestamp ultima sync, n. record locali, età della sync, eventuale segnalazione di staleness.

  _Per agenti che orchestrano sync automatico: decide se rinfrescare prima di rispondere o se i dati locali sono ancora freschi._

  ```bash
  ars-sicilia-pp-cli sync stale --json
  ```

## Recipes


### Sync iniziale completo XVIII legislatura

```bash
ars-sicilia-pp-cli sync --full --resources leggi,ddl,interrogazioni,mozioni,interpellanze,odg,risoluzioni,pareri,resoconti,convocazioni,sommari
```

Prima sincronizzazione di tutti gli archivi politici della XVIII legislatura — i dati restano in `~/.local/share/ars-sicilia-pp-cli/store.db`.

### Deep sync dei DDL (firmatari + iter)

```bash
ars-sicilia-pp-cli sync --resources ddl --legisl 18 --deep
```

Per ogni ddl scarica anche la scheda di dettaglio ed estrae i **firmatari** e lo **stato dell'iter** (assenti nella short-list). Sblocca `analytics --group-by cofirmatari --type ddl` e `ddl drift` (quest'ultimo richiede due deep sync a distanza di tempo). Costa ~1 richiesta extra per ddl, quindi è più lento di una sync normale.

### Iter completo di un DDL con output narrowing

```bash
ars-sicilia-pp-cli ddl iter 18 1153 --json --select fase,data,sede,oratori
```

Timeline del DDL 1153, mostrando solo i campi essenziali — riduce il payload per agenti.

### Network di co-firmatari su DDL

```bash
ars-sicilia-pp-cli sync --resources ddl --deep
ars-sicilia-pp-cli analytics --type ddl --group-by cofirmatari --limit 30 --csv
```

Produce un CSV con le coppie di deputati che firmano DDL insieme — pronto per import in `duckdb` o gephi. **La deep sync è obbligatoria**: senza, il comando restituisce `[]` (con un hint su stderr), perché i firmatari stanno solo nelle schede di dettaglio.

### Classifica DDL per proponente o gruppo (1 richiesta, senza sync)

```bash
ars-sicilia-pp-cli analytics --type ddl --group-by proponente --limit 20
ars-sicilia-pp-cli analytics --type ddl --group-by gruppo --json
```

Legge le viste già aggregate dal portale (`/edem/`): la classifica dei disegni di legge per deputato **proponente** (primo firmatario) o per **gruppo** parlamentare, con una sola richiesta e senza sincronizzazione. Copre la legislatura corrente; `--legisl` non filtra queste classifiche (viene ignorato con un avviso).

### Drift settimanale dei DDL

```bash
ars-sicilia-pp-cli ddl drift --since 7d --json
```

Confronta lo stato dell'iter rispetto a una settimana fa — i DDL che si sono mossi (commissione → aula, voto, ritiro) compaiono qui.

### Analytics sui cofirmatari

```bash
ars-sicilia-pp-cli sync --resources ddl --legisl 18 --deep
ars-sicilia-pp-cli analytics --type ddl --group-by cofirmatari --limit 20 --legisl 18 --json
```

Top 20 coppie di deputati che firmano insieme DDL nella XVIII legislatura — aggregazione locale sul DB sincronizzato, che va popolato con una **deep sync**: senza, il risultato è `[]`.

### Ricerca per tema (vocabolario materie)

```bash
# Scopri le materie disponibili, filtra per parola chiave
ars-sicilia-pp-cli ddl materie | grep -i "sanit\|salut\|lavoro\|ambiente"

# Tutti i DDL sull'ambiente nella XVIII
ars-sicilia-pp-cli ddl cerca --legisl 18 --materia "Ambiente" --json | \
  jq -r '.[] | "\(.data) — \(.title)"'
```

Utile per giornalisti che seguono un tema: la lista completa delle 123 materie è navigabile offline senza aprire il portale.

### Veterani del parlamento — chi dura di più

```bash
ars-sicilia-pp-cli ddl firmatari --json | \
  jq -r 'group_by(.nome)[] | select(length >= 4) | "\(length) legislature — \(.[0].nome)"' | \
  sort -rn | head -10
```

Identifica i parlamentari con la carriera più lunga: quante e quali legislature hanno coperto. Cracolici Antonino è il record attuale con 6 legislature consecutive (XIII→XVIII).

### Seguire un deputato — carriera e attività

```bash
# In quali legislature ha operato?
ars-sicilia-pp-cli ddl firmatari --search "Scoma" --json | jq -r '.[].legisl' | sort | tr '\n' ' '

# Tutti i DDL presentati nella XVIII
ars-sicilia-pp-cli ddl cerca --legisl 18 --firmatario "Scoma Francesco" --json | \
  jq -r '.[] | "\(.data) — \(.title)"'
```

### Nuovi deputati — chi è al primo mandato

```bash
ars-sicilia-pp-cli ddl firmatari --json | \
  jq -r 'group_by(.nome)[] | select(length == 1 and .[0].legisl == "18") | .[0].nome'
```

Filtra i deputati presenti solo nella XVIII — al loro primo mandato regionale.

### Iniziative parlamentari vs governative

```bash
# Tipi di iniziativa disponibili
ars-sicilia-pp-cli ddl iniziative

# DDL a iniziativa governativa nella XVIII
ars-sicilia-pp-cli ddl cerca --legisl 18 \
  --isis-query "(18.LEGISL E Governativa.FIRMAT)" --limit 50 --json | jq 'length'
```

Distingue le proposte dei deputati (parlamentare) da quelle dell'esecutivo regionale (governativa).

## Usage

Run `ars-sicilia-pp-cli --help` for the full command reference and flag list.

Per query avanzate con `--isis-query` (operatori `NOT`/`WITH`/`NEAR`/`ADJ`, qualificazione di
campo, range di date, radici) vedi la guida [docs/isis-query-syntax.md](docs/isis-query-syntax.md),
con la tabella delle sigle di campo verificate.

## Commands

### biblioteca

Catalogo Bibliografico (archivio 205) e Opere Multimediali (205multimedia).

- **`ars-sicilia-pp-cli biblioteca cerca`** - Cerca nel catalogo bibliografico per autore, titolo, soggetto o ISBN.
- **`ars-sicilia-pp-cli biblioteca multimediali`** - Cerca nelle opere multimediali.

### commissioni

Lavori delle Commissioni: convocazioni (229) e sommari (230).

- **`ars-sicilia-pp-cli commissioni convocazioni`** - Convocazioni delle Commissioni.
- **`ars-sicilia-pp-cli commissioni sommari`** - Sommari dei lavori di commissione.

### ddl

Disegni di Legge (archivio 221): proposte di legge presentate all'ARS.

- **`ars-sicilia-pp-cli ddl cerca`** - Cerca disegni di legge per legislatura, anno, firmatario, materia o testo.
- **`ars-sicilia-pp-cli ddl get`** - Scarica un singolo disegno di legge.

### interpellanze

Interpellanze parlamentari (archivio 234).

- **`ars-sicilia-pp-cli interpellanze cerca`** - Cerca interpellanze.
- **`ars-sicilia-pp-cli interpellanze get`** - Scarica una singola interpellanza.

### interrogazioni

Interrogazioni parlamentari (archivio 233).

- **`ars-sicilia-pp-cli interrogazioni cerca`** - Cerca interrogazioni per legislatura, firmatario o rubrica.
- **`ars-sicilia-pp-cli interrogazioni get`** - Scarica una singola interrogazione.

### leggi

Leggi della Regione Siciliana (archivio 201): testo storico delle leggi regionali.

- **`ars-sicilia-pp-cli leggi cerca`** - Cerca leggi regionali per legislatura, anno, numero o testo.
- **`ars-sicilia-pp-cli leggi get`** - Scarica una singola legge regionale.

### mozioni

Mozioni parlamentari (archivio 235).

- **`ars-sicilia-pp-cli mozioni cerca`** - Cerca mozioni.
- **`ars-sicilia-pp-cli mozioni get`** - Scarica una singola mozione.

### odg

Ordini del Giorno (archivio 236).

- **`ars-sicilia-pp-cli odg cerca`** - Cerca ordini del giorno.
- **`ars-sicilia-pp-cli odg get`** - Scarica un singolo ordine del giorno.

### pareri

Pareri richiesti dal Governo regionale alle Commissioni (archivio 226).

- **`ars-sicilia-pp-cli pareri cerca`** - Cerca pareri richiesti dal Governo.
- **`ars-sicilia-pp-cli pareri get`** - Scarica un singolo parere.

### resoconti

Resoconti delle Sedute d'Aula (archivio 217).

- **`ars-sicilia-pp-cli resoconti cerca`** - Cerca resoconti per data, oratore o argomento.
- **`ars-sicilia-pp-cli resoconti get`** - Scarica un singolo resoconto. Non restituisce la trascrizione integrale: l'archivio Icaro ne conserva solo frammenti per punto dell'ordine del giorno e si ferma alla seduta n. 232 del 25.02.2026. Quando Icaro non ha la seduta, `get` ripiega sulla scheda del backend corrente e restituisce `pdf_url`, dove sta il resoconto stenografico completo (il PDF non viene scaricato; l'URL è stabile e citabile). La scheda non ha il campo `body` — che invece c'è sui record serviti da Icaro — e porta un campo `nota` che spiega perché: l'assenza di `body` non è «testo non disponibile».

### risoluzioni

Risoluzioni parlamentari (archivio 238).

- **`ars-sicilia-pp-cli risoluzioni cerca`** - Cerca risoluzioni.
- **`ars-sicilia-pp-cli risoluzioni get`** - Scarica una singola risoluzione.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
ars-sicilia-pp-cli ddl get 18 1153

# JSON for scripting and agents
ars-sicilia-pp-cli ddl get 18 1153 --json

# Filter to specific fields
ars-sicilia-pp-cli ddl get 18 1153 --json --select data,numero,title

# Dry run — show the request without sending
ars-sicilia-pp-cli ddl get 18 1153 --dry-run

# Agent mode — JSON + compact + no prompts in one flag
ars-sicilia-pp-cli ddl get 18 1153 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
ars-sicilia-pp-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Config file: `~/.config/ars-sicilia-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **I comandi `cerca` restituiscono 0 risultati ma il sito ne mostra molti.** — Verifica la legislatura: senza `--legisl` la query usa il default. Il portale ARS richiede sempre una legislatura nel criterio. Esempio: `--legisl 18` per XVIII.
- **Errore di sessione o redirect inatteso.** — Il portale resetta la sessione dopo 30 minuti di inattività. Riprova il comando: il client acquisisce una nuova `JSESSIONID` automaticamente.
- **Comando `ddl iter`, `deputato profilo`, `legge cronologia` o `commissione dossier` non trova nulla.** — Queste viste interrogano il portale **in diretta** (non richiedono `sync`): verifica `--legisl` e gli identificativi (numero DDL, nome deputato, numero legge, nome commissione). I comandi che invece leggono dal DB locale e richiedono `sync` sono solo `search`, `analytics`, `ddl drift` e `sync stale`.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**opendatasicilia/RSSdisegniLeggeAssembleaRegionaleSiciliana**](https://github.com/opendatasicilia/RSSdisegniLeggeAssembleaRegionaleSiciliana) — Shell
- [**aborruso/ars_sicilia**](https://github.com/aborruso/ars_sicilia) — Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
