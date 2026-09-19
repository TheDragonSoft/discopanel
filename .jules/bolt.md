## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-09-12 - Eliminate Allocations in Hot Paths (String Matching)
**Learning:** Found significant allocation bottlenecks in string matching (`pkg/strmatch`) caused by string array allocation in `strings.FieldsFunc` and large row allocations in Levenshtein distance calculations. Go string slicing `c[start:i]` is cheap, making zero-allocation token scanning possible. Levenshtein can be optimized to 1 allocation per call or 0 allocations using a small stack array `[64]int` and an optimal 1D DP array.
**Action:** Always prefer zero-allocation parsing with indexes and stack arrays over heap allocations in hot paths like `Score` functions.
