# Historical Geth branch provenance

Round2's reported source revision matches the official
[`glamsterdam-devnet-8` branch](https://github.com/ethereum/go-ethereum/tree/glamsterdam-devnet-8).
The user's correction is supported for round2: an older source date does not
make this the wrong devnet build.

## Official branch check

The read-only remote query on **2026-09-08 UTC** was:

```text
git ls-remote https://github.com/ethereum/go-ethereum.git refs/heads/glamsterdam-devnet-8
aa1f2fcf512988eb8890d9352e601b898d6fdb2c	refs/heads/glamsterdam-devnet-8
```

The isolated source clone independently contains the same remote-tracking ref.
At `2026-09-08T07:52:59Z`, the local check returned:

```text
git -C /tmp/geth-build-diff.VT0MBD/go-ethereum rev-parse origin/glamsterdam-devnet-8
aa1f2fcf512988eb8890d9352e601b898d6fdb2c
```

## Historical runtime match

All 12 retained round2 `execution.log` startup records report:

```text
Geth/v1.17.6-unstable-aa1f2fcf-20260813/linux-amd64/go1.26.5
```

A direct raw anchor is `runs/round2/round2-prysm-geth-1.tar.gz`, member
`./execution.log:65`. The source object matching that embedded commit prefix
is exactly the full branch-head hash above. Coverage is recorded in
[binary-comparison.md](binary-comparison.md).

The sampled round1 processes report `ff083d45` or `d799b1a3`, both newer
descendants of `aa1f2fcf`. They do not match the checked branch head. The logs
alone do not identify their deployment's branch or container tag.

## Distinctions retained

A branch or image tag is a name that can move. The 2026-09-08 branch check and
historical runtime string establish a matching source revision. They do not
recover the exact September-5 launch command, requested image tag, or OCI image
digest, none of which is present in the retained historical node archives.

The repository's example `kurtosis/network_params.yaml:20` also names
`ethpandaops/geth:glamsterdam-devnet-8` in both historical Prysm revisions.
Its header specifies a five-node recipe, and its comment associates the tag
with an earlier `366048ea` build. That example is not an exact manifest of the
1,000-node deployment; it must not override the direct round2 runtime match to
`aa1f2fcf`.

The earlier source-diff inventory remains a valid comparison of the observed
commits. Calling its reverse direction a "rollback" did not establish a
deployment error. Selecting the `glamsterdam-devnet-8` revision can be an
intentional configuration choice; the source differences alone do not show
that Geth caused the observed chain problems.
