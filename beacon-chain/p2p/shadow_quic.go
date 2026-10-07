package p2p

import (
	"net"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	quicgo "github.com/quic-go/quic-go"
	"go.uber.org/fx"
)

// shadowQUIC lets QUIC connect in the Shadow network simulator. quic-go uses socket
// features on a *net.UDPConn that Shadow does not have, and Shadow needs one writer at a time
// on a UDP socket. So each QUIC socket is a plain net.PacketConn with serialized writes.
// Shadow has no routing table, so a dial uses the listen IP as its source IP.
func shadowQUIC(listenIP net.IP) libp2p.Option {
	return libp2p.QUICReuse(
		func(l fx.Lifecycle, key quicgo.StatelessResetKey, tokenKey quicgo.TokenGeneratorKey,
			opts ...quicreuse.Option) (*quicreuse.ConnManager, error) {
			cm, err := quicreuse.NewConnManager(key, tokenKey, opts...)
			if err != nil {
				return nil, err
			}
			l.Append(fx.StopHook(cm.Close))
			return cm, nil
		},
		quicreuse.OverrideListenUDP(func(network string, laddr *net.UDPAddr) (net.PacketConn, error) {
			conn, err := net.ListenUDP(network, laddr)
			if err != nil {
				return nil, err
			}
			return &serialUDPConn{PacketConn: conn}, nil
		}),
		quicreuse.OverrideSourceIPSelector(func() (quicreuse.SourceIPSelector, error) {
			return fixedSourceIP(listenIP), nil
		}),
	)
}

type serialUDPConn struct {
	net.PacketConn
	mu sync.Mutex
}

func (c *serialUDPConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.PacketConn.WriteTo(p, addr)
}

type fixedSourceIP net.IP

func (ip fixedSourceIP) PreferredSourceIPForDestination(*net.UDPAddr) (net.IP, error) {
	return net.IP(ip), nil
}
