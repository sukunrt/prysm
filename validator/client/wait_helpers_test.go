package client

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestSlotComponentDeadline(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	cfg := params.BeaconConfig()
	v := &validator{genesisTime: time.Unix(1700000000, 0)}
	slot := primitives.Slot(5)
	component := cfg.AttestationDueBPS

	got, err := v.slotComponentDeadline(slot, component)
	require.NoError(t, err)

	startTime, err := slots.StartTime(v.genesisTime, slot)
	require.NoError(t, err)
	expected := startTime.Add(cfg.SlotComponentDuration(component))

	require.Equal(t, expected, got)
}

func TestSlotComponentSpanName(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	cfg := params.BeaconConfig()
	v := &validator{}
	tests := []struct {
		name      string
		component primitives.BP
		expected  string
	}{
		{
			name:      "attestation",
			component: cfg.AttestationDueBPS,
			expected:  "validator.waitAttestationWindow",
		},
		{
			name:      "aggregate",
			component: cfg.AggregateDueBPS,
			expected:  "validator.waitAggregateWindow",
		},
		{
			name:      "default",
			component: cfg.AttestationDueBPS + 7,
			expected:  "validator.waitSlotComponent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, v.slotComponentSpanName(tt.component))
		})
	}
}

func TestWaitUntilSlotComponent_ContextCancelReturnsImmediately(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = 10000
	params.OverrideBeaconConfig(cfg)

	v := &validator{genesisTime: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		v.waitUntilSlotComponent(ctx, 1, cfg.AttestationDueBPS)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitUntilSlotComponent did not return after context cancellation")
	}
}

func TestFFGVoteJitter(t *testing.T) {
	assert.Equal(t, time.Duration(0), ffgVoteJitter(0))
	assert.Equal(t, time.Duration(0), ffgVoteJitter(-time.Second))

	const bound = 200 * time.Millisecond
	distinct := make(map[time.Duration]bool)
	for range 1000 {
		got := ffgVoteJitter(bound)
		require.Equal(t, true, got >= 0, "jitter is negative")
		require.Equal(t, true, got < bound, "jitter is not below the bound")
		distinct[got] = true
	}
	// A constant would be a silent regression of the anti-burst property.
	require.Equal(t, true, len(distinct) > 1, "jitter never varied")
}

func TestWaitSlotStartJitter(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = 200
	params.OverrideBeaconConfig(cfg)
	reset := features.InitWithReset(&features.Flags{
		DecoupledFFGVoteAtSlotStart: true,
		DecoupledFFGVoteJitter:      50 * time.Millisecond,
	})
	defer reset()

	// A slot that started long ago never waits, whatever the jitter draws.
	v := &validator{genesisTime: time.Now().Add(-time.Hour)}
	start := time.Now()
	v.waitSlotStartJitter(t.Context(), 0)
	assert.Equal(t, true, time.Since(start) < 50*time.Millisecond, "waited for a past slot")

	// A slot that has not started yet waits at most the slot offset plus the bound.
	genesis := time.Now()
	v = &validator{genesisTime: genesis}
	start = time.Now()
	v.waitSlotStartJitter(t.Context(), 1)
	waited := time.Since(start)
	slotDuration := params.BeaconConfig().SlotDuration()
	assert.Equal(t, true, waited >= slotDuration, "returned before the slot started")
	assert.Equal(t, true, waited < slotDuration+time.Second, "waited past the jitter bound")

}

func TestWaitSlotStartJitterCancelled(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	reset := features.InitWithReset(&features.Flags{DecoupledFFGVoteJitter: time.Hour})
	defer reset()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	v := &validator{genesisTime: time.Now()}
	start := time.Now()
	v.waitSlotStartJitter(ctx, 1)
	assert.Equal(t, true, time.Since(start) < time.Second, "cancellation ignored")
}

func TestLatePublishDelay(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = 12000
	params.OverrideBeaconConfig(cfg)

	tests := []struct {
		name     string
		idx      primitives.ValidatorIndex
		bps      uint64
		everyNth uint64
		want     time.Duration
	}{
		{name: "knob off", idx: 3, bps: 0, everyNth: 1, want: 0},
		{name: "every proposer", idx: 3, bps: 5000, everyNth: 1, want: 6 * time.Second},
		{name: "zero nth reads as every proposer", idx: 7, bps: 2500, everyNth: 0,
			want: 3 * time.Second},
		{name: "selected proposer", idx: 6, bps: 5000, everyNth: 3, want: 6 * time.Second},
		{name: "unselected proposer", idx: 7, bps: 5000, everyNth: 3, want: 0},
		{name: "index zero is selected", idx: 0, bps: 5000, everyNth: 4, want: 6 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, latePublishDelay(tt.idx, tt.bps, tt.everyNth))
		})
	}
}

func TestWaitLatePublish(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotDurationMilliseconds = 400
	params.OverrideBeaconConfig(cfg)

	pubKey := [fieldparams.BLSPubkeyLength]byte{1}
	newValidator := func(idx primitives.ValidatorIndex, genesis time.Time) *validator {
		v := &validator{genesisTime: genesis, duties: &dutyStore{}}
		v.duties.write(dutyStoreData{
			initialized: true,
			currentDuties: map[pubkey]*ethpb.ValidatorDuty{
				pubKey: {ValidatorIndex: idx, PublicKey: pubKey[:]},
			},
		})
		return v
	}

	// Knob off: no wait at all.
	reset := features.InitWithReset(&features.Flags{})
	v := newValidator(0, time.Now())
	start := time.Now()
	v.waitLatePublish(t.Context(), 1, pubKey)
	reset()
	assert.Equal(t, true, time.Since(start) < 100*time.Millisecond, "waited with the knob off")

	// Knob on, proposer selected: waits into the slot.
	reset = features.InitWithReset(&features.Flags{
		DecoupledLateBlockPublishBPS:      5000,
		DecoupledLateBlockPublishEveryNth: 2,
	})
	defer reset()
	genesis := time.Now()
	v = newValidator(4, genesis)
	start = time.Now()
	v.waitLatePublish(t.Context(), 1, pubKey)
	waited := time.Since(start)
	slotDuration := params.BeaconConfig().SlotDuration()
	assert.Equal(t, true, waited >= slotDuration, "published before the delay elapsed")

	// Knob on, proposer not selected: no wait.
	v = newValidator(5, time.Now())
	start = time.Now()
	v.waitLatePublish(t.Context(), 1, pubKey)
	assert.Equal(t, true, time.Since(start) < 100*time.Millisecond, "delayed an unselected proposer")
}
