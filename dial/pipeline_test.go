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

func (tp *testPreparer) Prepare(req *dial.Request) {
	vals := append(*tp.target, tp.Value)
	*tp.target = vals
}

var _ dial.Preparer = (*testPreparer)(nil)

func TestPreparerSequence(t *testing.T) {
	expected := []string{"four", "three", "seven", "one", "five", "eight"}

	var target []string
	var seq dial.PreparerSeq

	seq.AddLast("one", &testPreparer{"one", &target})
	seq.AddLast("two", &testPreparer{"two", &target})
	seq.AddFirst("three", &testPreparer{"three", &target})
	seq.InsertBefore("three", "four", &testPreparer{"four", &target})
	seq.InsertBefore("two", "five", &testPreparer{"five", &target})
	seq.InsertAfter("two", "six", &testPreparer{"six", &target})
	seq.InsertAfter("three", "seven", &testPreparer{"seven", &target})
	seq.Remove("non-existent")
	seq.Remove("two")
	seq.Replace("six", "eight", &testPreparer{"eight", &target})

	if err := seq.Replace("non-existent", "nine", &testPreparer{"nine", &target}); err == nil {
		t.Fatal("expected an error when replacing non-existent preparer")
	}

	seq.Prepare(&dial.Request{})

	if !reflect.DeepEqual(expected, target) {
		t.Fatalf("unexpected result when manipulating preparer sequence; expected: %v, got: %v", expected, target)
	}
}
