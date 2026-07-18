package main

import "time"

const recentPublicationYearCount = 6

// recentPublicationYearRange returns an inclusive range covering the current
// calendar year and the preceding five calendar years.
func recentPublicationYearRange(now time.Time) (start, end int) {
	end = now.Year()
	start = end - recentPublicationYearCount + 1
	return start, end
}
