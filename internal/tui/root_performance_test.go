package tui

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// BenchmarkLibraryInteraction includes the root key handling and rendering used
// while typing and navigating, rather than measuring the matcher in isolation.
// Setup is excluded. Run without -race for representative interactive latency.
func BenchmarkLibraryInteraction(b *testing.B) {
	for _, count := range []int{10_000, 100_000} {
		tracks := make([]library.Track, count)
		for i := range tracks {
			tracks[i] = library.Track{
				Hash: fmt.Sprintf("h%d", i), Path: fmt.Sprintf("/music/%d.flac", i),
				Title: fmt.Sprintf("Étude number %06d", i), Artist: fmt.Sprintf("Artist %04d", i%1000),
				Album: fmt.Sprintf("Album %04d", i%200), Duration: 180,
			}
		}
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			m := New(context.Background(), WithLibraryTracks(tracks))
			m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
			b.Run("type", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					next := pressSlash(m)
					for _, r := range "etude 09" {
						next, _, _ = next.handleFilterKeyPress(tea.KeyPressMsg{Code: r, Text: string(r)})
						_ = next.View()
					}
				}
			})
			// Measure each edit with its preceding query already applied. This
			// exposes broad-query latency that an aggregate sequence can hide.
			for _, query := range []string{"e", "etude", "etude 09", "t:etude number 09", "not-found"} {
				b.Run("key/"+query, func(b *testing.B) {
					base := pressSlash(m)
					runes := []rune(query)
					for _, r := range runes[:len(runes)-1] {
						base, _, _ = base.handleFilterKeyPress(tea.KeyPressMsg{Code: r, Text: string(r)})
					}
					key := tea.KeyPressMsg{Code: runes[len(runes)-1], Text: string(runes[len(runes)-1])}
					// Warm the normalization cache even when running this sub-benchmark alone.
					_, _, _ = base.handleFilterKeyPress(key)
					b.ReportAllocs()
					for b.Loop() {
						next, _, _ := base.handleFilterKeyPress(key)
						_ = next.View()
					}
				})
			}
			b.Run("render", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_ = m.View()
				}
			})
			b.Run("sort", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					v := m.library
					v.SortState.Field = sorting.SortTitle
					v = v.RecomputeSortedTracks()
					_ = v
				}
			})
		})
	}
}
