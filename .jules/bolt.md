## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-24 - Expensive Regular Expressions in Log Parsers
**Learning:** Found multiple dynamic `regexp.MustCompile` calls in frequently called functions (like `ParseTPSFromOutput` and chat log parsers). Compiling regex at runtime inside functions repeatedly causes significant performance overhead and unnecessary garbage collection allocations.
**Action:** Extract all `regexp.MustCompile` calls that use static patterns to package-level variables. Use `regexp.MustCompile` at init time rather than inside function execution paths.
