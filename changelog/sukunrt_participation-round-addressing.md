### Added

- `GET /prysm/v1/validators/{state_id}/participation` accepts `?round=N` to sample a finished
  round's participation, and reports the round-scoped balances under honest
  `previous_round_*_gwei` names plus the `round` they describe.
