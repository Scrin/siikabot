package fmi

import "math"

const earthRadiusKm = 6371.0

// point is a position as latitude and longitude, in degrees
type point struct {
	lat float64
	lon float64
}

// distanceKm is the great-circle distance between two points
func distanceKm(a, b point) float64 {
	lat1, lat2 := a.lat*math.Pi/180, b.lat*math.Pi/180
	dLat := lat2 - lat1
	dLon := (b.lon - a.lon) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(h))
}

// polygonContains reports whether a point is inside a polygon, by counting how many edges a ray from the point
// crosses. Treating latitude and longitude as plane coordinates is accurate enough for warning areas, which are
// at most a few hundred kilometres across.
func polygonContains(polygon []point, p point) bool {
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		a, b := polygon[i], polygon[j]
		if (a.lat > p.lat) != (b.lat > p.lat) &&
			p.lon < (b.lon-a.lon)*(p.lat-a.lat)/(b.lat-a.lat)+a.lon {
			inside = !inside
		}
	}
	return inside
}

// boundingBox returns a box that contains every point within radiusKm of the centre, as minimum and maximum
// latitude and longitude
func boundingBox(centre point, radiusKm float64) (minLat, minLon, maxLat, maxLon float64) {
	dLat := radiusKm / earthRadiusKm * 180 / math.Pi
	dLon := dLat / math.Cos(centre.lat*math.Pi/180)
	return centre.lat - dLat, centre.lon - dLon, centre.lat + dLat, centre.lon + dLon
}
