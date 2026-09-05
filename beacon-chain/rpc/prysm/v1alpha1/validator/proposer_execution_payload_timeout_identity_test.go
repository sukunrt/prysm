//go:build minimal

package validator

import (
	"context"
	"errors"
	"testing"

	chainmock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	executiontesting "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

type timeoutIdentityEngine struct {
	*executiontesting.EngineClient
	cachedErr       error
	getPayloadCalls int
	forkchoiceCalls int
}

func (e *timeoutIdentityEngine) GetPayload(_ context.Context, _ [8]byte, _ primitives.Slot) (*consensusblocks.GetPayloadResponse, error) {
	e.getPayloadCalls++
	if e.getPayloadCalls == 1 {
		return nil, e.cachedErr
	}
	return e.GetPayloadResponse, nil
}

func (e *timeoutIdentityEngine) ForkchoiceUpdated(
	ctx context.Context,
	state *enginev1.ForkchoiceState,
	attrs payloadattribute.Attributer,
) (*enginev1.PayloadIDBytes, []byte, error) {
	e.forkchoiceCalls++
	return e.EngineClient.ForkchoiceUpdated(ctx, state, attrs)
}

func TestGetLocalPayloadFromEngine_CachedTimeoutErrorIdentityControlsFallback(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	const slot = primitives.Slot(1)
	parentRoot := [32]byte{0x42}
	st, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))

	executionData, err := consensusblocks.WrappedExecutionPayloadGloas(&enginev1.ExecutionPayloadGloas{
		FeeRecipient: make([]byte, 20),
	})
	require.NoError(t, err)
	response := &consensusblocks.GetPayloadResponse{ExecutionData: executionData}

	tests := []struct {
		name                string
		cachedErr           error
		wantErr             string
		wantGetPayloadCalls int
		wantForkchoiceCalls int
	}{
		{
			name:                "raw context deadline reaches fresh payload preparation",
			cachedErr:           context.DeadlineExceeded,
			wantGetPayloadCalls: 2,
			wantForkchoiceCalls: 1,
		},
		{
			name:                "execution timeout sentinel aborts cached payload retrieval",
			cachedErr:           execution.ErrHTTPTimeout,
			wantErr:             "could not get cached payload from execution client: timeout from http.Client",
			wantGetPayloadCalls: 1,
			wantForkchoiceCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &timeoutIdentityEngine{
				EngineClient: &executiontesting.EngineClient{
					PayloadIDBytes:     &enginev1.PayloadIDBytes{0x02},
					GetPayloadResponse: response,
				},
				cachedErr: tt.cachedErr,
			}
			chain := &chainmock.ChainService{BlockSlot: 0}
			server := &Server{
				ExecutionEngineCaller:    engine,
				ForkchoiceFetcher:        chain,
				FinalizationFetcher:      chain,
				PayloadIDCache:           cache.NewPayloadIDCache(),
				ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
			}
			server.PayloadIDCache.Set(slot, parentRoot, true, [8]byte{0x01})

			got, err := server.getLocalPayloadFromEngine(t.Context(), st, parentRoot, slot, 0, true)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, got)
			} else {
				require.ErrorContains(t, tt.wantErr, err)
				require.Equal(t, true, errors.Is(err, execution.ErrHTTPTimeout))
				require.Equal(t, false, errors.Is(err, context.DeadlineExceeded))
				require.Equal(t, (*consensusblocks.GetPayloadResponse)(nil), got)
			}
			require.Equal(t, tt.wantGetPayloadCalls, engine.getPayloadCalls)
			require.Equal(t, tt.wantForkchoiceCalls, engine.forkchoiceCalls)
		})
	}
}
