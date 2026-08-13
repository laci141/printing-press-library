# Task: consolidate the patch ledger into one record (Greptile P2 on PR #1689)

Working folder: `library/health/grants`. Do NOT commit, do NOT push.

---

## The finding

> This PR adds three separate customization records, while the repository
> convention requires one new `.printing-press-patches` file per PR.
> Consolidating the NIH, NSF, and eligibility changes into one self-contained
> record preserves the expected one-PR/one-record audit shape.

Measured: this branch adds four files under `.printing-press-patches/`
relative to `origin/main`:

- grants-gov-posted-status-does-not-mean-still-open.json
- nih-keyword-hits-concept-tags-and-unapplicable-award-types.json
- nih-returns-one-record-per-support-year.json
- nsf-keyword-hits-abstracts-so-results-must-be-pooled-and-ranked.json

The convention is one record per PR. The finding is valid.

---

## What to do

1. Read `CLAUDE.md` at the repository root and any documentation it points to
   about `.printing-press-patches` conventions. Follow what it actually says
   over anything stated here.

2. Read all four files above in full.

3. Write ONE consolidated record that replaces them. Delete the four originals
   with `git rm` so the branch ends with exactly one new patch file.

4. Every measured number in the four originals must survive into the merged
   record. Nothing may be summarised away or softened into an approximation.
   In particular these must all still be present and attributable to the
   source they came from:
   - NIH concept-tag noise: totals before and after dropping the `terms`
     search field, and the activity-code counts.
   - NIH one-record-per-support-year: 100 records to 71 distinct projects,
     5R01CA092447 appearing in 11 rows, 7 distinct projects in the top 15,
     the three observed project-number forms and why truncation fails as a
     dedup key, and 15 rows / 15 distinct projects after the fix.
   - NSF keyword matching abstracts, requiring pooling and ranking.
   - Grants.gov posted status not meaning still open.

5. Give the merged file a name that describes the whole change, not one of the
   four sub-changes. The existing names are the style guide for length and
   shape.

6. If the schema has a field for affected files or sources, list all of them,
   not just one.

---

## Constraints

- Do NOT run `gofmt -w` or any whole-file formatter on any file in this CLI.
  The working tree is CRLF and a formatter would rewrite every line.
- English only. No Hungarian anywhere.
- Do not touch Go source in this task. There are already uncommitted Go
  changes in this working tree from a previous task — leave them exactly as
  they are.

---

## Verification

Run these without piping, because a pipe hides the exit code: