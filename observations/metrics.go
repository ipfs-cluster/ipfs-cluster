package observations

import (
	"github.com/ipfs/ipfs-cluster/api"

	logging "github.com/ipfs/go-log"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var logger = logging.Logger("observations")

var (
	// taken from ocgrpc (https://github.com/census-instrumentation/opencensus-go/blob/master/plugin/ocgrpc/stats_common.go)
	latencyDistribution      = view.Distribution(0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)
	bytesDistribution        = view.Distribution(0, 24, 32, 64, 128, 256, 512, 1024, 2048, 4096, 16384, 65536, 262144, 1048576)
	messageCountDistribution = view.Distribution(1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536)
)

// attributes
var (
	ClientIPAttribute = "http.client.ip"
)

// keys
var (
	HostKey       = makeKey("host")
	RemotePeerKey = makeKey("remote_peer")
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

	StatusClusterError = stats.Int64("pintracker/cluster_error_count", "Number of pins with Cluster Error", stats.UnitDimensionless)
	StatusPinError     = stats.Int64("pintracker/pin_error_count", "Number of pins with status Pin Error", stats.UnitDimensionless)
	StatusUnpinError   = stats.Int64("pintracker/unpin_error_count", "Number of pins with status Unpin Error", stats.UnitDimensionless)
	StatusPinned       = stats.Int64("pintracker/pinned_count", "Number of pins with status Pinned", stats.UnitDimensionless)
	StatusPinning      = stats.Int64("pintracker/pinning_count", "Number of pins with status Pinning", stats.UnitDimensionless)
	StatusUnpinning    = stats.Int64("pintracker/unpinning_count", "Number of pins with status Unpinning", stats.UnitDimensionless)
	StatusUnpinned     = stats.Int64("pintracker/unpinned_count", "Number of pins with status Unpinned", stats.UnitDimensionless)
	StatusRemote       = stats.Int64("pintracker/remote_count", "Number of pins with status Remote", stats.UnitDimensionless)
	StatusPinQueued    = stats.Int64("pintracker/pin_queued_count", "Number of pins with status PinQueued", stats.UnitDimensionless)
	StatusUnpinQueued  = stats.Int64("pintracker/unpin_queued_count", "Number of pins with status UnpinQueued", stats.UnitDimensionless)
	StatusSharded      = stats.Int64("pintracker/sharded_count", "Number of pins with status Sharded", stats.UnitDimensionless)
)

// GetMeasureFromStatus gets measure for given tracker status.
func GetMeasureFromStatus(ts api.TrackerStatus) *stats.Int64Measure {
	switch ts {
	case api.TrackerStatusClusterError:
		return StatusClusterError
	case api.TrackerStatusPinError:
		return StatusPinError
	case api.TrackerStatusUnpinError:
		return StatusUnpinError
	case api.TrackerStatusPinned:
		return StatusPinned
	case api.TrackerStatusPinning:
		return StatusPinning
	case api.TrackerStatusUnpinning:
		return StatusUnpinning
	case api.TrackerStatusUnpinned:
		return StatusUnpinned
	case api.TrackerStatusRemote:
		return StatusRemote
	case api.TrackerStatusPinQueued:
		return StatusPinQueued
	case api.TrackerStatusUnpinQueued:
		return StatusUnpinQueued
	case api.TrackerStatusSharded:
		return StatusSharded
	default:
		// this would never get executed.
		return nil
	}
}

// views, which is just the aggregation of the metrics
var (
	PinsView = view.View{
		Measure:     Pins,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	TrackerPinsView = view.View{
		Measure:     TrackerPins,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	PeersView = view.View{
		Measure:     Peers,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	AlertsView = view.View{
		Measure:     Alerts,
		TagKeys:     []tag.Key{HostKey, RemotePeerKey},
		Aggregation: messageCountDistribution,
	}

	StatusClusterErrorView = view.View{
		Measure:     StatusClusterError,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusPinErrorView = view.View{
		Measure:     StatusPinError,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusUnpinErrorView = view.View{
		Measure:     StatusUnpinError,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusPinnedView = view.View{
		Measure:     StatusPinned,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusPinningView = view.View{
		Measure:     StatusPinning,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusUnpinningView = view.View{
		Measure:     StatusUnpinning,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusUnpinnedView = view.View{
		Measure:     StatusUnpinned,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusRemoteView = view.View{
		Measure:     StatusRemote,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusPinQueuedView = view.View{
		Measure:     StatusPinQueued,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusUnpinQueuedView = view.View{
		Measure:     StatusUnpinQueued,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	StatusShardedView = view.View{
		Measure:     StatusSharded,
		TagKeys:     []tag.Key{HostKey},
		Aggregation: view.Sum(),
	}

	DefaultViews = []*view.View{
		&PinsView,
		&TrackerPinsView,
		&PeersView,
		&AlertsView,
		&StatusClusterErrorView,
		&StatusPinErrorView,
		&StatusUnpinErrorView,
		&StatusPinnedView,
		&StatusPinningView,
		&StatusUnpinningView,
		&StatusUnpinnedView,
		&StatusRemoteView,
		&StatusPinQueuedView,
		&StatusUnpinQueuedView,
		&StatusShardedView,
	}
)

func makeKey(name string) tag.Key {
	key, err := tag.NewKey(name)
	if err != nil {
		logger.Fatal(err)
	}
	return key
}
