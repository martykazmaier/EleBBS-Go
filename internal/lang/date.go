package lang

import (
	"fmt"
	"strings"
	"time"
)

var monthNames = [13]string{
	"", "Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
}

// FormatDate is Pascal RaFormatDate(D, 0, y2k_MsgDate, user DateFormat).
func FormatDate(d time.Time, userFormat byte, ral *File) string {
	if d.IsZero() {
		return ""
	}
	d = d.In(time.Local)
	format := int(userFormat)
	if format < 1 || format > 8 {
		format = 5
	}
	day := fmt.Sprintf("%02d", d.Day())
	month := fmt.Sprintf("%02d", int(d.Month()))
	years := d.Format("2006")
	if format <= 4 {
		years = d.Format("06")
	}
	mn := int(d.Month())
	if mn < 1 || mn > 12 {
		mn = 13
	}
	name := monthNames[mn]
	if ral != nil && (format == 4 || format == 8) && mn >= 1 && mn <= 12 {
		if s := strings.TrimSpace(ral.Get(Jan + mn - 1)); s != "" {
			name = s
		}
	}
	switch format {
	case 1:
		return day + "-" + month + "-" + years
	case 2:
		return month + "-" + day + "-" + years
	case 3:
		return years + "-" + month + "-" + day
	case 4:
		return day + "-" + name + "-" + years
	case 5:
		return day + "-" + month + "-" + years
	case 6:
		return month + "-" + day + "-" + years
	case 7:
		return years + "-" + month + "-" + day
	case 8:
		return day + "-" + name + "-" + years
	default:
		return day + "-" + month + "-" + years
	}
}
