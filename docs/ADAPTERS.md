# Add an agent adapter in under an hour

Sekats now runs the same review pipeline for **poker** and an offline **summarizer**.
The latter is deliberately synthetic: 60 editorial decisions with source text,
proposed summaries, agent rationale, observed fixture outcomes, and linked mock
shadow reviews. It makes no LLM calls and publishes no documents.

```sh
SEKATS_ADAPTER=summarizer SEKATS_DB=off SEKATS_ATTEST=off go run ./cmd/server
# Existing behavior, including read-only live snapshot detection:
SEKATS_ADAPTER=poker go run ./cmd/server
```

`DATA_DIR` still defaults to `data`. Poker's existing verdict/promotion state stays
at its original paths. Other adapters persist verdicts, drafts, pattern evidence,
and attestation records under `DATA_DIR/state/<adapter-name>/`, so overlapping
decision IDs do not mix domains. To reset a demo, use a separate `DATA_DIR`.
The summarizer rejects an explicit live `SEKATS_DB` path and ignores poker's
auto-detected snapshot.

## Boundary before and after

Previously `internal/adapters.Adapter` owned policy lists, declarative rules,
pattern keys, and draft construction, but the server always loaded poker fixtures
and the poker SQLite schema. The policy bridge translated `Street`/`HandCode`,
and UI/report copy assumed hands, pots, and `poker-policy.json`.

Now the adapter also owns `LoadFixtures` and display `Labels`. The optional
`LiveAdapter` owns live loading and the default snapshot path. The shared store
accepts `phase`, `context_code`, `inputs`, `rationale`, and `outcome`; normalization
methods retain the old poker JSON/schema without changing existing fixtures.
The engine's legacy `Pot` field remains a compatibility metric, unused and hidden
for the summarizer. Rules, quality math, trends, queues, durable verdicts,
promotion thresholds, patches, and canonical attestation documents are shared.
Signing, anchoring, contracts, and the treasury are unchanged.

## Third-adapter checklist (about 45 minutes)

1. **10 min, interface:** Create `internal/adapters/<domain>/` with `New()` and
   implement `adapters.Adapter`:
   - `Name`, `Agent`, `PolicyFile`, `RulesFile`, `PolicyTarget`.
   - `LoadFixtures(dataDir) (*store.Store, error)`, translating your own JSON or
     log format. Use stable, unique nonzero decision IDs and 1:1 shadow links.
   - `Labels() store.Labels`: context/stage labels and an optional numeric metric
     label. Leave `Metric` empty when there is no meaningful numeric value.
   - `LoadPolicy(path)`: return `engine.NewPolicyDoc(version, raw, lists)`.
   - `PatternKey(engine.Review)`: group genuinely equivalent rejected cases.
   - `NewDraft(key, evidence)`: create a pending draft, stable URL-safe ID,
     target, payload, and copied evidence. Evidence is nonempty and sorted.
2. **15 min, fixtures:** Add a domain policy, rules, and decision dataset under
   `data/`. Decisions should include inputs, action, rationale, and outcome.
   Return shared `store.DecisionsExport` / `QualityStats`; derive quality counts
   from linked reviews rather than hard-coding dashboard numbers.
   Mark mock data with `Store.Synthetic=true`. Keep real user content and secrets
   out of public fixtures. `tools/generate_summarizer_fixture.py` is an offline,
   deterministic example.
3. **5 min, declarative rules:** Use `name`, optional `phase`/`actions`, a
   `list_ref` resolved by `LoadPolicy`, and `detail` with `{{code}}`/`{{action}}`.
   These are *violation* rules: a matching action and policy-list membership flag
   a decision. The target must point to an existing array in the policy JSON.
4. **5 min, registration:** Import the package and add
   `adapters.Register(domain.New())` in `cmd/server.init`. Select it with
   `SEKATS_ADAPTER=<name>`. No handler or template fork is required.
   Only implement `adapters.LiveAdapter` if you have your own read-only loader.
5. **10 min, proof:** Add the adapter to `TestSharedAdapterPipeline` in
   `cmd/server/main_test.go`, then run:
   ```sh
   go build ./... && go vet ./... && go test ./...
   ```

## Offline demo

With the summarizer selected, Quality Surface shows 24/60 agreement (40%),
36 disagreements, four pending suggestions, and zero provider errors. Policy
checks flag 36 unsafe publish decisions. Reject decisions **1, 6, 11** to stage
an unsupported-claim policy draft. Approve it on Promotions, then download its
patch and byte-stable attestation document. Accepting a different decision also
appears in Verdict Report and JSON export. Restarting restores human verdicts
and draft state.

Approval is not policy application. The patch tool runs separately on the host
holding the agent's policy. Attestation document generation is offline; no keys
or chain writes are needed for this proof. No per-review timeline is invented:
Trends shows suggestion timestamps and an agreement snapshot only.
