# AGENTS.md - Totally

`totally` is a local agent-usage analysis tool. Its core job is to help a
developer or team find sessions and understand their token use, estimated cost,
activity, duration, model/provider use, and project context. It should remain
useful interactively in a terminal and as a stable JSON data source for scripts.

## Core User Flows

The core user flows, roughly ordered by how much they constrain the product, are
as follows:

1. Aggregate and compare AI usage
  - A developer or team can generate useful statistics about usage and cost
    broken down across meaningdul dimensions.
  - Users should be able to answer, for example:
    - "How much did I spend on AI this week?"
    - "Which project did I spend the most resources on this month?"
    - "Which model provider have I used more tokens with?"
  - This information should be exportable in a variety of useful formats.

2. Inspect one session
  - Given a session ID, a developer can understand its its metadata, transcript
    location, token breakdown, cost, activity, duration, and pricing.
  - This information should be exportable in a variety of useful formats.

3. Find and browse sessions
  - A developer can narrow sessions by project, time, model, and provider.
  - This information should be exportable in a variety of useful formats.

4. Inspect and configure model pricing
  - A developer can inspect model prices per token/credit.
  - Users should be able to configure this information as it can change.

5. Diagnose raw transcript discovery/storage
  - When necessary, users should be able to diagnose issues and anomalies in
    their transcript storage.

Additional features, such as tool config or rendering, should be in service of
the above flows. Features that are antithetical or tangential to these flows
should be flagged before they are implemented.
