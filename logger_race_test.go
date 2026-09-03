package cfggo

import (
	"sync"
	"testing"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
)

func TestSetLoggerAndErrorWrapperAreSafeDuringUse(t *testing.T) {
	setProcessArgs(t)
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cfg.SetLogger(&cfglogger.NoopLogger{})
				cfg.SetErrorWrapper(cfgerror.NewDefaultWrapper())
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = cfg.ReloadConfig()    // logs through the instance logger
				_ = cfg.Set("missing", 1) // wraps an error
				_ = cfg.GetLogger()
			}
		}()
	}
	wg.Wait()
}
