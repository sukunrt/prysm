# Historical applicability of the reproduced count path

The repeated-count mechanism does not depend on a test-only shared-state
feature. The deployed revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5` already uses the same validator
representation and read path unconditionally.

This review read the historical files directly with
`jj --ignore-working-copy file show -r 0280403c70d88967f49d2d4c730f4c5417dabdf5`.
The historical Heze SSZ decoder calls
`state_native.InitializeFromProtoUnsafeHeze`
(`encoding/ssz/detect/configfork.go:200`); the database decoder uses the same
initializer (`beacon-chain/db/kv/state.go:596`). That initializer always assigns
`validatorsMultiValue = NewMultiValueValidators(st.Validators)`
(`beacon-chain/state/state-native/state_trie.go:1007`). Gloas does the same at
line 894, as do every earlier fork's initializers. The safe Heze initializer
clones the protobuf and delegates to this same constructor.

`NewMultiValueValidators` converts the input to compact validators and
initializes `Slice[CompactValidator]`
(`state-native/multi_value_slices.go:115–128`). There is no feature conditional
on that construction. `ValidatorsReadOnlySeq` then calls the slice's `At` for
each validator (`state-native/getters_validator.go:227–248`), and `At` always
takes and releases its shared `RWMutex`
(`container/multi-value-slice/multi_value_slice.go:255–257`). None of those
files has a build-tag alternative. The native state's separate reference
tracking is not the per-validator atomic work implicated here.

At review time, a historical-to-working-copy `jj diff` was empty for all six
of these production files:

- `beacon-chain/core/helpers/validators.go`
- `beacon-chain/state/state-native/getters_validator.go`
- `beacon-chain/state/state-native/state_trie.go`
- `beacon-chain/state/state-native/multi_value_slices.go`
- `container/multi-value-slice/multi_value_slice.go`
- `config/features/config.go`

The historical registry size is also observed directly. Node 201's beacon log
identifies the deployed revision at line 1 and records `genesisValidators=120000`
at line 27. Node 400 line 28 and node 500 line 26 agree. A fallback count
therefore visits 120,000 entries regardless of how many entries ultimately
satisfy the active predicate. Full scans on the same cached checkpoint share
the slice and its reader-count bookkeeping; multiple independent state
objects are not required to trigger that shared-lock cost.

The reproduction uses the same registry size and production path, not the
identical historical genesis. H and I2 both record 120,000 validators at
`results/beacon3.log:29`; their genesis validator root begins `7ccc1d6b`, while
the historical root begins `7dc9ac77`. The new direct payload test loads H's
retained genesis SSZ, asserts slot zero and 120,000 validators, warms the
committee cache, and then calls the unchanged production count helper. Its
memoized arm supplies that immutable fixture's previously computed correct
count. This is a controlled removal of repeated work, not a proposal to remove
the general slot-zero correctness guard.

These checks support transfer of the concrete scan/reader-atomic mechanism to
the historical source and registry. They do not equate the experiments' CPU
allocation, message admission, validator keys, runtime schedule, or timeout
frequency with the historical network. H's packet capture and runtime trace
remain the direct measurement of response-reader starvation; the historical
owner logs establish matching failures and the cached-payload early-return
branch without recording each owner's exact transport stack.
