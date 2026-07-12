package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/rybkr/totally/internal/pricing"
	"github.com/spf13/cobra"
)

type pricesOptions struct{ model string }

func newPricesCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	var opts pricesOptions
	cmd := &cobra.Command{
		Use: "prices", Short: "Show model pricing assumptions and rates", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runPrices(stdout, *globals, opts) },
	}
	cmd.Flags().StringVar(&opts.model, "model", "", "limit prices to a model")
	cmd.AddCommand(newPricesVerifyCommand(stdout, globals))
	return cmd
}

type pricesVerifyReport struct {
	Valid          bool                `json:"valid"`
	Config         string              `json:"config,omitempty"`
	CatalogVersion string              `json:"catalog_version"`
	Overrides      int                 `json:"overrides"`
	Issues         []pricesVerifyIssue `json:"issues,omitempty"`
}

type pricesVerifyIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func newPricesVerifyCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Validate configured pricing overrides",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPricesVerify(cmd, stdout, *globals)
		},
	}
}

func runPricesVerify(cmd *cobra.Command, stdout io.Writer, globals globalOptions) error {
	report := pricesVerifyReport{Valid: true, Config: globals.config, CatalogVersion: pricing.CatalogVersion}
	catalog := globals.prices
	if globals.priceConfigErr != nil {
		report.Valid = false
		report.Issues = append(report.Issues, pricesVerifyIssue{Path: globals.config, Message: globals.priceConfigErr.Error()})
	} else {
		keys := make([]string, 0, len(globals.priceConfig))
		for key := range globals.priceConfig {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := globals.priceConfig[key]
			report.Overrides++
			path := `prices."` + key + `"`
			parts := strings.Split(key, "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				report.Valid = false
				report.Issues = append(report.Issues, pricesVerifyIssue{Path: path, Message: "expected provider/model"})
				continue
			}
			fields, ok := value.(map[string]any)
			if !ok {
				report.Valid = false
				report.Issues = append(report.Issues, pricesVerifyIssue{Path: path, Message: "must be a pricing table"})
				continue
			}
			rate, replace, issues := decodeConfiguredRate(parts[0], parts[1], fields, path)
			report.Issues = append(report.Issues, issues...)
			rateIssues := pricing.ValidateRate(rate)
			for _, issue := range rateIssues {
				report.Issues = append(report.Issues, pricesVerifyIssue{Path: path + "." + issue.Field, Message: issue.Message})
			}
			if len(issues) == 0 && len(rateIssues) == 0 {
				var err error
				if replace {
					err = catalog.Override(rate)
				} else {
					err = catalog.Overlay(rate)
				}
				if err != nil {
					report.Issues = append(report.Issues, pricesVerifyIssue{Path: path, Message: err.Error()})
				}
			}
		}
		if len(report.Issues) == 0 {
			if err := catalog.Validate(); err != nil {
				report.Issues = append(report.Issues, pricesVerifyIssue{Path: "catalog", Message: err.Error()})
			}
		}
		if len(report.Issues) > 0 {
			report.Valid = false
		}
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Path == report.Issues[j].Path {
			return report.Issues[i].Message < report.Issues[j].Message
		}
		return report.Issues[i].Path < report.Issues[j].Path
	})
	if globals.format == outputFormatJSON {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			return err
		}
		if !report.Valid {
			return fmt.Errorf("pricing configuration is invalid")
		}
		return nil
	}
	if report.Valid {
		_, err := fmt.Fprintf(stdout, "Pricing configuration is valid.\n\n  config: %s\n  overrides checked: %d\n  bundled catalog: %s\n", priceConfigLabel(report.Config), report.Overrides, report.CatalogVersion)
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Pricing configuration is invalid (%d %s).\n\n  config: %s\n  overrides checked: %d\n  bundled catalog: %s\n\n", len(report.Issues), priceErrorLabel(len(report.Issues)), priceConfigLabel(report.Config), report.Overrides, report.CatalogVersion); err != nil {
		return err
	}
	for _, issue := range report.Issues {
		if _, err := fmt.Fprintf(stdout, "  %s\n    %s\n\n", issue.Path, issue.Message); err != nil {
			return err
		}
	}
	return fmt.Errorf("pricing configuration is invalid")
}

func priceConfigLabel(config string) string {
	if config == "" {
		return "built-in defaults (no config file)"
	}
	return config
}

func priceErrorLabel(count int) string {
	if count == 1 {
		return "error"
	}
	return "errors"
}

func decodeConfiguredRate(provider, model string, fields map[string]any, path string) (pricing.Rate, bool, []pricesVerifyIssue) {
	rate := pricing.Rate{Provider: provider, Model: model}
	var replace bool
	var issues []pricesVerifyIssue
	var longContext *pricing.LongContextRule
	legacyLongContext := func() *pricing.LongContextRule {
		if longContext == nil {
			longContext = &pricing.LongContextRule{Type: "long_context"}
		}
		return longContext
	}
	values := map[string]*string{
		"input_per_million_usd": &rate.InputPerMillionUSD, "cached_input_per_million_usd": &rate.CachedInputPerMillionUSD, "output_per_million_usd": &rate.OutputPerMillionUSD,
		"source": &rate.Source, "effective_from": &rate.EffectiveFrom, "effective_until": &rate.EffectiveUntil,
		"cache_write_input_scale": &rate.CacheWriteInputScale, "cache_write_per_million_usd": &rate.CacheWritePerMillionUSD,
	}
	for name, value := range fields {
		if name == "replace" {
			parsed, ok := value.(bool)
			if !ok {
				issues = append(issues, pricesVerifyIssue{Path: path + ".replace", Message: "must be a boolean"})
			} else {
				replace = parsed
			}
			continue
		}
		if target, ok := values[name]; ok {
			text, ok := value.(string)
			if !ok {
				issues = append(issues, pricesVerifyIssue{Path: path + "." + name, Message: "must be a string"})
				continue
			}
			*target = text
			continue
		}
		if name == "long_context_input_scale" || name == "long_context_cached_input_scale" || name == "long_context_output_scale" {
			text, ok := value.(string)
			if !ok {
				issues = append(issues, pricesVerifyIssue{Path: path + "." + name, Message: "must be a string"})
				continue
			}
			rule := legacyLongContext()
			switch name {
			case "long_context_input_scale":
				rule.InputScale = text
			case "long_context_cached_input_scale":
				rule.CachedInputScale = text
			case "long_context_output_scale":
				rule.OutputScale = text
			}
			continue
		}
		if name == "long_context_threshold" {
			value, ok := value.(int64)
			if !ok || value < 0 {
				issues = append(issues, pricesVerifyIssue{Path: path + "." + name, Message: "must be a non-negative integer"})
			} else {
				legacyLongContext().ThresholdTokens = value
			}
			continue
		}
		issues = append(issues, pricesVerifyIssue{Path: path + "." + name, Message: "unknown pricing field"})
	}
	if longContext != nil {
		rate.Rules = append(rate.Rules, *longContext)
	}
	return rate, replace, issues
}

func runPrices(stdout io.Writer, globals globalOptions, opts pricesOptions) error {
	rates := globals.prices.Rates()
	if opts.model != "" {
		filtered := rates[:0]
		for _, rate := range rates {
			if rate.Model == opts.model {
				filtered = append(filtered, rate)
			}
		}
		rates = filtered
	}
	switch globals.format {
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(struct {
			Version string         `json:"version"`
			Rates   []pricing.Rate `json:"rates"`
		}{pricing.CatalogVersion, rates})
	case outputFormatTable:
		if _, err := fmt.Fprintln(stdout, "PROVIDER\tMODEL\tINPUT / 1M\tCACHED / 1M\tOUTPUT / 1M\tEFFECTIVE"); err != nil {
			return err
		}
		for _, rate := range rates {
			values := []string{rate.Provider, rate.Model, "$" + rate.InputPerMillionUSD, "$" + rate.CachedInputPerMillionUSD, "$" + rate.OutputPerMillionUSD, rate.EffectiveFrom}
			if _, err := fmt.Fprintln(stdout, strings.Join(values, "\t")); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
}
