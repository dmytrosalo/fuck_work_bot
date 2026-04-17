package handlers

import (
	"fmt"
	"time"
)

// timeUntilMidnight returns formatted time until next day reset (00:00 Kyiv)
func timeUntilReset() string {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		kyiv = time.FixedZone("Kyiv", 2*60*60)
	}

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
