package rb

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type cronField struct {
	label    string
	min, max int
}

var cronFields = [5]cronField{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day of month", 1, 31},
	{"month", 1, 12},
	{"day of week", 0, 7},
}

var cronMacros = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

var cronNames = map[int]map[string]int{
	3: {"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12},
	4: {"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6},
}

var cronSpaces = regexp.MustCompile(`[ \t\n\x0B\f\r]+`)

type cronExpr struct {
	Expr    string
	sets    [5][64]bool
	domStar bool
	dowStar bool
}

func cronParse(expr string) (*cronExpr, error) {
	c := &cronExpr{Expr: strings.Trim(expr, " \t\n\r\x00\x0B")}
	text := asciiLower(c.Expr)
	if m, ok := cronMacros[text]; ok {
		text = m
	}
	parts := cronSpaces.Split(text, -1)
	if text == "" || len(parts) != 5 {
		return nil, errors.New("A schedule needs five fields: minute, hour, day of month, month, day of week")
	}
	for i, part := range parts {
		set, err := cronParseField(part, i)
		if err != nil {
			return nil, err
		}
		c.sets[i] = set
	}
	if c.sets[4][7] {
		c.sets[4][7] = false
		c.sets[4][0] = true
	}
	c.domStar = parts[2][0] == '*'
	c.dowStar = parts[4][0] == '*'
	return c, nil
}

func cronProblem(expr string) string {
	if _, err := cronParse(expr); err != nil {
		return err.Error()
	}
	return ""
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, ch := range b {
		if ch >= 'A' && ch <= 'Z' {
			b[i] = ch + 32
		}
	}
	return string(b)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func digitsInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return math.MaxInt
	}
	return n
}

func cronParseField(text string, index int) ([64]bool, error) {
	f := cronFields[index]
	var set [64]bool
	for _, item := range strings.Split(text, ",") {
		if item == "" {
			return set, fmt.Errorf("%s: empty list entry", f.label)
		}
		step := 1
		if strings.Contains(item, "/") {
			p := strings.SplitN(item, "/", 2)
			item = p[0]
			if !isDigits(p[1]) || digitsInt(p[1]) < 1 {
				return set, fmt.Errorf("%s: step \"%s\" must be a whole number of at least 1", f.label, p[1])
			}
			step = digitsInt(p[1])
		}
		var lo, hi int
		if item == "*" {
			lo, hi = f.min, f.max
		} else if strings.Contains(item, "-") {
			p := strings.SplitN(item, "-", 2)
			var err error
			if lo, err = cronValue(p[0], index); err != nil {
				return set, err
			}
			if hi, err = cronValue(p[1], index); err != nil {
				return set, err
			}
			if lo > hi {
				return set, fmt.Errorf("%s: range %s runs backwards", f.label, item)
			}
		} else {
			var err error
			if lo, err = cronValue(item, index); err != nil {
				return set, err
			}
			hi = lo
			if step > 1 {
				hi = f.max
			}
		}
		for v := lo; ; v += step {
			set[v] = true
			if hi-v < step {
				break
			}
		}
	}
	return set, nil
}

func cronValue(text string, index int) (int, error) {
	f := cronFields[index]
	if v, ok := cronNames[index][text]; ok {
		return v, nil
	}
	if !isDigits(text) {
		suffix := ""
		if index >= 3 {
			suffix = " or a name"
		}
		return 0, fmt.Errorf("%s: \"%s\" is not a number%s", f.label, text, suffix)
	}
	v := digitsInt(text)
	if v < f.min || v > f.max {
		return 0, fmt.Errorf("%s: %d is outside %d-%d", f.label, v, f.min, f.max)
	}
	return v, nil
}

func cronDaysIn(y, m int) int {
	return time.Date(y, time.Month(m)+1, 0, 12, 0, 0, 0, time.UTC).Day()
}

func cronLocal(y, m, d, h, i int) int64 {
	return time.Date(y, time.Month(m), d, h, i, 0, 0, time.Local).Unix()
}

func cronFieldsOf(ts int64) (y, m, d, h, i int) {
	t := time.Unix(ts, 0).In(time.Local)
	return t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute()
}

func (c *cronExpr) dayMatches(y, m, d int) bool {
	if !c.sets[3][m] {
		return false
	}
	dom := c.sets[2][d]
	dow := c.sets[4][int(time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.UTC).Weekday())]
	if c.domStar || c.dowStar {
		return dom && dow
	}
	return dom || dow
}

func (c *cronExpr) matches(ts int64) bool {
	y, m, d, h, i := cronFieldsOf(ts)
	return c.dayMatches(y, m, d) && c.sets[1][h] && c.sets[0][i]
}

func (c *cronExpr) prev(ts int64) (int64, bool) {
	y, m, d, h, i := cronFieldsOf(ts)
	for day := 0; day < 366*28; day++ {
		if c.dayMatches(y, m, d) {
			hh := 23
			if day == 0 {
				hh = h
			}
			for ; hh >= 0; hh-- {
				if !c.sets[1][hh] {
					continue
				}
				mm := 59
				if day == 0 && hh == h {
					mm = i
				}
				for ; mm >= 0; mm-- {
					if c.sets[0][mm] {
						if t := cronLocal(y, m, d, hh, mm); t <= ts {
							return t, true
						}
					}
				}
			}
		}
		if d--; d < 1 {
			if m--; m < 1 {
				m = 12
				y--
			}
			d = cronDaysIn(y, m)
		}
	}
	return 0, false
}

func (c *cronExpr) next(ts int64) (int64, bool) {
	y, m, d, h, i := cronFieldsOf(ts)
	for day := 0; day < 366*28; day++ {
		if c.dayMatches(y, m, d) {
			hh := 0
			if day == 0 {
				hh = h
			}
			for ; hh <= 23; hh++ {
				if !c.sets[1][hh] {
					continue
				}
				mm := 0
				if day == 0 && hh == h {
					mm = i + 1
				}
				for ; mm <= 59; mm++ {
					if c.sets[0][mm] {
						if t := cronLocal(y, m, d, hh, mm); t > ts {
							return t, true
						}
					}
				}
			}
		}
		if d++; d > cronDaysIn(y, m) {
			d = 1
			if m++; m > 12 {
				m = 1
				y++
			}
		}
	}
	return 0, false
}

func (c *cronExpr) upcoming(ts int64, n int) []int64 {
	out := []int64{}
	for len(out) < n {
		t, ok := c.next(ts)
		if !ok {
			break
		}
		out = append(out, t)
		ts = t
	}
	return out
}
