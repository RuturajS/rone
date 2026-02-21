/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr represents a parsed 5-field cron expression.
// Fields: minute, hour, day-of-month, month, day-of-week
type CronExpr struct {
	Minute     []int // 0-59
	Hour       []int // 0-23
	DayOfMonth []int // 1-31
	Month      []int // 1-12
	DayOfWeek  []int // 0-6 (0=Sunday)
}

// ParseCron parses a standard 5-field cron expression string.
func ParseCron(expr string) (*CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}

	return &CronExpr{
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dom,
		Month:      month,
		DayOfWeek:  dow,
	}, nil
}

// Next calculates the next occurrence after the given time.
func (c *CronExpr) Next(from time.Time) time.Time {
	t := from.UTC().Truncate(time.Minute).Add(time.Minute)

	// Iterate up to 4 years to find the next match
	limit := t.Add(4 * 365 * 24 * time.Hour)
	for t.Before(limit) {
		if !contains(c.Month, int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			continue
		}
		if !contains(c.DayOfMonth, t.Day()) || !contains(c.DayOfWeek, int(t.Weekday())) {
			t = t.Add(24 * time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			continue
		}
		if !contains(c.Hour, t.Hour()) {
			t = t.Add(time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
			continue
		}
		if !contains(c.Minute, t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}

	// Fallback: should not happen with valid expressions
	return from.Add(time.Hour)
}

// parseField parses a single cron field (supports *, N, N-M, */N, N-M/N, comma lists).
func parseField(field string, min, max int) ([]int, error) {
	var result []int

	parts := strings.Split(field, ",")
	for _, part := range parts {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}

	return result, nil
}

func parsePart(part string, min, max int) ([]int, error) {
	// Handle step: */N or N-M/N
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return nil, fmt.Errorf("invalid step: %s", part)
		}
		step = s
		part = part[:idx]
	}

	// Handle wildcard
	if part == "*" {
		var vals []int
		for i := min; i <= max; i += step {
			vals = append(vals, i)
		}
		return vals, nil
	}

	// Handle range: N-M
	if idx := strings.Index(part, "-"); idx != -1 {
		lo, err := strconv.Atoi(part[:idx])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", part)
		}
		hi, err := strconv.Atoi(part[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", part)
		}
		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("range out of bounds: %d-%d", lo, hi)
		}
		var vals []int
		for i := lo; i <= hi; i += step {
			vals = append(vals, i)
		}
		return vals, nil
	}

	// Single value
	val, err := strconv.Atoi(part)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", part)
	}
	if val < min || val > max {
		return nil, fmt.Errorf("value out of bounds: %d", val)
	}
	return []int{val}, nil
}

func contains(set []int, val int) bool {
	for _, v := range set {
		if v == val {
			return true
		}
	}
	return false
}

