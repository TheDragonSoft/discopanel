## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - N+1 APIs pattern
**Learning:** Found N+1 docker API calls pattern inside loops when extracting statuses. Also learned that `internal/db` is aliased as `storage` and `models` inside `internal/rpc/services` and `internal/docker` files which caused confusion with compiler errors.
**Action:** Extract status loading outside of loops to use bulk retrieval (like `ContainerList`) and be aware of package aliases.
