### Added

- `--enable-waggle` flag. It starts the waggle protocol on the libp2p host.
- `--waggle-attestations` flag. It makes a waggle topic for each attestation subnet and follows the gossipsub subscriptions. It implies `--enable-waggle`.
- EIP-8334 `AttestationBundle` and `CommitteeAttestationPartsMetadata` containers, and their waggle wire format.
- Attestation propagation over waggle (EIP-8334): push to D peers, available lists to K peers, and request and serve. Attestations from waggle go through the gossip validation, and then to the pool and to gossipsub.
- Flags for the waggle attestation parameters: `--waggle-push-peers`, `--waggle-push-interval`, `--waggle-metadata-peers`, `--waggle-metadata-interval`, `--waggle-request-peers`, `--waggle-request-timeout` and `--waggle-max-validations`.
- Waggle keeps attestation state while the attestations are valid for gossip, so late attestations also go over waggle. An attestation goes in available lists for 3s after it arrives. After 6s, waggle keeps only its validator, for dedup.
- Waggle records the peers that send each attestation that fails validation, for peer scoring later.
- Waggle keys attestations by (data root, validator). It propagates up to 2 votes of a validator in a slot, so that a double vote reaches the slashers. `CommitteeAttestationPartsMetadata` has a `data_root` field.
