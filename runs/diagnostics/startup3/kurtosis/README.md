# Three-node startup diagnostic

This local Kurtosis package starts three Geth/Prysm pairs and one 596-key Prysm validator attached to `bn-3`. Node 1 is started first; its REST identity ENR bootstraps nodes 2 and 3. The package does not generate or modify genesis.

The runtime bundle must have this layout:

```
network-configs/config.yaml
network-configs/genesis.ssz
network-configs/genesis.json
jwt/jwtsecret
validator-keys/prysm/direct/accounts/all-accounts.keystore.json
prysm-password/prysm-password.txt
keymanager/keymanager.txt
```

`config.yaml` must specify `SLOTS_PER_ROUND: 4`, and `genesis.ssz` must be the matching fresh 120,000-validator genesis in which the wallet owns the slot 1–3 proposers. Before packaging, make `validator-keys/prysm/direct/accounts/all-accounts.keystore.json` mode `0600`; Prysm rejects an existing wallet file with broader permissions. Keep the bundle outside the repository because it contains secrets.

Kurtosis cannot upload an absolute path outside its package. Make the private bundle itself the package by copying `main.star` and `kurtosis.yml` from this directory into the bundle root, then run that directory with `bundle_path` set to `.`:

```
cp main.star kurtosis.yml /tmp/startup3-bundle/
kurtosis run /tmp/startup3-bundle '{"bundle_path":".","slots_per_round":4,"ledger":false}'
```

`ledger` defaults to false to match the sampled failing node. Set it only for a distinct diagnostic run. BN and VC processes get `GOMAXPROCS=4` without a CPU quota. REST, RPC, metrics, pprof, and EL RPC/engine ports are published by Kurtosis; use the mapped host ports shown by `kurtosis enclave inspect`.
