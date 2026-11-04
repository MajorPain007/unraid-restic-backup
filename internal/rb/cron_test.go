package rb

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func cronTestZone(t *testing.T) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

func cronAt(t *testing.T, s string) int64 {
	t.Helper()
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if tm, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return tm.Unix()
		}
	}
	t.Fatalf("bad time %s", s)
	return 0
}

func cronFmt(ts int64, ok bool) string {
	if !ok {
		return "null"
	}
	return time.Unix(ts, 0).In(time.Local).Format("2006-01-02 15:04 Mon")
}

func cronMust(t *testing.T, expr string) *cronExpr {
	t.Helper()
	c, err := cronParse(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return c
}

func cronEq(t *testing.T, want, got any, what ...string) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("%s: expected %v, got %v", strings.Join(what, ""), want, got)
	}
}

func TestCronMistakesAreNamed(t *testing.T) {
	cases := map[string]string{
		"":             "five fields",
		"* * * *":      "five fields",
		"60 * * * *":   "minute: 60 is outside 0-59",
		"* 24 * * *":   "hour: 24 is outside 0-23",
		"* * 0 * *":    "day of month: 0 is outside 1-31",
		"* * 32 * *":   "day of month: 32",
		"* * * 13 *":   "month: 13",
		"* * * * 8":    "day of week: 8",
		"5-1 * * * *":  "runs backwards",
		"*/0 * * * *":  "step \"0\"",
		"x * * * *":    "minute: \"x\" is not a number",
		"* * * foo *":  "month: \"foo\" is not a number or a name",
		"1,,2 * * * *": "empty list entry",
		"@reboot":      "five fields",
	}
	for expr, needle := range cases {
		p := cronProblem(expr)
		if p == "" {
			t.Errorf("%q was accepted", expr)
		} else if !strings.Contains(strings.ToLower(p), strings.ToLower(needle)) {
			t.Errorf("%q: %q lacks %q", expr, p, needle)
		}
	}
}

func TestCronUsualFormsAccepted(t *testing.T) {
	for _, expr := range []string{"*/15 * * * *", "0 3 * * *", "30 2 * * 1-5", "0 0 1 */3 *", "0 9 * * mon-fri",
		"0 4 * jan,jul sun", "5/15 * * * *", "  0  3  *  *  *  ", "@daily", "@HOURLY", "0 0 * * 7"} {
		cronEq(t, "", cronProblem(expr), expr)
	}
}

func TestCronStepRunsOnMultiples(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "*/15 * * * *")
	cronEq(t, true, c.matches(cronAt(t, "2026-10-01 10:15")), "10:15")
	cronEq(t, false, c.matches(cronAt(t, "2026-10-01 10:16")), "10:16")
	cronEq(t, "2026-10-01 10:15 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01 10:29:59"))))
	cronEq(t, "2026-10-01 10:30 Thu", cronFmt(c.next(cronAt(t, "2026-10-01 10:15:00"))))
}

func TestCronStepStartsAtA(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "5/15 * * * *")
	mins := []string{}
	for _, ts := range c.upcoming(cronAt(t, "2026-10-01 09:59"), 4) {
		mins = append(mins, time.Unix(ts, 0).In(time.Local).Format("04"))
	}
	cronEq(t, []string{"05", "20", "35", "50"}, mins)
}

func TestCronLatestRunTodayOrYesterday(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 3 * * *")
	cronEq(t, "2026-10-01 03:00 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01 10:00"))))
	cronEq(t, "2026-09-30 03:00 Wed", cronFmt(c.prev(cronAt(t, "2026-10-01 02:59"))))
	cronEq(t, "2026-10-01 03:00 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01 03:00:30"))), "within the minute")
	cronEq(t, "2026-10-02 03:00 Fri", cronFmt(c.next(cronAt(t, "2026-10-01 03:00:00"))), "next is strictly later")
}

func TestCronTurnOfMonthAndYear(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "30 23 31 12 *")
	cronEq(t, "2025-12-31 23:30 Wed", cronFmt(c.prev(cronAt(t, "2026-10-01 12:00"))))
	cronEq(t, "2026-12-31 23:30 Thu", cronFmt(c.next(cronAt(t, "2026-10-01 12:00"))))
}

func TestCronDomOrDowWhenBothGiven(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 3 1 * mon")
	days := []string{}
	for _, ts := range c.upcoming(cronAt(t, "2026-10-01 04:00"), 4) {
		days = append(days, time.Unix(ts, 0).In(time.Local).Format("Mon 2"))
	}
	cronEq(t, []string{"Mon 5", "Mon 12", "Mon 19", "Mon 26"}, days)
	ts, _ := c.next(cronAt(t, "2026-10-31 12:00"))
	cronEq(t, "Sun 1", time.Unix(ts, 0).In(time.Local).Format("Mon 2"), "November 1st is a Sunday")
}

func TestCronStarFieldMakesItBoth(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 */2 * 1")
	for _, ts := range c.upcoming(cronAt(t, "2026-10-01"), 6) {
		tm := time.Unix(ts, 0).In(time.Local)
		cronEq(t, "Mon", tm.Format("Mon"), tm.Format("2006-01-02"))
		cronEq(t, 1, tm.Day()%2, tm.Format("2006-01-02"), " is an odd day")
	}
}

func TestCronSundayAndNames(t *testing.T) {
	cronTestZone(t)
	cronEq(t, cronFmt(cronMust(t, "0 0 * * 0").next(cronAt(t, "2026-10-01"))),
		cronFmt(cronMust(t, "0 0 * * 7").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2026-10-04 00:00 Sun", cronFmt(cronMust(t, "0 0 * * sun").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2026-10-02 09:00 Fri", cronFmt(cronMust(t, "0 9 * * mon-fri").next(cronAt(t, "2026-10-01 09:00"))))
	cronEq(t, "2026-10-05 09:00 Mon", cronFmt(cronMust(t, "0 9 * * mon-fri").next(cronAt(t, "2026-10-02 09:00"))))
}

func TestCronFebruary29(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 29 2 *")
	cronEq(t, "2024-02-29 00:00 Thu", cronFmt(c.prev(cronAt(t, "2026-10-01"))))
	cronEq(t, "2028-02-29 00:00 Tue", cronFmt(c.next(cronAt(t, "2026-10-01"))))
}

func TestCronNeverFires(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 31 2 *")
	cronEq(t, "null", cronFmt(c.prev(cronAt(t, "2026-10-01"))))
	cronEq(t, "null", cronFmt(c.next(cronAt(t, "2026-10-01"))))
}

func TestCronDSTSkippedHour(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "30 2 * * *")
	cronEq(t, "2026-03-29 03:30 Sun", cronFmt(c.next(cronAt(t, "2026-03-28 12:00"))))
	cronEq(t, "2026-03-28 02:30 Sat", cronFmt(c.prev(cronAt(t, "2026-03-29 03:10"))), "not yet at 03:10")
	cronEq(t, "2026-03-29 03:30 Sun", cronFmt(c.prev(cronAt(t, "2026-03-29 03:40"))))
	cronEq(t, "2026-03-30 02:30 Mon", cronFmt(c.next(cronAt(t, "2026-03-29 03:30"))))
}

func TestCronMacros(t *testing.T) {
	cronTestZone(t)
	cronEq(t, "2026-10-02 00:00 Fri", cronFmt(cronMust(t, "@daily").next(cronAt(t, "2026-10-01 00:00"))))
	cronEq(t, "2026-10-04 00:00 Sun", cronFmt(cronMust(t, "@weekly").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2026-11-01 00:00 Sun", cronFmt(cronMust(t, "@monthly").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2027-01-01 00:00 Fri", cronFmt(cronMust(t, "@yearly").next(cronAt(t, "2026-10-01"))))
	cronEq(t, "2026-10-01 11:00 Thu", cronFmt(cronMust(t, "@hourly").next(cronAt(t, "2026-10-01 10:00"))))
}

func TestCronRareScheduleIsQuick(t *testing.T) {
	cronTestZone(t)
	c := cronMust(t, "0 0 29 2 1")
	t0 := time.Now()
	for i := 0; i < 50; i++ {
		cronMust(t, "0 0 29 2 *").prev(cronAt(t, "2026-10-01"))
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Errorf("fifty look-ups took %v", d)
	}
	cronEq(t, "2027-02-01 00:00 Mon", cronFmt(c.next(cronAt(t, "2026-10-01"))), "the first Monday in February")
}
