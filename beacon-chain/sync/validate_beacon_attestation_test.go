package sync

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestService_validateCommitteeIndexBeaconAttestation(t *testing.T) {
	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	chain := &mockChain.ChainService{
		// 1 slot ago.
		Genesis:          time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second),
		ValidatorsRoot:   [32]byte{'A'},
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	s.initCaches()
	go s.verifierRoutine()

	invalidRoot := [32]byte{'A', 'B', 'C', 'D'}
	s.setBadBlock(ctx, invalidRoot)

	digest := s.currentForkDigest()

	blk := util.NewBeaconBlock()
	blk.Block.Slot = 1
	util.SaveBlock(t, ctx, db, blk)

	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}

	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(1))
	require.NoError(t, db.SaveState(t.Context(), savedState, validBlockRoot))
	chain.State = savedState

	tests := []struct {
		name                      string
		msg                       ethpb.Att
		topic                     string
		validAttestationSignature bool
		want                      bool
	}{
		{
			name: "valid attestation signature",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &ethpb.Checkpoint{
						Epoch: 0,
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      true,
		},
		{
			name: "valid attestation signature with nil topic",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &ethpb.Checkpoint{
						Epoch: 0,
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     "",
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "bad target epoch",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target: &ethpb.Checkpoint{
						Epoch: 10,
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "already seen",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "invalid beacon block",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: invalidRoot[:],
					CommitteeIndex:  0,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "committee index exceeds committee length",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  4,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_2", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "wrong committee index",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  2,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_2", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "already aggregated",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b1011},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "missing block",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte("missing"), fieldparams.RootLength),
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: true,
			want:                      false,
		},
		{
			name: "invalid attestation",
			msg: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b101},
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            1,
					Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
			},
			topic:                     fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix(),
			validAttestationSignature: false,
			want:                      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.ClearCache()
			chain.ValidAttestation = tt.validAttestationSignature
			if tt.validAttestationSignature {
				com, err := helpers.BeaconCommitteeFromState(t.Context(), savedState, tt.msg.GetData().Slot, tt.msg.GetData().CommitteeIndex)
				require.NoError(t, err)
				domain, err := signing.Domain(savedState.Fork(), tt.msg.GetData().Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
				require.NoError(t, err)
				attRoot, err := signing.ComputeSigningRoot(tt.msg.GetData(), domain)
				require.NoError(t, err)
				for i := 0; ; i++ {
					if tt.msg.GetAggregationBits().BitAt(uint64(i)) {
						tt.msg.SetSignature(keys[com[i]].Sign(attRoot[:]).Marshal())
						break
					}
				}
			} else {
				tt.msg.SetSignature(make([]byte, 96))
			}
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, tt.msg)
			require.NoError(t, err)
			m := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: &tt.topic,
				},
			}
			if tt.topic == "" {
				m.Message.Topic = nil
			}

			res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "", m)
			received := res == pubsub.ValidationAccept
			if received != tt.want {
				t.Fatalf("Did not received wanted validation. Got %v, wanted %v", !tt.want, tt.want)
			}
			if tt.want && err != nil {
				t.Errorf("Non nil error returned: %v", err)
			}
			if tt.want && m.ValidatorData == nil {
				t.Error("Expected validator data to be set")
			}
		})
	}
}

func TestService_validateCommitteeIndexBeaconAttestationElectra(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()

	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	currentSlot := 1 + (primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * params.BeaconConfig().SlotsPerEpoch)
	genesisOffset := time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	chain := &mockChain.ChainService{
		Genesis:          time.Now().Add(-1 * genesisOffset),
		ValidatorsRoot:   params.BeaconConfig().GenesisValidatorsRoot,
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	require.Equal(t, currentSlot, s.cfg.clock.CurrentSlot())
	s.initCaches()
	go s.verifierRoutine()

	digest := s.currentForkDigest()

	blk := util.NewBeaconBlock()
	blk.Block.Slot = s.cfg.clock.CurrentSlot()
	util.SaveBlock(t, ctx, db, blk)

	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{
		Root:  validBlockRoot[:],
		Epoch: 0,
	}

	validators := uint64(64)
	savedState, keys := util.DeterministicGenesisState(t, validators)
	require.NoError(t, savedState.SetSlot(s.cfg.clock.CurrentSlot()))
	require.NoError(t, db.SaveState(t.Context(), savedState, validBlockRoot))
	chain.State = savedState
	committee, err := helpers.BeaconCommitteeFromState(ctx, savedState, s.cfg.clock.CurrentSlot(), 0)
	require.NoError(t, err)

	tests := []struct {
		name string
		msg  ethpb.Att
		want bool
	}{
		{
			name: "valid",
			msg: &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            s.cfg.clock.CurrentSlot(),
					Target: &ethpb.Checkpoint{
						Epoch: s.cfg.clock.CurrentEpoch(),
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: committee[0],
			},
			want: true,
		},
		{
			name: "non-zero committee index in att data",
			msg: &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            s.cfg.clock.CurrentSlot(),
					Target: &ethpb.Checkpoint{
						Epoch: s.cfg.clock.CurrentEpoch(),
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: committee[0],
			},
			want: false,
		},
		{
			name: "attesting index not in committee",
			msg: &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  1,
					Slot:            s.cfg.clock.CurrentSlot(),
					Target: &ethpb.Checkpoint{
						Epoch: s.cfg.clock.CurrentEpoch(),
						Root:  validBlockRoot[:],
					},
					Source: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: 999999,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helpers.ClearCache()
			com, err := helpers.BeaconCommitteeFromState(t.Context(), savedState, tt.msg.GetData().Slot, tt.msg.GetData().CommitteeIndex)
			require.NoError(t, err)
			domain, err := signing.Domain(savedState.Fork(), tt.msg.GetData().Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
			require.NoError(t, err)
			attRoot, err := signing.ComputeSigningRoot(tt.msg.GetData(), domain)
			require.NoError(t, err)
			tt.msg.SetSignature(keys[com[0]].Sign(attRoot[:]).Marshal())
			buf := new(bytes.Buffer)
			_, err = p.Encoding().EncodeGossip(buf, tt.msg)
			require.NoError(t, err)
			topic := fmt.Sprintf("/eth2/%x/beacon_attestation_1", digest) + p.Encoding().ProtocolSuffix()
			m := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: &topic,
				},
			}

			res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "", m)
			received := res == pubsub.ValidationAccept
			if received != tt.want {
				t.Fatalf("Did not received wanted validation. Got %v, wanted %v", !tt.want, tt.want)
			}
			if tt.want && err != nil {
				t.Errorf("Non nil error returned: %v", err)
			}
			if tt.want && m.ValidatorData == nil {
				t.Error("Expected validator data to be set")
			}
		})
	}
}

// A round shorter than an epoch repeats the committees at the same slot offset in every
// round of the epoch. The gossip validator resolves the committee from the attestation's
// own slot, so an attestation from a later round must validate exactly like the first
// round's, with the same committee and the same attester.
func TestService_validateCommitteeIndexBeaconAttestation_RepeatSlotsOfARound(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	cfg.ElectraForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	epochStart, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	// The last of the four repeats is the current slot, so all four are inside
	// ATTESTATION_PROPAGATION_SLOT_RANGE.
	repeats := slots.RoundRepeats(epochStart + 1)
	require.DeepEqual(t, []primitives.Slot{
		epochStart + 1, epochStart + 9, epochStart + 17, epochStart + 25,
	}, repeats)
	currentSlot := repeats[len(repeats)-1]

	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	genesisOffset := time.Duration(currentSlot) * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	chain := &mockChain.ChainService{
		Genesis:          time.Now().Add(-1 * genesisOffset),
		ValidatorsRoot:   params.BeaconConfig().GenesisValidatorsRoot,
		ValidAttestation: true,
		DB:               db,
		Optimistic:       true,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync:         &mockSync.Sync{IsSyncing: false},
			p2p:                 p,
			beaconDB:            db,
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attestationNotifier: (&mockChain.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]any),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	require.Equal(t, currentSlot, s.cfg.clock.CurrentSlot())
	s.initCaches()
	go s.verifierRoutine()

	digest := s.currentForkDigest()

	blk := util.NewBeaconBlock()
	blk.Block.Slot = currentSlot
	util.SaveBlock(t, ctx, db, blk)
	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	chain.FinalizedCheckPoint = &ethpb.Checkpoint{Root: validBlockRoot[:], Epoch: 0}

	savedState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, savedState.SetSlot(currentSlot))
	require.NoError(t, db.SaveState(t.Context(), savedState, validBlockRoot))
	chain.State = savedState

	// 64 validators over a round of 8 slots is one committee of 8 per slot.
	require.Equal(t, uint64(1), helpers.SlotCommitteeCount(64))

	// The four repeat slots carry the same committee; the slot next to them does not.
	firstCommittee, err := helpers.BeaconCommitteeFromState(ctx, savedState, repeats[0], 0)
	require.NoError(t, err)
	require.Equal(t, 8, len(firstCommittee))
	for _, slot := range repeats[1:] {
		committee, err := helpers.BeaconCommitteeFromState(ctx, savedState, slot, 0)
		require.NoError(t, err)
		require.DeepEqual(t, firstCommittee, committee, "committee at slot %d", slot)
	}
	neighbour, err := helpers.BeaconCommitteeFromState(ctx, savedState, repeats[0]+1, 0)
	require.NoError(t, err)
	require.Equal(t, false, reflect.DeepEqual(firstCommittee, neighbour))

	attester := firstCommittee[0]
	for _, slot := range repeats {
		t.Run(fmt.Sprintf("slot %d", slot), func(t *testing.T) {
			att := &ethpb.SingleAttestation{
				Data: &ethpb.AttestationData{
					BeaconBlockRoot: validBlockRoot[:],
					CommitteeIndex:  0,
					Slot:            slot,
					Target:          &ethpb.Checkpoint{Epoch: slots.ToEpoch(slot), Root: validBlockRoot[:]},
					Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AttesterIndex: attester,
			}
			domain, err := signing.Domain(savedState.Fork(), att.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, savedState.GenesisValidatorsRoot())
			require.NoError(t, err)
			attRoot, err := signing.ComputeSigningRoot(att.Data, domain)
			require.NoError(t, err)
			att.SetSignature(keys[attester].Sign(attRoot[:]).Marshal())

			buf := new(bytes.Buffer)
			_, err = p.Encoding().EncodeGossip(buf, att)
			require.NoError(t, err)
			// The subnet follows the slot's offset in the epoch, so each repeat rides a
			// different subnet.
			subnet := helpers.ComputeSubnetForAttestation(64, att)
			topic := fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, subnet) + p.Encoding().ProtocolSuffix()
			m := &pubsub.Message{Message: &pubsubpb.Message{Data: buf.Bytes(), Topic: &topic}}

			res, err := s.validateCommitteeIndexBeaconAttestation(ctx, "", m)
			require.NoError(t, err)
			require.Equal(t, pubsub.ValidationAccept, res)
			require.NotNil(t, m.ValidatorData)
		})
	}
}

func TestService_setSeenUnaggregatedAtt(t *testing.T) {
	s := NewService(t.Context(), WithP2P(p2ptest.NewTestP2P(t)))

	// Helper function to generate key and handle errors in tests
	generateKey := func(t *testing.T, att ethpb.Att) string {
		key, err := generateUnaggregatedAttCacheKey(att)
		require.NoError(t, err)
		return key
	}

	t.Run("phase0", func(t *testing.T) {
		s.initCaches()

		s0c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s0c0a1 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1010},
		}
		s0c0a2 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1100},
		}
		s0c1a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 1},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s0c2a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 0, CommitteeIndex: 2},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s1c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 1, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s2c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 2, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}
		s3c0a0 := &ethpb.Attestation{
			Data:            &ethpb.AttestationData{Slot: 3, CommitteeIndex: 0},
			AggregationBits: bitfield.Bitlist{0b1001},
		}

		t.Run("empty cache", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key))
		})
		t.Run("ok", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			first := s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
			assert.Equal(t, true, first)
		})
		t.Run("already seen", func(t *testing.T) {
			key := generateKey(t, s3c0a0)
			first := s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
			assert.Equal(t, true, first)
			first = s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
			assert.Equal(t, false, first)
		})
		t.Run("different slot", func(t *testing.T) {
			key1 := generateKey(t, s1c0a0)
			key2 := generateKey(t, s2c0a0)
			first := s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
			assert.Equal(t, true, first)
		})
		t.Run("different committee index", func(t *testing.T) {
			key1 := generateKey(t, s0c1a0)
			key2 := generateKey(t, s0c2a0)
			first := s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
			assert.Equal(t, true, first)
		})
		t.Run("different bit", func(t *testing.T) {
			key1 := generateKey(t, s0c0a1)
			key2 := generateKey(t, s0c0a2)
			first := s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
			assert.Equal(t, true, first)
		})
		t.Run("0 bits set is considered not seen", func(t *testing.T) {
			a := &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b1000}}
			_, err := generateUnaggregatedAttCacheKey(a)
			require.Equal(t, err != nil, true, "Should error because no bits set is invalid")
		})
		t.Run("multiple bits set is considered not seen", func(t *testing.T) {
			a := &ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b1111}}
			_, err := generateUnaggregatedAttCacheKey(a)
			require.Equal(t, err != nil, true, "Should error because no bits set is invalid")
		})
	})
	t.Run("electra", func(t *testing.T) {
		s.initCaches()

		s0c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   0,
			AttesterIndex: 0,
		}
		s0c0a1 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   0,
			AttesterIndex: 1,
		}
		s0c0a2 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   0,
			AttesterIndex: 2,
		}
		s0c1a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   1,
			AttesterIndex: 0,
		}
		s0c2a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 0},
			CommitteeId:   2,
			AttesterIndex: 0,
		}
		s1c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 1},
			CommitteeId:   0,
			AttesterIndex: 0,
		}
		s2c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 2},
			CommitteeId:   0,
			AttesterIndex: 0,
		}
		s3c0a0 := &ethpb.SingleAttestation{
			Data:          &ethpb.AttestationData{Slot: 2},
			CommitteeId:   0,
			AttesterIndex: 0,
		}

		t.Run("empty cache", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key))
		})
		t.Run("ok", func(t *testing.T) {
			key := generateKey(t, s0c0a0)
			first := s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
			assert.Equal(t, true, first)
		})
		t.Run("different slot", func(t *testing.T) {
			key1 := generateKey(t, s1c0a0)
			key2 := generateKey(t, s2c0a0)
			first := s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
			assert.Equal(t, true, first)
		})
		t.Run("already seen", func(t *testing.T) {
			key := generateKey(t, s3c0a0)
			first := s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
			assert.Equal(t, true, first)
			first = s.setSeenUnaggregatedAtt(key)
			assert.Equal(t, true, s.hasSeenUnaggregatedAtt(key))
			assert.Equal(t, false, first)
		})
		t.Run("different committee index", func(t *testing.T) {
			key1 := generateKey(t, s0c1a0)
			key2 := generateKey(t, s0c2a0)
			first := s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
			assert.Equal(t, true, first)
		})
		t.Run("different attester", func(t *testing.T) {
			key1 := generateKey(t, s0c0a1)
			key2 := generateKey(t, s0c0a2)
			first := s.setSeenUnaggregatedAtt(key1)
			assert.Equal(t, false, s.hasSeenUnaggregatedAtt(key2))
			assert.Equal(t, true, first)
		})
		t.Run("single attestation is considered not seen", func(t *testing.T) {
			a := &ethpb.AttestationElectra{}
			_, err := generateUnaggregatedAttCacheKey(a)
			require.Equal(t, err != nil, true, "Should error because no bits set is invalid")
		})
	})
}

func Test_validateCommitteeIndexAndCount_Boundary(t *testing.T) {
	ctx := t.Context()

	// Create a minimal state with a known number of validators.
	validators := uint64(64)
	bs, _ := util.DeterministicGenesisState(t, validators)
	require.NoError(t, bs.SetSlot(1))

	s := &Service{}

	// Build a minimal Phase0 attestation (unaggregated path).
	att := &ethpb.Attestation{
		Data: &ethpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 0,
		},
	}

	// First call to obtain the active validator count used to derive committees per slot.
	_, valCount, res, err := s.validateCommitteeIndexAndCount(ctx, att, bs)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)

	count := helpers.SlotCommitteeCount(valCount)

	// committee_index == count - 1 should be accepted.
	att.Data.CommitteeIndex = primitives.CommitteeIndex(count - 1)
	_, _, res, err = s.validateCommitteeIndexAndCount(ctx, att, bs)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)

	// committee_index == count should be rejected (out of range).
	att.Data.CommitteeIndex = primitives.CommitteeIndex(count)
	_, _, res, err = s.validateCommitteeIndexAndCount(ctx, att, bs)
	require.ErrorContains(t, "committee index", err)
	require.Equal(t, pubsub.ValidationReject, res)
}

func Test_validateGloasCommitteeIndex(t *testing.T) {
	blockRoot := bytesutil.PadTo([]byte("blockroot"), 32)
	blockRoot32 := bytesutil.ToBytes32(blockRoot)

	tests := []struct {
		name            string
		committeeIndex  primitives.CommitteeIndex
		attestationSlot primitives.Slot
		blockSlot       primitives.Slot
		hasFullNode     bool
		hasBadPayload   bool
		wantResult      pubsub.ValidationResult
		wantErr         string
	}{
		{
			name:            "committee index >= 2 should reject",
			committeeIndex:  2,
			attestationSlot: 10,
			blockSlot:       10,
			wantResult:      pubsub.ValidationReject,
			wantErr:         "committee index must be < 2",
		},
		{
			name:            "committee index 0 should accept",
			committeeIndex:  0,
			attestationSlot: 10,
			blockSlot:       10,
			wantResult:      pubsub.ValidationAccept,
			wantErr:         "",
		},
		{
			name:            "committee index 1 same-slot should reject",
			committeeIndex:  1,
			attestationSlot: 10,
			blockSlot:       10,
			wantResult:      pubsub.ValidationReject,
			wantErr:         "same slot attestations must use committee index 0",
		},
		{
			name:            "committee index 1 different-slot with bad payload should reject",
			committeeIndex:  1,
			attestationSlot: 10,
			blockSlot:       9,
			hasBadPayload:   true,
			wantResult:      pubsub.ValidationReject,
			wantErr:         "execution payload for attested block is invalid",
		},
		{
			name:            "committee index 1 different-slot without full node should ignore",
			committeeIndex:  1,
			attestationSlot: 10,
			blockSlot:       9,
			hasFullNode:     false,
			wantResult:      pubsub.ValidationIgnore,
			wantErr:         "execution payload for attested block has not been seen",
		},
		{
			name:            "committee index 1 different-slot with full node should accept",
			committeeIndex:  1,
			attestationSlot: 10,
			blockSlot:       9,
			hasFullNode:     true,
			wantResult:      pubsub.ValidationAccept,
			wantErr:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &mockChain.ChainService{
				BlockSlot:           tt.blockSlot,
				FinalizedCheckPoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
			}
			if tt.hasFullNode {
				mc.ForkchoiceRoots = map[[32]byte]bool{blockRoot32: true}
			}
			s := &Service{
				ctx: t.Context(),
				cfg: &config{
					chain:    mc,
					p2p:      p2ptest.NewTestP2P(t),
					beaconDB: dbtest.SetupDB(t),
				},
				badPayloadCache: lruwrpr.New(10),
			}
			if tt.hasBadPayload {
				s.badPayloadCache.Add(string(blockRoot32[:]), true)
			}

			data := &ethpb.AttestationData{
				Slot:            tt.attestationSlot,
				CommitteeIndex:  tt.committeeIndex,
				BeaconBlockRoot: blockRoot,
			}

			result, err := s.validateGloasCommitteeIndex(data)

			require.Equal(t, tt.wantResult, result)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestService_validateUnaggregatedAttTopic_SubnetMatch(t *testing.T) {
	ctx := t.Context()
	p := p2ptest.NewTestP2P(t)
	s := &Service{cfg: &config{p2p: p}}

	st, _ := util.DeterministicGenesisState(t, 64)
	require.NoError(t, st.SetSlot(1))

	att := &ethpb.Attestation{
		AggregationBits: bitfield.Bitlist{0b101},
		Data: &ethpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 0,
			Target:         &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
			Source:         &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		},
	}

	epoch := slots.ToEpoch(att.Data.Slot)
	valCount, err := helpers.ActiveValidatorCount(ctx, st, epoch)
	require.NoError(t, err)
	subnet := helpers.ComputeSubnetForAttestation(valCount, att)
	digest := params.ForkDigest(epoch)
	base := fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, subnet)
	suffix := p.Encoding().ProtocolSuffix()

	tests := []struct {
		name  string
		topic string
		want  pubsub.ValidationResult
	}{
		{"correct subnet", base + suffix, pubsub.ValidationAccept},
		// base ends in the subnet digits; appending another digit must not still match.
		{"subnet that shares a prefix", base + "0" + suffix, pubsub.ValidationReject},
		{"different subnet", fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, subnet+1) + suffix, pubsub.ValidationReject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := s.validateUnaggregatedAttTopic(ctx, att, st, tt.topic)
			require.Equal(t, tt.want, res)
			if tt.want == pubsub.ValidationAccept {
				require.NoError(t, err)
			}
		})
	}
}

func TestService_validateAvailableAttestation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	// The mock available committee is drawn from the genesis validator set, so
	// the config has to name the same count the test state has.
	cfg.MinGenesisActiveValidatorCount = 64
	params.OverrideBeaconConfig(cfg)

	p := p2ptest.NewTestP2P(t)
	db := dbtest.SetupDB(t)
	// Two slots past genesis so that stale (slot 1) and current (slot 2) votes are
	// both constructible without tripping the slot-0 special case.
	currentSlot := primitives.Slot(2)
	genesisOffset := time.Duration(uint64(currentSlot)*params.BeaconConfig().SecondsPerSlot) * time.Second
	chain := &mockChain.ChainService{
		Genesis:        time.Now().Add(-genesisOffset),
		ValidatorsRoot: [32]byte{'A'},
		DB:             db,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Service{
		ctx: ctx,
		cfg: &config{
			initialSync: &mockSync.Sync{IsSyncing: false},
			p2p:         p,
			beaconDB:    db,
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		blkRootToPendingAtts: make(map[[32]byte][]any),
		signatureChan:        make(chan *signatureVerifier, verifierLimit),
	}
	s.initCaches()
	go s.verifierRoutine()

	blk := util.NewBeaconBlock()
	blk.Block.Slot = currentSlot
	util.SaveBlock(t, ctx, db, blk)
	validBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	validatorCount := decoupled.CommitteeValidatorCount()
	savedState, keys := util.DeterministicGenesisState(t, validatorCount)
	require.NoError(t, savedState.SetSlot(currentSlot))
	require.NoError(t, db.SaveState(ctx, savedState, validBlockRoot))
	chain.State = savedState
	chain.TargetRoot = validBlockRoot

	digest := s.currentForkDigest()
	topic := fmt.Sprintf(p2p.AvailableAttestationTopicFormat, digest) + p.Encoding().ProtocolSuffix()

	seatBits := func(slot primitives.Slot, idx primitives.ValidatorIndex) bitfield.Bitvector512 {
		bits := bitfield.NewBitvector512()
		for _, seat := range decoupled.AvailableAttestationSeats(slot, idx, validatorCount) {
			bits.SetBitAt(seat, true)
		}
		return bits
	}
	// A vote with the seat bits of validator idx, signed by signer over the mock domain.
	vote := func(slot primitives.Slot, idx primitives.ValidatorIndex, signer bls.SecretKey,
		blockRoot []byte) *ethpb.AvailableAttestation {
		att := &ethpb.AvailableAttestation{
			AggregationBits: seatBits(slot, idx),
			Data: &ethpb.AvailableAttestationData{
				Slot:            slot,
				BeaconBlockRoot: blockRoot,
			},
		}
		root, err := signing.ComputeSigningRoot(att.Data, decoupled.AvailableAttDomain)
		require.NoError(t, err)
		att.Signature = signer.Sign(root[:]).Marshal()
		return att
	}

	twoSigners := vote(currentSlot, 5, keys[5], validBlockRoot[:])
	for _, seat := range decoupled.AvailableAttestationSeats(currentSlot, 6, validatorCount) {
		twoSigners.AggregationBits.SetBitAt(seat, true)
	}
	emptySeats := vote(currentSlot, 5, keys[5], validBlockRoot[:])
	emptySeats.AggregationBits = bitfield.NewBitvector512()
	unknownRoot := [32]byte{'m', 'i', 's', 's', 'i', 'n', 'g'}

	tests := []struct {
		name  string
		att   *ethpb.AvailableAttestation
		topic string
		want  pubsub.ValidationResult
	}{
		{
			name:  "valid vote",
			att:   vote(currentSlot, 5, keys[5], validBlockRoot[:]),
			topic: topic,
			want:  pubsub.ValidationAccept,
		},
		{
			name:  "wrong signer",
			att:   vote(currentSlot, 5, keys[6], validBlockRoot[:]),
			topic: topic,
			want:  pubsub.ValidationReject,
		},
		{
			name:  "seats of two validators",
			att:   twoSigners,
			topic: topic,
			want:  pubsub.ValidationReject,
		},
		{
			name:  "empty seat bits",
			att:   emptySeats,
			topic: topic,
			want:  pubsub.ValidationReject,
		},
		{
			name:  "stale slot",
			att:   vote(currentSlot-1, 5, keys[5], validBlockRoot[:]),
			topic: topic,
			want:  pubsub.ValidationIgnore,
		},
		{
			name:  "future slot",
			att:   vote(currentSlot+3, 5, keys[5], validBlockRoot[:]),
			topic: topic,
			want:  pubsub.ValidationIgnore,
		},
		{
			name:  "slot zero",
			att:   vote(0, 5, keys[5], validBlockRoot[:]),
			topic: topic,
			want:  pubsub.ValidationIgnore,
		},
		{
			name:  "unknown block root",
			att:   vote(currentSlot, 5, keys[5], unknownRoot[:]),
			topic: topic,
			want:  pubsub.ValidationIgnore,
		},
		{
			name:  "nil topic",
			att:   vote(currentSlot, 5, keys[5], validBlockRoot[:]),
			topic: "",
			want:  pubsub.ValidationReject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			_, err := p.Encoding().EncodeGossip(buf, tt.att)
			require.NoError(t, err)
			m := &pubsub.Message{
				Message: &pubsubpb.Message{
					Data:  buf.Bytes(),
					Topic: &tt.topic,
				},
			}
			if tt.topic == "" {
				m.Message.Topic = nil
			}

			res, err := s.validateAvailableAttestation(ctx, "", m)
			require.Equal(t, tt.want, res)
			if tt.want == pubsub.ValidationAccept {
				require.NoError(t, err)
				require.NotNil(t, m.ValidatorData, "expected validator data to be set")
			}
		})
	}

	// The unknown block root case is ignored on the wire but must not be lost:
	// it belongs in the pending queue, waiting for its block to arrive.
	t.Run("unknown block root is queued", func(t *testing.T) {
		require.Equal(t, 1, len(s.blkRootToPendingAtts[unknownRoot]))
	})
}
