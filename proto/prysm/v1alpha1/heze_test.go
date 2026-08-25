package eth_test

import (
	"testing"

	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// TestBeaconStateHeze_MatchesGloas proves that BeaconStateHeze is a
// field-for-field copy of BeaconStateGloas: the same SSZ bytes decode into
// both containers, re-encode identically, and hash to the same root.
func TestBeaconStateHeze_MatchesGloas(t *testing.T) {
	st, err := util.NewBeaconStateGloas(fillGloasVariableLengthFields)
	require.NoError(t, err)

	gs, ok := st.ToProtoUnsafe().(*eth.BeaconStateGloas)
	require.Equal(t, true, ok)

	enc, err := gs.MarshalSSZ()
	require.NoError(t, err)

	hs := &eth.BeaconStateHeze{}
	require.NoError(t, hs.UnmarshalSSZ(enc))

	require.Equal(t, gs.SizeSSZ(), hs.SizeSSZ())

	reEnc, err := hs.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, enc, reEnc)

	gr, err := gs.HashTreeRoot()
	require.NoError(t, err)
	hr, err := hs.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, gr, hr)
}

// fillGloasVariableLengthFields populates the offset-encoded fields the
// minimal seed leaves empty, so the round trip exercises them too.
func fillGloasVariableLengthFields(s *eth.BeaconStateGloas) error {
	s.Validators = []*eth.Validator{{
		PublicKey:             make([]byte, 48),
		WithdrawalCredentials: make([]byte, 32),
		EffectiveBalance:      32_000_000_000,
	}}
	s.Balances = []uint64{32_000_000_000}
	s.PreviousEpochParticipation = []byte{1}
	s.CurrentEpochParticipation = []byte{2}
	s.InactivityScores = []uint64{3}
	s.HistoricalRoots = [][]byte{make([]byte, 32)}
	s.Eth1DataVotes = []*eth.Eth1Data{{
		DepositRoot: make([]byte, 32),
		BlockHash:   make([]byte, 32),
	}}
	s.HistoricalSummaries = []*eth.HistoricalSummary{{
		BlockSummaryRoot: make([]byte, 32),
		StateSummaryRoot: make([]byte, 32),
	}}
	s.PendingDeposits = []*eth.PendingDeposit{{
		PublicKey:             make([]byte, 48),
		WithdrawalCredentials: make([]byte, 32),
		Signature:             make([]byte, 96),
	}}
	s.PendingPartialWithdrawals = []*eth.PendingPartialWithdrawal{{}}
	s.PendingConsolidations = []*eth.PendingConsolidation{{}}
	s.Builders = []*eth.Builder{{
		Pubkey:           make([]byte, 48),
		Version:          make([]byte, 1),
		ExecutionAddress: make([]byte, 20),
	}}
	s.BuilderPendingWithdrawals = []*eth.BuilderPendingWithdrawal{{
		FeeRecipient: make([]byte, 20),
	}}
	s.PayloadExpectedWithdrawals = []*enginev1.Withdrawal{{
		Address: make([]byte, 20),
	}}
	return nil
}
