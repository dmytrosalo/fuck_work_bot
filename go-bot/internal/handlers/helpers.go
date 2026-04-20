package handlers

import (
	"fmt"
	"time"
)

// timeUntilMidnight returns formatted time until next day reset (00:00 Kyiv)
func timeUntilReset() string {
	kyiv := kyivLocation()
	now := time.Now().In(kyiv)
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, kyiv)
	diff := midnight.Sub(now)

	hours := int(diff.Hours())
	minutes := int(diff.Minutes()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dг %dхв", hours, minutes)
	}
	return fmt.Sprintf("%dхв", minutes)
}

// timeUntilNextHour returns formatted time until next hour
func timeUntilNextHour() string {
	now := time.Now()
	next := now.Truncate(time.Hour).Add(time.Hour)
	diff := next.Sub(now)
	return fmt.Sprintf("%dхв", int(diff.Minutes()))
}

// kyivLocation returns the Kyiv timezone
func kyivLocation() *time.Location {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		kyiv = time.FixedZone("Kyiv", 2*60*60)
	}
	return kyiv
}

// todayKyiv returns today's date string in Kyiv timezone
func todayKyiv() string {
	return time.Now().In(kyivLocation()).Format("2006-01-02")
}

// nowHourKyiv returns current date-hour string in Kyiv timezone (for hourly cooldowns)
func nowHourKyiv() string {
	return time.Now().In(kyivLocation()).Format("2006-01-02-15")
}
