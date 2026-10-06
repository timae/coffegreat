package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

func fStr(r *http.Request, k string) string { return strings.TrimSpace(r.FormValue(k)) }

func fStrMax(r *http.Request, k string, max int) string {
	s := fStr(r, k)
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

func fFloat(r *http.Request, k string) *float64 {
	s := strings.ReplaceAll(fStr(r, k), ",", ".")
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f > 100000 {
		return nil
	}
	return &f
}

func fInt(r *http.Request, k string) *int64 {
	s := fStr(r, k)
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil {
		i := int64(f + 0.5)
		if f < 0 {
			i = int64(f - 0.5)
		}
		return &i
	}
	return nil
}

func fRange(r *http.Request, k string, lo, hi int64) *int64 {
	v := fInt(r, k)
	if v == nil || *v < lo || *v > hi {
		return nil
	}
	return v
}

func fID(r *http.Request, k string) *int64 {
	v := fInt(r, k)
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}

func fBool(r *http.Request, k string) bool {
	switch fStr(r, k) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

func fDate(r *http.Request, k string) *time.Time {
	s := fStr(r, k)
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func fDateTime(r *http.Request, k string) time.Time {
	s := fStr(r, k)
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local); err == nil {
		return t
	}
	return time.Now()
}

// oneOf returns v if it's in the list (case-insensitive), else "".
func oneOf(v string, list []string) string {
	for _, o := range list {
		if strings.EqualFold(o, v) {
			return o
		}
	}
	return ""
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func queryID(r *http.Request, k string) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get(k), 10, 64)
	return id
}
