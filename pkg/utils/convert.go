package utils

import "strconv"

// Int64ToString converts int64 to string
func Int64ToString(n int64) string {
	return strconv.FormatInt(n, 10)
}

// IntToString converts int to string
func IntToString(n int) string {
	return strconv.Itoa(n)
}

// StringToInt64 converts string to int64
func StringToInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// StringToInt converts string to int
func StringToInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
