package fmi

import "testing"

func TestDistanceKm(t *testing.T) {
	// Helsinki to Tampere is about 160 km as the crow flies
	got := distanceKm(point{60.1695, 24.9354}, point{61.4991, 23.7871})
	if got < 155 || got > 165 {
		t.Errorf("distanceKm() = %.1f, want about 160", got)
	}
	if d := distanceKm(point{60, 25}, point{60, 25}); d != 0 {
		t.Errorf("distance to itself = %v", d)
	}
}

func TestPolygonContains(t *testing.T) {
	square := []point{{60.0, 24.5}, {60.0, 25.3}, {60.4, 25.3}, {60.4, 24.5}, {60.0, 24.5}}
	if !polygonContains(square, point{60.17, 24.94}) {
		t.Error("Helsinki should be inside")
	}
	if polygonContains(square, point{61.5, 23.8}) {
		t.Error("Tampere should be outside")
	}

	// A concave shape: an L, with the point in its notch outside
	l := []point{{0, 0}, {0, 2}, {1, 2}, {1, 1}, {2, 1}, {2, 0}}
	if polygonContains(l, point{1.5, 1.5}) {
		t.Error("the notch of the L should be outside")
	}
	if !polygonContains(l, point{0.5, 1.5}) {
		t.Error("the L's arm should be inside")
	}
}

func TestBoundingBox(t *testing.T) {
	centre := point{60.17, 24.94}
	minLat, minLon, maxLat, maxLon := boundingBox(centre, 50)
	// Every point 50 km away in each direction has to fit
	for _, edge := range []point{{maxLat, centre.lon}, {minLat, centre.lon}, {centre.lat, maxLon}, {centre.lat, minLon}} {
		if d := distanceKm(centre, edge); d < 49.5 {
			t.Errorf("box edge %v is only %.1f km away", edge, d)
		}
	}
}
