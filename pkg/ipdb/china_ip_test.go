package ipdb

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// TestIsChinaIPWithoutLoadedData verifies that IsChinaIP does not panic when
// the embedded China IP data failed to load (chinaRanger never stored).
// Before the fix, the unchecked type assertion on the nil interface panicked
// on every call, crashing the process on any DNS query and leaving the pf
// redirect rules behind (full network outage).
func TestIsChinaIPWithoutLoadedData(t *testing.T) {
	// Simulate the "load failed" state: the once has already fired with an
	// error and nothing was stored in chinaRanger. Restore package state
	// afterwards so other tests see the default behavior.
	oldRanger := chinaRanger.Load()
	t.Cleanup(func() {
		chinaRanger = atomic.Value{}
		if oldRanger != nil {
			chinaRanger.Store(oldRanger)
		}
		chinaCIDRsOnce = sync.Once{}
		chinaCIDRsErr = nil
	})

	chinaRanger = atomic.Value{}
	chinaCIDRsOnce = sync.Once{}
	chinaCIDRsErr = nil
	chinaCIDRsOnce.Do(func() {
		chinaCIDRsErr = fmt.Errorf("simulated embed load failure")
	})

	// Must not panic; should degrade to "not a China IP".
	if IsChinaIP("1.2.3.4") {
		t.Error("IsChinaIP should return false when China IP data is not loaded")
	}
}
