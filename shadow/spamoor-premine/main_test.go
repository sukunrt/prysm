package main

import "testing"

// The vector is run 66's eoatx instance: seed r66-eoa-pub on ethshadow's
// DEFAULT_MNEMONIC account 0, whose first child spamoor funded on chain.
func TestWallets(t *testing.T) {
	got, err := wallets(instance{
		name:    "spamoor_eoatx_public",
		privkey: "0x306cb89d3f8c1da466d8c2762b600b98e911dd45d0daa885c073ac94f45ded31",
		seed:    "r66-eoa-pub",
		count:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"0x500502A21d83e342193AAeD5b23C7091a9cbffE6",
		"0x6896C8165d9d1C1E35aF1b41220A3f65E8a9CC9D",
		"0x58dfbe341C345904E945941ed063C312aFC57105",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d addresses, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("address %d: got %s, want %s", i, got[i], want[i])
		}
	}
}
