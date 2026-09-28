## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-09-28 - Regex MustCompile in functions
**Learning:** Common pattern in Go codebase where regexp.MustCompile is called inside a frequently executed function (like output parsers). This causes parsing and building the regex state machine repeatedly.
**Action:** Extracted static Regexs into package-level variables using regexp.MustCompile.
