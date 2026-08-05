package cli

import (
	"html"
	"regexp"
	"strings"
)

// OpenAlex passes publisher JATS markup straight through into display_name and
// title, so a work can arrive as "<scp>HPV</scp>-<scp>DNA</scp> sperm infection"
// or "<i>C. elegans</i>". Measured 2026-08-05 on the live "HPV vaccine causes
// infertility" run: the tags reached the web UI, the JSON export and the BibTeX
// export, so a reader importing into Zotero saw "<scp>" inside the citation.
//
// The tag list is a whitelist rather than a generic <[^>]*> sweep. Scientific
// titles legitimately contain comparison operators — "p<0.05", "n>100" — and a
// generic sweep deletes the real text sitting between them. A whitelist can
// only ever remove markup that was never content in the first place.
var markupTagPattern = regexp.MustCompile(`(?i)</?(?:scp|sc|i|b|u|em|strong|sub|sup|italic|bold|underline|br|p|span|small|monospace|roman|sans-serif|tex-math|inline-formula|mml:[a-z]+)(?:\s[^<>]*)?/?>`)

var whitespaceRun = regexp.MustCompile(`\s+`)

// cleanMarkupText strips inline publisher markup and decodes HTML entities.
//
// Unescaping runs FIRST so that a double-escaped tag ("&lt;scp&gt;") becomes a
// real tag and is then removed; running it last would leave those visible.
func cleanMarkupText(s string) string {
	if s == "" {
		return s
	}
	s = html.UnescapeString(s)
	s = markupTagPattern.ReplaceAllString(s, "")
	s = whitespaceRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}