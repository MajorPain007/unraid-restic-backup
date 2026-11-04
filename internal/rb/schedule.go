package rb

import (
	"fmt"
	"strconv"
	"strings"
)

const schedGrace = 600

type schedDecision struct {
	Due     bool
	Skip    bool
	Changed bool
	Entry   M
}

func schedKey(s Schedule) string {
	if s.Mode == "cron" {
		return "cron:" + s.Cron
	}
	if s.Mode == "interval" {
		unit := ""
		if s.Unit != "" {
			unit = s.Unit[:1]
		}
		return "interval:" + strconv.Itoa(s.Every) + unit
	}
	return "off"
}

func schedInterval(s Schedule) int64 {
	unit := map[string]int64{"minutes": 60, "hours": 3600, "days": 86400}
	return int64(s.Every) * unit[s.Unit]
}

func schedEntryFrom(entry M) int64 {
	since, last := toInt64(entry["since"]), toInt64(entry["last_start"])
	if last > since {
		return last
	}
	return since
}

func schedDecide(s Schedule, entry M, now int64) schedDecision {
	key := schedKey(s)
	out := schedDecision{Entry: M{}}
	for k, v := range entry {
		out.Entry[k] = v
	}
	if key == "off" {
		return out
	}
	if str(entry["key"]) != key {
		out.Entry = M{"key": key, "since": now, "slot": int64(0)}
		out.Changed = true
		if s.Mode == "cron" {
			if c, err := cronParse(s.Cron); err == nil {
				if slot, ok := c.prev(now); ok {
					out.Entry["slot"] = slot
				}
			}
		}
		return out
	}
	if s.Mode == "cron" {
		c, err := cronParse(s.Cron)
		if err != nil {
			return out
		}
		slot, ok := c.prev(now)
		if !ok || slot <= toInt64(entry["slot"]) {
			return out
		}
		out.Entry["slot"] = slot
		out.Changed = true
		if !s.CatchUp && now-slot > schedGrace {
			out.Skip = true
		} else {
			out.Due = true
		}
		return out
	}
	if now >= schedEntryFrom(entry)+schedInterval(s) {
		out.Due = true
		out.Changed = true
		out.Entry["last_start"] = now
	}
	return out
}

func schedNext(s Schedule, entry M, now int64) (int64, bool) {
	key := schedKey(s)
	if key == "off" {
		return 0, false
	}
	same := str(entry["key"]) == key
	if s.Mode == "cron" {
		c, err := cronParse(s.Cron)
		if err != nil {
			return 0, false
		}
		if same {
			slot, ok := c.prev(now)
			if ok && slot > toInt64(entry["slot"]) && (s.CatchUp || now-slot <= schedGrace) {
				return slot, true
			}
		}
		return c.next(now)
	}
	from := now
	if same {
		from = schedEntryFrom(entry)
	}
	return from + schedInterval(s), true
}

func schedDescribe(s Schedule) string {
	if s.Mode == "off" {
		return "Manual only"
	}
	if s.Mode == "interval" {
		unit := s.Unit
		every := ""
		if s.Every == 1 {
			unit = strings.TrimRight(unit, "s")
		} else {
			every = strconv.Itoa(s.Every) + " "
		}
		return "Every " + every + unit
	}
	f := strings.Split(s.Cron, " ")
	if len(f) == 5 && isDigits(f[0]) && isDigits(f[1]) {
		time := fmt.Sprintf("%02d:%02d", digitsInt(f[1]), digitsInt(f[0]))
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
		if f[2] == "*" && f[3] == "*" && f[4] == "*" {
			return "Daily at " + time
		}
		if f[2] == "*" && f[3] == "*" && isDigits(f[4]) && digitsInt(f[4]) <= 7 {
			return days[digitsInt(f[4])] + "s at " + time
		}
		if isDigits(f[2]) && f[3] == "*" && f[4] == "*" {
			n := digitsInt(f[2])
			suffix := "th"
			if n%10 == 1 && n != 11 {
				suffix = "st"
			} else if n%10 == 2 && n != 12 {
				suffix = "nd"
			} else if n%10 == 3 && n != 13 {
				suffix = "rd"
			}
			return fmt.Sprintf("Monthly on the %d%s at %s", n, suffix, time)
		}
	}
	if len(f) == 5 && isDigits(f[0]) && f[1] == "*" && f[2] == "*" && f[3] == "*" && f[4] == "*" {
		return fmt.Sprintf("Hourly at :%02d", digitsInt(f[0]))
	}
	return "Cron " + s.Cron
}
