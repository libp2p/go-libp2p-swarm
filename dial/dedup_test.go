package dial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	testutil "github.com/libp2p/go-libp2p-core/test"
	"github.com/libp2p/go-libp2p-swarm/dial"
	testing2 "github.com/libp2p/go-libp2p-swarm/testing"
)

func TestDedup(t *testing.T) {
	dedup := dial.NewDedup()
	sw := testing2.GenSwarm(t, context.Background())
	defer sw.Close()

	id1, _ := testutil.RandPeerID()
	id2, _ := testutil.RandPeerID()

	req1 := dial.NewDialRequest(context.Background(), id1)
	req2 := dial.NewDialRequest(context.Background(), id2)

	// req3 is a duplicate of req1.
	req3 := dial.NewDialRequest(context.Background(), id1)

	if dedup.Prepare(req1); req2.IsComplete() {
		t.Error("dedup should've allowed req1 to proceed")
	}

	if dedup.Prepare(req2); req2.IsComplete() {
		t.Error("dedup should've allowed req2 to proceed")
	}

	var conn1 network.Conn
	err1 := errors.New("dummy err")

	done := make(chan struct{})
	go func() {
		// --- Prepare() blocks until req1 completes. ---
		dedup.Prepare(req3)
		if !req1.IsComplete() || !req3.IsComplete() {
			t.Errorf("req3 should've completed with req1's completion, req1 complete: %v, req3 complete: %v", req1.IsComplete(), req3.IsComplete())
		}

		conn3, err3 := req3.Result()
		if conn1 != conn3 || err1 != err3 {
			t.Error("values of req3 should equal values of req1")
		}
		done <- struct{}{}
	}()

	// sleep to give the above goroutine a chance to block.
	time.Sleep(200 * time.Millisecond)
	req1.Complete(conn1, err1)
	<-done
}
