## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-10-23 - Single-pass array evaluation on Svelte components
**Learning:** Found an anti-pattern in Svelte derived values where multiple filters/reduces are used to calculate related stats from a single array, causing O(N*M) passes on every render.
**Action:** Use `$derived.by(() => { ... })` to group related calculations into a single O(N) `for...of` pass, especially for arrays that can grow large (like server lists).
