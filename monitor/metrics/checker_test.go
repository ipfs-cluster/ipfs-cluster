package metrics

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	peer "github.com/libp2p/go-libp2p-core/peer"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

func TestChecker_CheckPeers(t *testing.T) {
	t.Run("check with single metric", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(context.Background(), metrics, 2.0)

		metr := &api.Metric{
			Name:  "ping",
			Peer:  test.PeerID1,
			Value: "1",
			Valid: true,
		}
		metr.SetTTL(2 * time.Second)

		metrics.Add(metr)

		checker.CheckPeers([]peer.ID{test.PeerID1})
		select {
		case <-checker.Alerts():
			t.Error("there should not be an alert yet")
		default:
		}

		time.Sleep(3 * time.Second)
		err := checker.CheckPeers([]peer.ID{test.PeerID1})
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-checker.Alerts():
		default:
			t.Error("an alert should have been triggered")
		}

		checker.CheckPeers([]peer.ID{test.PeerID2})
		select {
		case <-checker.Alerts():
			t.Error("there should not be alerts for different peer")
		default:
		}
	})
}

func TestChecker_CheckAll(t *testing.T) {
	t.Run("checkall with single metric", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(context.Background(), metrics, 2.0)

		metr := &api.Metric{
			Name:  "ping",
			Peer:  test.PeerID1,
			Value: "1",
			Valid: true,
		}
		metr.SetTTL(2 * time.Second)

		metrics.Add(metr)

		checker.CheckAll()
		select {
		case <-checker.Alerts():
			t.Error("there should not be an alert yet")
		default:
		}

		time.Sleep(3 * time.Second)
		err := checker.CheckAll()
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-checker.Alerts():
		default:
			t.Error("an alert should have been triggered")
		}

		checker.CheckAll()
		select {
		case <-checker.Alerts():
			t.Error("there should not be alerts for different peer")
		default:
		}
	})
}

func TestChecker_Watch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	metrics := NewStore()
	checker := NewChecker(context.Background(), metrics, 2.0)

	metr := &api.Metric{
		Name:  "ping",
		Peer:  test.PeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(100 * time.Millisecond)
	metrics.Add(metr)

	peersF := func(context.Context) ([]peer.ID, error) {
		return []peer.ID{test.PeerID1}, nil
	}

	go checker.Watch(ctx, peersF, 200*time.Millisecond)

	select {
	case a := <-checker.Alerts():
		t.Log("received alert:", a)
	case <-ctx.Done():
		t.Fatal("should have received an alert")
	}
}

func TestChecker_Failed(t *testing.T) {
	t.Run("standard failure check", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(context.Background(), metrics, 2.0)

		for i := 0; i < 10; i++ {
			metrics.Add(makePeerMetric(test.PeerID1, "1", 3*time.Millisecond))
			time.Sleep(time.Duration(2) * time.Millisecond)
		}
		for i := 0; i < 10; i++ {
			metrics.Add(makePeerMetric(test.PeerID1, "1", 3*time.Millisecond))
			got := checker.FailedMetric("ping", test.PeerID1)
			// the magic number 17 represents the point at which
			// the time between metrics addition has gotten
			// so large that the probability that the service
			// has failed goes over the threshold.
			if i >= 17 && !got {
				t.Fatal("threshold should have been passed by now")
			}
			time.Sleep(time.Duration(i) * time.Millisecond)
		}
	})

	t.Run("ttl must expire before phiv causes failure", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(context.Background(), metrics, 0.05)

		for i := 0; i < 10; i++ {
			metrics.Add(makePeerMetric(test.PeerID1, "1", 10*time.Millisecond))
			time.Sleep(time.Duration(200) * time.Millisecond)
		}
		for i, j := 0, 10; i < 8; i, j = i+1, j*2 {
			time.Sleep(time.Duration(j) * time.Millisecond)
			metrics.Add(makePeerMetric(test.PeerID1, "1", 10*time.Millisecond))
			v, _, phiv, got := checker.failed("ping", test.PeerID1)
			t.Logf("i: %d: j: %d v: %f, phiv: %f, got: %v\n", i, j, v, phiv, got)
			if i < 7 && got {
				t.Fatal("threshold should not have been reached already")
			}
			if i >= 10 && !got {
				t.Fatal("threshold should have been reached by now")
			}
		}
	})
}

func TestChecker_alert(t *testing.T) {
	t.Run("remove peer from store after alert", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		metrics := NewStore()
		checker := NewChecker(ctx, metrics, 2.0)

		metr := &api.Metric{
			Name:  "ping",
			Peer:  test.PeerID1,
			Value: "1",
			Valid: true,
		}
		metr.SetTTL(100 * time.Millisecond)
		metrics.Add(metr)

		peersF := func(context.Context) ([]peer.ID, error) {
			return []peer.ID{test.PeerID1}, nil
		}

		go checker.Watch(ctx, peersF, 200*time.Millisecond)

		var alertCount int
		for {
			select {
			case a := <-checker.Alerts():
				t.Log("received alert:", a)
				alertCount++
				if alertCount > MaxAlertThreshold {
					t.Fatalf("there should no more than %d alert", MaxAlertThreshold)
				}
			case <-ctx.Done():
				if alertCount < 1 {
					t.Fatal("should have received an alert")
				}
				return
			}
		}
	})
}

//////////////////
// HELPER TESTS //
//////////////////

func TestThresholdValues(t *testing.T) {
	t.Log("TestThresholdValues is useful for testing out different threshold values")
	t.Log("It doesn't actually perform any 'tests', so it is skipped by default")
	t.SkipNow()

	thresholds := []float64{0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.8, 0.9, 1.3, 1.4, 1.5, 1.8, 2.0, 3.0, 4.0, 5.0, 7.0, 10.0, 20.0}

	t.Run("linear threshold test", func(t *testing.T) {
		dists := make([]timeseries, 0)
		phivs := make([]timeseries, 0)
		for _, v := range thresholds {
			metrics := NewStore()
			checker := NewChecker(context.Background(), metrics, v)
			tsName := fmt.Sprintf("%f", v)
			distTS := newTS(tsName)
			phivTS := newTS(tsName)

			var failed bool
			output := false

			check := func(i int) bool {
				inputv, dist, phiv, got := checker.failed("ping", test.PeerID1)
				if output {
					fmt.Println(i)
					fmt.Printf("phiv: %f\n", phiv)
					fmt.Printf("threshold: %f\n", v)
					fmt.Printf("latest: %f\n", inputv)
					fmt.Printf("distribution: %v\n", dist)
				}
				phivTS.record(phiv)
				failed = got
				if failed {
					distTS.record(float64(i))
				}
				return got
			}

			// set baseline static distribution of ping intervals
			for i := 0; i < 10; i++ {
				check(i)
				distTS.record(float64(10))
				metrics.Add(makePeerMetric(test.PeerID1, "1", 1*time.Second))
				time.Sleep(time.Duration(10) * time.Millisecond)
			}
			// start linearly increasing the interval values
			for i := 10; i < 100 && !check(i); i++ {
				distTS.record(float64(i))
				metrics.Add(makePeerMetric(test.PeerID1, "1", 1*time.Second))
				time.Sleep(time.Duration(i) * time.Millisecond)
			}

			if failed {
				dists = append(dists, *distTS)
				phivs = append(phivs, *phivTS)
				continue
			}
			dists = append(dists, *distTS)
			phivs = append(phivs, *phivTS)

			// if a threshold has made it to this point, all greater
			// thresholds will make it here so print the threshold and skip
			t.Log("threshold that was not meet by linear increase: ", v)
			break
		}
		plotDistsPhivs("linear_", dists, phivs)
	})

	t.Run("cubic threshold test", func(t *testing.T) {
		dists := make([]timeseries, 0)
		phivs := make([]timeseries, 0)
		for _, v := range thresholds {
			metrics := NewStore()
			checker := NewChecker(context.Background(), metrics, v)
			tsName := fmt.Sprintf("%f", v)
			distTS := newTS(tsName)
			phivTS := newTS(tsName)

			var failed bool
			output := false

			check := func(i int) bool {
				inputv, dist, phiv, got := checker.failed("ping", test.PeerID1)
				if output {
					fmt.Println(i)
					fmt.Printf("phiv: %f\n", phiv)
					fmt.Printf("threshold: %f\n", v)
					fmt.Printf("latest: %f\n", inputv)
					fmt.Printf("distribution: %v\n", dist)
				}
				phivTS.record(phiv)
				failed = got
				if failed {
					diff := math.Pow(float64(i), float64(i))
					distTS.record(diff)
				}
				return got
			}

			// set baseline static distribution of ping intervals
			for i := 0; i < 10; i++ {
				check(i)
				distTS.record(float64(8))
				metrics.Add(makePeerMetric(test.PeerID1, "1", 1*time.Second))
				time.Sleep(time.Duration(8) * time.Millisecond)
			}
			for i := 2; !check(i) && i < 20; i++ {
				diff := math.Pow(float64(i), 3)
				distTS.record(diff)
				metrics.Add(makePeerMetric(test.PeerID1, "1", 1*time.Second))
				time.Sleep(time.Duration(diff) * time.Millisecond)
			}

			if failed {
				dists = append(dists, *distTS)
				phivs = append(phivs, *phivTS)
				continue
			}
			dists = append(dists, *distTS)
			phivs = append(phivs, *phivTS)

			// if a threshold has made it to this point, all greater
			// thresholds will make it here so print the threshold and skip
			t.Log("threshold that was not meet by cubic increase: ", v)
			break
		}
		plotDistsPhivs("cubic_", dists, phivs)
	})

	t.Run("14x threshold test", func(t *testing.T) {
		dists := make([]timeseries, 0)
		phivs := make([]timeseries, 0)
		for _, v := range thresholds {
			metrics := NewStore()
			checker := NewChecker(context.Background(), metrics, v)
			tsName := fmt.Sprintf("%f", v)
			distTS := newTS(tsName)
			phivTS := newTS(tsName)

			var failed bool
			output := false

			check := func(i int) bool {
				inputv, dist, phiv, got := checker.failed("ping", test.PeerID1)
				if output {
					fmt.Println(i)
					fmt.Printf("phiv: %f\n", phiv)
					fmt.Printf("threshold: %f\n", v)
					fmt.Printf("latest: %f\n", inputv)
					fmt.Printf("distribution: %v\n", dist)
				}
				phivTS.record(phiv)
				failed = got
				if failed {
					diff := i * 50
					distTS.record(float64(diff))
				}
				return got
			}

			for i := 0; i < 10; i++ {
				check(i)
				distTS.record(float64(i))
				metrics.Add(makePeerMetric(test.PeerID1, "1", 1*time.Second))
				time.Sleep(time.Duration(i) * time.Millisecond)
			}
			for i := 10; !check(i) && i < 30; i++ {
				diff := i * 50
				distTS.record(float64(diff))
				metrics.Add(makePeerMetric(test.PeerID1, "1", 1*time.Second))
				time.Sleep(time.Duration(diff) * time.Millisecond)
			}

			if failed {
				dists = append(dists, *distTS)
				phivs = append(phivs, *phivTS)
				continue
			}
			dists = append(dists, *distTS)
			phivs = append(phivs, *phivTS)

			// if a threshold has made it to this point, all greater
			// thresholds will make it here so print the threshold and skip
			t.Log("threshold that was not meet by 14x increase: ", v)
			break
		}
		plotDistsPhivs("14x_", dists, phivs)
	})
}

func makePeerMetric(pid peer.ID, value string, ttl time.Duration) *api.Metric {
	metr := &api.Metric{
		Name:  "ping",
		Peer:  pid,
		Value: value,
		Valid: true,
	}
	metr.SetTTL(ttl)
	return metr
}

type timeseries struct {
	name   string
	values []float64
}

func newTS(name string) *timeseries {
	v := make([]float64, 0)
	return &timeseries{name: name, values: v}
}

func (ts *timeseries) record(v float64) {
	ts.values = append(ts.values, v)
}

func (ts *timeseries) ToXYs() plotter.XYs {
	pts := make(plotter.XYs, len(ts.values))
	for i, v := range ts.values {
		pts[i].X = float64(i)
		if math.IsInf(v, 0) {
			pts[i].Y = -5
		} else {
			pts[i].Y = v
		}
	}
	return pts
}

func toPlotLines(ts []timeseries) ([]string, [][]plot.Plotter) {
	labels := make([]string, 0)
	plotters := make([][]plot.Plotter, 0)
	for i, t := range ts {
		l, s, err := plotter.NewLinePoints(t.ToXYs())
		if err != nil {
			panic(err)
		}
		l.Width = vg.Points(2)
		l.Color = plotutil.Color(i)
		l.Dashes = plotutil.Dashes(i)
		s.Color = plotutil.Color(i)
		s.Shape = plotutil.Shape(i)

		labels = append(labels, t.name)
		plotters = append(plotters, []plot.Plotter{l, s})
	}
	return labels, plotters
}

func plotDistsPhivs(prefix string, dists, phivs []timeseries) {
	plotTS(prefix+"dists", dists)
	plotTS(prefix+"phivs", phivs)
}

func plotTS(name string, ts []timeseries) {
	p, err := plot.New()
	if err != nil {
		panic(err)
	}
	p.Title.Text = name
	p.Add(plotter.NewGrid())

	labels, plotters := toPlotLines(ts)
	for i := range labels {
		l, pts := plotters[i][0], plotters[i][1]
		p.Add(l, pts)
		p.Legend.Add(labels[i], l.(*plotter.Line), pts.(*plotter.Scatter))
	}

	if err := p.Save(20*vg.Inch, 15*vg.Inch, name+".png"); err != nil {
		panic(err)
	}
}
