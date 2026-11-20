package llmtools

import "testing"

// The language code becomes the first label of the host the summary is fetched from
func TestWikipediaLanguage(t *testing.T) {
	for code, valid := range map[string]bool{
		"en": true, "fi": true, "simple": true, "zh-min-nan": true, "be-tarask": true,
		"db/": false, "en.evil.example/x?": false, "a": false, "en:8080": false, "e n": false, "": false,
	} {
		if got := wikipediaLanguage.MatchString(code); got != valid {
			t.Errorf("wikipediaLanguage matches %q: %v, want %v", code, got, valid)
		}
	}
}
