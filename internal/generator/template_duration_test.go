package generator

import "testing"

func TestTmplDuration(t *testing.T) {
	for _, tt := range []struct {
		in   interface{}
		lang []string
		want string
	}{
		{"PT1H25M", nil, "1 hr 25 min"},
		{"pt45m", nil, "45 min"},
		{"P1DT2H", nil, "1 d 2 hr"},
		{"PT0H0M", nil, "0 min"},
		{"PT30S", nil, "30 s"},
		{"PT1H25M", []string{"pl"}, "1 godz. 25 min"},
		{"PT1H25M", []string{"de-AT"}, "1 Std. 25 Min."},
		{"PT2H", []string{"xx"}, "2 hr"},
		{"P1Y", nil, "P1Y"},
		{"PT", nil, "PT"},
		{"1 hour", nil, "1 hour"},
		{42, nil, ""},
	} {
		if got := tmplDuration(tt.in, tt.lang...); got != tt.want {
			t.Errorf("duration(%v, %v) = %q, want %q", tt.in, tt.lang, got, tt.want)
		}
	}
}
