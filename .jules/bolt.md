## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - Precomputing Strings in Derived Blocks
**Learning:** Found O(N) repetitive computation bottleneck where string `toLowerCase()` conversions occurred inside Svelte `$derived` filter and walk loops.
**Action:** Extract expensive `toLowerCase()` standardizations outside of loops when filtering or searching arrays by wrapping in `$derived.by()` blocks to run just once per reactivity cycle.
