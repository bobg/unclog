package unclog

import (
	"errors"
	"regexp"
	"strconv"
	"time"
)

type Date struct {
	Y int
	M time.Month
	D int
}

var dateRegex = regexp.MustCompile(`(\d+)-(\d+)-(\d+)`)

var errDateParse = errors.New("bad dates")

// Parses dates of the form "yyyy-mm-dd."
func ParseDate(s string) (Date, error) {
	m := dateRegex.FindStringSubmatch(s)
	if len(m) != 4 {
		return Date{}, errDateParse
	}
	y, err := strconv.Atoi(m[1])
	if err != nil {
		return Date{}, errDateParse
	}
	mon, err := strconv.Atoi(m[2])
	if err != nil {
		return Date{}, errDateParse
	}
	d, err := strconv.Atoi(m[3])
	if err != nil {
		return Date{}, errDateParse
	}
	if y <= 0 {
		return Date{}, errDateParse
	}
	if mon < 1 || mon > 12 {
		return Date{}, errDateParse
	}
	if d < 1 || d > daysInMonth(y, time.Month(mon)) {
		return Date{}, errDateParse
	}
	return Date{Y: y, M: time.Month(mon), D: d}, nil
}

func daysInMonth(y int, m time.Month) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	}
	if isLeapYear(y) {
		return 29
	}
	return 28
}

func isLeapYear(y int) bool {
	if y%400 == 0 {
		return true
	}
	if y%100 == 0 {
		return false
	}
	return y%4 == 0
}

func nextDate(d Date) Date {
	if d.D == daysInMonth(d.Y, d.M) {
		d.D = 1
		if d.M == 12 {
			d.M = 1
			d.Y++
		} else {
			d.M++
		}
	} else {
		d.D++
	}
	return d
}
