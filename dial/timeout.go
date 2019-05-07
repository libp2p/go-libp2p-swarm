package dial

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/transport"
	mafilter "github.com/libp2p/go-maddr-filter"
	mamask "github.com/whyrusleeping/multiaddr-filter"
)

var TimeoutLocal = 5 * time.Second

// http://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml
var lowTimeoutFilters = mafilter.NewFilters()

func init() {
	for _, p := range []string{
		"/ip4/10.0.0.0/ipcidr/8",
		"/ip4/100.64.0.0/ipcidr/10",
		"/ip4/169.254.0.0/ipcidr/16",
		"/ip4/172.16.0.0/ipcidr/12",
		"/ip4/192.0.0.0/ipcidr/24",
		"/ip4/192.0.0.0/ipcidr/29",
		"/ip4/192.0.0.8/ipcidr/32",
		"/ip4/192.0.0.170/ipcidr/32",
		"/ip4/192.0.0.171/ipcidr/32",
		"/ip4/192.0.2.0/ipcidr/24",
		"/ip4/192.168.0.0/ipcidr/16",
		"/ip4/198.18.0.0/ipcidr/15",
		"/ip4/198.51.100.0/ipcidr/24",
		"/ip4/203.0.113.0/ipcidr/24",
		"/ip4/240.0.0.0/ipcidr/4",
	} {
		f, err := mamask.NewMask(p)
		if err != nil {
			panic("error in lowTimeoutFilters init: " + err.Error())
		}
		lowTimeoutFilters.AddDialFilter(f)
	}
}

func SetJobTimeout(job *Job) {
	timeout := transport.DialTimeout
	if lowTimeoutFilters.AddrBlocked(job.Address()) {
		timeout = TimeoutLocal
	}
	job.UpdateContext(func(orig context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(orig, timeout)
	})
}

type reqTimeout struct{}

var _ Preparer = (*reqTimeout)(nil)

func NewRequestTimeout() Preparer {
	return &reqTimeout{}
}

func (t *reqTimeout) Prepare(req *Request) error {
	req.UpdateContext(func(orig context.Context) (context.Context, context.CancelFunc) {
		// apply the DialPeer timeout
		return context.WithTimeout(orig, network.GetDialPeerTimeout(orig))
	})
	return nil
}
