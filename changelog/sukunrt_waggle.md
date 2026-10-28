### Added

- `--enable-waggle` flag. It starts the waggle protocol on the libp2p host.
- `--waggle-attestations` flag. It makes a waggle topic for each attestation subnet and follows the gossipsub subscriptions. It implies `--enable-waggle`.
- EIP-8334 `AttestationBundle` and `CommitteeAttestationPartsMetadata` containers, and their waggle wire format.
