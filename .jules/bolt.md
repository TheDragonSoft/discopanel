## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-09-18 - Levenshtein Distance Allocations
**Learning:** The Levenshtein distance algorithm used two arrays causing excessive allocation per comparison. A 1D array reduces allocation footprint by half, and iterating over the shorter string guarantees minimal space complexity.
**Action:** Always verify string distance DP implementations to use O(min(m,n)) space complexity rather than O(max(m,n)) or O(N^2).
