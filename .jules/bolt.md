## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-05-24 - String Matching Optimizations
**Learning:** Found multiple bottlenecks in string searching: dynamic programming allocation in Levenshtein (2 arrays instead of 1), high allocation in `strings.FieldsFunc` due to closure and rune usage for tokenizing, and unneeded computation of components that cannot affect the final string match score.
**Action:** Use 1D arrays for dynamic programming grid where only current and previous rows are needed. Avoid `strings.FieldsFunc` in hot loops. Add early returns based on known boundaries.
