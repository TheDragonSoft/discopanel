## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-06-25 - Push JSON Filtering to DB Layer (Cross-Dialect)
**Learning:** Avoid inefficient in-memory filtering for JSON array columns. Filtering a large dataset of JSON arrays in Go causes unnecessary memory allocations and loop overhead. However, dialect-specific functions like `json_each` break cross-database compatibility when using GORM (e.g. Postgres vs MySQL vs SQLite).
**Action:** Use a cross-dialect string `LIKE` query to filter JSON arrays at the database layer safely, like `Where("column LIKE ?", "%[1]%").Or("column LIKE ?", "%[1,%").Or(...)`.
