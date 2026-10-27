## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-05-24 - Unnecessary Preload in Authentication Path
**Learning:** Found `Preload("User")` being called during `GetSession`, which executes an extra database query on every request (since it's part of the authentication pipeline). The preloaded `User` object was immediately discarded because `auth/manager.go` re-fetched the user manually.
**Action:** Remove `Preload("User")` from `GetSession` to save a database query on every authenticated API request.
