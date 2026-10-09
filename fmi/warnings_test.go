package fmi

import (
	"context"
	"os"
	"testing"
)

func warningsFeed(t *testing.T) string {
	t.Helper()
	feed, err := os.ReadFile("testdata/warnings.xml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(feed)
}

func TestParseWarnings(t *testing.T) {
	warnings, err := parseWarnings([]byte(warningsFeed(t)))
	if err != nil {
		t.Fatalf("parseWarnings() error = %v", err)
	}

	// The cancelled warning is left out; the expired one is filtered later, by ActiveWarnings
	if len(warnings) != 3 {
		t.Fatalf("got %d warnings, want 3", len(warnings))
	}
	for _, w := range warnings {
		if w.Headline == "REMOVED: Yellow wind warning for sea area: Gulf of Finland" {
			t.Errorf("cancelled warning was kept")
		}
	}

	// Ordered by onset, so the expired one from 2000 comes first
	if warnings[0].Headline != "Yellow rain warning: Uusimaa" {
		t.Errorf("first warning = %q", warnings[0].Headline)
	}

	uusimaa := warnings[2]
	if uusimaa.Headline != "Yellow wind warning for land areas: Uusimaa, Fri 20.00 - Sat 0.00" {
		t.Errorf("the English text wasn't chosen: %q", uusimaa.Headline)
	}
	if uusimaa.Color != "yellow" || uusimaa.Severity != "Moderate" || uusimaa.Event != "Wind warning for land areas" {
		t.Errorf("warning = %+v", uusimaa)
	}
	if len(uusimaa.Areas) != 1 || uusimaa.Areas[0] != "Uusimaa" || len(uusimaa.polygons) != 1 {
		t.Errorf("areas = %v, polygons = %d", uusimaa.Areas, len(uusimaa.polygons))
	}
}

func TestActiveWarningsAndCoverage(t *testing.T) {
	requests := fakeFMI(t, map[string]response{"warnings": ok(warningsFeed(t))})
	ctx := context.Background()

	active, err := ActiveWarnings(ctx)
	if err != nil {
		t.Fatalf("ActiveWarnings() error = %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("got %d active warnings, want 2 (one cancelled, one expired)", len(active))
	}

	covering, err := WarningsFor(ctx, helsinki)
	if err != nil {
		t.Fatalf("WarningsFor() error = %v", err)
	}
	if len(covering) != 1 || covering[0].Event != "Wind warning for land areas" {
		t.Errorf("warnings for Helsinki = %+v", covering)
	}

	tampere := Place{Name: "Tampere", Lat: 61.4991112, Lon: 23.7871208}
	if covering, _ := WarningsFor(ctx, tampere); len(covering) != 0 {
		t.Errorf("got %d warnings for Tampere, which no warning covers", len(covering))
	}

	// All three calls were answered from one download
	if len(requests.list()) != 1 {
		t.Errorf("fetched the feed %d times, want 1", len(requests.list()))
	}
}

func TestParsePolygon(t *testing.T) {
	polygon := parsePolygon("64.500701,24.236756 64.500701,23.506115  bad 64.1,x 63.9,22.1")
	want := []point{{64.500701, 24.236756}, {64.500701, 23.506115}, {63.9, 22.1}}
	if len(polygon) != len(want) {
		t.Fatalf("got %v, want %v", polygon, want)
	}
	for i := range want {
		if polygon[i] != want[i] {
			t.Errorf("point %d = %v, want %v", i, polygon[i], want[i])
		}
	}
}
