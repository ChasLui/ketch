package extract

import "unicode/utf8"

// Truncate caps s at maxChars Unicode code points, appending a truncation
// marker. maxChars <= 0 disables truncation.
func Truncate(s string, maxChars int) string {
	if maxChars <= 0 || utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxChars]) + "\n\n[truncated]"
}

// PostProcess applies trim (StripMarkdown) then Truncate to markdown content.
// It is the shared output post-processing step for the CLI's --trim/--max-chars
// flags and the MCP tools' trim/max_chars params.
func PostProcess(s string, trim bool, maxChars int) string {
	if trim {
		s = StripMarkdown(s)
	}
	return Truncate(s, maxChars)
}
