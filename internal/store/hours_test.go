package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/price"
	"github.com/vizkid/burns/internal/usage"
)

var hourNow = time.Date(2026, 11, 2, 12, 30, 0, 0, time.UTC)

func hourRequest(key, model string, at time.Time, input, write, write1h int64) usage.Request {
	return usage.Request{Provider: usage.Claude, Key: key, Account: "a", Session: "s", Model: model, Input: input,
		CacheWrite: write, CacheWrite1h: write1h, Output: 1, Source: usage.SourceOTel, At: at}
}

// Like requests add up in their hour, and a one-hour cache write is split from the rest.
func TestRollUpGroupsAnHour(t *testing.T) {
	s := open(t)
	at := hourNow.Add(-2 * time.Hour)
	if err := s.AddRequests([]usage.Request{hourRequest("r1", "m", at, 10, 100, 60), hourRequest("r2", "m", at.Add(time.Minute), 20, 50, 0),
		hourRequest("r3", "m", at.Add(time.Hour), 5, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	if err := s.RollUp(hourNow.Add(-Retention), hourNow); err != nil {
		t.Fatal(err)
	}
	got, err := s.Hours(hourNow.Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Requests != 2 || got[0].Input != 30 || got[0].Write5m != 90 || got[0].Write1h != 60 || !got[0].Hour.Equal(at.Truncate(time.Hour)) {
		t.Fatalf("hours = %+v", got)
	}
}

// Requests sit in a band by their whole prompt, cached input included.
func TestRollUpBands(t *testing.T) {
	s := open(t)
	r := hourRequest("r1", "m", hourNow, 1, 0, 0)
	r.CachedInput = 300_000
	if err := s.AddRequests([]usage.Request{r, hourRequest("r2", "m", hourNow, 150_000, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	if err := s.RollUp(hourNow.Add(-Retention), hourNow); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Hours(hourNow.Add(-time.Hour))
	if len(got) != 2 || got[0].Band != 1 || got[1].Band != 2 {
		t.Fatalf("hours = %+v", got)
	}
}

// The stored band uses the price table's own thresholds, at their exact edges: a prompt
// over the limit is in the higher band, one at it is not.
func TestRollUpBandEdges(t *testing.T) {
	s := open(t)
	var rs []usage.Request
	for i, prompt := range []int64{price.BandMid, price.BandMid + 1, price.BandHigh, price.BandHigh + 1} {
		rs = append(rs, hourRequest(fmt.Sprint("r", i), fmt.Sprint("m", i), hourNow, prompt, 0, 0))
	}
	if err := s.AddRequests(rs); err != nil {
		t.Fatal(err)
	}
	if err := s.RollUp(hourNow.Add(-Retention), hourNow); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Hours(hourNow.Add(-time.Hour))
	bands := map[string]int{}
	for _, h := range got {
		bands[h.Model] = h.Band
	}
	if len(bands) != 4 || bands["m0"] != 0 || bands["m1"] != 1 || bands["m2"] != 1 || bands["m3"] != 2 {
		t.Fatalf("bands = %v", bands)
	}
}

// Rebuilding twice does not double, and a request that changed is counted as it now stands.
func TestRollUpRebuilds(t *testing.T) {
	s := open(t)
	if err := s.AddRequests([]usage.Request{hourRequest("r1", "m", hourNow, 10, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	s.RollUp(hourNow.Add(-Retention), hourNow)
	if err := s.AddRequests([]usage.Request{hourRequest("r1", "m", hourNow, 40, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	s.RollUp(hourNow.Add(-time.Hour), hourNow)
	got, _ := s.Hours(hourNow.Add(-time.Hour))
	if len(got) != 1 || got[0].Input != 40 || got[0].Requests != 1 {
		t.Fatalf("hours = %+v", got)
	}
}

// Totals outlive their request rows, which is why they exist.
func TestHoursOutliveRequests(t *testing.T) {
	s := open(t)
	if err := s.AddRequests([]usage.Request{hourRequest("r1", "m", hourNow.Add(-2*time.Hour), 10, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	s.RollUp(hourNow.Add(-Retention), hourNow)
	later := hourNow.Add(Retention + 24*time.Hour)
	if err := s.Sweep(later); err != nil {
		t.Fatal(err)
	}
	if err := s.RollUp(later.Add(-Retention), later); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Hours(hourNow.Add(-24 * time.Hour))
	if len(got) != 1 || got[0].Input != 10 {
		t.Fatalf("hours = %+v", got)
	}
}
