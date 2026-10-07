package p2p

import (
	"net"
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	ma "github.com/multiformats/go-multiaddr"
)

func TestShadowQUIC_Connects(t *testing.T) {
	newHost := func() host.Host {
		h, err := libp2p.New(
			libp2p.ListenAddrStrings("/ip4/127.0.0.1/udp/0/quic-v1"),
			libp2p.Transport(libp2pquic.NewTransport),
			shadowQUIC(net.IPv4(127, 0, 0, 1)),
		)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, h.Close()) })
		return h
	}
	h1, h2 := newHost(), newHost()
	require.NoError(t, h1.Connect(t.Context(), peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}))
	conns := h1.Network().ConnsToPeer(h2.ID())
	require.Equal(t, 1, len(conns))
	// The source IP selector makes the dial reuse the listen socket.
	require.DeepEqual(t, h1.Addrs()[0], conns[0].LocalMultiaddr())
}

func TestBuildOptions_ShadowQUIC(t *testing.T) {
	for _, on := range []bool{false, true} {
		reset := features.InitWithReset(&features.Flags{EnableQUIC: true, ShadowQUIC: on})
		svc := &Service{cfg: &Config{TCPPort: 3000, QUICPort: 3000,
			StateNotifier: &mock.MockStateNotifier{}}}
		var err error
		svc.privKey, err = privKey(svc.cfg)
		require.NoError(t, err)
		opts, err := svc.buildOptions(net.IPv4(127, 0, 0, 1), svc.privKey)
		require.NoError(t, err)
		var cfg libp2p.Config
		require.NoError(t, cfg.Apply(opts...))
		require.Equal(t, on, cfg.QUICReuse != nil)
		hasTCP := false
		for _, a := range cfg.ListenAddrs {
			_, err := a.ValueForProtocol(ma.P_TCP)
			hasTCP = hasTCP || err == nil
		}
		require.Equal(t, !on, hasTCP)
		transports := 2 // TCP and QUIC
		if on {
			transports = 1
		}
		require.Equal(t, transports, len(cfg.Transports))
		reset()
	}
}
