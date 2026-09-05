# Historical bandwidth shaping and internal RPC scope

The retained evidence does not establish that node169's internal gRPC or
Engine-proxy flows traversed the bandwidth limiter. It also does not contain
the historical limiter configuration needed to exclude that possibility
directly. An adjacent real deployment configuration explicitly excludes
private and Docker networks, making it especially inappropriate to assume
that all traffic shared the configured 20/50 Mbps limits.

## What is retained for the actual run

The user's deployment clarification is preserved in
`runs/diagnostics/startup_diagnosis.md:20`: one node per machine, 800 machines
limited to 20 Mbps outbound / 50 Mbps inbound, and 200 unlimited. That note
explicitly says the saved node numbers had not been conclusively mapped to
the bandwidth classes. Neither a bandwidth-class inventory nor a shaping
interface/filter definition was found in the current run metadata or the
earlier large-run analysis retained in jj commit `0b1b0bdaa063`.

Node169's five retained component logs establish these endpoints:

| Evidence | Recorded endpoint or address |
| --- | --- |
| `validator.log:11` | One gRPC provider, `beacon:4000` |
| `beacon.log:20` | gRPC listener `0.0.0.0:4000` |
| `beacon.log:35` | Beacon P2P address `172.18.0.7:9000` |
| `beacon.log:41` | Engine endpoint `http://snooper-engine:8561` |
| `snooper-engine.log:2` | Proxy target `http://execution:8551` |
| `snooper-engine.log:22657` | An earlier restart-time DNS error uses Docker resolver `127.0.0.11:53` |
| `xatu-sentry.log:9` | Network name `glamsterdam-devnet-10` |

Paths in this table are under
`/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-169/`.
The DNS error precedes round2 genesis by about 41 minutes; it is endpoint
evidence, not a slot-1 DNS failure. Combined with the one-node-per-machine
deployment clarification, the service names and private address support an
internal container-network topology. They do not identify its Linux qdisc,
routing rules, or interface exemptions.

The node169 archive and the initially saved node1 archive each contain only
`execution.log`, `xatu-sentry.log`, `beacon.log`, `validator.log`, and
`snooper-engine.log`. No Docker Compose file, rendered Ansible variables,
limiter log/configuration, `tc` filter/counter dump, or packet capture is in
those archives. The corresponding `/home/sukun/runs` tree supplies no extra
deployment metadata. This audit inspected selected existing logs and metadata;
it did not download or scan all 1,000 archives again.

## A concrete adjacent configuration, with a different provenance

`/home/sukun/dev/blob-devnets/ansible/inventories/devnet-0/group_vars/ethereum_node.yaml:16`
configures `ethpandaops.general.brakebear` on containers named `execution` and
`beacon`. Both entries explicitly specify:

```yaml
exclusions:
  private-networks: true
  docker-networks:
    names: ["*"]
```

Its playbook invokes the brakebear role at `ansible/playbook.yaml:85`, and
`ansible/requirements.yaml` obtains `ethpandaops.general` from its Git repository.
The same variables choose speed from `ethereum_node_cl_supernode_enabled`.
This is an actual deployment configuration in a nearby checkout, not a
guessed command line.

It is **not** the run's configuration: that checkout's latest commit is
`e8928f6` dated 2026-04-16, its inventory is blob devnet-0, and its rates are
50 Mbps upload / 100 Mbps download for ordinary nodes and 1 Gbps for supernodes,
with 80 ms latency. Those differ from the retained round2 deployment note.
Its exemptions cannot be silently transferred to glamsterdam-devnet-10.
Likewise, node169's `Operating in supernode mode` log and 596-key wallet do not
prove its historical bandwidth class without the actual inventory mapping.

Current local Kurtosis/disruptoor and Shadow files provide other mechanisms
and configurations. None located in this audit is identified as the historical
glamsterdam-devnet-10 deployment. The startup3 reproduction's own Starlark and
launcher do not configure these historical bandwidth limits.

## What earlier payload work actually excluded

The H/I2 local reproduction excludes delayed wire delivery for its three
captured timeout calls: complete responses reached the BN's captured veth and
were acknowledged before the response reader spent 325–682 ms runnable.
`runs/diagnostics/startup3/wire-causation-results.md:33` explicitly limits
that direct packet/runtime claim to the reproduction.

The historical payload audit is also explicit that proxy-to-client buffered
delivery remains unmeasured
(`runs/diagnostics/round2-slots-0-16/payload-root-cause-audit.md:203`). A quick
proxy `io.Copy` confirms that its output was accepted by the response writer;
it does not establish when the BN received or consumed every byte. No previous
retained analysis located here demonstrates historical shaping exemptions.

Therefore shaping is not a newly established cause of node169's extra
RANDAO delay or of the historical payload timeouts. The source-backed
checkpoint/count/scheduling explanation has positive reproductions; a shared
limiter queue lacks positive historical evidence, and adjacent deployment
practice instead includes internal-network exemptions. The precise remaining
configuration fact is the deployed brakebear/other limiter rules and node169's
inventory class. No production code or network configuration was changed.
