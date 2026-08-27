### Changed

- `GET /eth/v1/beacon/states/{state_id}/finality_checkpoints` and the `finalized_checkpoint`
  event now report epoch-valued checkpoints (epoch of the round's FFG target slot, with the
  epoch-boundary block root) so stock beacon-API consumers read finality correctly; the raw
  round checkpoint is carried in additive `round`/`round_root` fields.
