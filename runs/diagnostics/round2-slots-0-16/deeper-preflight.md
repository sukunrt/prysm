# Deeper preflight search ledger

This follow-up records new discriminants beyond `prior-startup-audit.md`. Source
anchors refer to `0280403c70d88967f49d2d4c730f4c5417dabdf5`, read using
`jj --ignore-working-copy`. Raw node169/node191 logs are under
`/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-N/`. The source audit was
performed by an Astra agent; the accompanying diagnostic code was implemented
by a Sol agent. Completed test results are linked where relevant and are kept
separate from historical observations.

## Check 1: are 75 attester failures independent stalled RPCs?

**Result: no.** Exact `validator/client/validator.go:763–822` uses the
post-Electra cached-attestation-data path during Gloas too. Cache misses acquire
`cachedAttestationDataLock.Lock()` at 801, hold it through the BN RPC at 809–812,
and fill the cache only on success at 817. Each failed RPC leaves the cache
empty. Thus one request can occupy the remaining slot budget, after which
queued callers successively issue RPCs with expired contexts. The earlier
count of 75 failures is valid role accounting, not a count of 75 concurrently
outstanding BN requests.

Node169's slot1 attester failures start at validator.log:688,
`01:30:24.002384599`, and continue through :774, `01:30:24.018253954`.
Their roughly 16 ms drain interval is consistent with serialization and immediate
expired calls. Without request starts it does not establish which attester
owned the first RPC or how long that first request ran. Importantly, these
attesters cannot have owned `domainDataLock`: their failures occur before
`signAtt` (`attest.go:80–90`). The two caches have separate locks.

## Check 2: did some slot1 domain access succeed before RANDAO failed?

**Result: yes, through sync-role selection.** Node169 validator.log:708 at
`01:30:24.006131913` is “Could not get sync subcommittee index,” emitted from
the already dispatched sync-aggregator role (`sync_committee.go:127–133`).
That role is appended only after `SyncCommitteeAggregators` successfully
returns an aggregator key (`validator.go:646–653`). The local selector must
have produced at least one sync-selection signature to choose that key
(`aggregator_selector.go:131–172`). Every such signature calls `domainData`
(`sync_committee.go:237`), even if the domain itself is cached.

All domain-cache hits acquire `domainDataLock.RLock()` first
(`validator.go:725–736`). Consequently the lock admitted a slot1 preflight
domain lookup after slot1 began and before role dispatch. The PTC result at
validator.log:686 bounds dispatch by `01:30:21.915067414`.

This excludes a **single uninterrupted domain write-lock holder** extending
from genesis or slot1 start all the way through the RANDAO deadline. Such a
holder would also have prevented the successful sync-selection lookup and
therefore role dispatch. Multiple holders, a holder beginning after preflight,
or delay in RANDAO's own RPC remain possible. It also means the domain failure
was not simply an entirely unusable epoch0 signing-domain facility.

## Check 3: could the proposer wait on its own attester's logical role lock?

**Result: the role keys do not collide.** A search over all non-test historical
files below `validator/client` found only three `NewMultilock` sites:

- `propose.go:54`: two keys, the printable proposer-role number and raw 48-byte
  pubkey;
- `attest.go:45–59`: one key containing a role byte followed by 48-byte pubkey;
- `available_attestation.go:109–123`: the same concatenated form with another
  role byte.

The fact that node169's proposer key also has an attestation duty therefore
does not make that attester's RPC own the proposer's logical key. Slot0's
submission reports at validator.log:681–685 are emitted after the slot0 role
waitgroup completes (`runner.go:273–285`); deferred role unlocks run before
those goroutines complete. There is no still-running slot0 role handler implied
by a late aggregate report. Slot0 also never acquires the proposer lock.

The proposer can still wait on the package-global multilock registry, whose
`getChan` and `Clean` share a lock, or be descheduled during acquisition. That
registry lock protects bookkeeping and is not itself held across the other
role's network RPC (`async/multilock.go:37–67,93–123`). Thus “proposer blocked
behind its attester's long RPC on the same pubkey” is excluded by the key
construction; registry scheduling/contention is a different, narrower claim.

## Check 4: are available-attestation roles missing domain-lock competitors?

**Result: no.** `available_attestation.go:156–175` signs using the static
`decoupled.AvailableAttDomain`; it never calls `domainData`. These roles can
contribute scheduling, keymanager, transport, and multilock-registry work,
but cannot hold `domainDataLock`. Their own data fetch also has a separate
cached-data path. Their absence from the earlier domain-role accounting does
not create an unidentified signing-domain owner.

## Check 5: what exactly does slot1's successful PTC NotFound prove?

**Result: actual RPC progress, but no fork-choice lock acquisition.** The VC
log at :686 occurs only after a NotFound response from PayloadAttestationData
(`payload_attestation.go:44–59`). The gRPC adapter explicitly disables retry for
this call (`grpc_validator_client.go:556–564`). On the server, it passes the
sync-ready gate (`rpc/prysm/v1alpha1/validator/payload_attestation.go:34–39`)
and the core method's fork/current-slot checks and singleflight wrapper
(`rpc/core/validator.go:1070–1115`). It reaches the no-block return at 1141–1143.

That return checks `HighestReceivedBlockSlot`. The actual getter
(`forkchoice/doubly-linked-tree/store.go:479–483`) does **not** acquire the
fork-choice lock. It returns before highest-root lookup, canonical-shuffling
checks, DataAvailable, or payload presence. Hence the successful result does
not exclude a concurrent fork-choice-lock blockage affecting other methods.
It does exclude a complete uninterrupted transport/process outage covering
every operation throughout slot1.

There is a potentially tighter numerical bound. Node169 validator.log:647
confirms that its gRPC adapter ignores the execution-payload-available topic.
The adapter indeed streams only head events (`grpc_validator_client.go:397–415,
423,468–471`). Consequently its PTC waiter cannot wake from a payload-available
event; it waits until its configured component timer, or cancellation
(`wait_helpers.go:172–193`). With the source-default PayloadAttestationDueBPS 7500
(`mainnet_config.go:141`), the first possible slot1 PTC request is at
`01:30:21`, so this successful request's complete envelope is at most 915.067 ms.
This numerical bound remains conditional on confirming the historical config
did not override 7500; the qualitative no-payload-event result is directly
confirmed by the owner log.

## Check 6: could genesis reorg events cause VC refresh/backpressure loops?

**Result: the proposed direct event route is absent.** The actual gRPC VC calls
StreamSlots with VerifiedOnly=true (`grpc_validator_client.go:423`). The BN's
`blocks.go:55–74` subscribes to StateFeed, but forwards only BlockProcessed,
discarding Reorg and NewHead events. The generated VC event is EventHead with
slot/dependent roots, no block root, no payload availability.

Before any non-genesis block is processed, the same-root reorg messages in
node169 BN:517,519 and Xatu:971,972 do not themselves generate VC head events.
They therefore cannot directly trigger `ProcessEvent`'s dependent-root refresh
or fill the VC event channel with those reorgs. This excludes the specific
loop “RolesAt prevents VC reorg consumption, which blocks reorg gRPC delivery,
which blocks that RolesAt.” A BN StreamSlots subscriber can still fail to drain
its one-element StateFeed channel while descheduled, and other subscribers
have their own behavior; those possibilities do not use this nonexistent
forwarded-reorg route.

## Check 7: is a terminal domain status necessarily one server call?

**Result: no; client retry is a separate conditional envelope.** Historical
`service.go:352–365` installs grpc-middleware retry with configurable maximum
and linear backoff. Defaults are 5 attempts and 1-second delay
(`cmd/validator/flags/flags.go:90–103`); actual invocation flags are not yet
independently recovered. Historical go.mod pins grpc-middleware v1.2.2. That
dependency retries ResourceExhausted/Unavailable, and cancellation during
backoff is converted to gRPC DeadlineExceeded (`retry/retry.go:261–275`,
`retry/options.go:15–25`). Thus the final error may originate from a retry
envelope, not necessarily a DomainData handler which itself failed.

No historical intermediate retry logs were found in the owner interval.
This is not evidence that retries happened. Default backoff alone also does
not explain a 12-second delay: five attempts have four 1-second delays, so an
entire slot needs additional RPC/wait/scheduling time. PTC's explicit zero-retry
override makes its successful result cleaner than this generic domain path.

## Check 8: source before sync preflight and a diagnostic that separates it

The earlier statement that proposer settings precede RolesAt can be narrowed.
Post-Gloas, `PushProposerSettings` does not synchronously call
PrepareBeaconProposer (`validator.go:908–943`). Proposer-preference submission
is deferred by half a slot (`:948–965`). Preference signing itself is
synchronous and uses DomainData (`registration.go:98–110`), but current-epoch
proposer slots at or before slot+1 are skipped (`validator.go:1406–1412`).
Node169 has only one epoch0 proposer slot, slot1, so its own slot1 preference
does not create this extra domain call during slot1. Ordinary status-cache
refresh is epoch-start/key-count-change gated (`validator.go:1183–1188`).

More substantively, ordinary attester selection precedes sync selection in
RolesAt. `localSelector.AttestationSelectionProof` uses blocking singleflight
with the winning caller's context (`aggregator_selector.go:87–111`). A
background subnet worker can be the winner. A slot-limited RolesAt waiter can
therefore be delayed behind that background operation and then receive a valid
proof after its own deadline; it proceeds to sync-index lookup with an already
expired context. This yields the same terminal sync-index/RANDAO log chain as
a genuinely slow sync-index RPC, without the index RPC being slow at all.

The completed differential diagnostic uses real RolesAt/localSelector and
subsequent real ProposeBlock with the same absolute deadline. One case delays
sync-index service; the other delays the preceding background-winner selection
proof and makes sync-index return immediately on expired context. A stack
observation verifies that RolesAt actually joined the singleflight wait before
the winner is released. Both preserve the proposer role, reach expired RANDAO,
and never request a block. A separate live-budget control reaches BeaconBlock.

The completed fan-out diagnostic sends 75 real cached-attestation-data callers
through one deadline. It records one predeadline underlying RPC admission and
maximum concurrency one despite 75 terminal errors. These passing tests make
the source-level ambiguity concrete; neither identifies the historical wait.
Commands, results, and fixture limits are in [offline-preflight-results.md](offline-preflight-results.md).

## Check 9: was a previously fetched RANDAO domain evicted?

**Result: no earlier RANDAO-specific fetch is established for any of these
owners, and the normal source path makes each its first such request.** The
512 archived epoch-0 duty rows cover all 32 slots on each of the 16 owners.
Each owner's first nonzero proposal is its slot in 1–16. Nodes 35 and 76 also
have later assignments at 21 and 28, respectively; those cannot prewarm their
earlier proposals. The rows and original member anchors are in
[validator_role_progress.tsv](validator_role_progress.tsv).

At the historical revision, `service.go:157–161,213` creates a fresh domain
cache for the validator. `validator.go:755` fills it only after a successful
DomainData request. The only RANDAO-specific callers are
`propose.go:403` and `UpdateDomainDataCaches` (`validator.go:704,714`).
The latter is invoked only at epoch end (`runner.go:139–143`): the first
possible invocation is slot 31 and requests epoch 1. It cannot prewarm epoch 0
before these proposals. `ProposeBlock` skips genesis before signing RANDAO.

With the standard distinct signing-domain values, these are cold RANDAO
entries, including on the successful slot-15 and slot-16 owners. Historical
domain configuration was not independently recovered, so this conclusion
does not assert unseen configuration bytes. It does exclude using the cache
capacity test alone as proof that a prior RANDAO entry was evicted in this
run. The small cache can still increase other-domain RPC traffic and contention
on the shared cache mutex; the test does not measure that historical effect.
