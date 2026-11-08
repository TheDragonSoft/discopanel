## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-25 - Avoid N+1 and full row loads in DB lookups
**Learning:** Found massive N+1 query loop searching for available proxy ports which fetched entire Module structs via `Find()`. Because `Module` contains several JSON-serialized large fields (like config/overrides/hooks), this consumed excessive memory.
**Action:** Always pre-fetch data outside of loops. Use `Select("id", "ports")` in GORM queries when checking availability to avoid allocating memory for large unused JSON fields.
