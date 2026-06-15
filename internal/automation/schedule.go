package automation

import (
	"fmt"
	"strings"
	"time"
)

const (
	ScheduleManual     = "manual"
	ScheduleDaily      = "daily"
	ScheduleWeekly     = "weekly"
	ScheduleMonthly    = "monthly"
	ScheduleEveryHours = "every_hours"
)

func NormalizeSchedule(schedule Schedule) (Schedule, error) {
	kind := strings.TrimSpace(schedule.Kind)
	if kind == "" {
		kind = ScheduleManual
	}
	schedule.Kind = kind
	if schedule.Hour < 0 || schedule.Hour > 23 {
		return Schedule{}, fmt.Errorf("hour must be between 0 and 23")
	}
	if schedule.Minute < 0 || schedule.Minute > 59 {
		return Schedule{}, fmt.Errorf("minute must be between 0 and 59")
	}
	switch kind {
	case ScheduleManual:
		schedule.Cron = ""
	case ScheduleDaily:
		schedule.Cron = fmt.Sprintf("%d %d * * *", schedule.Minute, schedule.Hour)
	case ScheduleWeekly:
		if schedule.Weekday < 0 || schedule.Weekday > 6 {
			return Schedule{}, fmt.Errorf("weekday must be between 0 and 6")
		}
		schedule.Cron = fmt.Sprintf("%d %d * * %d", schedule.Minute, schedule.Hour, schedule.Weekday)
	case ScheduleMonthly:
		if schedule.DayOfMonth < 1 || schedule.DayOfMonth > 31 {
			return Schedule{}, fmt.Errorf("day_of_month must be between 1 and 31")
		}
		schedule.Cron = fmt.Sprintf("%d %d %d * *", schedule.Minute, schedule.Hour, schedule.DayOfMonth)
	case ScheduleEveryHours:
		if schedule.EveryHours < 1 || schedule.EveryHours > 168 {
			return Schedule{}, fmt.Errorf("every_hours must be between 1 and 168")
		}
		schedule.Cron = fmt.Sprintf("%d */%d * * *", schedule.Minute, schedule.EveryHours)
	default:
		return Schedule{}, fmt.Errorf("unknown schedule kind %q", kind)
	}
	return schedule, nil
}

func Due(now time.Time, task Task) bool {
	if !task.Enabled {
		return false
	}
	if len(task.Triggers) == 0 {
		last := time.Time{}
		if task.LastRun != nil {
			last = task.LastRun.StartedAt
		}
		return scheduleDue(now, last, task.Schedule)
	}
	for _, trigger := range task.Triggers {
		if trigger.Type != TriggerTypeSchedule || !trigger.Enabled {
			continue
		}
		last := time.Time{}
		if state, ok := task.TriggerState[trigger.ID]; ok {
			last = state.LastMatchedAt
		}
		if last.IsZero() && task.LastRun != nil {
			last = task.LastRun.StartedAt
		}
		if scheduleDue(now, last, trigger.Schedule) {
			return true
		}
	}
	return false
}

func ScheduleDue(now, last time.Time, schedule Schedule) bool {
	return scheduleDue(now, last, schedule)
}

func scheduleDue(now, last time.Time, schedule Schedule) bool {
	if schedule.Kind == "" || schedule.Kind == ScheduleManual {
		return false
	}
	if !last.IsZero() && !last.Before(now) {
		return false
	}
	candidate := last
	if candidate.IsZero() {
		candidate = now.Add(-366 * 24 * time.Hour)
	}
	switch schedule.Kind {
	case ScheduleDaily:
		return hasDailyDue(now, candidate, schedule.Hour, schedule.Minute)
	case ScheduleWeekly:
		return hasWeeklyDue(now, candidate, schedule.Weekday, schedule.Hour, schedule.Minute)
	case ScheduleMonthly:
		return hasMonthlyDue(now, candidate, schedule.DayOfMonth, schedule.Hour, schedule.Minute)
	case ScheduleEveryHours:
		interval := time.Duration(schedule.EveryHours) * time.Hour
		if interval <= 0 {
			return false
		}
		return last.IsZero() || now.Sub(last) >= interval
	default:
		return false
	}
}

func hasDailyDue(now, last time.Time, hour, minute int) bool {
	for day := startOfDay(last); !day.After(now); day = day.AddDate(0, 0, 1) {
		due := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
		if due.After(last) && !due.After(now) {
			return true
		}
	}
	return false
}

func hasWeeklyDue(now, last time.Time, weekday, hour, minute int) bool {
	for day := startOfDay(last); !day.After(now); day = day.AddDate(0, 0, 1) {
		if int(day.Weekday()) != weekday {
			continue
		}
		due := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
		if due.After(last) && !due.After(now) {
			return true
		}
	}
	return false
}

func hasMonthlyDue(now, last time.Time, dayOfMonth, hour, minute int) bool {
	for cursor := startOfMonth(last); !cursor.After(now); cursor = cursor.AddDate(0, 1, 0) {
		due := time.Date(cursor.Year(), cursor.Month(), dayOfMonth, hour, minute, 0, 0, now.Location())
		if due.Month() != cursor.Month() {
			continue
		}
		if due.After(last) && !due.After(now) {
			return true
		}
	}
	return false
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
