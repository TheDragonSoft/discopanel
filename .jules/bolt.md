## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2025-02-18 - Avoid Repeated Slice/Array Searching on RPC endpoints
**Learning:** Found an O(N^2) latency bottleneck in `GetMinecraftVersions` where the RPC endpoint called `GetAllVersions` (fetching version IDs) and then looped over each ID calling `GetVersionInfo` (which internally re-searched the full manifest array).
**Action:** Expose a bulk getter like `GetAllVersionInfos` to replace "List + Get-Each" patterns with a single O(N) operation to avoid repeated linear searching inside loops on API calls.
