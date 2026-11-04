1. **Optimize memory allocation in `GetModuleByHostPort` and `CheckPortAvailability` in `internal/db/store.go`**
   - The current code uses `s.db.WithContext(ctx).Find(&modules)` which loads all columns (including large fields like Metadata, Dependencies, Configs) of all Modules into memory just to check their ports.
   - We will change the initial query to only `Select("id", "ports")` into a slice of partial `Module` structs.
   - If a matching port is found, we will query the full `Module` using the matching ID and return it (or the `PortConflict` containing it).
   - This directly follows the rule: "In GORM models within this project, fields using the JSON serializer... To optimize memory when querying, select only the JSON column rather than loading the entire struct if other fields are unnecessary."
2. **Review performance impact**
   - Less memory allocation for all the unneeded fields of all module rows when checking port conflicts or lookups.
   - Faster deserialization from the database.
   - We will write a benchmark or add a performance comment.
