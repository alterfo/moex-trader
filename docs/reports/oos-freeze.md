# OOS freeze (Task 10)

The deployed abs-10d recipe is frozen as of 2026-09-17. The next two quarters
(at least 183 days of collection) must run against this exact recipe without
modification. Any change to the model artifact or the live-relevant config
fields resets the collection count to zero.

## Frozen artifact

- Manifest: `artifacts/oos/freeze.json`
- Frozen at: 2026-09-17T16:09:40Z
- Strategy code SHA: 30a422a9eb7193ab9300d00b4ec7c9190b98e94e
- Recipe SHA256: 7d6e616c8973e8d2c32110e049b5a539c616ba367fab86732ac0aa850e51de9e
- Model SHA256: feb827494ba0bc943ab722d1dff888e0fe4c523bd170d463a5e3ca2fa11e6140
- Config SHA256: 54c8d1949fd8c3b514c8a2b9c23a6c9048d7e20a433ad24038ab8eec6540b1ef
- Ensemble: `ensemble_model.json`
- Config: `config.sandbox.yaml`

## Collection rule

The collection start is the manifest `frozen_at`. The minimum collection
window is 183 days (two quarters). `cmd/oosfreeze` recomputes the recipe hash
from the current model and config; when it differs from the recorded hash the
reported collection days reset to zero and a new freeze must be recorded.

Check status:

```sh
go run ./cmd/oosfreeze -action status -config config.sandbox.yaml -out artifacts/oos/freeze.json
```

Record a new freeze after an intentional recipe change:

```sh
go run ./cmd/oosfreeze -action record -config config.sandbox.yaml -out artifacts/oos/freeze.json
```

## Real-money policy

No real-money actions happen while this data accumulates. The manifest records
`real_money_enabled: false`, and `oosfreeze.Manifest.Validate` rejects a
manifest that enables real-money execution. The sandbox remains active only as
data collection and infrastructure validation.
