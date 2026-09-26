package app

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestBridgeScanChannel(t *testing.T) {
	raw := make(chan library.Track, 3)
	raw <- library.Track{Hash: "a", Path: "/a.mp3"}
	raw <- library.Track{Hash: "b", Path: "/b.mp3"}
	raw <- library.Track{Hash: "c", Path: "/c.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	ch, _ := bridgeScanChannel(context.Background(), raw, writer, nil, 2)

	var got []library.Track
	for track := range ch {
		got = append(got, track)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 forwarded tracks, got %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Hash != want {
			t.Errorf("track %d: got hash %q, want %q", i, got[i].Hash, want)
		}
	}

	upserted := writer.allTracks()
	if len(upserted) != 3 {
		t.Fatalf("expected 3 upserted tracks, got %d", len(upserted))
	}
}

func TestBridgeScanChannel_EmptyInput(t *testing.T) {
	raw := make(chan library.Track)
	close(raw)

	writer := &stubUpsertBatcher{}
	ch, _ := bridgeScanChannel(context.Background(), raw, writer, nil, 10)

	var got []library.Track
	for track := range ch {
		got = append(got, track)
	}

	if len(got) != 0 {
		t.Fatalf("expected 0 tracks, got %d", len(got))
	}
	if len(writer.batches) != 0 {
		t.Fatalf("expected 0 batches, got %d", len(writer.batches))
	}
}

func TestBridgeScanChannel_CreatedAtSetBeforeForward(t *testing.T) {
	raw := make(chan library.Track, 1)
	raw <- library.Track{Hash: "s1", Path: "/s1.mp3"}
	close(raw)

	writer := &stubUpsertBatcher{}
	ch, _ := bridgeScanChannel(context.Background(), raw, writer, nil, 10)

	track := <-ch
	if track.CreatedAt == 0 {
		t.Error("TUI channel received track with CreatedAt=0; ensureCreatedAt was not applied before forwarding")
	}
}
