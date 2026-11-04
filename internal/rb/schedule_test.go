package rb

import (
	"testing"
	"time"
)

func schedCron(expr string, catchUp bool) Schedule {
	return Schedule{Mode: "cron", Cron: expr, Every: 24, Unit: "hours", CatchUp: catchUp}
}

func schedEvery(n int, unit string) Schedule {
	return Schedule{Mode: "interval", Cron: "", Every: n, Unit: unit, CatchUp: true}
}

func schedSimulate(t *testing.T, s Schedule, from, to string, entry *M, up func(int64) bool) []string {
	t.Helper()
	runs := []string{}
	for ts := cronAt(t, from); ts <= cronAt(t, to); ts += 60 {
		if up != nil && !up(ts) {
			continue
		}
		d := schedDecide(s, *entry, ts)
		*entry = d.Entry
		if d.Due {
			runs = append(runs, time.Unix(ts, 0).In(time.Local).Format("01-02 15:04"))
		}
	}
	return runs
}

func TestSchedDailyRunsOnce(t *testing.T) {
	cronTestZone(t)
	e := M{}
	cronEq(t, []string{"10-02 03:00", "10-03 03:00"}, schedSimulate(t, schedCron("0 3 * * *", true), "2026-10-01 10:00", "2026-10-03 23:59", &e, nil))
}

func TestSchedNewJobDoesNotFireForPast(t *testing.T) {
	cronTestZone(t)
	d := schedDecide(schedCron("0 3 * * *", true), M{}, cronAt(t, "2026-10-01 10:00"))
	cronEq(t, false, d.Due)
	cronEq(t, cronAt(t, "2026-10-01 03:00"), d.Entry["slot"])
}

func TestSchedMissedSlotRunsOnce(t *testing.T) {
	cronTestZone(t)
	e := M{}
	a, b := cronAt(t, "2026-10-02 00:00"), cronAt(t, "2026-10-05 09:00")
	off := func(ts int64) bool { return ts < a || ts >= b }
	cronEq(t, []string{"10-05 09:00"}, schedSimulate(t, schedCron("0 3 * * *", true), "2026-10-01 10:00", "2026-10-05 12:00", &e, off),
		"three missed nights, one catch-up run")
}

func TestSchedNoCatchUpPassesOver(t *testing.T) {
	cronTestZone(t)
	e := M{}
	a, b := cronAt(t, "2026-10-02 00:00"), cronAt(t, "2026-10-02 09:00")
	off := func(ts int64) bool { return ts < a || ts >= b }
	cronEq(t, []string{"10-03 03:00"}, schedSimulate(t, schedCron("0 3 * * *", false), "2026-10-01 10:00", "2026-10-03 12:00", &e, off))
}

func TestSchedSlightlyLateStillRuns(t *testing.T) {
	cronTestZone(t)
	e := M{}
	a, b := cronAt(t, "2026-10-02 03:00"), cronAt(t, "2026-10-02 03:04")
	late := func(ts int64) bool { return ts < a || ts >= b }
	cronEq(t, []string{"10-02 03:04"}, schedSimulate(t, schedCron("0 3 * * *", false), "2026-10-01 10:00", "2026-10-02 12:00", &e, late))
}

func TestSchedSlotStaysDueUntilStarted(t *testing.T) {
	cronTestZone(t)
	s := schedCron("0 3 * * *", true)
	e := schedDecide(s, M{}, cronAt(t, "2026-10-01 10:00")).Entry
	busyUntil := cronAt(t, "2026-10-02 03:47")
	started := ""
	for ts := cronAt(t, "2026-10-02 02:58"); ts <= cronAt(t, "2026-10-02 05:00") && started == ""; ts += 60 {
		d := schedDecide(s, e, ts)
		if d.Due && ts >= busyUntil {
			started = time.Unix(ts, 0).In(time.Local).Format("15:04")
			e = d.Entry
		}
	}
	cronEq(t, "03:47", started)
}

func TestSchedChangeStartsAfresh(t *testing.T) {
	cronTestZone(t)
	e := M{}
	schedSimulate(t, schedCron("0 3 * * *", true), "2026-10-01 10:00", "2026-10-01 10:05", &e, nil)
	d := schedDecide(schedCron("0 9 * * *", true), e, cronAt(t, "2026-10-01 10:06"))
	cronEq(t, false, d.Due, "the 09:00 slot that passed today does not count")
	cronEq(t, "cron:0 9 * * *", d.Entry["key"])
}

func TestSchedEvery15Minutes(t *testing.T) {
	cronTestZone(t)
	e := M{}
	cronEq(t, []string{"10-01 10:15", "10-01 10:30", "10-01 10:45", "10-01 11:00"},
		schedSimulate(t, schedCron("*/15 * * * *", true), "2026-10-01 10:01", "2026-10-01 11:05", &e, nil))
}

func TestSchedIntervalCountsFromSet(t *testing.T) {
	cronTestZone(t)
	e := M{}
	cronEq(t, []string{"10-01 16:00", "10-01 22:00", "10-02 04:00"},
		schedSimulate(t, schedEvery(6, "hours"), "2026-10-01 10:00", "2026-10-02 05:00", &e, nil))
}

func TestSchedManualRunMovesNext(t *testing.T) {
	cronTestZone(t)
	e := schedDecide(schedEvery(6, "hours"), M{}, cronAt(t, "2026-10-01 10:00")).Entry
	e["last_start"] = cronAt(t, "2026-10-01 14:00")
	next, ok := schedNext(schedEvery(6, "hours"), e, cronAt(t, "2026-10-01 14:01"))
	cronEq(t, true, ok)
	cronEq(t, cronAt(t, "2026-10-01 20:00"), next)
}

func TestSchedLongOutageOneRun(t *testing.T) {
	cronTestZone(t)
	e := M{}
	a, b := cronAt(t, "2026-10-01 11:00"), cronAt(t, "2026-10-03 08:00")
	off := func(ts int64) bool { return ts < a || ts >= b }
	cronEq(t, []string{"10-03 08:00", "10-03 09:00"},
		schedSimulate(t, schedEvery(1, "hours"), "2026-10-01 10:00", "2026-10-03 09:30", &e, off))
}

func TestSchedNextCron(t *testing.T) {
	cronTestZone(t)
	s := schedCron("0 3 * * *", true)
	e := schedDecide(s, M{}, cronAt(t, "2026-10-01 10:00")).Entry
	next, _ := schedNext(s, e, cronAt(t, "2026-10-01 10:00"))
	cronEq(t, cronAt(t, "2026-10-02 03:00"), next)
	next, _ = schedNext(s, e, cronAt(t, "2026-10-02 03:30"))
	cronEq(t, cronAt(t, "2026-10-02 03:00"), next, "a due slot not yet started is still the next run")
	off := s
	off.Mode = "off"
	_, ok := schedNext(off, e, cronAt(t, "2026-10-01 10:00"))
	cronEq(t, false, ok)
}

func TestSchedDescribe(t *testing.T) {
	cronEq(t, "Daily at 03:00", schedDescribe(schedCron("0 3 * * *", true)))
	cronEq(t, "Sundays at 04:30", schedDescribe(schedCron("30 4 * * 0", true)))
	cronEq(t, "Monthly on the 1st at 05:00", schedDescribe(schedCron("0 5 1 * *", true)))
	cronEq(t, "Monthly on the 22nd at 05:00", schedDescribe(schedCron("0 5 22 * *", true)))
	cronEq(t, "Hourly at :15", schedDescribe(schedCron("15 * * * *", true)))
	cronEq(t, "Cron */15 * * * *", schedDescribe(schedCron("*/15 * * * *", true)))
	cronEq(t, "Every 6 hours", schedDescribe(schedEvery(6, "hours")))
	cronEq(t, "Every day", schedDescribe(schedEvery(1, "days")))
	cronEq(t, "Manual only", schedDescribe(Schedule{Mode: "off"}))
}
