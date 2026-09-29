## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-05-24 - API Calls Inside Loops Over Caches
**Learning:** Calling `GetVersionInfo(id)` inside a loop over all IDs, when both use the same underlying cached array (`manifest.Versions`), creates a hidden O(N²) iteration bottleneck (millions of iterations for a ~1400 item list).
**Action:** Expose a method to return the full cached slice (or a copy of it) in one O(N) operation instead of forcing callers to iterate and fetch individually.
