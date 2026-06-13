package service

import (
	"sync"
	"testing"
)

// TestGetAppVersion
func TestGetAppVersion(t *testing.T) {
	s := &AppService{}
	v := s.GetAppVersion()

	t.Logf("App Version: %s", v)
}

func TestMultipleGetAppVersion(t *testing.T) {
	s := &AppService{}

	//  WaitGroup  goroutine 
	wg := sync.WaitGroup{}
	wg.Add(10) //  10  goroutine
	//  10  goroutine
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done() // 
			v := s.GetAppVersion()

			t.Logf("App Version: %s", v)
		}()
	}
	//  goroutine 
	wg.Wait()
}
