## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2024-05-25 - Pre-compiling Regex in Loops
**Learning:** Found an O(N) allocation bottleneck where `regexp.MustCompile` was inside frequently-called functions. Compiling regexes dynamically inside functions takes ~50,000ns per call versus ~2,000ns for precompiled regex variables.
**Action:** Extract `regexp.MustCompile` calls outside of functions into package-level global variables when they are frequently used on server logs or generic output.
