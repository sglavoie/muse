package filter

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoryPersistenceRoundTripAndSanitization(t *testing.T) {
	dir := t.TempDir()
	history := []string{"", "   "}
	for i := range 25 {
		history = append(history, fmt.Sprint(i))
	}
	history = append(history, "10")
	if err := SaveHistory(dir, history); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 20 || got[0] != "5" || got[19] != "10" {
		t.Fatal(got)
	}
	if err := SaveHistory(dir, []string{"Björk", "東京"}); err != nil {
		t.Fatal(err)
	}
	got, err = LoadHistory(dir)
	if err != nil || !reflect.DeepEqual(got, []string{"Björk", "東京"}) {
		t.Fatal(got, err)
	}
	info, err := os.Stat(filepath.Join(dir, historyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal(info.Mode())
	}
}

func TestHistoryPersistenceUnavailable(t *testing.T) {
	for _, dir := range []string{"", t.TempDir()} {
		got, err := LoadHistory(dir)
		if err != nil || len(got) != 0 {
			t.Fatal(got, err)
		}
	}
	if err := SaveHistory("", []string{"ignored"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, historyFile)
	if err := os.WriteFile(path, []byte("broken json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadHistory(dir); err == nil || got != nil {
		t.Fatal(got, err)
	}
	if err := SaveHistory(path, []string{"unwritable"}); err == nil {
		t.Fatal("expected error writing below a file")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "broken json" {
		t.Fatal(string(data), err)
	}
}
