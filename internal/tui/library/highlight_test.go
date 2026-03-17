package library

import "testing"

func TestSearchFieldOffsetsAllBasic(t *testing.T) {
	aOff, aLen, tOff, tLen, lOff, lLen := searchFieldOffsetsAll("Alice", "Song One", "Debut")
	if aOff != 0 {
		t.Errorf("artistOffset: expected 0, got %d", aOff)
	}
	if aLen != 5 {
		t.Errorf("artistLen: expected 5, got %d", aLen)
	}
	// "Alice" (5) + SP = title starts at 6
	if tOff != 6 {
		t.Errorf("titleOffset: expected 6, got %d", tOff)
	}
	if tLen != 8 {
		t.Errorf("titleLen: expected 8, got %d", tLen)
	}
	// 6 + 8 + 1 = 15
	if lOff != 15 {
		t.Errorf("albumOffset: expected 15, got %d", lOff)
	}
	if lLen != 5 {
		t.Errorf("albumLen: expected 5, got %d", lLen)
	}
}

func TestSearchFieldOffsetsAllEmptyAlbum(t *testing.T) {
	_, _, _, _, lOff, lLen := searchFieldOffsetsAll("Art", "Tit", "")
	// "Art" (3) + SP + "Tit" (3) + SP = 8
	if lOff != 8 {
		t.Errorf("albumOffset with empty album: expected 8, got %d", lOff)
	}
	if lLen != 0 {
		t.Errorf("albumLen with empty album: expected 0, got %d", lLen)
	}
}

func TestFieldMatchPositionsAlbum(t *testing.T) {
	// Layout: "Alice Song One Debut file.mp3"
	// Artist: 0-4, Title: 6-13, Album: 15-19
	_, _, _, _, albumOff, albumLen := searchFieldOffsetsAll("Alice", "Song One", "Debut")
	matchedIndexes := []int{15, 16, 17} // "Deb" in album
	got := fieldMatchPositions(matchedIndexes, albumOff, albumLen)
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Errorf("fieldMatchPositions for album: expected [0 1 2], got %v", got)
	}
}

func TestFieldMatchPositionsIgnoresOutOfRange(t *testing.T) {
	_, _, _, _, albumOff, albumLen := searchFieldOffsetsAll("Alice", "Song One", "Debut")
	// positions in artist and title should be excluded
	matchedIndexes := []int{0, 1, 6, 7, 15}
	got := fieldMatchPositions(matchedIndexes, albumOff, albumLen)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("fieldMatchPositions filter: expected [0], got %v", got)
	}
}
