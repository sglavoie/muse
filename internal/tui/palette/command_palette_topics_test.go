package palette

import "testing"

func TestTopicSearchFavorsInspectionButExplicitSearchKeepsIntent(t *testing.T) {
	usage := NewUsageStore("")
	for range 100 {
		usage.Bump("clear")
	}
	cp, _ := NewCommandPalette([]PaletteCommand{
		{ID: "clear", Name: "Clear queue", Intent: IntentRemoval},
		{ID: "open", Name: "Go to Queue", Intent: IntentInspect},
		{ID: "summary", Name: "Show queue summary", Intent: IntentInspect},
		{ID: "append", Name: "Enqueue tracks", Intent: IntentListening},
		{ID: "disabled", Name: "Queue", Intent: IntentInspect, Enabled: func() bool { return false }},
	}, usage)
	for _, tc := range []struct{ query, want string }{
		{" queue ", "disabled"}, // exact name still wins and retains its explanation
		{"clear queue", "clear"},
		{"clear", "clear"},
		{"go to queue", "open"},
	} {
		cp.Filter.Text = tc.query
		cp.Refilter()
		if cp.Filtered[0].ID != tc.want {
			t.Fatalf("%q selects %s, want %s", tc.query, cp.Filtered[0].ID, tc.want)
		}
	}
	cp.all = cp.all[:4]
	cp.Filter.Text = "QUEUE"
	cp.Refilter()
	if cp.Filtered[0].ID != "open" || cp.Filtered[1].ID != "summary" || cp.Filtered[len(cp.Filtered)-1].ID != "clear" {
		t.Fatalf("topic ranking = %+v", cp.Filtered)
	}
}

func TestTopicSearchPreservesExactAlias(t *testing.T) {
	cp, _ := NewCommandPalette([]PaletteCommand{
		{ID: "open", Name: "Go to Queue", Intent: IntentInspect},
		{ID: "alias", Name: "Inspect playback", Aliases: []string{"queue"}},
	}, nil)
	cp.Filter.Text = "queue"
	cp.Refilter()
	if cp.Filtered[0].ID != "alias" {
		t.Fatal("exact alias lost to broad topic priority")
	}
}
