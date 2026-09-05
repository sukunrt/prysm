# Historical parity of the validator-registry finalizer gate

The natural finalizer mechanism seen in the local full-gossip trace exists
unchanged in deployed Prysm revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`. Cleanup of an unreachable native
state can take a **shared validator-slice write lock**, collect live states'
Count readers behind it, and release those readers together. The traced
release and the subsequent expensive scans are different phases: a short
cleanup can make a much larger amount of work runnable.

## Exact source comparison

This read-only check returned no differences for the four production files:

```text
jj --ignore-working-copy diff \
  --from 0280403c70d88967f49d2d4c730f4c5417dabdf5 --to @ --summary \
  beacon-chain/state/state-native/state_trie.go \
  container/multi-value-slice/multi_value_slice.go \
  beacon-chain/state/state-native/getters_validator.go \
  beacon-chain/core/helpers/validators.go
```

The historical-to-current diff in `sync/validate_beacon_attestation.go` changes
vote logging/summary accounting and returns the already-computed subnet. It
does not change the call to `ActiveValidatorCount` or the Count algorithm.
Historical `go.mod` declares Go 1.26.5, also used by the local traced test.
The source parity establishes the available production mechanism; it does not
recover the original machine's runtime events or their frequency.

## How an unreachable state can block live readers

The Heze initializer installs `runtime.SetFinalizer(b, finalizerCleanup)` at
`state_trie.go:1043`. Native `Copy` preserves the `validatorsMultiValue` field
reference at line 1106, registers the destination with that slice at line
1157, and installs the same finalizer on the new state at line 1217. Copies
have separate state objects and state locks, but share the underlying validator
slice and its mutex.

`finalizerCleanup` first locks the state being cleaned, clears its tracking
maps, and detaches its shared fields. The **validator** call is specifically
`b.validatorsMultiValue.Detach(b)` at `state_trie.go:1747`; preceding balances
and other fields use different slices. `Slice.Detach` acquires its write lock
at `multi_value_slice.go:356` and releases it through the deferred Unlock at
line 357. It removes object IDs from individual/appended entries and cached
lengths. It does not traverse the 120,000 shared validators merely to remove
an unchanged genesis copy.

The cleaned state's own lock is not the cross-state dependency. The shared
slice's lock is: living checkpoint states still read that same slice. A live
copy can keep the registry reachable without keeping every other state object
that once shared it reachable. The trace does not identify which of those
state objects became eligible for finalization.

## Why a brief writer collects and releases readers

In the pinned Go `sync/rwmutex.go`, `Lock` first announces the pending writer
by subtracting `rwmutexMaxReaders` from `readerCount` at line 152. It then waits
for already-active readers. New `RLock` calls see the negative count and park
on `readerSem` at lines 72–74. The last old reader wakes the writer through
`RUnlock`; writer `Unlock` restores the count and calls `Semrelease` once for
each accumulated reader at lines 209–216.

Therefore readers can accumulate while the writer is waiting to acquire the
lock or waiting for processor time, even if its useful mutation is tiny.
Unlock makes them runnable; it does not run all of them immediately.

The local full-gossip trace contains this concrete sequence:

| Event | Trace-clock nanoseconds | Original parsed lines |
|---|---:|---:|
| G6 parks in validator `Detach` | 277054661840512 | 1031405–1031425 |
| Count G4968's `RUnlock` wakes G6 | 277054662598208 | 1047454–1047486 |
| G6 runs | 277054662600896 | Joined native transition |
| G6's validator `Detach` Unlock wakes 101 Count readers | 277054662602688–277054662630784 | 1047650–1049082 |

The wake wave lasts 28.096 microseconds. The joined prior waits identify 98
readers in Count's validator `Len` and three in validator `At`. The exact
source call site distinguishes this finalizer from the earlier cold-sync and
attestation-data `Copy` operations. The same run's raw trace and provenance
are retained in the [transport audit](full-gossip-domain-writer-trace-audit.md).
The [full cohort trace join](full-gossip-finalizer-cohort-trace-audit.md)
and [101-member table](full-gossip-finalizer-cohort-members.tsv) establish
the individual park/wake relationships and subsequent scheduling and BLS waits.

## Why those released counts remain costly

`ActiveValidatorCount` reads the cached committee count, but its fast return
requires `s.Slot() != 0` (`validators.go:154`). A warm slot-zero checkpoint
instead traverses `ValidatorsReadOnlySeq` at line 168. The helper does not use
its `ctx` parameter to cancel either lock waits or the scan.

The native iterator holds the living state's read lock for its entire loop.
It calls the shared slice's `Len` once and `At` for every validator
(`getters_validator.go:227–248`). Each `At` acquires and releases the same
slice's read lock and performs the indexed lookup
(`multi_value_slice.go:255–277`). Neither the iterator nor those lock
operations checks a context.

The 98 readers parked at `Len` have not begun their per-validator loop.
On this valid, unchanged 120,000-validator fixture, they still have **11.76
million validator visits** to perform after being woken. The three readers
parked at `At` are already partway through their loops; their remaining
positions are unknown. These numbers describe required work, not elapsed CPU
time or 101 simultaneously running goroutines. Canceling an enclosing RPC or
gossip context does not itself interrupt a Count already in this path.

## Attribution limits

The trace's reader/writer handoffs identify the actual shared lock interaction
and a production cleanup source. They do not contain the finalized state's
object ID, allocation stack, copy origin, or the event that made it unreachable.
It cannot be assigned specifically to the timed GetAttestationData response,
the temporary preload check, cold sync setup, or another native copy.
Historical node169 was already connected roughly 39 minutes before genesis;
this fixture constructs its state shortly before load and performs its API
preflight near the slot boundary. Equal cleanup source therefore does not
establish equal populations of unreachable states, allocation timing, or
garbage-collection timing. The reproduced finalizer is a demonstrated cohort
formation mechanism, not an identified historical trigger.

The fresh [timed-request omission pair](full-gossip-domain-no-timed-attdata-results.md)
retains preload and sync copies in both arms and stays fast in both. It does
not discriminate the trigger of the earlier large cohort. The 101-reader
wave belongs to the **850 ms traced repetition**, not the untraced run with
1,324 peak iterators and a 6.856-second RPC. Source parity makes the same
mechanism applicable to historical deployment; it does not prove that node169
experienced this exact wave or locate its historical RANDAO request.
