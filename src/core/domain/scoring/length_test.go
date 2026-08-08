package scoring

import "testing"

func TestWordCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"whitespace", "   \t\n  ", 0},
		{"one", "Hello", 1},
		{"simple", "I told my boss I needed a raise", 8},
		{"multi-space", "a  b   c", 3},
		{"newlines", "line one\nline two", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := WordCount(tc.text); got != tc.want {
				t.Fatalf("WordCount(%q) = %d, want %d", tc.text, got, tc.want)
			}
		})
	}
}

func TestClassifyLength(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		want string
	}{
		{"empty_is_short", "", LengthShort},
		{"boundary_15_short", nWords(15), LengthShort},
		{"boundary_16_medium", nWords(16), LengthMedium},
		{"boundary_40_medium", nWords(40), LengthMedium},
		{"boundary_41_long", nWords(41), LengthLong},
		{"long", nWords(100), LengthLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyLength(tc.text); got != tc.want {
				t.Fatalf("ClassifyLength(%d words) = %q, want %q",
					WordCount(tc.text), got, tc.want)
			}
		})
	}
}

func nWords(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, 'w')
	}
	return string(b)
}
