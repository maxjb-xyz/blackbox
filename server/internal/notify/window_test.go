package notify

import "testing"

func TestInWindow(t *testing.T) {
	cases := []struct {
		name            string
		now, start, end string
		want            bool
	}{
		{"same-day inside", "10:30", "09:00", "17:00", true},
		{"same-day before", "08:59", "09:00", "17:00", false},
		{"same-day at start (inclusive)", "09:00", "09:00", "17:00", true},
		{"same-day at end (exclusive)", "17:00", "09:00", "17:00", false},
		{"wrap inside after midnight", "02:00", "22:00", "07:00", true},
		{"wrap inside before midnight", "23:30", "22:00", "07:00", true},
		{"wrap outside", "12:00", "22:00", "07:00", false},
		{"wrap at end (exclusive)", "07:00", "22:00", "07:00", false},
		{"zero-length", "10:00", "10:00", "10:00", false},
		{"bad format", "10:00", "9am", "5pm", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			now := mustMinutes(t, c.now)
			if got := inWindowMinutes(now, c.start, c.end); got != c.want {
				t.Fatalf("inWindowMinutes(%s,%s,%s)=%v want %v", c.now, c.start, c.end, got, c.want)
			}
		})
	}
}

func mustMinutes(t *testing.T, hhmm string) int {
	m, ok := parseHHMM(hhmm)
	if !ok {
		t.Fatalf("parseHHMM(%q) failed", hhmm)
	}
	return m
}

func TestAppendNote(t *testing.T) {
	if got := appendNote("body", ""); got != "body" {
		t.Fatalf("empty note: %q", got)
	}
	if got := appendNote("body", "(+2 suppressed in the last hour)"); got != "body\n\n(+2 suppressed in the last hour)" {
		t.Fatalf("with note: %q", got)
	}
	if got := appendNote("", "(+1 suppressed in the last day)"); got != "(+1 suppressed in the last day)" {
		t.Fatalf("empty body: %q", got)
	}
}
