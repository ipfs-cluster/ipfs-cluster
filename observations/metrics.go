package observations

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("observations")

var (
	// taken from ocgrpc (https://github.com/census-instrumentation/opencensus-go/blob/master/plugin/ocgrpc/stats_common.go)
	latencyDistribution = view.Distribution(0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)

	bytesDistribution = view.Distribution(0, 24, 32, 64, 128, 256, 512, 1024, 2048, 4096, 16384, 65536, 262144, 1048576)
)

// attributes
var (
	ClientIPAttribute = "http.client.ip"
)

// keys
var (
	HostKey = makeKey("host")
)

// metrics
var (
	// Pins counts the number of pins ipfs-cluster is tracking.
	Pins = stats.Int64("cluster/pin_count", "Number of pins", stats.UnitDimensionless)
	// TrackerPins counts the number of pins the local peer is tracking.
	TrackerPins = stats.Int64("pintracker/pin_count", "Number of pins", stats.UnitDimensionless)
	// Peers counts the number of ipfs-cluster peers are currently in the cluster.
	Peers = stats.Int64("cluster/peers", "Number of cluster peers", stats.UnitDimensionless)
	// Alerts is the number of alerts that have been sent due to peers not sending "ping" heartbeats in time.
	Alerts = stats.Int64("cluster/alerts", "Number of alerts triggered", stats.UnitDimensionless)
)

// views, which is just the aggregation of the metrics
var (
	PinsView = &view.View{
		Measure:     Pins,
		Aggregation: view.LastValue(),
	}

	TrackerPinsView = &view.View{
		Measure:     TrackerPins,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	PeersView = &view.View{
		Measure:     Peers,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	AlertsView = &view.View{
		Measure:     Alerts,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	DefaultViews = []*view.View{
		PinsView,
		TrackerPinsView,
		PeersView,
		AlertsView,
	}
)

func makeKey(name string) tag.Key {
	key, err := tag.NewKey(name)
	if err != nil {
		logger.Fatal(err)
	}
	return key
}
