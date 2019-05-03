package dial

import (
	"fmt"
	"sync"
)

type preparerBinding struct {
	name string
	p    Preparer
}

// PreparerSeq is a Preparer that daisy-chains the Request through an ordered list of preparers. It short-circuits
// the process if a Preparer completes the Request. Preparers are bound by unique names.
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

func (ps *PreparerSeq) Remove(name string) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if i, prev := ps.find(name); prev != nil {
		ps.seq = append(ps.seq[:i], ps.seq[i+1:]...)
	}
}

func (ps *PreparerSeq) Get(name string) Preparer {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	if _, pb := ps.find(name); pb != nil {
		return pb.p
	}
	return nil
}
