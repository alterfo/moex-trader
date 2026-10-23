# Agent hooks

## arnold-guard.sh

Reminds the agent about structural stability when editing tuned trading parameters.

### Usage

Manual:
```bash
./scripts/hooks/arnold-guard.sh config.sandbox.yaml internal/risk/gate.go
```

With git staged files:
```bash
git diff --cached --name-only | xargs ./scripts/hooks/arnold-guard.sh
```

### OpenCode / Claude Code / Kilo
Add as a pre-edit/pre-write hook pointing to this script. The script exits 0 if nothing sensitive is touched so it won't block normal edits.

### Why
Based on V.I. Arnold "Hard and soft mathematical models". Optimizing to the historical maximum sits on the boundary of stability; prefer state-dependent rules and test perturbations.
