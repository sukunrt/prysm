# Plan-final: the finality gadget era

Successor to `plan-next.md` (Goldfish, timing knobs, harnesses — executed
2026-08-20). This file collects the decisions and work items for the real
finality gadget; it becomes a full plan when that work is scoped.

## Items

1. **Per-round finality vote target (fast finality).** The finality vote
   in the round starting at slot N targets the block at slot N-1 — a
   per-ROUND target; that is what fast finality means (user, 2026-08-20).
   The per-epoch FFG target shift (`StartSlot(E) - 1`, plan-next step 5.2)
   is the interim mock and stays as is until the gadget's round-based
   finality-vote stream lands; this item replaces it then. Also tracked in
   the global todo (`per-round-finality-vote-target`).
