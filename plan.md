1. **Optimize `CheckPortAvailability`:**
   - Update `CheckPortAvailability` in `internal/db/store.go` to use `.Select("id", "name", "ports")` when fetching modules. This avoids loading large, unnecessary fields (like JSON configurations, metadata, and dependencies) for every module when simply checking for port conflicts, optimizing memory usage based on the codebase's GORM JSON serializer patterns.

2. **Optimize `GetModuleByHostPort`:**
   - Update `GetModuleByHostPort` in `internal/db/store.go` to fetch only `id` and `ports` initially. If a match is found, query and return the full module. This saves significant memory by not loading all full modules.

3. **Eliminate N+1 Queries in `FindAvailableListenerPort`:**
   - Refactor `FindAvailableListenerPort` to pre-fetch all module ports and build the `usedPorts` map before the loop. Currently, it calls `CheckPortAvailability` inside a loop that can run thousands of times (from 25565 to 65535). This causes thousands of full-table scans of the modules table. Pre-fetching avoids this O(P * M) bottleneck.

4. **Run Pre-Commit Checks:**
   - Ensure proper testing, verification, review, and reflection are done by running `go test ./...` and linters.

5. **Submit PR:**
   - Submit the PR with the performance optimization details.
