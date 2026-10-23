## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2025-02-23 - DP 1D Array and Min3 Inlining
**Learning:** In string matching algorithms like `levenshtein`, using two slices inside loops leads to high allocation counts and garbage collection pressure, especially for short strings. Small strings are matched frequently so allocations dominate the execution time.
**Action:** Instead of two dynamically allocated arrays per call, use a 1D slice over a pre-allocated stack-allocated array (e.g. `[64]int`). Also inline highly-frequently-called helpers like `min3` inside tight loops in Go to eliminate function call overhead.
