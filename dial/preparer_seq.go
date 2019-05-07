package dial

import (
	"fmt"
	"sync"
)

type preparerBinding struct {
	name string
	p    Preparer
}

// PreparerSeq is a Preparer that pipes the Request through an daisy-chained list of preparers.
//
// It short-circuits a Preparer fails or completes the Request. Preparers are bound by unique names, to aid debugging.
type PreparerSeq struct {
	lk  sync.Mutex
	seq []preparerBinding
}

var _ Preparer = (*PreparerSeq)(nil)

func (ps *PreparerSeq) Prepare(req *Request) error {
	for _, p := range ps.seq {
		if err := p.p.Prepare(req); err != nil {
			return err
		} else if req.IsComplete() {
			return nil
		}
	}
	return nil
}

func (ps *PreparerSeq) find(name string) (i int, res *preparerBinding) {
	for i, pb := range ps.seq {
		if pb.name == name {
			return i, &pb
		}
	}
	return -1, nil
}

// AddFirst prepends a Preparer at the starting position of this sequence.
func (ps *PreparerSeq) AddFirst(name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, prev := ps.find(name); prev != nil {
		return fmt.Errorf("a preparer with name %s already exists", name)
	}
	pb := preparerBinding{name, preparer}
	ps.seq = append([]preparerBinding{pb}, ps.seq...)
	return nil
}

// AddFirst appends a Preparer at the starting position of this sequence.
func (ps *PreparerSeq) AddLast(name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, prev := ps.find(name); prev != nil {
		return fmt.Errorf("a preparer with name %s already exists", name)
	}
	pb := preparerBinding{name, preparer}
	ps.seq = append(ps.seq, pb)
	return nil
}

// InsertBefore locates the specified Preparer by name, and inserts the new Preparer ahead of it.
func (ps *PreparerSeq) InsertBefore(before, name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	i, prev := ps.find(before)
	if prev == nil {
		return fmt.Errorf("no preparers found with name %s", name)
	}

	pb := preparerBinding{name, preparer}
	ps.seq = append(ps.seq, pb)
	copy(ps.seq[i+1:], ps.seq[i:])
	ps.seq[i] = pb

	return nil
}

// InsertAfter locates the specified Preparer by name, and inserts the new Preparer after it.
func (ps *PreparerSeq) InsertAfter(after, name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	i, prev := ps.find(after)
	if prev == nil {
		return fmt.Errorf("no preparers found with name %s", name)
	}

	pb := preparerBinding{name, preparer}
	ps.seq = append(ps.seq, pb)
	copy(ps.seq[i+2:], ps.seq[i+1:])
	ps.seq[i+1] = pb

	return nil
}

// Replace locates the specified Preparer by name, and replaces it with the specified one.
func (ps *PreparerSeq) Replace(old, name string, preparer Preparer) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	i, prev := ps.find(old)
	if prev == nil {
		return fmt.Errorf("no preparers found with name %s", name)
	}
	ps.seq[i] = preparerBinding{name, preparer}
	return nil
}

// Remove locates the specified Preparer by name, and removes it from the sequence.
// It returns whether the sequence was modified as a result.
func (ps *PreparerSeq) Remove(name string) (ok bool) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if i, prev := ps.find(name); prev != nil {
		ps.seq = append(ps.seq[:i], ps.seq[i+1:]...)
		ok = true
	}
	return ok
}

// Get returns the specified Preparer, locating it by name.
func (ps *PreparerSeq) Get(name string) (Preparer, bool) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, pb := ps.find(name); pb != nil {
		return pb.p, true
	}
	return nil, false
}
