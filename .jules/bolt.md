## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2023-11-20 - Optimize DB JSON Queries
**Learning:** Found a memory bottleneck where GORM was loading entire structs into memory just to check a single JSON column in `GetModuleByHostPort` and `CheckPortAvailability`.
**Action:** Use `.Select()` in GORM to only fetch the ID and the needed JSON column when querying large tables, then fetch the full struct only if a match is found to optimize memory usage.
