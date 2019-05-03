package dial_test

import (
	"reflect"
	"testing"

	"github.com/libp2p/go-libp2p-swarm/dial"
)

type testPreparer struct {
	Value  string
	target *[]string
}

func (tp *testPreparer) Prepare(req *dial.Request) error {
	vals := append(*tp.target, tp.Value)
	*tp.target = vals
	return nil
}

var _ dial.Preparer = (*testPreparer)(nil)

func TestPreparerSequence(t *testing.T) {
	expected := []string{"four", "three", "seven", "one", "five", "eight"}

	var target []string
	var seq dial.PreparerSeq

	_ = seq.AddLast("one", &testPreparer{"one", &target})
	_ = seq.AddLast("two", &testPreparer{"two", &target})
	_ = seq.AddFirst("three", &testPreparer{"three", &target})
	_ = seq.InsertBefore("three", "four", &testPreparer{"four", &target})
	_ = seq.InsertBefore("two", "five", &testPreparer{"five", &target})
	_ = seq.InsertAfter("two", "six", &testPreparer{"six", &target})
	_ = seq.InsertAfter("three", "seven", &testPreparer{"seven", &target})
	seq.Remove("non-existent")
	seq.Remove("two")
	_ = seq.Replace("six", "eight", &testPreparer{"eight", &target})

	if err := seq.Replace("non-existent", "nine", &testPreparer{"nine", &target}); err == nil {
		t.Fatal("expected an error when replacing non-existent preparer")
	}

	_ = seq.Prepare(&dial.Request{})

	if !reflect.DeepEqual(expected, target) {
		t.Fatalf("unexpected result when manipulating preparer sequence; expected: %v, got: %v", expected, target)
	}
}
