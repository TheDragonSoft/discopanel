## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - Caching static data with sync.Once
**Learning:** Found a function `GetAllModLoaders()` allocating a slice of 27 `ModLoaderInfo` objects every time it was called (27 allocs/op, ~4300 ns/op). This was used in a busy path during string matching.
**Action:** Use `sync.Once` to build static reference slices once and return the cached slice on subsequent calls. This eliminated allocations and reduced time to ~3 ns/op.
