package fmi

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// Place is a location as FMI resolves its name
type Place struct {
	Name   string
	Region string
	Lat    float64
	Lon    float64
}

// Label names the place for people, with its region when that adds anything, such as "Kallio, Helsinki". For a
// Finnish town FMI gives the country, "Suomi", as the region, which adds nothing.
func (p Place) Label() string {
	if p.Region == "" || strings.EqualFold(p.Region, p.Name) || p.Region == "Suomi" {
		return p.Name
	}
	return p.Name + ", " + p.Region
}

// maxCachedPlaces bounds the place cache. Places don't move, so entries never expire, but the names people ask
// for are unbounded.
const maxCachedPlaces = 1000

var (
	placeCache      = map[string]Place{}
	placeCacheMutex sync.Mutex
)

// ResolvePlace looks up a place by name, such as "Espoo" or "Kallio,Helsinki". An unknown name is an *APIError
// whose message says so.
func ResolvePlace(ctx context.Context, name string) (Place, error) {
	key := strings.ToLower(strings.TrimSpace(name))

	placeCacheMutex.Lock()
	place, cached := placeCache[key]
	placeCacheMutex.Unlock()
	if cached {
		return place, nil
	}

	// The location parameters work with any producer. FMI's own forecast covers all of Finland, so it always
	// has a time to answer for.
	rows, err := timeseries(ctx, url.Values{
		"producer":  {"pal_skandinavia"},
		"place":     {strings.TrimSpace(name)},
		"param":     {"name,region,lat,lon"},
		"timesteps": {"1"},
	})
	if err != nil {
		return Place{}, err
	}
	if len(rows) == 0 {
		log.Warn().Ctx(ctx).Str("place", name).Msg("FMI returned no location for a place")
		return Place{}, fmt.Errorf("FMI returned no location for %q", name)
	}

	lat, latOK := rows[0].number("lat")
	lon, lonOK := rows[0].number("lon")
	if !latOK || !lonOK {
		log.Warn().Ctx(ctx).Str("place", name).Msg("FMI returned a location without coordinates")
		return Place{}, fmt.Errorf("FMI returned no coordinates for %q", name)
	}
	place = Place{Name: rows[0].text("name"), Region: rows[0].text("region"), Lat: lat, Lon: lon}
	if place.Name == "" {
		place.Name = strings.TrimSpace(name)
	}

	placeCacheMutex.Lock()
	if len(placeCache) >= maxCachedPlaces {
		placeCache = map[string]Place{}
	}
	placeCache[key] = place
	placeCacheMutex.Unlock()

	log.Debug().Ctx(ctx).
		Str("place", name).
		Str("resolved_name", place.Name).
		Float64("lat", place.Lat).
		Float64("lon", place.Lon).
		Msg("Resolved FMI place")
	return place, nil
}
