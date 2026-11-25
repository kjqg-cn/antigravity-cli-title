package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

func isDarwinWeekday(s string) bool {
	switch s {
	case "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun":
		return true
	}
	return false
}

func isDarwinChineseWeekday(s string) bool {
	switch s {
	case "一", "二", "三", "四", "五", "六", "日",
		"周一", "周二", "周三", "周四", "周五", "周六", "周日",
		"星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日":
		return true
	}
	return false
}

func isDarwinMonth(s string) bool {
	switch s {
	case "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec":
		return true
	}
	return false
}

func parseDarwinTime(lstartStr string) (int64, bool) {
	if t, err := time.Parse("Mon Jan _2 15:04:05 2006", lstartStr); err == nil {
		return t.UnixNano(), true
	}
	if t, err := time.Parse("Mon Jan 2 15:04:05 2006", lstartStr); err == nil {
		return t.UnixNano(), true
	}
	if m := darwinChineseTime.FindStringSubmatch(lstartStr); m != nil {
		normalized := m[4] + "-" + m[1] + "-" + m[2] + " " + m[3]
		if t, err := time.Parse("2006-1-2 15:04:05", normalized); err == nil {
			return t.UnixNano(), true
		}
	}
	return 0, false
}

var darwinChineseTime = regexp.MustCompile(`^\S+\s+(\d{1,2})月?/\s*(\d{1,2})\s+(\d{2}:\d{2}:\d{2})\s+(\d{4})$`)

// parseDarwinPsLine parses a single line from:
// ps -A -o pid=,ppid=,comm=,lstart=,command=
func parseDarwinPsLine(line string) (pid int, ppid int, comm string, ticks int64, cmdline string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	fields := strings.Fields(line)
	if len(fields) < 7 { // pid, ppid, comm, at least 4 tokens for lstart...
		return
	}
	var err error
	pid, err = strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return
	}
	ppid, err = strconv.Atoi(fields[1])
	if err != nil {
		return
	}

	// Find the lstart tokens:
	// Format 1: standard 5 tokens (Weekday Month Day Time Year)
	// Format 2: localized 4 tokens (e.g. zh_CN Weekday Month/Day Time Year)
	lstartIdx := -1
	lstartLen := 5

	// First attempt: match known English weekday and month (5 tokens)
	for i := 2; i <= len(fields)-5; i++ {
		if isDarwinWeekday(fields[i]) && isDarwinMonth(fields[i+1]) {
			timeField := fields[i+3]
			yearField := fields[i+4]
			if strings.Count(timeField, ":") >= 1 && len(yearField) == 4 {
				if _, yErr := strconv.Atoi(yearField); yErr == nil {
					lstartIdx = i
					lstartLen = 5
					break
				}
			}
		}
	}

	// Second attempt: match Chinese date format (%a %b/%e %T %Y)
	// Can be 4 tokens: [Weekday, Month/Day, Time, Year] (e.g. 四 10月/30 01:45:12 2026)
	// Or 5 tokens: [Weekday, Month/, Day, Time, Year] (single-digit day space-padded: 四 10月/ 5 01:45:12 2026)
	if lstartIdx == -1 {
		for i := 2; i <= len(fields)-4; i++ {
			if isDarwinChineseWeekday(fields[i]) && (strings.Contains(fields[i+1], "月") || strings.Contains(fields[i+1], "/")) {
				// 4-token check
				timeField := fields[i+2]
				yearField := fields[i+3]
				if strings.Count(timeField, ":") >= 1 && len(yearField) == 4 {
					if _, yErr := strconv.Atoi(yearField); yErr == nil {
						lstartIdx = i
						lstartLen = 4
						break
					}
				}
				// 5-token check (if len allows)
				if i <= len(fields)-5 {
					dayNum, dayErr := strconv.Atoi(fields[i+2])
					if dayErr == nil && dayNum >= 1 && dayNum <= 31 {
						time5 := fields[i+3]
						year5 := fields[i+4]
						if strings.Count(time5, ":") >= 1 && len(year5) == 4 {
							if _, y5Err := strconv.Atoi(year5); y5Err == nil {
								lstartIdx = i
								lstartLen = 5
								break
							}
						}
					}
				}
			}
		}
	}

	// Third attempt: 5 tokens with arbitrary locale (day number 1..31 at i+2, time at i+3, 4-digit year at i+4)
	if lstartIdx == -1 {
		for i := 2; i <= len(fields)-5; i++ {
			dayNum, dayErr := strconv.Atoi(fields[i+2])
			if dayErr == nil && dayNum >= 1 && dayNum <= 31 {
				timeField := fields[i+3]
				yearField := fields[i+4]
				if strings.Count(timeField, ":") >= 1 && len(yearField) == 4 {
					if _, yErr := strconv.Atoi(yearField); yErr == nil {
						lstartIdx = i
						lstartLen = 5
						break
					}
				}
			}
		}
	}

	if lstartIdx != -1 {
		comm = strings.Join(fields[2:lstartIdx], " ")
		lstartStr := strings.Join(fields[lstartIdx:lstartIdx+lstartLen], " ")
		if nano, okParse := parseDarwinTime(lstartStr); okParse {
			ticks = nano
		} else {
			ticks = int64(pid)
		}
		if len(fields) > lstartIdx+lstartLen {
			cmdline = strings.Join(fields[lstartIdx+lstartLen:], " ")
		}
		ok = true
		return
	}

	// Fallback if lstart pattern didn't match: assume comm is 1 token, lstart is next 5 tokens
	if len(fields) >= 8 {
		comm = fields[2]
		lstartStr := strings.Join(fields[3:8], " ")
		if nano, okParse := parseDarwinTime(lstartStr); okParse {
			ticks = nano
		} else {
			ticks = int64(pid)
		}
		if len(fields) > 8 {
			cmdline = strings.Join(fields[8:], " ")
		}
		ok = true
		return
	}
	return
}
