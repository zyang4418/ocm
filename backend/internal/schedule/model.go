package schedule

import "time"

// Regime is a bell-time regime (e.g. 冬令时/夏令时). Its periods define how
// many class periods a day has and the clock time of each. The active regime
// for any date is determined by EffectiveMonth/EffectiveDay, which recur
// annually.
type Regime struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	EffectiveMonth int       `json:"effectiveMonth"`
	EffectiveDay   int       `json:"effectiveDay"`
	CreatedAt      time.Time `json:"createdAt"`
	Periods        []Period  `json:"periods"`
}

// Period is one class slot within a regime.
type Period struct {
	ID          int64  `json:"id"`
	RegimeID    int64  `json:"regimeId"`
	PeriodIndex int    `json:"periodIndex"`
	StartTime   string `json:"startTime"` // "HH:MM"
	EndTime     string `json:"endTime"`   // "HH:MM"
}

// RegimeInput carries the mutable regime fields for create and update.
type RegimeInput struct {
	Name           string `json:"name"`
	EffectiveMonth int    `json:"effectiveMonth"`
	EffectiveDay   int    `json:"effectiveDay"`
}

// PeriodInput is one period in a bulk-replace request.
type PeriodInput struct {
	PeriodIndex int    `json:"periodIndex"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
}
