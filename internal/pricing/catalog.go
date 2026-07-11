package pricing

const openAIModelSourcePrefix = "https://developers.openai.com/api/docs/models/"

// defaultCatalogRates contains the bundled API-equivalent price schedule.
// Keep this data separate from estimation logic: model pricing changes over
// time and will eventually need multiple dated entries per model.
func defaultCatalogRates() []Rate {
	rate := func(model, input, cached, output, effectiveFrom string) Rate {
		return Rate{Provider: "openai", Model: model, InputPerMillionUSD: input, CachedInputPerMillionUSD: cached, OutputPerMillionUSD: output, Source: openAIModelSourcePrefix + model, EffectiveFrom: effectiveFrom}
	}
	longContext := func(model, input, cached, output, effectiveFrom string) Rate {
		r := rate(model, input, cached, output, effectiveFrom)
		r.LongContextThreshold, r.LongContextInputScale, r.LongContextOutputScale = 272_000, "2", "1.5"
		return r
	}
	cacheWrite := func(model, input, cached, output string) Rate {
		r := longContext(model, input, cached, output, "")
		r.CacheWriteInputScale = "1.25"
		return r
	}
	return []Rate{
		rate("codex-mini-latest", "1.50", "0.375", "6.00", ""),
		rate("gpt-4.1", "2.00", "0.50", "8.00", "2025-04-14"),
		rate("gpt-4.1-mini", "0.40", "0.10", "1.60", "2025-04-14"),
		rate("gpt-4.1-nano", "0.10", "0.025", "0.40", "2025-04-14"),
		rate("gpt-5", "1.25", "0.125", "10.00", "2025-08-07"),
		rate("gpt-5-mini", "0.25", "0.025", "2.00", "2025-08-07"),
		rate("gpt-5-nano", "0.05", "0.005", "0.40", "2025-08-07"),
		rate("gpt-5-codex", "1.25", "0.125", "10.00", ""),
		rate("gpt-5.1", "1.25", "0.125", "10.00", "2025-11-13"),
		rate("gpt-5.1-codex", "1.25", "0.125", "10.00", ""),
		rate("gpt-5.1-codex-max", "1.25", "0.125", "10.00", ""),
		rate("gpt-5.1-codex-mini", "0.25", "0.025", "2.00", ""),
		rate("gpt-5.2", "1.75", "0.175", "14.00", "2025-12-11"),
		rate("gpt-5.2-codex", "1.75", "0.175", "14.00", ""),
		rate("gpt-5.3-codex", "1.75", "0.175", "14.00", ""),
		longContext("gpt-5.4", "2.50", "0.25", "15.00", "2026-03-05"),
		longContext("gpt-5.5", "5.00", "0.50", "30.00", "2026-04-23"),
		cacheWrite("gpt-5.6-sol", "5.00", "0.50", "30.00"),
		cacheWrite("gpt-5.6-terra", "2.50", "0.25", "15.00"),
		cacheWrite("gpt-5.6-luna", "1.00", "0.10", "6.00"),
		rate("o3", "2.00", "0.50", "8.00", "2025-04-16"),
		rate("o4-mini", "1.10", "0.275", "4.40", ""),
	}
}
