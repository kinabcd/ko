package time

import "time"

// [NextDayOf] now
func NextDay(hour, minute int) time.Time {
	return NextDayOf(time.Now(), hour, minute)
}

// This function calculates the time for the next occurrence of the specified hour and minute within a day.
//
// hour is an integer representing the desired hour (0-23).
// minute is an integer representing the desired minute (0-59).
// Return A time.Time object representing the date and time for the next occurrence of the specified hour and minute.
func NextDayOf(from time.Time, hour, minute int) time.Time {
	next := time.Date(from.Year(), from.Month(), from.Day(), hour, minute, 0, 0, from.Location())
	if next.Before(from) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func UntilNextDay(hour, minute int) time.Duration {
	return time.Until(NextDay(hour, minute))
}
