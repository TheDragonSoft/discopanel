## Analysis
The application has a performance issue in `GetMinecraftVersions` due to an O(N^2) loop where `GetAllVersions()` is called and then it loops through each version to call `GetVersionInfo(versionID)`. Both functions acquire a lock and do a small amount of work, but looping through all versions inside `GetVersionInfo` results in O(N) work being done N times, creating an O(N^2) loop.
Since there are roughly 1000 Minecraft versions, this does around 1,000,000 loop iterations. By simply returning all version infos at once in an O(N) function, we can speed this up significantly and save a few milliseconds on the RPC call and avoid unnecessary lock contention.

I will implement a `GetAllVersionInfos()` function in `internal/minecraft/versions.go` that returns `[]Version` in one go. Then I will refactor `GetMinecraftVersions` to use it.

## Plan
1.  **Add `GetAllVersionInfos()` to `internal/minecraft/versions.go`.**
    -   This function will fetch the manifest once and return `manifest.Versions`.
2.  **Refactor `GetMinecraftVersions()` in `internal/rpc/services/minecraft.go`.**
    -   Replace the `GetAllVersions()` and `GetVersionInfo()` loop with a single call to `GetAllVersionInfos()`.
3.  **Run tests.**
    -   Make sure `go test ./...` passes.
4.  **Complete pre commit steps.**
    -   Follow standard verification procedures to ensure proper testing, verification, review, and reflection are done.
5.  **Submit PR.**
    -   Create a PR with a description matching Bolt's preferred format.
