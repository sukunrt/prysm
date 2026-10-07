package partialattestationbroadcaster

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	bundlesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_partial_attestation_bundles_total",
		Help: "Attestation bundles by path: push and serve are sent, recv is received.",
	}, []string{"path"})
	bundleSignatures = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "p2p_partial_attestation_bundle_signatures",
		Help:    "Signatures in one attestation bundle, by path.",
		Buckets: []float64{1, 2, 4, 8, 16, 32, 50},
	}, []string{"path"})
)
