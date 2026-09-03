# Spec: scratch space in the consensus block and the Goldfish vote

## Goal

Grow two wire messages by a configured number of bytes, for stress tests.
The bytes carry no meaning. Nothing reads them.

| message | where the bytes live | default |
| --- | --- | --- |
| consensus block (`SignedBeaconBlockGloas`) | gossip encoder prefix | 0 bytes |
| `AvailableAttestation` (Goldfish vote) | SSZ field `scratch_space` | 100 bytes |

Every file and line named below was read at change `tlryulyz`. A claim
without a line is marked "assumed".

## Facts that shape the design

- There is no Heze block type. Blocks at Heze use `SignedBeaconBlockGloas`.
  A new SSZ field on it changes every Gloas spec vector. So the block bytes
  do not go into SSZ. They go into the gossip encoding only.
- `AvailableAttestation` (`proto/prysm/v1alpha1/heze.proto:29`) is our own
  type, with no spec vectors. A new SSZ field is cheap. Its signature covers
  `AvailableAttestationData` only. That stays. The new field is not signed.
- Gossip snappy-compresses messages. Zero bytes compress to almost nothing.
  The scratch bytes MUST be random, or the wire size does not grow.
- `EncodeGossip` (`beacon-chain/p2p/encoder/ssz.go:38`) and `DecodeGossip`
  (`ssz.go:83`) are the only gossip encode and decode points. Each sees a
  Go value, but NOT the same type on both sides. See section 2.
- The node fills both: the encoder pads the block at broadcast, and
  `proposeAvailableAtt` (`beacon-chain/rpc/prysm/v1alpha1/validator/attester.go`)
  fills the vote before broadcast. The validator client does not read the
  config. One validator client file changes for an encoding reason only
  (section 3).
- The nogo analyzer `cryptorand` bans `math/rand` under `beacon-chain/`,
  `validator/`, `shared/`, and `slasher/`. The randomness helper lives
  outside those trees (section 4).

## 1. Config

Two fields on `BeaconChainConfig` (`config/params/config.go`), type
`uint64`, unit bytes, in a new group directly after
`AvailableAttestationDueBPSHeze` (line 104). Follow the `SlotsPerRound`
pattern (line 73): yaml key, `spec:"true"`.

| Go field | yaml key | default |
| --- | --- | --- |
| `ConsensusBlockScratchSpace` | `CONSENSUS_BLOCK_SCRATCH_SPACE` | 0 |
| `GoldfishScratchSpace` | `GOLDFISH_SCRATCH_SPACE` | 100 |

Defaults go in `mainnet_config.go`. `MinimalSpecConfig` starts from
`mainnetBeaconConfig.Copy()` (`minimal_config.go:11`), so that file needs
no edit.

New file `config/params/scratch.go`: the constant
`MaxScratchSpace = 65536` and the function `VerifyScratchSpace`, which
rejects either value above the constant. The proto SSZ max (section 3)
is a literal and repeats the number; nothing checks one against the
other.

Call site: `beacon-chain/node/node.go`, where `return params.VerifyRounds(...)`
is the last statement of its function. Turn it into an `if err != nil`
block and return `VerifyScratchSpace` after it.

`config/params/loader.go`, `ConfigToYaml`: add the two keys next to
`SLOTS_PER_ROUND`. The e2e beacon nodes and validators read that output.

`beacon-chain/rpc/eth/config/handlers_test.go`, `TestGetSpec`: it counts
the spec keys and asserts every value. Two new `spec:"true"` fields raise
the count by two and need two values and two cases.

Kurtosis: the CL config template is in an external repo
(`github.com/sukunrt/ethereum-genesis-generator`, branch `decoupled`).
The local checkout at `~/dev/ethereum-genesis-generator` already carries
the two keys. In this repo, add a row for the two keys to the injection
table in `kurtosis/README.md`, the same route as `SLOTS_PER_ROUND`.

## 2. Block: gossip encoder prefix

Wire layout of a padded gossip message, before snappy:

    [0xFFFFFFFF][N as uint32 little-endian][scratch bytes, N][ssz bytes]

SSZ has no header. But `SignedBeaconBlockGloas` is a variable-size
container, so its first 4 bytes are the offset of the first variable
field, always 100 here, and never more than `MaxPayloadSize` (10 MiB) for
any variable-size type. The magic `0xFFFFFFFF` can never be a valid
offset, so it marks a padded message without ambiguity.

### The two sides see different Go types

Send side. `Broadcast` calls `EncodeGossip(buf, obj)`
(`beacon-chain/p2p/broadcaster.go:679`). Both block broadcasters pass the
proto: `proposer.go:500-504` calls `block.Proto()` then `Broadcast`, and
`pending_blocks_queue.go:313-319` does the same. So `msg` is
`*ethpb.SignedBeaconBlockGloas`.

Receive side. The block validator calls `decodePubsubMessage`
(`validate_beacon_blocks.go:61`). That function builds the target from
`extractValidDataTypeFromTopic` (`decode_pubsub.go:57-66`), which for the
block topic reads `types.BlockMap` (`decode_pubsub.go:84-85`). `BlockMap`
returns `blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{...})`
(`beacon-chain/p2p/types/object_mapping.go:86-89`). So `to` is the
consensus-types wrapper `*blocks.SignedBeaconBlock`, which satisfies
`interfaces.ReadOnlySignedBeaconBlock` and has `Version()`. It is NOT
`*ethpb.SignedBeaconBlockGloas`.

A type test on the proto only pads on send and never strips on receive.
Every foreign block then fails SSZ decode, the validator turns that into
REJECT (`validate_beacon_blocks.go:64`), gossipsub penalises the sender,
and the nodes disconnect from each other. This happened on a network.

The type test, `scratchPadded(msg any) bool`, MUST return true for both:

    case *ethpb.SignedBeaconBlockGloas:            true
    case interfaces.ReadOnlySignedBeaconBlock:     m.Version() >= version.Gloas
    default:                                       false

`consensus-types/interfaces` and `runtime/version` do not import
`beacon-chain/p2p`, so the encoder can import them.

### Encode and decode

New file `beacon-chain/p2p/encoder/scratch.go` with the prefix helpers.

`EncodeGossip`: after `MarshalSSZ`, if `scratchPadded(msg)` and
`ConsensusBlockScratchSpace` is not 0, prepend the magic, N, and N
random bytes. Reject a configured value above `MaxScratchSpace`; N is
written as a uint32. Then the size check, then snappy. The size check
counts the prefix. With N = 0 the wire bytes are identical to today.

`DecodeGossip`: after snappy, if `scratchPadded(to)` and the first 4
bytes are the magic: require `len(b) >= 8`, read N, require
`N <= MaxScratchSpace` and `8 + N <= len(b)`, skip `8 + N` bytes, then
`doDecode`. Otherwise `doDecode` as is.

Pre-Gloas block types get no prefix. The decoder MUST NOT check the magic
for other types. A fixed-size type has raw field bytes at the front, and
the magic can occur there.

Effects:

- No proto, SSZ, consensus-types, JSON, DB, or spectest change.
- The block root and signature do not change.
- The gossip message ID hashes the decompressed bytes, so dedup still works
  for forwarded copies. A re-broadcast from the pending queue re-encodes
  with fresh random bytes and gets a new message ID. Assumed harmless: the
  receiver already has the block and returns IGNORE.
- The bytes ride on gossip only. Blocks-by-range and by-root do not carry
  them. The DB does not store them.
- The bytes are not signed. A peer can strip them and re-gossip. Not a
  concern in a closed test network.

## 3. Vote: SSZ field

`heze.proto`, `AvailableAttestation`, new last field:

    bytes scratch_space = 4 [ (ethereum.eth.ext.ssz_max) = "65536" ];

Regenerate with both `make gen proto` and `make gen ssz`. The proto step
alone leaves `heze.ssz.go` stale.

Consequence: the container changes from fixed-size to variable-size. The
fixed part grows by a 4-byte offset: 201 bytes becomes 205 bytes plus the
scratch length. An SSZ list of these is an offset table followed by the
elements, not a concatenation. Every producer and consumer of that list
changes.

Fan-out:

- `proposeAvailableAtt`: after the checks and before `Broadcast`, set the
  field to `GoldfishScratchSpace` random bytes. Overwrite what the client
  sent. This one site covers both APIs: the gRPC `ProposeAvailableAttestation`
  (`attester.go:153`) calls it, and the REST `SubmitAvailableAttestations`
  (`beacon-chain/rpc/eth/beacon/handlers_heze.go:47`) calls the gRPC
  handler for each vote.
- `validateAvailableAttestation` (`beacon-chain/sync/validate_beacon_attestation.go:524`):
  no change. Seat resolution and signature checks ignore the field.
- Server SSZ decode, `decodeAvailableAttestationsSSZ` (`handlers_heze.go:82`):
  it divides the body by `SizeSSZ()`. Replace with an SSZ list decode: read
  the offset table, then each element. The SSZ path is live: the REST
  validator client posts SSZ first.
- Client SSZ encode, `validator/client/beacon-api/available_attestation.go`:
  it calls `PostSSZ` with `att.MarshalSSZ()` as the whole body and falls
  back to JSON only on a 415. A bare element is no longer a valid
  one-element list. Build the body as an SSZ list: a 4-byte offset table
  holding the value 4, then the element.
- JSON (`api/server/structs/conversions_block.go:2994-3040`):
  `AvailableAttestation` gets `scratch_space` as hex, `omitempty`.
  `hexutil.Encode(nil)` returns `"0x"`, which `omitempty` keeps, so the
  writer emits `""` for an empty field. On input, absent, `""`, and `"0x"`
  all decode to nil. Bound the input length by `MaxScratchSpace`.
- No change: keymanager `SignRequest_AvailableAttestationData`, gossip
  scoring, topic mapping, `decoupled/` seat math, validator gRPC client.
- Docs: replace the 201-byte figure in `plan/task.md` and in
  `plan/plan-rest-client.md` (three places, plus the sentence that calls
  the SSZ body "concatenated fixed 201-byte elements").

## 4. Randomness

`bytesutil.RandomBytes(n)` in `encoding/bytesutil/bytes.go`: return `n`
random bytes, or nil when `n` is 0. `math/rand/v2` has no `Read`; fill
with `rand.Uint64`. Both fill sites call it. Crypto randomness is not
necessary. Snappy needs only high entropy. It lives in `encoding/`
because nogo bans `math/rand` at both fill sites.

## 5. Build

- After new files or new imports, run `bazel run //:gazelle -- fix`. Expect
  BUILD changes in `beacon-chain/p2p/encoder`, `api/server/structs`,
  `beacon-chain/rpc/eth/beacon` (test deps), and `beacon-chain/sync`
  (test deps). Gazelle also fixes unrelated drift in
  `validator/client/BUILD.bazel`; commit that as its own change.
- Commit BUILD changes with the code that needs them.
- Three test targets fail on the base change already and are not part of
  this work: `TestSubmitAttestationsV2/post-electra` in
  `beacon-chain/rpc/eth/beacon`, `TestService_BroadcastAttestationWithDiscoveryAttempts`
  in `beacon-chain/p2p`, and the package `beacon-chain/rpc/prysm/v1alpha1/beacon`.
- `TestSszNetworkEncoder_BufferedWriter` in `beacon-chain/p2p/encoder`
  asserts `sync.Pool` pointer identity and can fail once under GC
  pressure. A pass on rerun is a pass.

## 6. Tests

- Config: a value above `MaxScratchSpace` fails `VerifyScratchSpace`.
- `bytesutil.RandomBytes`: length, nil at 0, not all zero.
- Encoder, the seam test: encode `*ethpb.SignedBeaconBlockGloas` with
  `EncodeGossip`; decode with `DecodeGossip` into the wrapper that
  `types.BlockMap` returns for the Gloas fork version, the way the network
  does; compare the SSZ. Run with N = 0 and N = 1000. A second case decodes
  into the bare proto. A test that decodes into the proto only does not
  count.
- Encoder, the rest: with N = 0 the bytes equal plain `MarshalSSZ`; the
  compressed size grows by about N; a non-block type gets no prefix; a
  truncated prefix is an error; a config above the bound is an encode
  error.
- `beacon-chain/sync/decode_pubsub_test.go`: one case encodes a Gloas
  block with the real `EncodeGossip` under N = 1000 and decodes it with
  `decodePubsubMessage` on the block topic. The result is a block whose
  SSZ equals the input.
- SSZ round trip for `AvailableAttestation` with a non-empty field
  (`heze_test.go`). That test package already declares an identifier
  named `bytes`, so do not import the `bytes` package there.
- `proposeAvailableAtt`: the broadcast message carries the configured
  length, through the gRPC handler and through the REST handler. The
  existing REST tests use a gomock delegate that never reaches the fill
  site. The REST fill test builds a real `validator.Server` as the
  delegate; the test file needs the `validator`, `mockp2p`, `params`, and
  `bls` imports, and a real BLS signature, because `proposeAvailableAtt`
  rejects the placeholder signature the existing helper uses.
- Server REST decode: the existing SSZ tests in `handlers_heze_test.go`
  send a bare element and two concatenated elements. Rebuild both as real
  SSZ lists. Two attestations with different scratch lengths in one body
  decode to two elements. The size assertion becomes 4 + 205.
- Client REST encode: the posted body starts with the offset 4 and the
  remainder unmarshals to the attestation. The server decoder is
  unexported in another package, so no test can hold both ends.
- JSON conversion round trip for `AvailableAttestation`, including the
  empty cases.

## 7. Open points

- The block bytes are gossip-only. A node that syncs from peers sees
  smaller blocks than one that follows gossip. Acceptable for the stress
  test. If req/resp and DB must carry them, the only path is an SSZ field
  on `SignedBeaconBlockGloas`, and that breaks the Gloas spectests.
- Dora and buildoor do not subscribe to gossip, so the prefix does not
  reach them. Nothing to check.

## 8. Misses (third execution)

- `api/server/structs/conversions_block.go`: section 3 says `hexutil.Encode(nil)`
  returns `"0x"`, "which `omitempty` keeps, so the writer emits `""` for an
  empty field". That contradicts itself. `omitempty` drops only `""`, so a
  writer that always calls `hexutil.Encode` emits `"0x"` and the tag never
  fires. The spec should have said: the writer emits `"0x"` for an empty
  field; on input, absent, `""` and `"0x"` all read as nil.
- `api/server/structs/block.go:628`: the spec named only
  `conversions_block.go` for the JSON work. The `AvailableAttestation` struct
  and its new `scratch_space` tag live in `block.go`.
- `api/server/structs/conversions_block.go`: the spec did not say the package
  needs a new `config/params` import to bound the input by `MaxScratchSpace`.
- `beacon-chain/rpc/eth/beacon/handlers_heze.go`: the spec left the list
  decoder's error contract open. The existing test
  `TestSubmitAvailableAttestations_BadRequests/"misaligned ssz body"` asserts
  the string `"Invalid SSZ available attestation list size"`, so the offset
  table decoder must keep that exact message for every malformed body.
- `beacon-chain/rpc/eth/beacon/handlers_heze.go`: the cited lines drift. The
  REST call into the gRPC handler is at line 69, not 47;
  `decodeAvailableAttestationsSSZ` starts at line 81, not 82. Same for
  `beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go`, where
  `broadcastBlock` is at 497-505, not 500-504.
- `beacon-chain/rpc/prysm/v1alpha1/validator/attester_mainnet_test.go`:
  section 6 asks for a gRPC fill test but does not name the existing
  `TestProposeAvailableAttestation`, which already builds the whole server and
  asserts the broadcast message. Extending it is the small change. The file
  carries the `!minimal` build tag.
- `validator/client/beacon-api/available_attestation_test.go`: section 6 asks
  for a new client encode test but does not say the existing
  `TestProposeAvailableAttestation/"valid sends SSZ"` matches the posted body
  against `att.MarshalSSZ()` and therefore has to be rebuilt as a list too.
- `api/server/structs/conversions_block_heze_test.go`: the spec did not name
  the file that holds the JSON round trip tests.
- `beacon-chain/p2p/types/object_mapping.go:276`: `BlockMap` has no Heze
  entry. `aliasHezeEntries()` maps the Heze fork version onto the Gloas
  constructors after `InitializeDataMaps`. The spec should have said so,
  because it is why a Heze block decodes into the Gloas wrapper.
- `beacon-chain/sync/BUILD.bazel`: section 5 expects gazelle to add test deps
  there. It did not; every import the new decode test needs was already in the
  file. Gazelle did touch `beacon-chain/p2p/encoder`, `api/server/structs`,
  `beacon-chain/rpc/eth/beacon` and `validator/client` as the spec says.
- `proto/prysm/v1alpha1/heze.minimal.ssz.go` and `heze.minimal.pb.go`: the
  spec names only `heze.ssz.go` as the stale file. Both minimal variants are
  regenerated too.
- Section 6, the encoder error cases: a truncated or oversized prefix is only
  an error when the decode target is a padded type. The decoder skips the
  magic check for anything else, so the test has to decode into the block
  wrapper. The spec did not say it.
- Section 5's list of known failures is short by one: `go build -tags minimal
  ./...` fails in
  `proto/prysm/v1alpha1/attestation/aggregation/testing/bitlistutils.go:102`
  on the base change, and `go vet -tags minimal ./proto/prysm/v1alpha1/` fails
  in `cloners_test.go`. Neither is part of this work.
