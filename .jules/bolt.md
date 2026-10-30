## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-10-30 - Unnecessary Database Preloads (N+1 equivalent)
**Learning:** Found `Preload("User")` on a `Session` fetch where the preloaded association was never read, and the `Auth Manager` immediately ran an explicit `GetUser` via another query based on the `userID` claims in a token. This adds useless SELECT statement overhead during every single session verification.
**Action:** Remove unnecessary `.Preload()` calls on records where relations are either explicitly fetched downstream or entirely unused, particularly in hot paths like session validation.
