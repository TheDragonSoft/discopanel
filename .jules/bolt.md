## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2025-02-23 - Global regex optimization
**Learning:** Found an issue where `regexp.MustCompile` was inside function loops causing performance loss due to continuous compilation.
**Action:** Extract expensive `regexp.MustCompile` to global package level variables outside of function calls to increase backend performance significantly.
