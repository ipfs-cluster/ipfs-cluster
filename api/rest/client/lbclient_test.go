package client

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	ma "github.com/multiformats/go-multiaddr"
)

func TestFailoverConcurrently(t *testing.T) {
	// Create a load balancing client with 5 empty clients and 5 clients with APIs
	// say we want to retry the request for at most 5 times
	cfgs := make([]*Config, 10)

	// 5 clients with an invalid api address
	for i := 0; i < 5; i++ {
		maddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
		cfgs[i] = &Config{
			APIAddr:           maddr,
			DisableKeepAlives: true,
		}
	}

	// 5 clients with APIs
	for i := 5; i < 10; i++ {
		cfgs[i] = &Config{
			APIAddr:           apiMAddr(testAPI(t)),
			DisableKeepAlives: true,
		}
	}

	// Run many requests at the same time

	// With Failover strategy, it would go through first 5 empty clients
	// and then 6th working client. Thus, all requests should always succeed.
	testRunManyRequestsConcurrently(t, cfgs, &Failover{}, 200, 6, true)
	// First 5 clients are empty. Thus, all requests should fail.
	testRunManyRequestsConcurrently(t, cfgs, &Failover{}, 200, 5, false)
}

type dummyClient struct {
	defaultClient
	i int
}

// ID returns dummy client's serial number.
func (d *dummyClient) ID(ctx context.Context) (api.ID, error) {
	return api.ID{
		Peername: fmt.Sprintf("%d", d.i),
	}, nil
}

func TestRoundRobin(t *testing.T) {
	var clients []Client
	// number of clients
	n := 5
	// create n dummy clients
	for i := 0; i < n; i++ {
		c := &dummyClient{
			i: i,
		}
		clients = append(clients, c)
	}

	roundRobin := loadBalancingClient{
		strategy: &RoundRobin{
			clients: clients,
			length:  uint32(len(clients)),
		},
	}

	// clients should be used in the sequence 1, 2,.., 4, 0.
	for i := 0; i < n; i++ {
		id, _ := roundRobin.ID(context.Background())
		if id.Peername != fmt.Sprintf("%d", (i+1)%n) {
			t.Errorf("clients are not being tried in sequence, expected client: %d, but found: %s", i, id.Peername)
		}
	}

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
			_, err := c.ID(ctx)
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
