// Copyright 2016 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	gotime "time"

	"github.com/juju/errors"
)

type mysqlTime struct {
	year        uint16 // year <= 9999
	month       uint8  // month <= 12
	day         uint8  // day <= 31
	hour        uint8  // hour <= 23
	minute      uint8  // minute <= 59
	second      uint8  // second <= 59
	microsecond uint32
}

func (t mysqlTime) Year() int {
	return int(t.year)
}

func (t mysqlTime) Month() int {
	return int(t.month)
}

func (t mysqlTime) Day() int {
	return int(t.day)
}

func (t mysqlTime) Hour() int {
	return int(t.hour)
}

func (t mysqlTime) Minute() int {
	return int(t.minute)
}

func (t mysqlTime) Second() int {
	return int(t.second)
}

func (t mysqlTime) Microsecond() int {
	return int(t.microsecond)
}

func (t mysqlTime) Weekday() gotime.Weekday {
	// TODO: Consider time_zone variable.
	t1, err := t.GoTime(gotime.Local)
	if err != nil {
		return 0
	}
	return t1.Weekday()
}

func (t mysqlTime) YearDay() int {
	if t.month == 0 || t.day == 0 {
		return 0
	}
	return calcDaynr(int(t.year), int(t.month), int(t.day)) -
		calcDaynr(int(t.year), 1, 1) + 1
}

func (t mysqlTime) YearWeek(mode int) (int, int) {
	behavior := weekMode(mode) | weekBehaviourYear
	return calcWeek(&t, behavior)
}

func (t mysqlTime) Week(mode int) int {
	if t.month == 0 || t.day == 0 {
		return 0
	}
	_, week := calcWeek(&t, weekMode(mode))
	return week
}

func (t mysqlTime) GoTime(loc *gotime.Location) (gotime.Time, error) {
	// gotime.Time can't represent month 0 or day 0, date contains 0 would be converted to a nearest date,
	// For example, 2006-12-00 00:00:00 would become 2015-11-30 23:59:59.
	tm := gotime.Date(t.Year(), gotime.Month(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Microsecond()*1000, loc)
	year, month, day := tm.Date()
	hour, minute, second := tm.Clock()
	microsec := tm.Nanosecond() / 1000
	// This function will check the result, and return an error if it's not the same with the origin input.
	if year != t.Year() || int(month) != t.Month() || day != t.Day() ||
		hour != t.Hour() || minute != t.Minute() || second != t.Second() ||
		microsec != t.Microsecond() {
		return tm, errors.Trace(ErrInvalidTimeFormat)
	}
	return tm, nil
}

func newMysqlTime(year, month, day, hour, minute, second, microsecond int) mysqlTime {
	return mysqlTime{
		uint16(year),
		uint8(month),
		uint8(day),
		uint8(hour),
		uint8(minute),
		uint8(second),
		uint32(microsecond),
	}
}

func calcTimeFromSec(to *mysqlTime, seconds, microseconds int) {
	to.hour = uint8(seconds / 3600)
	seconds = seconds % 3600
	to.minute = uint8(seconds / 60)
	to.second = uint8(seconds % 60)
	to.microsecond = uint32(microseconds)
}

const secondsIn24Hour = 86400

// calcTimeDiff calculates difference between two datetime values as seconds + microseconds.
// t1 and t2 should be TIME/DATE/DATETIME value.
// sign can be +1 or -1, and t2 is preprocessed with sign first.
func calcTimeDiff(t1, t2 TimeInternal, sign int) (seconds, microseconds int, neg bool) {
	days := calcDaynr(t1.Year(), t1.Month(), t1.Day())
	days -= sign * calcDaynr(t2.Year(), t2.Month(), t2.Day())

	tmp := (int64(days)*secondsIn24Hour+
		int64(t1.Hour())*3600+int64(t1.Minute())*60+
		int64(t1.Second())-
		int64(sign)*(int64(t2.Hour())*3600+int64(t2.Minute())*60+
			int64(t2.Second())))*
		1e6 +
		int64(t1.Microsecond()) - int64(sign)*int64(t2.Microsecond())

	if tmp < 0 {
		tmp = -tmp
		neg = true
	}
	seconds = int(tmp / 1e6)
	microseconds = int(tmp % 1e6)
	return
}

// datetimeToUint64 converts time value to integer in YYYYMMDDHHMMSS format.
func datetimeToUint64(t TimeInternal) uint64 {
	return dateToUint64(t)*1e6 + timeToUint64(t)
}

// dateToUint64 converts time value to integer in YYYYMMDD format.
func dateToUint64(t TimeInternal) uint64 {
	return (uint64)(uint64(t.Year())*10000 +
		uint64(t.Month())*100 +
		uint64(t.Day()))
}

// timeToUint64 converts time value to integer in HHMMSS format.
func timeToUint64(t TimeInternal) uint64 {
	return uint64(uint64(t.Hour())*10000 +
		uint64(t.Minute())*100 +
		uint64(t.Second()))
}

// calcDaynr calculates days since 0000-00-00.
func calcDaynr(year, month, day int) int {
	if year == 0 && month == 0 {
		return 0
	}

	delsum := 365*year + 31*(month-1) + day
	if month <= 2 {
		year--
	} else {
		delsum -= (month*4 + 23) / 10
	}
	temp := ((year/100 + 1) * 3) / 4
	return delsum + year/4 - temp
}

// calcDaysInYear calculates days in one year, it works with 0 <= year <= 99.
func calcDaysInYear(year int) int {
	if (year&3) == 0 && (year%100 != 0 || (year%400 == 0 && (year != 0))) {
		return 366
	}
	return 365
}

// calcWeekday calculates weekday from daynr, returns 0 for Monday, 1 for Tuesday ...
func calcWeekday(daynr int, sundayFirstDayOfWeek bool) int {
	daynr += 5
	if sundayFirstDayOfWeek {
		daynr++
	}
	return daynr % 7
}

type weekBehaviour uint

const (
	// If set, Sunday is first day of week, otherwise Monday is first day of week.
	weekBehaviourMondayFirst weekBehaviour = 1 << iota
	// If set, Week is in range 1-53, otherwise Week is in range 0-53.
	// Note that this flag is only releveant if WEEK_JANUARY is not set.
	weekBehaviourYear
	// If not set, Weeks are numbered according to ISO 8601:1988.
	// If set, the week that contains the first 'first-day-of-week' is week 1.
	weekBehaviourFirstWeekday
)

func (v weekBehaviour) test(flag weekBehaviour) bool {
	return (v & flag) != 0
}

func weekMode(mode int) weekBehaviour {
	var weekFormat weekBehaviour
	weekFormat = weekBehaviour(mode & 7)
	if (weekFormat & weekBehaviourMondayFirst) == 0 {
		weekFormat ^= weekBehaviourFirstWeekday
	}
	return weekFormat
}

// calcWeek calculates week and year for the time.
func calcWeek(t *mysqlTime, wb weekBehaviour) (year int, week int) {
	var days int
	daynr := calcDaynr(int(t.year), int(t.month), int(t.day))
	firstDaynr := calcDaynr(int(t.year), 1, 1)
	mondayFirst := wb.test(weekBehaviourMondayFirst)
	weekYear := wb.test(weekBehaviourYear)
	firstWeekday := wb.test(weekBehaviourFirstWeekday)

	weekday := calcWeekday(int(firstDaynr), !mondayFirst)

	year = int(t.year)

	if t.month == 1 && int(t.day) <= 7-weekday {
		if !weekYear &&
			((firstWeekday && weekday != 0) || (!firstWeekday && weekday >= 4)) {
			week = 0
			return
		}
		weekYear = true
		(year)--
		days = calcDaysInYear(year)
		firstDaynr -= days
		weekday = (weekday + 53*7 - days) % 7
	}

	if (firstWeekday && weekday != 0) ||
		(!firstWeekday && weekday >= 4) {
		days = daynr - (firstDaynr + 7 - weekday)
	} else {
		days = daynr - (firstDaynr - weekday)
	}

	if weekYear && days >= 52*7 {
		weekday = (weekday + calcDaysInYear(year)) % 7
		if (!firstWeekday && weekday < 4) ||
			(firstWeekday && weekday == 0) {
			year++
			week = 1
			return
		}
	}
	week = days/7 + 1
	return
}

const (
	intervalYEAR        = "YEAR"
	intervalQUARTER     = "QUARTER"
	intervalMONTH       = "MONTH"
	intervalWEEK        = "WEEK"
	intervalDAY         = "DAY"
	intervalHOUR        = "HOUR"
	intervalMINUTE      = "MINUTE"
	intervalSECOND      = "SECOND"
	intervalMICROSECOND = "MICROSECOND"
)

func timestampDiff(intervalType string, t1 TimeInternal, t2 TimeInternal) int64 {
	seconds, microseconds, neg := calcTimeDiff(t2, t1, 1)
	months := uint(0)
	if intervalType == intervalYEAR || intervalType == intervalQUARTER ||
		intervalType == intervalMONTH {
		var (
			yearBeg, yearEnd, monthBeg, monthEnd, dayBeg, dayEnd uint
			secondBeg, secondEnd, microsecondBeg, microsecondEnd uint
		)

		if neg {
			yearBeg = uint(t2.Year())
			yearEnd = uint(t1.Year())
			monthBeg = uint(t2.Month())
			monthEnd = uint(t1.Month())
			dayBeg = uint(t2.Day())
			dayEnd = uint(t1.Day())
			secondBeg = uint(t2.Hour()*3600 + t2.Minute()*60 + t2.Second())
			secondEnd = uint(t1.Hour()*3600 + t1.Minute()*60 + t1.Second())
			microsecondBeg = uint(t2.Microsecond())
			microsecondEnd = uint(t1.Microsecond())
		} else {
			yearBeg = uint(t1.Year())
			yearEnd = uint(t2.Year())
			monthBeg = uint(t1.Month())
			monthEnd = uint(t2.Month())
			dayBeg = uint(t1.Day())
			dayEnd = uint(t2.Day())
			secondBeg = uint(t1.Hour()*3600 + t1.Minute()*60 + t1.Second())
			secondEnd = uint(t2.Hour()*3600 + t2.Minute()*60 + t2.Second())
			microsecondBeg = uint(t1.Microsecond())
			microsecondEnd = uint(t2.Microsecond())
		}

		// calc years
		years := yearEnd - yearBeg
		if monthEnd < monthBeg ||
			(monthEnd == monthBeg && dayEnd < dayBeg) {
			years--
		}

		// calc months
		months = 12 * years
		if monthEnd < monthBeg ||
			(monthEnd == monthBeg && dayEnd < dayBeg) {
			months += 12 - (monthBeg - monthEnd)
		} else {
			months += (monthEnd - monthBeg)
		}

		if dayEnd < dayBeg {
			months--
		} else if (dayEnd == dayBeg) &&
			((secondEnd < secondBeg) ||
				(secondEnd == secondBeg && microsecondEnd < microsecondBeg)) {
			months--
		}
	}

	negV := int64(1)
	if neg {
		negV = -1
	}
	switch intervalType {
	case intervalYEAR:
		return int64(months) / 12 * negV
	case intervalQUARTER:
		return int64(months) / 3 * negV
	case intervalMONTH:
		return int64(months) * negV
	case intervalWEEK:
		return int64(seconds) / secondsIn24Hour / 7 * negV
	case intervalDAY:
		return int64(seconds) / secondsIn24Hour * negV
	case intervalHOUR:
		return int64(seconds) / 3600 * negV
	case intervalMINUTE:
		return int64(seconds) / 60 * negV
	case intervalSECOND:
		return int64(seconds) * negV
	case intervalMICROSECOND:
		// In MySQL difference between any two valid datetime values
		// in microseconds fits into longlong.
		return int64(seconds*1000000+microseconds) * negV
	}

	return 0
}
