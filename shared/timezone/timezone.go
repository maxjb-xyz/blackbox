package timezone

import (
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

// ConfigureLocal applies the standard TZ environment variable to the process.
func ConfigureLocal() (string, error) {
	tz := strings.TrimSpace(os.Getenv("TZ"))
	if tz == "" {
		return "", nil
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return tz, err
	}

	time.Local = loc
	return tz, nil
}
