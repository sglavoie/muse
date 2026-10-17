package sorting

import "testing"

func TestUnknownSentinelsUseLowercaseRatherThanUnicodeEquivalence(t *testing.T) {
	// Long s is case-fold equivalent to s, but is not its lowercase form.
	// Optimizing comparisons must not turn a distinct artist into a sentinel.
	if isUnknownValue("Unknown artiſt") {
		t.Fatal("a distinct Unicode name became unknown")
	}
	if !isUnknownValue("UNKNOWN ARTIST") {
		t.Fatal("uppercase sentinel became a named artist")
	}
	if !CompareStringsUnknownLast("Unknown artiſt", "Unknown Artist", SortAsc) ||
		!CompareStringsUnknownLast("Unknown artiſt", "Unknown Artist", SortDesc) {
		t.Fatal("known artist must precede unknown in both directions")
	}
}
