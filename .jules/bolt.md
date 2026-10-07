## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-10-07 - Levenshtein []rune and array allocation Bottleneck
**Learning:** Found an O(N) allocation bottleneck in string matching (`pkg/strmatch/strmatch.go`) where `levenshtein(a, b)` was continuously allocating `[]rune` for the input strings and dynamic arrays for `prev` and `curr` dynamic programming rows. This led to massive heap thrashing and degraded performance.
**Action:** Use a `[64]int` and `[64]rune` stack array for calculating DP rows for small strings (<= 64 runes), and decode the outer string character by character within the outer loop using `range` to avoid converting everything into slice of runes. This avoids O(N) dynamic allocations in string searches.
