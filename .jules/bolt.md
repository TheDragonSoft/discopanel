## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.
## 2023-11-20 - Optimizing levenshtein string distance algorithm
**Learning:** The levenshtein string distance function `pkg/strmatch/levenshtein` allocated two `[]rune` arrays out of `string` inputs for iteration, and 2 `[]int` slices in every call for computing the matrix distance values. When used in hot loops during string matching (for instance, when indexing, searching or autocompleting tokens against lots of candidates), this caused a high number of tiny allocations, pushing the Garbage Collector.
**Action:** Reimplemented `levenshtein` distance using `utf8.RuneCountInString` to know the dimension sizes, then iterate over string runes directly, without casting the strings to `[]rune`. Only a single fixed-size array is allocated, instead of allocating two, avoiding heap allocations if the strings are small (<64 chars), dramatically reducing memory allocations during scoring processes.
