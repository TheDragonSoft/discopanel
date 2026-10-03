## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2025-02-04 - Optimize querying large JSON arrays in SQLite
**Learning:** Querying database fields formatted as large JSON arrays (like module ports or scheduled task event triggers) and filtering them locally via a loop in Go was a performance bottleneck in Go API (`GetModuleByHostPort`, `CheckPortAvailability`, and `ListEventTriggeredTasks`). SQLite supports querying json payload array directly.
**Action:** Used `json_extract` or `json_each` natively via `s.db.WithContext(ctx).Where("EXISTS (SELECT 1 FROM json_each(json_field_name) WHERE value = ?)")` or `Where("EXISTS (SELECT 1 FROM json_each(json_field_name) WHERE json_extract(value, '$.property') = ?)")` to do the filtering on the SQLite level and prevent the huge memory allocation from SQLite to Go, significantly speeding up data reads (100x improvement from tests).
