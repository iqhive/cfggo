package cfggo

import (
	"sync"
	"testing"
)

// TestConcurrentGetFlagSetLazilyInitialisesOnce hammers GetFlagSet from N
// goroutines on a WithoutFlags config whose private set does not exist yet.
// Run with -race: before the fix, concurrent ensureFlagSet writes to c.flagSet
// race with each other.
func TestConcurrentGetFlagSetLazilyInitialisesOnce(t *testing.T) {
	setProcessArgs(t)
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if cfg.flagSet != nil {
		t.Fatalf("flagSet = %v, want nil after WithoutFlags Init", cfg.flagSet)
	}

	const goroutines = 16
	sets := make([]interface{}, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sets[n] = cfg.GetFlagSet()
		}(i)
	}
	wg.Wait()

	for i := 1; i < goroutines; i++ {
		if sets[i] != sets[0] {
			t.Fatalf("GetFlagSet() returned distinct sets: [0]=%p [%d]=%p", sets[0], i, sets[i])
		}
	}
	if cfg.GetFlagSet() == nil {
		t.Fatal("GetFlagSet() = nil, want non-nil private set")
	}
}
