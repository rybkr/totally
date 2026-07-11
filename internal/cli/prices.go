package cli

import (
	"encoding/json"
	"fmt"
	"io"
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
	return cmd
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
