<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		Dialog,
		DialogContent,
		DialogDescription,
		DialogHeader,
		DialogTitle
	} from '$lib/components/ui/dialog';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import {
		Users,
		Search,
		RefreshCw,
		Eye,
		Clock,
		Server as ServerIcon,
		UserX,
		ChevronLeft,
		ChevronRight,
		Activity
	} from '@lucide/svelte';
	import { create } from '@bufbuild/protobuf';
	import type {
		Player,
		OnlinePlayer,
		PlayerSession
	} from '$lib/proto/discopanel/v1/player_pb';
	import {
		ListPlayersRequestSchema,
		ListOnlinePlayersRequestSchema,
		GetPlayerRequestSchema
	} from '$lib/proto/discopanel/v1/player_pb';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { serversStore } from '$lib/stores/servers';
	import { debounce } from 'lodash-es';
	import { formatBytes, formatPlaytime, formatTimeAgo, timestampToDate } from '$lib/utils';

	const PAGE_SIZE = 100;

	let players = $state<Player[]>([]);
	let total = $state(0);
	let onlinePlayers = $state<OnlinePlayer[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);

	let servers = $derived<Server[]>($serversStore);

	// Filters
	let searchInput = $state('');
	let serverFilter = $state('');
	let offset = $state(0);

	// Detail dialog
	let detailOpen = $state(false);
	let detailLoading = $state(false);
	let selectedPlayer = $state<Player | null>(null);
	let selectedSessions = $state<PlayerSession[]>([]);

	let currentTime = $state(new Date());

	async function loadPlayers(silent = false) {
		if (silent) {
			refreshing = true;
		} else {
			loading = true;
		}
		try {
			const response = await rpcClient.player.listPlayers(
				create(ListPlayersRequestSchema, {
					serverId: serverFilter,
					search: searchInput.trim(),
					limit: PAGE_SIZE,
					offset
				}),
				silent ? silentCallOptions : undefined
			);
			players = response.players;
			total = response.total;
		} catch (error) {
			console.error('Failed to load players:', error);
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	async function loadOnlinePlayers() {
		try {
			const response = await rpcClient.player.listOnlinePlayers(
				create(ListOnlinePlayersRequestSchema, { serverId: serverFilter }),
				silentCallOptions
			);
			onlinePlayers = response.players;
		} catch (error) {
			console.error('Failed to load online players:', error);
		}
	}

	const searchPlayers = debounce(() => {
		offset = 0;
		loadPlayers();
		loadOnlinePlayers();
	}, 350);

	function applyServerFilter() {
		offset = 0;
		loadPlayers();
		loadOnlinePlayers();
	}

	function previousPage() {
		offset = Math.max(0, offset - PAGE_SIZE);
		loadPlayers(true);
	}

	function nextPage() {
		offset += PAGE_SIZE;
		loadPlayers(true);
	}

	async function openPlayerDetail(player: Player) {
		detailOpen = true;
		detailLoading = true;
		selectedPlayer = null;
		selectedSessions = [];
		try {
			const response = await rpcClient.player.getPlayer(
				create(GetPlayerRequestSchema, { id: player.id }),
				silentCallOptions
			);
			selectedPlayer = response.player ?? null;
			selectedSessions = response.sessions;
		} catch (error) {
			console.error('Failed to load player detail:', error);
		} finally {
			detailLoading = false;
		}
	}

	onMount(() => {
		loadPlayers();
		loadOnlinePlayers();
		const interval = setInterval(() => {
			currentTime = new Date();
			loadOnlinePlayers();
			loadPlayers(true);
		}, 30000);
		return () => clearInterval(interval);
	});
</script>

<div class="h-full flex-1 space-y-8 bg-linear-to-br from-background to-muted/10 p-8 pt-6">
	<div class="flex items-center justify-between border-b-2 border-border/50 pb-6">
		<div class="flex items-center gap-4">
			<div
				class="flex h-16 w-16 items-center justify-center rounded-2xl bg-linear-to-br from-primary/20 to-primary/10 shadow-lg"
			>
				<Users class="h-8 w-8 text-primary" />
			</div>
			<div class="space-y-1">
				<h2
					class="bg-linear-to-r from-foreground to-foreground/70 bg-clip-text text-4xl font-bold tracking-tight text-transparent"
				>
					Players
				</h2>
				<p class="text-base text-muted-foreground">
					Track players across your servers through the proxy
				</p>
			</div>
		</div>
		<Button
			variant="outline"
			onclick={() => {
				loadPlayers(true);
				loadOnlinePlayers();
			}}
			disabled={refreshing}
			class="border-2 shadow-sm transition-all hover:scale-[1.02] hover:shadow-md"
		>
			<RefreshCw class={`mr-2 h-5 w-5 ${refreshing ? 'animate-spin' : ''}`} />
			Refresh
		</Button>
	</div>

	<!-- Summary strip -->
	<div class="grid gap-4 md:grid-cols-3">
		<Card
			class="group relative animate-in overflow-hidden border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-2 hover:border-primary/30 hover:shadow-lg"
		>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-xs font-medium tracking-wider text-muted-foreground uppercase"
					>Known Players</CardTitle
				>
				<div
					class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-blue-500/20 to-blue-600/10 transition-transform group-hover:scale-110"
				>
					<Users class="h-5 w-5 text-blue-500" />
				</div>
			</CardHeader>
			<CardContent>
				{#if loading}
					<Skeleton class="mb-2 h-8 w-16" />
				{:else}
					<div class="text-2xl font-bold">{total}</div>
					<p class="mt-1 text-xs text-muted-foreground">tracked via proxy connections</p>
				{/if}
			</CardContent>
		</Card>

		<Card
			class="group relative animate-in overflow-hidden border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-2 hover:border-primary/30 hover:shadow-lg"
			style="animation-delay: 50ms"
		>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-xs font-medium tracking-wider text-muted-foreground uppercase"
					>Online Now</CardTitle
				>
				<div
					class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-green-500/20 to-green-600/10 transition-transform group-hover:scale-110"
				>
					<Activity class="h-5 w-5 text-green-500" />
				</div>
			</CardHeader>
			<CardContent>
				<div class="text-2xl font-bold">{onlinePlayers.length}</div>
				<p class="mt-1 text-xs text-muted-foreground">
					{onlinePlayers.length === 1 ? 'player' : 'players'} currently connected
				</p>
			</CardContent>
		</Card>

		<Card
			class="group relative animate-in overflow-hidden border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-2 hover:border-primary/30 hover:shadow-lg"
			style="animation-delay: 100ms"
		>
			<CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
				<CardTitle class="text-xs font-medium tracking-wider text-muted-foreground uppercase"
					>Currently Playing</CardTitle
				>
				<div
					class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-purple-500/20 to-purple-600/10 transition-transform group-hover:scale-110"
				>
					<Clock class="h-5 w-5 text-purple-500" />
				</div>
			</CardHeader>
			<CardContent>
				{#if onlinePlayers.length === 0}
					<div class="text-sm font-medium text-muted-foreground">Nobody online right now</div>
				{:else}
					<div class="flex flex-wrap gap-1.5">
						{#each onlinePlayers.slice(0, 12) as op (op.playerId + op.serverId)}
							<Badge variant="outline" class="border-green-500/20 bg-green-500/10 text-xs text-green-600 dark:text-green-400">
								{op.name}
								{#if op.serverName}
									<span class="ml-1 font-normal opacity-60">@ {op.serverName}</span>
								{/if}
							</Badge>
						{/each}
						{#if onlinePlayers.length > 12}
							<Badge variant="secondary" class="text-xs">+{onlinePlayers.length - 12} more</Badge>
						{/if}
					</div>
				{/if}
			</CardContent>
		</Card>
	</div>

	<!-- Filters -->
	<div class="flex flex-col gap-2 sm:flex-row">
		<div class="relative flex-1">
			<Search class="absolute top-2.5 left-2.5 h-4 w-4 text-muted-foreground" />
			<Input
				placeholder="Search players by name..."
				bind:value={searchInput}
				oninput={searchPlayers}
				class="pl-8"
			/>
		</div>
		<Select
			type="single"
			value={serverFilter}
			onValueChange={(v: string | undefined) => {
				serverFilter = v || '';
				applyServerFilter();
			}}
		>
			<SelectTrigger class="sm:w-56">
				<span>{serverFilter ? servers.find((s) => s.id === serverFilter)?.name || 'All Servers' : 'All Servers'}</span>
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="">All Servers</SelectItem>
				{#each servers as server (server.id)}
					<SelectItem value={server.id}>{server.name}</SelectItem>
				{/each}
			</SelectContent>
		</Select>
	</div>

	<!-- Players table -->
	<Card class="animate-in duration-500 fade-in-50 slide-in-from-bottom-2">
		<CardHeader>
			<CardTitle>All Players</CardTitle>
			<CardDescription>
				Showing {players.length} of {total} {total === 1 ? 'player' : 'players'}
			</CardDescription>
		</CardHeader>
		<CardContent>
			{#if loading}
				<div class="space-y-2">
					{#each Array(5) as _, i (i)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if players.length === 0}
				<div class="py-12 text-center">
					<UserX class="mx-auto mb-3 h-10 w-10 text-muted-foreground/50" />
					<h3 class="mb-1 text-sm font-semibold">No players yet</h3>
					<p class="text-sm text-muted-foreground">
						{#if searchInput || serverFilter}
							No players match your filters. Try adjusting the search or server filter.
						{:else}
							Players will appear here once they connect through the proxy.
						{/if}
					</p>
				</div>
			{:else}
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Status</TableHead>
							<TableHead>First Seen</TableHead>
							<TableHead>Last Seen</TableHead>
							<TableHead>Sessions</TableHead>
							<TableHead>Playtime</TableHead>
							<TableHead class="text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each players as player (player.id)}
							<TableRow
								class="cursor-pointer"
								onclick={() => openPlayerDetail(player)}
							>
								<TableCell class="font-medium">
									<div class="flex items-center gap-2">
										<img
											src="https://mc-heads.net/avatar/{player.name}/20"
											alt={player.name}
											class="h-5 w-5 rounded-sm"
											onerror={(e) => {
												const target = e.currentTarget as HTMLImageElement;
												target.style.display = 'none';
											}}
										/>
										{player.name}
									</div>
								</TableCell>
								<TableCell>
									{#if player.online}
										<Badge
											variant="outline"
											class="border-green-500/20 bg-green-500/10 text-xs text-green-600 dark:text-green-400"
										>
											<span class="mr-1 h-1.5 w-1.5 animate-pulse rounded-full bg-green-500"></span>
											Online
										</Badge>
									{:else}
										<Badge variant="secondary" class="text-xs text-muted-foreground">Offline</Badge>
									{/if}
								</TableCell>
								<TableCell class="text-muted-foreground">
									{timestampToDate(player.firstSeen).getTime() > 0
										? timestampToDate(player.firstSeen).toLocaleDateString()
										: '—'}
								</TableCell>
								<TableCell class="text-muted-foreground">
									{formatTimeAgo(timestampToDate(player.lastSeen), currentTime)}
								</TableCell>
								<TableCell>{player.sessionsCount}</TableCell>
								<TableCell class="font-mono text-sm">
									{formatPlaytime(player.totalPlaytimeSecs)}
								</TableCell>
								<TableCell class="text-right">
									<Button
										variant="ghost"
										size="sm"
										class="h-8 w-8 p-0"
										onclick={(e) => {
											e.stopPropagation();
											openPlayerDetail(player);
										}}
									>
										<Eye class="h-4 w-4" />
									</Button>
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>

				{#if total > PAGE_SIZE || offset > 0}
					<div class="mt-4 flex items-center justify-center gap-2">
						<Button variant="outline" size="sm" disabled={offset === 0} onclick={previousPage}>
							<ChevronLeft class="mr-1 h-4 w-4" />
							Previous
						</Button>
						<span class="text-sm text-muted-foreground">
							{offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
						</span>
						<Button
							variant="outline"
							size="sm"
							disabled={offset + PAGE_SIZE >= total}
							onclick={nextPage}
						>
							Next
							<ChevronRight class="ml-1 h-4 w-4" />
						</Button>
					</div>
				{/if}
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Player detail dialog -->
<Dialog bind:open={detailOpen}>
	<DialogContent
		class="flex max-h-[92vh] w-[95vw] max-w-3xl flex-col gap-0 overflow-hidden rounded-2xl border-2 bg-card p-0 shadow-2xl"
	>
		<DialogHeader class="border-b bg-muted/40 p-6">
			<div class="flex items-center gap-4">
				{#if selectedPlayer}
					<img
						src="https://mc-heads.net/avatar/{selectedPlayer.name}/48"
						alt={selectedPlayer.name}
						class="h-12 w-12 rounded-lg"
						onerror={(e) => {
							const target = e.currentTarget as HTMLImageElement;
							target.style.display = 'none';
						}}
					/>
				{:else}
					<div
						class="flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10 text-primary"
					>
						<Users class="h-6 w-6" />
					</div>
				{/if}
				<div class="min-w-0 flex-1">
					<DialogTitle class="truncate text-left text-xl font-bold tracking-tight">
						{selectedPlayer?.name || 'Player details'}
					</DialogTitle>
					<DialogDescription class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-left">
						{#if selectedPlayer}
							{#if selectedPlayer.online}
								<span class="flex items-center gap-1 text-green-600 dark:text-green-400">
									<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-green-500"></span>
									Online
								</span>
							{:else}
								<span>Offline</span>
							{/if}
							<span>
								{selectedPlayer.sessionsCount}
								{selectedPlayer.sessionsCount === 1 ? 'session' : 'sessions'}
							</span>
							<span>
								{formatPlaytime(selectedPlayer.totalPlaytimeSecs)} total playtime
							</span>
						{:else}
							<span>Loading...</span>
						{/if}
					</DialogDescription>
				</div>
			</div>
		</DialogHeader>

		<div class="flex-1 space-y-6 overflow-y-auto p-6">
			{#if detailLoading}
				<div class="space-y-2">
					<Skeleton class="h-8 w-full" />
					<Skeleton class="h-8 w-full" />
					<Skeleton class="h-8 w-full" />
				</div>
			{:else if selectedPlayer}
				<!-- Per-server stats -->
				<div class="space-y-3">
					<h3 class="flex items-center gap-2 text-sm font-semibold">
						<ServerIcon class="h-4 w-4 text-primary" />
						Per-Server Stats
					</h3>
					{#if selectedPlayer.serverStats.length === 0}
						<p class="text-sm text-muted-foreground">No per-server activity recorded yet.</p>
					{:else}
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Server</TableHead>
									<TableHead>Playtime</TableHead>
									<TableHead>Sessions</TableHead>
									<TableHead>Last Seen</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{#each selectedPlayer.serverStats as stat (stat.serverId)}
									<TableRow>
										<TableCell class="font-medium">{stat.serverName || stat.serverId}</TableCell>
										<TableCell class="font-mono text-sm">
											{formatPlaytime(stat.playtimeSecs)}
										</TableCell>
										<TableCell>{stat.sessionsCount}</TableCell>
										<TableCell class="text-muted-foreground">
											{formatTimeAgo(timestampToDate(stat.lastSeen), currentTime)}
										</TableCell>
									</TableRow>
								{/each}
							</TableBody>
						</Table>
					{/if}
				</div>

				<!-- Recent sessions -->
				<div class="space-y-3">
					<h3 class="text-sm font-semibold">Recent Sessions</h3>
					{#if selectedSessions.length === 0}
						<p class="text-sm text-muted-foreground">No sessions recorded yet.</p>
					{:else}
						<div class="space-y-2">
							{#each selectedSessions as session (session.id)}
								<div
									class="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 p-3"
								>
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-2">
											<ServerIcon class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
											<span class="truncate text-sm font-medium">
												{session.serverName || session.serverId}
											</span>
											{#if !session.leftAt}
												<Badge
													variant="outline"
													class="border-green-500/20 bg-green-500/10 text-[10px] text-green-600 dark:text-green-400"
												>
													Active
												</Badge>
											{/if}
										</div>
										<p class="mt-1 text-xs text-muted-foreground">
											Joined {timestampToDate(session.joinedAt).toLocaleString()}
											{#if session.leftAt}
												· Left {timestampToDate(session.leftAt).toLocaleString()}
											{/if}
										</p>
									</div>
									<div class="flex items-center gap-4 text-xs text-muted-foreground">
										<span class="font-mono">{formatPlaytime(session.durationSecs)}</span>
										<span class="font-mono">
											↓ {formatBytes(Number(session.bytesIn))} ↑ {formatBytes(Number(session.bytesOut))}
										</span>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{:else}
				<p class="py-8 text-center text-sm text-muted-foreground">Player not found.</p>
			{/if}
		</div>
	</DialogContent>
</Dialog>
