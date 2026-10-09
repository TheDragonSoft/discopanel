1. **Add `GetContainerStatuses` to `internal/docker/client.go`**
   - Create a method `GetContainerStatuses(ctx context.Context, containerIDs []string) (map[string]models.ServerStatus, error)`.
   - Call `c.docker.ContainerList(ctx, container.ListOptions{All: true})`.
   - Loop through the result, and map each container ID to its `models.ServerStatus` based on its `State` (and health status if available, which might require `ContainerInspect` for just the running ones, but we can do a quick mapping based on `State`). Wait, `ContainerList` might not include full health info unless filtered or inspected. Let's see what `types.Container` struct from `ContainerList` contains. It has a `State` string, and `Status` string (like "Up 2 days (healthy)"). We can parse `Status` string for health, e.g. `strings.Contains(c.Status, "(healthy)")`.

   If `c.State == "running"`:
     if `strings.Contains(c.Status, "(healthy)")` -> `models.StatusRunning`
     else if `strings.Contains(c.Status, "(unhealthy)")` -> `models.StatusUnhealthy`
     else if `strings.Contains(c.Status, "(starting)")` -> `models.StatusStarting`
     else -> `models.StatusRunning`

2. **Update `ListServers` in `internal/rpc/services/server.go`**
   - Extract all non-empty `ContainerID` from `servers`.
   - Call `s.docker.GetContainerStatuses(ctx, containerIDs)`.
   - In the `for _, server := range servers` loop, use the map to set `server.Status` instead of calling `GetContainerStatus` individually.

3. **Pre-commit checks**
   - Complete pre-commit steps to make sure proper testing, verifications, reviews and reflections are done.

4. **Submit the PR**
   - Submit the change using `submit` tool with title '⚡ Bolt: Optimize container status lookups in ListServers' and explain what was changed, why, and the impact (resolves N+1 API calls to Docker socket).
