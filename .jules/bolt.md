## 2026-08-30 - Svelte 5 $derived.by() Optimization
**Learning:** Chained array methods (.filter, .reduce, .map) inside Svelte 5 $derived blocks trigger unnecessary re-evaluations and memory allocations.
**Action:** Use $derived.by() with a single O(N) for...of loop to perform multiple aggregations and filtering in a single pass.
