package client

import (
	"context"
	"sync"
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

func TestLBClient(t *testing.T) {
	// Create a load balancing client with 5 empty clients and 5 clients with APIs
	// say we want to retry the request for at most 5 times
	cfgs := make([]*Config, 10)

	// 20 empty clients
	for i := 0; i < 5; i++ {
		maddr, _ := ma.NewMultiaddr("")
		cfgs[i] = &Config{
			APIAddr:           maddr,
			DisableKeepAlives: true,
		}
	}

	// 30 clients with APIs
	for i := 5; i < 10; i++ {
		cfgs[i] = &Config{
			APIAddr:           apiMAddr(testAPI(t)),
			DisableKeepAlives: true,
		}
	}

	// Run many requests at the same time

	// With Failover strategy, it would go through first 5 empty clients
	// and then 6th working client. Thus, all requests should always succeed.
	testRunManyRequestsConcurrently(t, cfgs, &Failover{}, 1000, 6, true)
	// First 5 clients are empty. Thus, all requests should fail.
	testRunManyRequestsConcurrently(t, cfgs, &Failover{}, 1000, 5, false)
}

func testRunManyRequestsConcurrently(t *testing.T, cfgs []*Config, strategy LBStrategy, requests int, retries int, pass bool) {
	c, err := NewLBClient(strategy, cfgs, retries)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, err = c.ID(ctx)
			if err != nil && pass {
				t.Error(err)
			}
			if err == nil && !pass {
				t.Error("request should fail with connection refusal")
			}
		}()
	}
	wg.Wait()
}
