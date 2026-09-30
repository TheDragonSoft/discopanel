## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2025-02-27 - Precomputing Regexes
**Learning:** Found a performance bottleneck where `regexp.MustCompile` was called repeatedly inside functions (`ParseTPSFromOutput`, `stripChatFormatting`, etc.) that are likely called frequently. Regex compilation is expensive and causes unnecessary allocations.
**Action:** Extract `regexp.MustCompile` calls out of functions and assign them to global variables so they are compiled only once during initialization.
