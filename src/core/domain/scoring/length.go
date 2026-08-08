package scoring

// Length thresholds (word count). Tunable named constants.
const (
	LengthShortMax  = 15 // Short:  ≤ LengthShortMax
	LengthMediumMax = 40 // Medium: LengthShortMax+1 … LengthMediumMax
	// Long: ≥ LengthMediumMax+1
)

// Length categories returned by ClassifyLength.
const (
	LengthShort  = "Short"
	LengthMedium = "Medium"
	LengthLong   = "Long"
)

// WordCount returns the number of whitespace-separated tokens in text.
// Empty / whitespace-only strings yield 0.
func WordCount(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if isWhitespace(r) {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}

// ClassifyLength buckets joke text by word count into Short / Medium / Long.
func ClassifyLength(text string) string {
	n := WordCount(text)
	switch {
	case n <= LengthShortMax:
		return LengthShort
	case n <= LengthMediumMax:
		return LengthMedium
	default:
		return LengthLong
	}
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
