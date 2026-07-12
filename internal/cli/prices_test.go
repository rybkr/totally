package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/totally/internal/pricing"
)

func TestPricesCommandPrintsBuiltInRates(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"prices", "--model", "gpt-5"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "openai\tgpt-5\t$1.25\t$0.125\t$10.00") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestPricesCommandLoadsConfigOverride(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.toml")
	contents := `[prices."openai/gpt-5"]
input_per_million_usd = "2"
cached_input_per_million_usd = "0.2"
output_per_million_usd = "12"
effective_from = "2026-01-01"
source = "user"
`
	if err := os.WriteFile(config, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"prices", "--model", "gpt-5", "--config", config, "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Rates []pricing.Rate `json:"rates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Rates) != 1 || result.Rates[0].InputPerMillionUSD != "2" || result.Rates[0].Source != "user" {
		t.Fatalf("unexpected rates: %+v", result.Rates)
	}
}

func TestPricesVerifyReportsAllConfigIssues(t *testing.T) {
	config := writeConfig(t, `[prices."openai/gpt-5"]
input_per_million_usd = "1"
cached_input_per_million_usd = "0.1"
output_per_million_usd = "nope"
effective_from = "2026-01-01"
unknown = "value"
`)
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"prices", "verify", "--config", config, "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected verification failure")
	}
	var result pricesVerifyReport
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Valid || len(result.Issues) != 2 {
		t.Fatalf("unexpected verification report: %+v", result)
	}
}

func TestPricesVerifyPrintsDiagnosticOutput(t *testing.T) {
	config := writeConfig(t, `[prices."openai/gpt-5"]
input_per_million_usd = "invalid"
cached_input_per_million_usd = "0.1"
output_per_million_usd = "12"
effective_from = "2026-01-01"
`)
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"prices", "verify", "--config", config})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected verification failure")
	}
	output := stdout.String()
	for _, want := range []string{"Pricing configuration is invalid (1 error).", "config: " + config, `prices."openai/gpt-5".input_per_million_usd`} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in:\n%s", want, output)
		}
	}
}

func TestPricesVerifyRequiresEffectiveFrom(t *testing.T) {
	config := writeConfig(t, `[prices."openai/gpt-5"]
input_per_million_usd = "1"
cached_input_per_million_usd = "0.1"
output_per_million_usd = "12"
`)
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"prices", "verify", "--config", config, "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected verification failure")
	}
	var result pricesVerifyReport
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Path != `prices."openai/gpt-5".effective_from` {
		t.Fatalf("unexpected verification report: %+v", result)
	}
}
