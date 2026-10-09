package fmi

import (
	"testing"
	"time"
)

func TestSunTimesMatchFMI(t *testing.T) {
	helsinkiTime, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}

	// FMI's own sunrise and sunset for these places and days, in Finnish time
	tests := []struct {
		place           string
		lat, lon        float64
		day             int
		sunrise, sunset string
	}{
		{"Helsinki", 60.1695213, 24.9354496, 12, "07:52:13", "18:20:04"},
		{"Rovaniemi", 66.5, 25.7166691, 12, "08:04:24", "18:01:19"},
		{"Utsjoki", 69.908638, 27.0284309, 12, "08:11:15", "17:43:44"},
		{"Helsinki", 60.1695213, 24.9354496, 18, "08:07:07", "18:02:33"},
		{"Oulu", 65.0123596, 25.4681606, 18, "08:20:06", "17:45:05"},
		{"Utsjoki", 69.908638, 27.0284309, 18, "08:36:35", "17:15:49"},
	}
	for _, tt := range tests {
		t.Run(tt.place, func(t *testing.T) {
			sun := SunTimes(2026, time.October, tt.day, tt.lat, tt.lon)
			if sun.PolarNight || sun.MidnightSun {
				t.Fatalf("SunTimes() = %+v, want a normal day", sun)
			}
			for _, event := range []struct {
				name string
				got  time.Time
				want string
			}{{"sunrise", sun.Sunrise, tt.sunrise}, {"sunset", sun.Sunset, tt.sunset}} {
				want, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-"+twoDigits(tt.day)+" "+event.want, helsinkiTime)
				if err != nil {
					t.Fatal(err)
				}
				if diff := event.got.Sub(want); diff > 30*time.Second || diff < -30*time.Second {
					t.Errorf("%s = %s, FMI says %s", event.name, event.got.In(helsinkiTime).Format("15:04:05"), event.want)
				}
			}
			if sun.DayLength != sun.Sunset.Sub(sun.Sunrise) {
				t.Errorf("day length %v doesn't match the times", sun.DayLength)
			}
		})
	}
}

func twoDigits(n int) string {
	return time.Date(2000, 1, n, 0, 0, 0, 0, time.UTC).Format("02")
}

func TestSunTimesPolarNightAndMidnightSun(t *testing.T) {
	night := SunTimes(2026, time.December, 21, 69.908638, 27.0284309)
	if !night.PolarNight || night.MidnightSun || !night.Sunrise.IsZero() || night.DayLength != 0 {
		t.Errorf("Utsjoki at midwinter = %+v, want polar night", night)
	}

	summer := SunTimes(2027, time.June, 21, 69.908638, 27.0284309)
	if !summer.MidnightSun || summer.PolarNight || !summer.Sunset.IsZero() || summer.DayLength != 24*time.Hour {
		t.Errorf("Utsjoki at midsummer = %+v, want midnight sun", summer)
	}

	// Helsinki has neither
	if day := SunTimes(2026, time.December, 21, 60.17, 24.94); day.PolarNight || day.DayLength < 5*time.Hour {
		t.Errorf("Helsinki at midwinter = %+v", day)
	}
}
