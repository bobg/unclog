package unclog

import "time"

func timeFromMillis(millis int64) time.Time {
	return time.Unix(millis/1000, millisToNanos(millis%1000))
}

func millisToNanos(millis int64) int64 {
	return millis * int64(time.Millisecond/time.Nanosecond)
}
