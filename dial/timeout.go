package dial

import (
	"context"
	"time"

	inet "github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-transport"
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

type reqTimeout struct{}

var _ RequestPreparer = (*reqTimeout)(nil)

func NewRequestTimeout() RequestPreparer {
	return &reqTimeout{}
}

func NewJobTimeout() JobPreparer {
	return &jobTimeout{}
}

func (t *reqTimeout) Prepare(req *Request) {
	// apply the DialPeer timeout
	ctx, cancel := context.WithTimeout(req.ctx, inet.GetDialPeerTimeout(req.ctx))
	req.ctx = ctx
	req.AddCallback(cancel)
}

type jobTimeout struct{}

var _ JobPreparer = (*jobTimeout)(nil)

func (*jobTimeout) Prepare(job *Job) {
	timeout := tpt.DialTimeout
	if lowTimeoutFilters.AddrBlocked(job.addr) {
		timeout = TimeoutLocal
	}
	ctx, cancelFn := context.WithTimeout(job.ctx, timeout)
	job.ctx = ctx
	job.AddCallback(cancelFn)
}
