# Slot 110 working hypothesis: slow attestation maintenance and possible pool contention

## Status

The best code-supported theory is that long background attestation maintenance and the ordinary vote burst created substantial concurrent CPU work, goroutine backlog, and possible shared-pool contention around slot 110. This remains a **working hypothesis**, not a reconstructed goroutine profile or proof that the proposer waited on a particular lock at `01:52:00Z`.

The strongest new runtime facts are:

- exact-old Prysm exposed 15,438 goroutines at `01:52:02.460Z`, during the 3.410-second build; nearby same-phase samples were 882 at slot 90, 847 at 95, 3,821 at 100, 849 at 105, and 870 at 115 ([metrics audit](metrics-slot110/metrics-audit.md));
- the aggregate preparation histogram completed two T1 passes totaling 6,306 ms between `01:51:32.460Z` and `01:52:02.460Z`; one was at most 500 ms and the other was therefore at least 5,806 ms. Two T2 passes totaled 1,221 ms; one was at most 10 ms and the other at least 1,211 ms ([native series](metrics-slot110/owner148-pool-compaction/aggregate-attestations-native-0150-0154.json), [per-scrape derivation](metrics-slot110/owner148-pool-compaction/aggregate-attestations-per-scrape.json));
- the T1/T2 metrics time the entire scheduled forkchoice-preparation pass. They do not measure a particular mutex hold, identify which scheduled invocation was slow, or prove overlap with the proposer. Their 30-second scrape cadence cannot recover those timestamps.

## Exact-old concurrency path

The deployed tree pins `github.com/libp2p/go-libp2p-pubsub v0.17.0` (`go.mod:50`). Its validation pipeline has an 8,192 global asynchronous-validation limit, a 1,024 per-topic default, and `runtime.NumCPU()` front workers (`validation.go:14-18,122-139,196-211`). For each remotely received message with an asynchronous validator it starts one goroutine after acquiring the global throttle (`validation.go:366-378`). Prysm registers its topic validator without a concurrency override (`beacon-chain/sync/subscriber.go:459-474`), so each attestation-subnet topic keeps the 1,024 limit. Even if the six FFG committees mapped to six distinct topics, ordinary single-attestation validator goroutines would be bounded by 6,144; all topics together remain bounded by 8,192. Therefore the observed 15,438 goroutines cannot all be libp2p validator goroutines.

Prysm sets the libp2p validation queue from `--pubsub-queue-size`, whose source default is 1,000 (`cmd/flags.go:188-192`, `beacon-chain/p2p/pubsub.go:163-175`). Queue insertion is nonblocking and drops when full (`go-libp2p-pubsub/validation.go:248-260`), so queued validation requests also cannot account for thousands of goroutines.

Each validator can wait in several places:

- ordinary FFG validation reads block/forkchoice/state and committee data (`validate_beacon_attestation.go:124-160`);
- signature validation sends to `signatureChan`, then waits on an unbuffered result channel (`batch_verifier.go:56-65`). The shared verifier batches at 1,000 requests or every 5 ms (`batch_verifier.go:15,27-52`); the source CLI default for `--batch-verifier-limit` is 1,000 (`cmd/beacon-chain/flags/base.go:349-354`);
- after signature acceptance, validation synchronously calls `OperationFeed().Send` (`validate_beacon_attestation.go:195-245`). The exact go-ethereum feed serializes concurrent senders with `sendLock` and waits until every subscribed channel accepts the event (`go-ethereum@v1.17.5/event/feed.go:118-175`). This is a possible waiting point. Prior paired tests found no slowdown from the synthetic 18-stream SSE fixture, so it is secondary evidence rather than the leading explanation.

After validation, a second fanout is unbounded. Every topic has a subscription loop that calls `go pipeline(msg)` for each delivered message (`beacon-chain/sync/subscriber.go:520-544`). Attestation subscriptions have a 5,000-message delivery buffer (`subscriber.go:437-455`), but there is no semaphore around these handler goroutines. Consequently a burst of accepted messages can transiently leave roughly one subscriber goroutine per message even though the upstream libp2p validator population is capped.

For ordinary singles, that handler calls `HasAggregatedAttestation` and then `SaveUnaggregatedAttestation` (`subscriber_beacon_attestation.go:19-36`). `HasAggregatedAttestation` takes the aggregate pool's `aggregatedAttLock.RLock` while checking the relevant group (`operations/attestations/kv/aggregated.go:300-364`). Subscriber goroutines can therefore pile up behind an aggregate writer.

Aggregate gossip follows the same unbounded subscriber fanout, then calls `SaveAggregatedAttestation` (`subscriber_beacon_aggregate_proof.go:13-35`). In the legacy pool, that method holds `aggregatedAttLock.Lock` while it runs `attaggregation.Aggregate` over the existing group plus the new object (`operations/attestations/kv/aggregated.go:115-160`). The aggregation uses repeated maximum-cover rounds and BLS signature aggregation (`proto/prysm/v1alpha1/attestation/aggregation/attestations/maxcover.go:14-95,151-200`). This establishes a shared writer path, but neither the historical telemetry nor the controlled profiles measure its slot-110 hold time.

The proposer begins `packAttestations` by calling `AttPool.AggregatedAttestations()` (`rpc/prysm/v1alpha1/validator/proposer_attestations.go:32-47`). That getter acquires the same `aggregatedAttLock.RLock` (`operations/attestations/kv/aggregated.go:176-188`). A concurrent `SaveAggregatedAttestation` writer can therefore delay proposal construction before the proposer reaches its own deduplication, aggregation, sorting, and signature filtering.

## Scheduled background aggregation

The legacy attestation service also calls `AggregateUnaggregatedAttestations` from its scheduled forkchoice preparation, then reads the aggregate pool and continues forkchoice consolidation (`operations/attestations/prepare_forkchoice.go:19-74`). Source defaults schedule T1 at slot +7 seconds, T2 at +9.5 seconds, and T3 at +11.8 seconds (`config/features/flags.go:58-74`, wired at `config/features/config.go:347`). The exact historical command line does not record whether these hidden defaults were overridden.

Under the source defaults, a T1 pass lasting at least 5.806 seconds can cross the next 12-second slot boundary. The histogram bracket contains two T1 invocations and cannot identify whether the long one began in slot 108 or slot 109, so it does not prove that this particular pass crossed into slot 110. It does establish that seconds-long aggregate preparation was live on the owner immediately around the incident, and similar multi-second T1 totals appear in every nearby native interval reported by the census.

A focused exact-old profile now identifies a concrete CPU-heavy part of that pass. With 13,000 one-bit inputs across six groups, `AggregateUnaggregatedAttestations` took 1.078 seconds/op unloaded. Across the profile's five invocations, serial raw deletion consumed 4.22 of 6.23 focused CPU-seconds; `insertSeenBit` consumed 4.09 seconds and its `Bitlist.Contains` scans 3.85 seconds. The six groups cause 14,076,834 containment checks because each deleted singleton is appended separately to the seen list rather than unioned. The parallel signature-aggregation workers consumed 1.73 CPU-seconds ([focused profile report](old028-heavy13k-raw-compaction-profile.md)).

This quadratic cleanup runs before the per-entry raw-pool write lock, so its duration is evidence of maintenance CPU load rather than a measured long writer critical section. The separate +11-second expiry scan holds the raw write lock across its map and seen-list scan and can block proposer raw reads, but no retained timer isolates that operation.

## Why this is the leading theory, and its limits

The theory connects three independently observed facts through the exact deployed code:

1. seconds-long background aggregate preparation existed around slot 110;
2. aggregate writers, single-attestation subscribers, and the proposer share `aggregatedAttLock`, while raw expiry can similarly contend with proposer raw reads;
3. the subscriber path can create an unbounded one-goroutine-per-message backlog, consistent in scale with 15,438 goroutines.

The missing decisive evidence is a historical goroutine dump, mutex/block profile, trace, or per-operation timestamps proving whether any writer held a pool lock and how long the proposer waited. The goroutine gauge includes runnable, blocked, and waiting goroutines and does not reveal stacks. The background histogram covers the full preparation pass rather than its lock-held segment. Its long T1 observation can include raw BLS aggregation, quadratic seen-list cleanup, forkchoice aggregation, lock waits, and scheduling.

The exact-old 13k-input packing benchmark took about 3.43 seconds, but it ran against a quiescent preloaded fixture, outside a concurrent pool writer, and used a controlled input shape. It demonstrates that proposer-side CPU work can independently be seconds-long; it neither reproduces nor excludes the live shared-lock path. The Xatu/SSE callback path and Go scheduler competition remain possible contributors, but current paired SSE tests were negative and the retained metrics do not identify them as the cause.
