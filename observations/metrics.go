// Package observations sets up metric and trace exporting for IPFS cluster.
package observations

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	logging "github.com/ipfs/go-log/v2"
)

var logger = logging.Logger("observations")

var (
// taken from ocgrpc (https://github.com/census-instrumentation/opencensus-go/blob/master/plugin/ocgrpc/stats_common.go)
// latencyDistribution      = view.Distribution(0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)
// bytesDistribution        = view.Distribution(0, 24, 32, 64, 128, 256, 512, 1024, 2048, 4096, 16384, 65536, 262144, 1048576)
// messageCountDistribution = view.Distribution(1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536)
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
	// This metric is managed in state/dsstate.
	Pins = stats.Int64("pins", "Total number of cluster pins", stats.UnitDimensionless)

	// These metrics are managed by the pintracker/optracker module.
	PinsQueued   = stats.Int64("pins/pin_queued", "Current number of pins queued for pinning", stats.UnitDimensionless)
	PinsPinning  = stats.Int64("pins/pinning", "Current number of pins currently pinning", stats.UnitDimensionless)
	PinsPinError = stats.Int64("pins/pin_error", "Current number of pins in pin_error state", stats.UnitDimensionless)

	// These metrics and managed in the ipfshttp module.
	PinsIpfsPins    = stats.Int64("pins/ipfs_pins", "Current number of items pinned on IPFS", stats.UnitDimensionless)
	PinsPinAdd      = stats.Int64("pins/pin_add", "Total number of IPFS pin requests", stats.UnitDimensionless)
	PinsPinAddError = stats.Int64("pins/pin_add_errors", "Total number of failed pin requests", stats.UnitDimensionless)
	BlocksPut       = stats.Int64("blocks/put", "Total number of blocks/put requests", stats.UnitDimensionless)
	BlocksAddedSize = stats.Int64("blocks/added_size", "Total size of blocks added in bytes", stats.UnitDimensionless)

	BlocksAdded      = stats.Int64("blocks/added", "Total number of blocks added", stats.UnitDimensionless)
	BlocksAddedError = stats.Int64("blocks/put_error", "Total number of block/put errors", stats.UnitDimensionless)
)

// views, which is just the aggregation of the metrics
var (
	PinsView = &view.View{
		Measure: Pins,
		// This would add a tag to the metric if a value for this key
		// is present in the context when recording the observation.

		//TagKeys: []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	PinsQueuedView = &view.View{
		Measure: PinsQueued,
		//TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	PinsPinningView = &view.View{
		Measure: PinsPinning,
		//TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	PinsPinErrorView = &view.View{
		Measure: PinsPinError,
		//TagKeys:     []tag.Key{HostKey},
		Aggregation: view.LastValue(),
	}

	PinsIpfsPinsView = &view.View{
		Measure:     PinsIpfsPins,
		Aggregation: view.LastValue(),
	}

	PinsPinAddView = &view.View{
		Measure:     PinsPinAdd,
		Aggregation: view.Sum(),
	}

	PinsPinAddErrorView = &view.View{
		Measure:     PinsPinAddError,
		Aggregation: view.Sum(),
	}

	BlocksPutView = &view.View{
		Measure:     BlocksPut,
		Aggregation: view.Sum(),
	}

	BlocksAddedSizeView = &view.View{
		Measure:     BlocksAddedSize,
		Aggregation: view.Sum(),
	}

	BlocksAddedView = &view.View{
		Measure:     BlocksAdded,
		Aggregation: view.Sum(),
	}

	BlocksAddedErrorView = &view.View{
		Measure:     PinsPinAddError,
		Aggregation: view.Sum(),
	}

	DefaultViews = []*view.View{
		PinsView,
		PinsQueuedView,
		PinsPinningView,
		PinsPinErrorView,
		PinsIpfsPinsView,
		PinsPinAddView,
		PinsPinAddErrorView,
		BlocksPutView,
		BlocksAddedSizeView,
		BlocksAddedView,
		BlocksAddedErrorView,
	}
)

func makeKey(name string) tag.Key {
	key, err := tag.NewKey(name)
	if err != nil {
		logger.Fatal(err)
	}
	return key
}
