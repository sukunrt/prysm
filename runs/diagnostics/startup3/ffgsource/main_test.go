package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

func TestParseSlots(t *testing.T) {
	got, err := parseSlots("3, 1,2")
	if err != nil {
		t.Fatal(err)
	}
	want := []primitives.Slot{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slot %d = %d, want %d", i, got[i], want[i])
		}
	}
	if _, err := parseSlots("1,1"); err == nil {
		t.Fatal("duplicate slots accepted")
	}
}

func TestReadMnemonicYAML(t *testing.T) {
	for _, contents := range []string{"- one two three four\n", "- mnemonic: one two three four\n  count: 120000\n"} {
		path := filepath.Join(t.TempDir(), "mnemonics.yaml")
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readMnemonic(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != "one two three four" {
			t.Fatalf("mnemonic = %q", got)
		}
	}
}
