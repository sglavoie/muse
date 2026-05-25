package library

import "time"

func formatDateAdded(createdAt int64) string {
	if createdAt <= 0 {
		return "--"
	}
	return time.Unix(createdAt, 0).Local().Format("2006-01-02")
}

func formatPlayedAt(ts int64) string {
	if ts <= 0 {
		return "--"
	}
	return time.Unix(ts, 0).Local().Format("2006-01-02 15:04")
}
