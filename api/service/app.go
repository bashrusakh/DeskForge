package service

import (
	"os"
	"sync"
	"time"
)

type AppService struct {
}

// version is populated lazily on the first successful read of resources/version.
// versionMu guards both the cached value and the read attempt so concurrent
// callers don't race, and so a transient read failure on first call (e.g. the
// /resources volume not mounted yet inside a container) doesn't permanently
// freeze the value at "" — subsequent calls will re-try the read. The previous
// sync.Once-based variant short-circuited every retry into a no-op.
var (
	version   = ""
	startTime = ""
	versionMu sync.Mutex
)

func (a *AppService) GetAppVersion() string {
	versionMu.Lock()
	defer versionMu.Unlock()
	if version != "" {
		return version
	}
	v, err := os.ReadFile("resources/version")
	if err != nil {
		return ""
	}
	version = string(v)
	return version
}

func init() {
	// Initialize the AppService if needed
	startTime = time.Now().Format("2006-01-02 15:04:05")
}

// GetStartTime
func (a *AppService) GetStartTime() string {
	return startTime
}
