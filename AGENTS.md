# AGENTS.md - Totally

`totally` is a **faithful, normalized, cross-provider data source** for local
agent-session transcripts, with a CLI as a convenience skin over that data. Its
job is to let a developer or team extract exactly the usage, cost, activity, and
project-context data *they* need — and slice it themselves — rather than
pre-building every possible analysis. It must stay useful interactively in a
terminal and, more importantly, be a stable JSON/CSV data source for scripts.

## What totally is (and is not)

- **A data source, not an analytics/advisor tool.** We emit clean, faithful,
  normalized rows; the user (or `jq`/`duckdb`/sqlite/a spreadsheet) does the
  analysis. We do not recommend actions or interpret trends for the user. The
  `stats` command is a convenience skin over the underlying data, not the
  product.
- **Faithful first.** The accuracy that earns trust is *token fidelity* and
  *cross-provider normalization*, not invoice-matching dollar decimals. We never
  fabricate: usage we cannot attribute is reported as unavailable/partial, not
  zero.
- **Local, private, dependency-light.** A single static binary (no npm
  toolchain) that reads local transcripts and never phones home. This is a core
  part of the wedge.

## Target users

We serve two readers from one dataset — they are **not** mutually exclusive:

- **The comparer** wants trends and self-serve slicing ("which project cost more
  this month?", "which provider am I using more?"). Fine with honest `±` /
  `unavailable`.
- **The accountant** wants the total to match the invoice. Needs a single
  reconciling number.

We serve both by exposing uncertainty as **structured data** — `known` vs
`estimated`, explicit `lower`/`upper` bounds, and a `partial` flag — and letting
each consumer collapse it however their job demands. Uncertainty is a
first-class part of the export contract, not a terminal-rendering detail.

## The accuracy goal

`totally` aims to be the **most accurate agent-cost tool**, while being honest
about *irreducible* uncertainty. That is a comparative, falsifiable claim, so it
carries obligations:

- **"Irreducible" must be earned per-uncertainty.** Distinguish uncertainty that
  is genuinely irreducible (the transcript does not contain the bit) from
  reducible-but-unbuilt gaps (e.g. unpriced non-default service tiers, whose
  rates are published). Do not launder unbuilt scope as physics.
- **Validate up a hierarchy of evidence.** Internal tests and an analytical
  error budget certify only *modeled* uncertainty. A `ccusage` cross-check is a
  cheap differential tripwire (belongs in CI) but shares transcript/rate blind
  spots, so it cannot certify correctness. Only **reconciliation against a real
  invoice** — even once, by hand — proves the known residual is the whole
  residual. The reconciliation residual, after pricing everything priceable, is
  the quantified irreducible uncertainty.

## Core capabilities

Roughly ordered by how much they constrain the product:

1. **Extract & export usage data** — faithful, normalized rows at a grain fine
   enough (per-request usage segments) that consumers can reconstruct slices we
   did not pre-build. The exported schema is a **versioned, documented contract**
   we do not break silently.
2. **Aggregate & compare** — convenience aggregation (`stats --by ...`) over the
   underlying data, for the common comparer questions.
3. **Inspect one session** — metadata, transcript location, token breakdown,
   cost, activity, duration, pricing.
4. **Find & browse sessions** — narrow by project, time, model, provider.
5. **Inspect & configure pricing** — traceable, auditable rates; user overrides;
   a (planned, possibly interactive) `config` path to make overrides easy.
6. **Diagnose raw transcript discovery/storage** — for anomalies in storage.

Features that are antithetical or tangential to the above — especially anything
that turns totally into an *advisor* rather than a *data source* — should be
flagged before implementation.

## Current priorities

The V0 tool is usable but unproven. In priority order:

1. **Second provider (Claude).** The single most important gap. totally is a
   comparison tool that cannot compare providers (OpenAI/Codex only) — running
   inside Claude Code, whose transcripts it cannot read. Adding Claude also
   *tests the normalization schema* before we freeze it, and unlocks the
   `ccusage` cross-check.
2. **Harden the export schema into a versioned contract** at per-request grain —
   only *after* a second provider has deformed it, so the contract survives
   first contact.
3. **CLI polish.**
4. **Pricing-catalog hardening → maintenance, not headline.** Catalog upkeep is
   a treadmill (each model launch needs rates); user overrides and eventual
   community maintenance mitigate it, but it is no longer the main event.
