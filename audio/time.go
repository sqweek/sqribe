package audio

import (
	"time"
)

var origin *time.Time = nil

/* stream.Time() isn't accurate for all host APIs (looking at you pulseaudio-alsa plugin).
** monotonicTime is a hack which transparently falls back to time.Now() as a monotonic
** timer if stream.Time() repeatedly returns zero */
var monotonicTime func() time.Duration

func init() {
	monotonicTime = timeThunk
}

func timeThunk() time.Duration {
	t := stream.Time()
	if t == 0 {
		if origin == nil {
			/* first time through, save current time and return 0 */
			origin = new(time.Time)
			*origin = time.Now()
			return 0
		} else {
			/* 0 again? clearly stream.Time() is broken, figure out the time ourselves */
			monotonicTime = fallbackTime
			return fallbackTime()
		}
	}
	/* non-zero, presumably stream.Time() will work fine from here on out */
	monotonicTime = streamTime
	return t
}

func fallbackTime() time.Duration {
	return time.Now().Sub(*origin)
}

func streamTime() time.Duration {
	return stream.Time()
}

