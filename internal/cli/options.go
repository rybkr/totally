package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/provider/codex"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

const allAgents = "all"

type globalOptions struct {
	agent    string
	homes    []string
	archived bool
	since    string
	until    string
	format   string
}

type timeRange struct {
	since time.Time
	until time.Time
}

func addGlobalFlags(cmd *cobra.Command, opts *globalOptions) {
	cmd.PersistentFlags().StringVar(&opts.agent, "agent", allAgents, "agent session format to discover: all, codex")
	cmd.PersistentFlags().StringArrayVar(&opts.homes, "home", nil, "agent home directory; may be repeated")
	cmd.PersistentFlags().BoolVar(&opts.archived, "archived", false, "include archived sessions")
	cmd.PersistentFlags().StringVar(&opts.since, "since", "", "include sessions at or after a duration or date")
	cmd.PersistentFlags().StringVar(&opts.until, "until", "", "include sessions at or before a duration or date")
	cmd.PersistentFlags().StringVar(&opts.format, "format", "table", "output format: table, json")
}

func (opts globalOptions) finders() ([]session.Finder, error) {
	switch strings.ToLower(opts.agent) {
	case "", allAgents:
		return []session.Finder{codex.NewFinder()}, nil
	case string(codex.Source):
		return []session.Finder{codex.NewFinder()}, nil
	default:
		return nil, fmt.Errorf("unknown agent %q", opts.agent)
	}
}

func (opts globalOptions) timeRange(now time.Time) (timeRange, error) {
	var result timeRange
	var err error

	if opts.since != "" {
		result.since, err = parseTimeBound(opts.since, now, false)
		if err != nil {
			return timeRange{}, fmt.Errorf("invalid --since: %w", err)
		}
	}
	if opts.until != "" {
		result.until, err = parseTimeBound(opts.until, now, true)
		if err != nil {
			return timeRange{}, fmt.Errorf("invalid --until: %w", err)
		}
	}
	if !result.since.IsZero() && !result.until.IsZero() && result.since.After(result.until) {
		return timeRange{}, fmt.Errorf("--since must be before --until")
	}

	return result, nil
}

func parseTimeBound(value string, now time.Time, endOfDate bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	if duration, ok, err := parseRelativeDuration(value); ok || err != nil {
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(-duration), nil
	}

	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}

	t, err := time.ParseInLocation(time.DateOnly, value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected duration like 7d, date like 2026-07-01, or RFC3339 timestamp")
	}
	if endOfDate {
		return t.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
	}
	return t, nil
}

func parseRelativeDuration(value string) (time.Duration, bool, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days < 0 {
			return 0, true, fmt.Errorf("invalid day duration %q", value)
		}
		return time.Duration(days) * 24 * time.Hour, true, nil
	}

	duration, err := time.ParseDuration(value)
	if err == nil {
		return duration, true, nil
	}

	last := value[len(value)-1]
	if last >= '0' && last <= '9' {
		return 0, false, nil
	}
	return 0, false, nil
}

func filterFilesByTimeRange(files []session.FileRef, bounds timeRange) []session.FileRef {
	if bounds.since.IsZero() && bounds.until.IsZero() {
		return files
	}

	filtered := files[:0]
	for _, file := range files {
		if file.CreatedAt.IsZero() {
			continue
		}
		if !bounds.since.IsZero() && file.CreatedAt.Before(bounds.since) {
			continue
		}
		if !bounds.until.IsZero() && file.CreatedAt.After(bounds.until) {
			continue
		}
		filtered = append(filtered, file)
	}
	return filtered
}
