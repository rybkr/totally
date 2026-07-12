package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/pricing"
	"github.com/rybkr/totally/internal/provider/codex"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const allAgents = "all"

const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
)

type usageError struct {
	err error
}

func (err usageError) Error() string {
	return err.err.Error()
}

func (err usageError) Unwrap() error {
	return err.err
}

func ExitCode(err error) int {
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

type globalOptions struct {
	config   string
	agent    string
	homes    []string
	archived bool
	since    string
	until    string
	format   string
	noPager  bool
	prices   pricing.Catalog
}

type timeRange struct {
	since time.Time
	until time.Time
}

func addGlobalFlags(cmd *cobra.Command, opts *globalOptions) {
	cmd.PersistentFlags().StringVar(&opts.config, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&opts.agent, "agent", allAgents, "agent session format to discover: all, codex")
	cmd.PersistentFlags().StringArrayVar(&opts.homes, "home", nil, "agent home directory; may be repeated")
	cmd.PersistentFlags().BoolVar(&opts.archived, "archived", false, "include archived sessions")
	cmd.PersistentFlags().StringVar(&opts.since, "since", "", "include sessions at or after TIME (duration units: h, d, w, y; or YYYY-MM-DD/RFC3339)")
	cmd.PersistentFlags().StringVar(&opts.since, "after", "", "alias for --since")
	cmd.PersistentFlags().StringVar(&opts.until, "until", "", "include sessions at or before TIME (duration units: h, d, w, y; or YYYY-MM-DD/RFC3339)")
	cmd.PersistentFlags().StringVar(&opts.until, "before", "", "alias for --until")
	cmd.PersistentFlags().StringVar(&opts.format, "format", "table", "output format: table, json")
	cmd.PersistentFlags().BoolVar(&opts.noPager, "no-pager", false, "disable terminal paging")
}

func loadGlobalOptions(cmd *cobra.Command, opts *globalOptions) error {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("toml")
	v.SetEnvPrefix("TOTALLY")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	v.SetDefault("agent", allAgents)
	v.SetDefault("archived", false)
	v.SetDefault("format", "table")

	for _, key := range []string{"config", "agent", "home", "archived", "since", "until", "format"} {
		if err := v.BindPFlag(key, cmd.Root().PersistentFlags().Lookup(key)); err != nil {
			return err
		}
	}

	configPath := strings.TrimSpace(v.GetString("config"))
	if configPath != "" {
		configPath = expandHomePath(configPath)
		v.SetConfigFile(configPath)
	} else if configDir, err := defaultConfigDir(); err == nil {
		v.AddConfigPath(configDir)
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if configPath != "" || !errors.As(err, &notFound) {
			return err
		}
	}

	opts.config = configPath
	opts.agent = v.GetString("agent")
	opts.homes = normalizeHomeValues(v.GetStringSlice("home"))
	opts.archived = v.GetBool("archived")
	// The aliases share the flag variables with their canonical names. Prefer a
	// command-line value when either spelling was supplied; otherwise retain the
	// configured canonical value.
	flags := cmd.Root().PersistentFlags()
	if flags.Changed("since") || flags.Changed("after") {
		opts.since = flags.Lookup("since").Value.String()
	} else {
		opts.since = v.GetString("since")
	}
	if flags.Changed("until") || flags.Changed("before") {
		opts.until = flags.Lookup("until").Value.String()
	} else {
		opts.until = v.GetString("until")
	}
	opts.format = strings.TrimSpace(strings.ToLower(v.GetString("format")))
	opts.prices = pricing.DefaultCatalog()
	var overrides map[string]pricing.Rate
	if err := v.UnmarshalKey("prices", &overrides); err != nil {
		return fmt.Errorf("decode prices: %w", err)
	}
	for key, rate := range overrides {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid price key %q: expected provider/model", key)
		}
		rate.Provider, rate.Model = parts[0], parts[1]
		if err := opts.prices.Override(rate); err != nil {
			return err
		}
	}
	if err := validateOutputFormat(opts.format); err != nil {
		return err
	}

	return nil
}

func validateOutputFormat(format string) error {
	switch format {
	case outputFormatTable, outputFormatJSON:
		return nil
	default:
		return usageError{err: fmt.Errorf("unknown format %q", format)}
	}
}

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("could not resolve user home directory")
	}
	return filepath.Join(home, ".config", "totally"), nil
}

func normalizeHomeValues(values []string) []string {
	var homes []string
	for _, value := range values {
		if value == "" {
			continue
		}
		parts := filepath.SplitList(value)
		if len(parts) == 0 {
			homes = append(homes, expandHomePath(value))
			continue
		}
		for _, part := range parts {
			if part != "" {
				homes = append(homes, expandHomePath(part))
			}
		}
	}
	return homes
}

func expandHomePath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
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

func (opts globalOptions) parsers() ([]session.Parser, error) {
	switch strings.ToLower(opts.agent) {
	case "", allAgents:
		return []session.Parser{codex.NewParser()}, nil
	case string(codex.Source):
		return []session.Parser{codex.NewParser()}, nil
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
		return time.Time{}, fmt.Errorf("expected duration like 24h, 7d, 2w, or 1y; date like 2026-07-01; or RFC3339 timestamp")
	}
	if endOfDate {
		return t.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
	}
	return t, nil
}

func parseRelativeDuration(value string) (time.Duration, bool, error) {
	for suffix, multiplier := range map[string]time.Duration{
		"d": 24 * time.Hour,
		"w": 7 * 24 * time.Hour,
		"y": 365 * 24 * time.Hour,
	} {
		if strings.HasSuffix(value, suffix) {
			amount, err := strconv.Atoi(strings.TrimSuffix(value, suffix))
			if err != nil || amount < 0 {
				return 0, true, fmt.Errorf("invalid duration %q", value)
			}
			return time.Duration(amount) * multiplier, true, nil
		}
	}

	duration, err := time.ParseDuration(value)
	if err == nil {
		if duration < 0 {
			return 0, true, fmt.Errorf("invalid duration %q", value)
		}
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
