package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestDomainDataCacheInternalCostDiagnostic(t *testing.T) {
	const domainCount = 13

	retained := func(t *testing.T, ignoreInternalCost bool) int {
		cache, err := ristretto.NewCache(&ristretto.Config[string, proto.Message]{
			NumCounters:        1920,
			MaxCost:            192,
			BufferItems:        64,
			IgnoreInternalCost: ignoreInternalCost,
		})
		require.NoError(t, err)
		t.Cleanup(cache.Close)

		for i := 0; i < domainCount; i++ {
			cache.Set(fmt.Sprintf("0,%08x", i), &emptypb.Empty{}, 1)
		}
		cache.Wait()

		count := 0
		for i := 0; i < domainCount; i++ {
			if _, ok := cache.Get(fmt.Sprintf("0,%08x", i)); ok {
				count++
			}
		}
		return count
	}

	productionRetained := retained(t, false)
	controlRetained := retained(t, true)
	require.Less(t, productionRetained, domainCount)
	require.Equal(t, domainCount, controlRetained)
	t.Logf("retained domains: production=%d ignore-internal-cost=%d", productionRetained, controlRetained)
}

// Same-domain callers across epochs still wait for the RPC holder despite cancellation.
func TestDomainDataCanceledWaiterDiagnostic(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := validatormock.NewMockValidatorClient(ctrl)
	cache, err := ristretto.NewCache(&ristretto.Config[string, proto.Message]{
		NumCounters: 1920,
		MaxCost:     192,
		BufferItems: 64,
	})
	require.NoError(t, err)
	t.Cleanup(cache.Close)

	holderEntered := make(chan struct{})
	releaseHolder := make(chan struct{})
	randaoRPCEntered := make(chan struct{})
	var releaseOnce sync.Once
	var workers sync.WaitGroup
	var cancelRandao context.CancelFunc
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseHolder) })
		if cancelRandao != nil {
			cancelRandao()
		}
		workersDone := make(chan struct{})
		go func() {
			workers.Wait()
			close(workersDone)
		}()
		select {
		case <-workersDone:
		case <-time.After(time.Second):
			t.Error("domain-data diagnostic workers did not stop during cleanup")
		}
	})
	client.EXPECT().DomainData(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
			switch req.Epoch {
			case 0:
				close(holderEntered)
				<-releaseHolder
				return &ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil
			case 1:
				close(randaoRPCEntered)
				return nil, ctx.Err()
			default:
				return nil, fmt.Errorf("unexpected epoch %d", req.Epoch)
			}
		}).Times(2)

	v := &validator{validatorClient: client, domainDataCache: cache}
	holderResult := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := v.domainData(
			context.Background(),
			primitives.Epoch(0),
			params.BeaconConfig().DomainRandao[:],
		)
		holderResult <- err
	}()

	select {
	case <-holderEntered:
	case <-time.After(time.Second):
		t.Fatal("background domain request did not acquire the lock")
	}

	var randaoCtx context.Context
	randaoCtx, cancelRandao = context.WithCancel(context.Background())
	randaoResult := make(chan error, 1)
	started := time.Now()
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := v.domainData(randaoCtx, primitives.Epoch(1), params.BeaconConfig().DomainRandao[:])
		randaoResult <- err
	}()
	cancelRandao()

	const minimumBlocked = 150 * time.Millisecond
	select {
	case err := <-randaoResult:
		t.Fatalf("canceled waiter returned before holder release: %v", err)
	case <-time.After(minimumBlocked):
	}
	select {
	case <-randaoRPCEntered:
		t.Fatal("next-epoch RANDAO RPC began while the current-epoch holder still held the domain lock")
	default:
	}

	releaseOnce.Do(func() { close(releaseHolder) })
	select {
	case err := <-holderResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("background domain request did not finish after release")
	}
	select {
	case err := <-randaoResult:
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled), "unexpected RANDAO error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("RANDAO request did not finish after holder release")
	}
	require.GreaterOrEqual(t, time.Since(started), minimumBlocked)
	select {
	case <-randaoRPCEntered:
	default:
		t.Fatal("RANDAO RPC was not reached after holder release")
	}
}
