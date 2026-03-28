package adapters

import "time"

type SystemClock struct{}

func (c SystemClock) Now() time.Time {
	return time.Now()
}
