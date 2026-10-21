## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - Avoiding Massive JSON Allocations in DB Queries
**Learning:** Found that `Find(&modules)` in methods like `CheckPortAvailability` loaded huge JSON blobs (like `Config`, `EnvOverrides`) for every module just to check the `Ports` array.
**Action:** When filtering in code rather than via DB queries (e.g. for cross-dialect compatibility with JSON fields), use GORM's `.Select()` to fetch only necessary fields (e.g. `Select("id", "ports")`) to drastically reduce memory allocations and serialization overhead.
