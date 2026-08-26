package beacon

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	blockchainmock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	p2pMock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpbv1alpha1 "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestSubmitAttestationsV2_FFGVoteLedger(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.BeaconConfig().Copy()
	c.SlotsPerEpoch = 1
	c.ElectraForkEpoch = 0
	params.OverrideBeaconConfig(c)

	_, keys, err := util.DeterministicDepositsAndKeys(2)
	require.NoError(t, err)
	validators := []*ethpbv1alpha1.Validator{
		{PublicKey: keys[0].PublicKey().Marshal(), ExitEpoch: params.BeaconConfig().FarFutureEpoch},
		{PublicKey: keys[1].PublicKey().Marshal(), ExitEpoch: params.BeaconConfig().FarFutureEpoch},
	}
	bs, err := util.NewBeaconState(func(state *ethpbv1alpha1.BeaconState) error {
		state.Validators = validators
		state.Slot = 1
		return nil
	})
	require.NoError(t, err)
	slot := primitives.Slot(0)
	chainService := &blockchainmock.ChainService{State: bs, Slot: &slot, Genesis: time.Now()}
	s := &Server{
		HeadFetcher:             chainService,
		ChainInfoFetcher:        chainService,
		TimeFetcher:             chainService,
		OptimisticModeFetcher:   chainService,
		SyncChecker:             &mockSync.Sync{IsSyncing: false},
		OperationNotifier:       &blockchainmock.MockOperationNotifier{},
		AttestationStateFetcher: chainService,
		AttestationsPool:        attestations.NewPool(),
		Broadcaster:             &p2pMock.MockBroadcaster{},
	}

	submit := func(t *testing.T) {
		t.Helper()
		request := httptest.NewRequest(
			http.MethodPost, "http://example.com", strings.NewReader(singleAttElectra))
		request.Header.Set(api.VersionHeader, version.String(version.Electra))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.SubmitAttestationsV2(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
	}
	ffgVotes := func(hook *logTest.Hook) []logrus.Fields {
		var out []logrus.Fields
		for _, e := range hook.AllEntries() {
			if e.Message == "FFG vote" {
				out = append(out, e.Data)
			}
		}
		return out
	}

	t.Run("quiet with the ledger off", func(t *testing.T) {
		hook := logTest.NewGlobal()
		submit(t)
		require.Equal(t, 0, len(ffgVotes(hook)))
	})

	t.Run("local outcome with the ledger on", func(t *testing.T) {
		reset := features.InitWithReset(&features.Flags{GoldfishVoteLedger: true})
		defer reset()
		hook := logTest.NewGlobal()
		submit(t)
		votes := ffgVotes(hook)
		require.Equal(t, 1, len(votes))
		fields := votes[0]
		require.Equal(t, "local", fields["outcome"])
		require.Equal(t, primitives.Slot(0), fields["attSlot"])
		require.Equal(t, primitives.Round(0), fields["targetRound"])
		require.Equal(t, primitives.CommitteeIndex(0), fields["committeeIndex"])
		require.Equal(t, uint64(1), fields["seats"])
		require.Equal(t, primitives.ValidatorIndex(1), fields["validator"])
		require.NotEqual(t, "", fields["dataRoot"])
	})
}
