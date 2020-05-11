package swarm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialErrorCombine(t *testing.T) {
	tcs := map[string]struct {
		nD1Errs    int
		nD2Errs    int
		nCbdErrs   int
		cbdSkipped int
	}{
		"tc1": {
			10, 10, maxDialDialErrors, 4,
		},
		"tc2": {
			5, 6, 11, 0,
		},
		"tc3": {
			0, 0, 0, 0,
		},
	}

	for _, tc := range tcs {
		d1 := &DialError{}
		d2 := &DialError{}
		for i := 0; i < tc.nD1Errs; i++ {
			d1.DialErrors = append(d1.DialErrors, TransportError{})
		}
		for i := 0; i < tc.nD2Errs; i++ {
			d2.DialErrors = append(d2.DialErrors, TransportError{})
		}

		d3 := d1.combine(d2)

		require.Len(t, d3.DialErrors, tc.nCbdErrs)
		require.Equal(t, tc.cbdSkipped, d3.Skipped)
	}
}
