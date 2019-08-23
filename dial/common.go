package dial

import (
	"context"
	"fmt"
	"sync"
)

// Status represents the status of a Request or a Job. It is a bit array where each possible
// status is represented by a bit, to enable efficient mask evaluation.
type Status uint32

const (
	// StatusInflight indicates that a request or job is currently making progress.
	StatusInflight Status = 1 << iota
	// StatusBlock indicates that a request or job is currently blocked, possibly as a
	// result of throttling or guarding.
	StatusBlocked
	// StatusCompleting indicates that a request or job has completed and its
	// callbacks are currently firing.
	StatusCompleting
	// StatusComplete indicates that a request or job has fully completed.
	StatusComplete
)

// internCallbackNames interns callback names, via the internedCallbackName function.
var internCallbackNames = make(map[string]string)

// Assert panics if the current status does not adhere to the specified bit mask.
func (s *Status) Assert(mask Status) {
	if *s&mask == 0 {
		// it may be worth decoding the mask to a friendlier format.
		panic(fmt.Sprintf("illegal state %s; mask: %b", s, mask))
	}
}

func (s Status) String() string {
	switch s {
	case StatusComplete:
		return "Status(Complete)"
	case StatusInflight:
		return "Status(Inflight)"
	case StatusCompleting:
		return "Status(Completing)"
	case StatusBlocked:
		return "Status(Blocked)"
	default:
		return fmt.Sprintf("Status(%d)", s)
	}
}

// internedCallbackName retrieves the interned string corresponding to this callback name.
func internedCallbackName(name string) string {
	if n, ok := internCallbackNames[name]; ok {
		return n
	}
	internCallbackNames[name] = name
	return name
}

// contextHolder is a mixin that adds context tracking and mutation capabilities to another struct.
type contextHolder struct {
	clk     sync.RWMutex
	ctx     context.Context
	cancels []context.CancelFunc
}

// UpdateContext updates the context and cancel functions atomically.
func (ch *contextHolder) UpdateContext(mutator func(orig context.Context) (context.Context, context.CancelFunc)) {
	ch.clk.Lock()
	defer ch.clk.Unlock()

	ctx, cancel := mutator(ch.ctx)
	ch.ctx = ctx
	ch.cancels = append(ch.cancels, cancel)
}

// Context returns the context in a thread-safe manner.
func (ch *contextHolder) Context() context.Context {
	ch.clk.RLock()
	defer ch.clk.RUnlock()

	return ch.ctx
}

// FireCancels invokes all cancel functions in the inverse order they were added.
func (ch *contextHolder) FireCancels() {
	ch.clk.RLock()
	defer ch.clk.RUnlock()

	for i := len(ch.cancels) - 1; i >= 0; i-- {
		ch.cancels[i]()
	}
}
