package search

// HasStemMatch checks whether two words share a stem via prefix overlap.
// Returns true if the shared prefix is >= 4 bytes and covers >= 75% of the shorter word.
// Returns false if the words are identical (exact match handled separately).
func HasStemMatch(a, b string) bool {
	if a == b {
		return false
	}
	la, lb := len(a), len(b)
	minLen := la
	if lb < minLen {
		minLen = lb
	}
	if minLen < 4 {
		return false
	}
	shared := 0
	for i := range minLen {
		if a[i] != b[i] {
			break
		}
		shared++
	}
	// shared/minLen >= 0.75, using integer math to avoid float
	return shared >= 4 && shared*4 >= minLen*3
}
