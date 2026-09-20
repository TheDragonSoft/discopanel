## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - Levenshtein Distance Optimization
**Learning:** Found a dynamic programming implementation (Levenshtein distance) in `pkg/strmatch` creating two slices `O(2*len)` on every comparison, and doing internal function calls on an `O(N*M)` nested loop.
**Action:** Replace `O(2*len)` dynamic programming states with an `O(len)` array and a diagonal tracker variable, and inline simple helper functions (like min3) inside tight loops.
