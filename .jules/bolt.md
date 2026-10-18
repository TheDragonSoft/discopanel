## 2024-05-24 - Precomputing Tokens in Loop
**Learning:** Found an O(N) allocation bottleneck where map creation (tokenSet) was inside a loop over candidates in string matching.
**Action:** Extract expensive normalizations outside of loops when matching one query against many candidates.

## 2024-05-24 - O(N) Database Queries in Loops
**Learning:** Checking for resource availability (like ports) by querying the database in a loop for each possible resource can lead to massive O(N*M) complexity and thousands of unnecessary database round trips.
**Action:** When searching for available resources among many candidates, fetch all allocated resources in a single query first, build an O(1) lookup map (like a hash map of used ports), and perform the check locally in memory.
