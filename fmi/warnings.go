package fmi

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Warning is one of FMI's weather warnings
type Warning struct {
	Event       string // such as "Wind warning for sea areas"
	Headline    string // such as "Yellow wind warning for sea area: Gulf of Finland, Sat 3.00 - 15.00"
	Description string
	Severity    string // CAP's scale: Minor, Moderate, Severe or Extreme
	Color       string // FMI's scale: yellow, orange or red
	Onset       time.Time
	Expires     time.Time
	Areas       []string
	polygons    [][]point
}

// Covers reports whether the warning's area contains a place
func (w Warning) Covers(place Place) bool {
	p := point{lat: place.Lat, lon: place.Lon}
	for _, polygon := range w.polygons {
		if polygonContains(polygon, p) {
			return true
		}
	}
	return false
}

// The CAP feed is an Atom feed with a full CAP alert in each entry. Each alert has the same warning in Finnish,
// Swedish and English, one info element per language.
type capFeed struct {
	Entries []struct {
		Alert capAlert `xml:"content>alert"`
	} `xml:"entry"`
}

type capAlert struct {
	Status  string    `xml:"status"`
	MsgType string    `xml:"msgType"`
	Infos   []capInfo `xml:"info"`
}

type capInfo struct {
	Language    string `xml:"language"`
	Event       string `xml:"event"`
	Severity    string `xml:"severity"`
	Onset       string `xml:"onset"`
	Expires     string `xml:"expires"`
	Headline    string `xml:"headline"`
	Description string `xml:"description"`
	Parameters  []struct {
		Name  string `xml:"valueName"`
		Value string `xml:"value"`
	} `xml:"parameter"`
	Areas []struct {
		Description string   `xml:"areaDesc"`
		Polygons    []string `xml:"polygon"`
	} `xml:"area"`
}

// warningsCacheDuration is how long a fetched feed is reused. FMI updates warnings a few times a day, and the
// feed is large, so there's no point fetching it for every question in a busy conversation.
const warningsCacheDuration = 5 * time.Minute

var (
	warningsCache   []Warning
	warningsFetched time.Time
	warningsMutex   sync.Mutex
)

// ActiveWarnings returns FMI's warnings that are in force or still to come, earliest first
func ActiveWarnings(ctx context.Context) ([]Warning, error) {
	warningsMutex.Lock()
	defer warningsMutex.Unlock()

	if warningsCache == nil || time.Since(warningsFetched) > warningsCacheDuration {
		body, err := get(ctx, warningsURL, nil)
		if err != nil {
			return nil, err
		}
		warnings, err := parseWarnings(body)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to parse FMI warnings feed")
			return nil, err
		}
		warningsCache = warnings
		warningsFetched = time.Now()
	}

	now := time.Now()
	var active []Warning
	for _, w := range warningsCache {
		if w.Expires.After(now) {
			active = append(active, w)
		}
	}
	return active, nil
}

// WarningsFor returns the active warnings whose area contains a place
func WarningsFor(ctx context.Context, place Place) ([]Warning, error) {
	all, err := ActiveWarnings(ctx)
	if err != nil {
		return nil, err
	}
	var covering []Warning
	for _, w := range all {
		if w.Covers(place) {
			covering = append(covering, w)
		}
	}
	return covering, nil
}

// parseWarnings reads the CAP feed. Cancelled warnings stay in the feed, marked "REMOVED", and are left out here.
func parseWarnings(body []byte) ([]Warning, error) {
	var feed capFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("failed to parse warnings feed: %w", err)
	}

	warnings := []Warning{}
	for _, entry := range feed.Entries {
		alert := entry.Alert
		if alert.Status != "Actual" || alert.MsgType == "Cancel" || len(alert.Infos) == 0 {
			continue
		}
		info := alert.Infos[0]
		for _, candidate := range alert.Infos {
			if strings.HasPrefix(candidate.Language, "en") {
				info = candidate
				break
			}
		}

		w := Warning{
			Event:       info.Event,
			Headline:    strings.TrimSpace(info.Headline),
			Description: strings.TrimSpace(info.Description),
			Severity:    info.Severity,
		}
		w.Onset, _ = time.Parse(time.RFC3339, info.Onset)
		w.Expires, _ = time.Parse(time.RFC3339, info.Expires)
		if w.Expires.IsZero() {
			// Without an end time there's nothing to say when it stops applying
			continue
		}
		for _, p := range info.Parameters {
			if p.Name == "color" {
				w.Color = p.Value
			}
		}
		for _, area := range info.Areas {
			w.Areas = append(w.Areas, strings.TrimSpace(area.Description))
			for _, polygon := range area.Polygons {
				if parsed := parsePolygon(polygon); len(parsed) >= 3 {
					w.polygons = append(w.polygons, parsed)
				}
			}
		}
		warnings = append(warnings, w)
	}

	sort.SliceStable(warnings, func(i, j int) bool { return warnings[i].Onset.Before(warnings[j].Onset) })
	return warnings, nil
}

// parsePolygon reads a CAP polygon: "lat,lon" pairs separated by spaces. Pairs that don't parse are skipped.
func parsePolygon(text string) []point {
	var polygon []point
	for _, pair := range strings.Fields(text) {
		latText, lonText, found := strings.Cut(pair, ",")
		if !found {
			continue
		}
		lat, latErr := strconv.ParseFloat(latText, 64)
		lon, lonErr := strconv.ParseFloat(lonText, 64)
		if latErr != nil || lonErr != nil {
			continue
		}
		polygon = append(polygon, point{lat: lat, lon: lon})
	}
	return polygon
}
