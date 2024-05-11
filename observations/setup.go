package observations

import (
	"context"
	"expvar"
	"net/http"
	"net/http/pprof"

	rpc "github.com/libp2p/go-libp2p-gorpc"
	manet "github.com/multiformats/go-multiaddr/net"

	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	ocgorpc "github.com/lanzafame/go-libp2p-ocgorpc"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"go.opencensus.io/zpages"
)

// PromRegistry is the metrics Registry used by Cluster.
var PromRegistry = prom.NewRegistry()

// SetupMetrics configures and starts stats tooling,
// if enabled.
func SetupMetrics(cfg *MetricsConfig) error {
	if cfg.EnableStats {
		logger.Infof("stats collection enabled on %s", cfg.PrometheusEndpoint)
		return setupMetrics(cfg)
	}
	return nil
}

// JaegerTracer implements ipfscluster.Tracer.
type JaegerTracer struct {
	jaeger *jaeger.Exporter
}

// SetClient no-op.
func (t *JaegerTracer) SetClient(*rpc.Client) {}

// Shutdown the tracer and flush any remaining traces.
func (t *JaegerTracer) Shutdown(context.Context) error {
	// nil check for testing, where tracer may not be configured
	if t != (*JaegerTracer)(nil) && t.jaeger != nil {
		t.jaeger.Flush()
	}
	return nil
}

// SetupTracing configures and starts tracing tooling,
// if enabled.
func SetupTracing(cfg *TracingConfig) (*JaegerTracer, error) {
	if !cfg.EnableTracing {
		return nil, nil
	}
	logger.Info("tracing enabled...")
	je, err := setupTracing(cfg)
	if err != nil {
		return nil, err
	}
	return &JaegerTracer{je}, nil
}

func setupMetrics(cfg *MetricsConfig) error {
	// setup Prometheus
	goCollector := collectors.NewGoCollector()
	procCollector := collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})
	PromRegistry.MustRegister(goCollector, procCollector)
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "ipfscluster",
		Registry:  PromRegistry,
	})
	if err != nil {
		return err
	}

	// register prometheus with opencensus
	view.RegisterExporter(pe)
	view.SetReportingPeriod(cfg.ReportingInterval)

	// register the metrics views of interest
	if err := view.Register(DefaultViews...); err != nil {
		return err
	}
	if err := view.Register(
		ochttp.ClientCompletedCount,
		ochttp.ClientRoundtripLatencyDistribution,
		ochttp.ClientReceivedBytesDistribution,
		ochttp.ClientSentBytesDistribution,
	); err != nil {
		return err
	}
	if err := view.Register(
		ochttp.ServerRequestCountView,
		ochttp.ServerRequestBytesView,
		ochttp.ServerResponseBytesView,
		ochttp.ServerLatencyView,
		ochttp.ServerRequestCountByMethod,
		ochttp.ServerResponseCountByStatusCode,
	); err != nil {
		return err
	}
	if err := view.Register(ocgorpc.DefaultServerViews...); err != nil {
		return err
	}

	_, promAddr, err := manet.DialArgs(cfg.PrometheusEndpoint)
	if err != nil {
		return err
	}
	go func() {
		mux := http.NewServeMux()
		zpages.Handle(mux, "/debug")
		mux.Handle("/metrics", pe)
		mux.Handle("/debug/vars", expvar.Handler())
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/debug/pprof/block", pprof.Handler("block"))
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		if err := http.ListenAndServe(promAddr, mux); err != nil {
			logger.Fatalf("Failed to run Prometheus /metrics endpoint: %v", err)
		}
	}()
	return nil
}

// setupTracing configures a OpenCensus Tracing exporter for Jaeger.
func setupTracing(cfg *TracingConfig) (*jaeger.Exporter, error) {
	_, agentAddr, err := manet.DialArgs(cfg.JaegerAgentEndpoint)
	if err != nil {
		return nil, err
	}
	// setup Jaeger
	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint: agentAddr,
		Process: jaeger.Process{
			ServiceName: cfg.ServiceName + "-" + cfg.ClusterPeername,
			Tags: []jaeger.Tag{
				jaeger.StringTag("cluster_id", cfg.ClusterID),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// register jaeger with opencensus
	trace.RegisterExporter(je)
	// configure tracing
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(cfg.SamplingProb)})
	return je, nil
}
