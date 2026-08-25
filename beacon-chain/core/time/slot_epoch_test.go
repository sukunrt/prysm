package time_test

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestSlotToEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: 0},
		{slot: 50, epoch: 1},
		{slot: 64, epoch: 2},
		{slot: 128, epoch: 4},
		{slot: 200, epoch: 6},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.epoch, slots.ToEpoch(tt.slot), "ToEpoch(%d)", tt.slot)
	}
}

func TestCurrentEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: 0},
		{slot: 50, epoch: 1},
		{slot: 64, epoch: 2},
		{slot: 128, epoch: 4},
		{slot: 200, epoch: 6},
	}
	for _, tt := range tests {
		st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{Slot: tt.slot})
		require.NoError(t, err)
		assert.Equal(t, tt.epoch, time.CurrentEpoch(st), "ActiveCurrentEpoch(%d)", st.Slot())
	}
}

func TestPrevEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: 0},
		{slot: 0 + params.BeaconConfig().SlotsPerEpoch + 1, epoch: 0},
		{slot: 2 * params.BeaconConfig().SlotsPerEpoch, epoch: 1},
	}
	for _, tt := range tests {
		st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{Slot: tt.slot})
		require.NoError(t, err)
		assert.Equal(t, tt.epoch, time.PrevEpoch(st), "ActivePrevEpoch(%d)", st.Slot())
	}
}

func TestNextEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: primitives.Epoch(0/params.BeaconConfig().SlotsPerEpoch + 1)},
		{slot: 50, epoch: primitives.Epoch(0/params.BeaconConfig().SlotsPerEpoch + 2)},
		{slot: 64, epoch: primitives.Epoch(64/params.BeaconConfig().SlotsPerEpoch + 1)},
		{slot: 128, epoch: primitives.Epoch(128/params.BeaconConfig().SlotsPerEpoch + 1)},
		{slot: 200, epoch: primitives.Epoch(200/params.BeaconConfig().SlotsPerEpoch + 1)},
	}
	for _, tt := range tests {
		st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{Slot: tt.slot})
		require.NoError(t, err)
		assert.Equal(t, tt.epoch, time.NextEpoch(st), "NextEpoch(%d)", st.Slot())
	}
}

func TestCanProcessEpoch_TrueOnEpochsLastSlot(t *testing.T) {
	tests := []struct {
		slot            primitives.Slot
		canProcessEpoch bool
	}{
		{
			slot:            1,
			canProcessEpoch: false,
		}, {
			slot:            63,
			canProcessEpoch: true,
		},
		{
			slot:            64,
			canProcessEpoch: false,
		}, {
			slot:            127,
			canProcessEpoch: true,
		}, {
			slot:            1000000000,
			canProcessEpoch: false,
		},
	}

	for _, tt := range tests {
		b := &eth.BeaconState{Slot: tt.slot}
		s, err := state_native.InitializeFromProtoPhase0(b)
		require.NoError(t, err)
		assert.Equal(t, tt.canProcessEpoch, time.CanProcessEpoch(s), "CanProcessEpoch(%d)", tt.slot)
	}
}

func TestAltairCompatible(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 1
	cfg.BellatrixForkEpoch = 2
	params.OverrideBeaconConfig(cfg)

	type args struct {
		s state.BeaconState
		e primitives.Epoch
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "phase0 state",
			args: args{
				s: func() state.BeaconState {
					st, _ := util.DeterministicGenesisState(t, 1)
					return st
				}(),
			},
			want: false,
		},
		{
			name: "altair state, altair epoch",
			args: args{
				s: func() state.BeaconState {
					st, _ := util.DeterministicGenesisStateAltair(t, 1)
					return st
				}(),
				e: params.BeaconConfig().AltairForkEpoch,
			},
			want: true,
		},
		{
			name: "bellatrix state, bellatrix epoch",
			args: args{
				s: func() state.BeaconState {
					st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
					return st
				}(),
				e: params.BeaconConfig().BellatrixForkEpoch,
			},
			want: true,
		},
		{
			name: "bellatrix state, altair epoch",
			args: args{
				s: func() state.BeaconState {
					st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
					return st
				}(),
				e: params.BeaconConfig().AltairForkEpoch,
			},
			want: true,
		},
		{
			name: "bellatrix state, phase0 epoch",
			args: args{
				s: func() state.BeaconState {
					st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
					return st
				}(),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := time.HigherEqualThanAltairVersionAndEpoch(tt.args.s, tt.args.e); got != tt.want {
				t.Errorf("HigherEqualThanAltairVersionAndEpoch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanUpgradeTo(t *testing.T) {
	cfg := params.BeaconConfig()

	outerTestCases := []struct {
		name        string
		forkEpoch   *primitives.Epoch
		upgradeFunc func(primitives.Slot) bool
	}{
		{
			name:        "Altair",
			forkEpoch:   &cfg.AltairForkEpoch,
			upgradeFunc: time.CanUpgradeToAltair,
		},
		{
			name:        "Bellatrix",
			forkEpoch:   &cfg.BellatrixForkEpoch,
			upgradeFunc: time.CanUpgradeToBellatrix,
		},
		{
			name:        "Capella",
			forkEpoch:   &cfg.CapellaForkEpoch,
			upgradeFunc: time.CanUpgradeToCapella,
		},
		{
			name:        "Deneb",
			forkEpoch:   &cfg.DenebForkEpoch,
			upgradeFunc: time.CanUpgradeToDeneb,
		},
		{
			name:        "Electra",
			forkEpoch:   &cfg.ElectraForkEpoch,
			upgradeFunc: time.CanUpgradeToElectra,
		},
		{
			name:        "Fulu",
			forkEpoch:   &cfg.FuluForkEpoch,
			upgradeFunc: time.CanUpgradeToFulu,
		},
		{
			name:        "Gloas",
			forkEpoch:   &cfg.GloasForkEpoch,
			upgradeFunc: time.CanUpgradeToGloas,
		},
	}

	for _, otc := range outerTestCases {
		params.SetupTestConfigCleanup(t)
		*otc.forkEpoch = 5
		params.OverrideBeaconConfig(cfg)

		innerTestCases := []struct {
			name string
			slot primitives.Slot
			want bool
		}{
			{
				name: "not epoch start",
				slot: 1,
				want: false,
			},
			{
				name: fmt.Sprintf("not %s epoch", otc.name),
				slot: params.BeaconConfig().SlotsPerEpoch,
				want: false,
			},
			{
				name: fmt.Sprintf("%s epoch", otc.name),
				slot: primitives.Slot(*otc.forkEpoch) * params.BeaconConfig().SlotsPerEpoch,
				want: true,
			},
		}

		for _, itc := range innerTestCases {
			t.Run(fmt.Sprintf("%s-%s", otc.name, itc.name), func(t *testing.T) {
				if got := otc.upgradeFunc(itc.slot); got != itc.want {
					t.Errorf("CanUpgradeTo%s() = %v, want %v", otc.name, got, itc.want)
				}
			})
		}
	}
}

// TestCanProcessRound_At8Over32 pins the round cadence at the devnet's non-identity
// shape: four round boundaries per 32-slot epoch, only the last of which is also an
// epoch boundary.
func TestCanProcessRound_At8Over32(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)

	st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{})
	require.NoError(t, err)

	var roundBoundaries, epochBoundaries []primitives.Slot
	for slot := primitives.Slot(0); slot < 32; slot++ {
		require.NoError(t, st.SetSlot(slot))
		if time.CanProcessRound(st) {
			roundBoundaries = append(roundBoundaries, slot)
		}
		if time.CanProcessEpoch(st) {
			epochBoundaries = append(epochBoundaries, slot)
		}
	}
	assert.DeepEqual(t, []primitives.Slot{7, 15, 23, 31}, roundBoundaries)
	assert.DeepEqual(t, []primitives.Slot{31}, epochBoundaries)
}

// TestCanProcessRound_IdentityUnderShippedConfigs is the identity rule for the cadence:
// with SlotsPerRound == SlotsPerEpoch every round boundary is an epoch boundary and
// vice versa, so the new hook fires exactly where epoch processing already did.
func TestCanProcessRound_IdentityUnderShippedConfigs(t *testing.T) {
	require.Equal(t, params.BeaconConfig().SlotsPerEpoch, params.BeaconConfig().SlotsPerRound)

	st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{})
	require.NoError(t, err)
	for slot := primitives.Slot(0); slot < params.BeaconConfig().SlotsPerEpoch*3; slot++ {
		require.NoError(t, st.SetSlot(slot))
		assert.Equal(t, time.CanProcessEpoch(st), time.CanProcessRound(st), "slot %d", slot)
	}
}

// TestCurrentRound_SourceFreshnessGateAt8Over32 pins the predicate the attestation-data
// server uses to decide whether the head state must be advanced before a validator reads
// its justified checkpoint (plan-finality-round 2.6). Keyed on rounds it fires at every
// round boundary; the epoch-keyed predicate it replaced missed three boundaries out of
// four at 8/32, which would have handed validators a round-stale source.
func TestCurrentRound_SourceFreshnessGateAt8Over32(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)

	st, err := state_native.InitializeFromProtoPhase0(&eth.BeaconState{})
	require.NoError(t, err)

	// The head state sits one slot behind each round's first slot -- the common case
	// when a validator asks for attestation data at the start of a new round.
	for _, requestSlot := range []primitives.Slot{8, 16, 24, 32} {
		require.NoError(t, st.SetSlot(requestSlot-1))
		assert.Equal(t, true, time.CurrentRound(st) < slots.RoundAt(requestSlot),
			"round gate should fire for request slot %d", requestSlot)
	}
	// The epoch-keyed gate only fires at slot 32, the one boundary that is also an
	// epoch boundary.
	for _, requestSlot := range []primitives.Slot{8, 16, 24} {
		require.NoError(t, st.SetSlot(requestSlot-1))
		assert.Equal(t, false, time.CurrentEpoch(st) < slots.ToEpoch(requestSlot),
			"epoch gate must not fire for request slot %d", requestSlot)
	}
}
