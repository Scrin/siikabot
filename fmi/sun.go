package fmi

import (
	"math"
	"time"
)

// SunDay is when the sun rises and sets on one day at one place
type SunDay struct {
	Sunrise     time.Time // UTC; zero when the sun doesn't rise or set that day
	Sunset      time.Time // UTC; zero when the sun doesn't rise or set that day
	DayLength   time.Duration
	PolarNight  bool // the sun stays below the horizon all day
	MidnightSun bool // the sun stays above the horizon all day
}

const degrees = math.Pi / 180

// sunHorizon is the sun's altitude, in degrees, at which it counts as rising or setting. The usual convention,
// -0.833°, gives days two to four minutes longer than FMI's at Finnish latitudes. -0.675° matches FMI's own
// sunrise and sunset to within ten seconds, and people here compare against FMI.
const sunHorizon = -0.675

// SunTimes computes sunrise, sunset and day length for a calendar date at a place, with the sunrise equation.
//
// It's computed here rather than asked from FMI, because FMI answers only for times its data covers: it has no
// answer for next summer, or for a day of polar night.
func SunTimes(year int, month time.Month, day int, lat, lon float64) SunDay {
	// Days since noon UTC on 1 January 2000, at the place's mean solar noon
	noon := time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
	n := math.Round(float64(noon.Unix())/86400 + 2440587.5 - 2451545.0 + 0.0008)
	meanNoon := n - lon/360

	// Whether the sun rises at all is decided by its position at noon
	_, declination := sunPosition(meanNoon)
	switch c := cosHourAngle(lat, declination); {
	case c > 1:
		return SunDay{PolarNight: true}
	case c < -1:
		return SunDay{MidnightSun: true, DayLength: 24 * time.Hour}
	}

	sunrise := sunEvent(meanNoon, lat, -1)
	sunset := sunEvent(meanNoon, lat, 1)
	return SunDay{Sunrise: sunrise, Sunset: sunset, DayLength: sunset.Sub(sunrise)}
}

// sunEvent finds sunrise (direction -1) or sunset (+1). The sun's declination changes during the day, by enough
// to move the times by a minute in spring and autumn, so each event is computed again with the sun's position
// at that event rather than at noon.
func sunEvent(meanNoon, lat, direction float64) time.Time {
	at := meanNoon
	var julianDay float64
	for i := 0; i < 4; i++ {
		transitOffset, declination := sunPosition(at)
		c := cosHourAngle(lat, declination)
		if c < -1 || c > 1 {
			// Only possible on the days around polar night or midnight sun; the previous estimate stands
			break
		}
		hourAngle := math.Acos(c) / degrees
		at = meanNoon + direction*hourAngle/360
		julianDay = 2451545.0 + meanNoon + transitOffset + direction*hourAngle/360
	}
	return fromJulianDay(julianDay)
}

// sunPosition returns, for a time in days since J2000, how far the solar transit is from mean solar noon in
// days, and the sun's declination in radians
func sunPosition(days float64) (transitOffset, declination float64) {
	meanAnomaly := math.Mod(357.5291+0.98560028*days, 360)
	center := 1.9148*math.Sin(meanAnomaly*degrees) + 0.0200*math.Sin(2*meanAnomaly*degrees) +
		0.0003*math.Sin(3*meanAnomaly*degrees)
	eclipticLongitude := math.Mod(meanAnomaly+center+180+102.9372, 360)
	transitOffset = 0.0053*math.Sin(meanAnomaly*degrees) - 0.0069*math.Sin(2*eclipticLongitude*degrees)
	declination = math.Asin(math.Sin(eclipticLongitude*degrees) * math.Sin(23.4397*degrees))
	return transitOffset, declination
}

// cosHourAngle is the cosine of the hour angle at which the sun crosses the horizon. Outside -1..1 it doesn't
// cross: above 1 it stays below, below -1 it stays above.
func cosHourAngle(lat, declination float64) float64 {
	return (math.Sin(sunHorizon*degrees) - math.Sin(lat*degrees)*math.Sin(declination)) /
		(math.Cos(lat*degrees) * math.Cos(declination))
}

// fromJulianDay converts a Julian day to a UTC time, to the second
func fromJulianDay(julianDay float64) time.Time {
	seconds := (julianDay - 2440587.5) * 86400
	return time.Unix(int64(math.Round(seconds)), 0).UTC()
}
