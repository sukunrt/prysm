package client

import (
	"fmt"
	"testing"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/stretchr/testify/require"
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
