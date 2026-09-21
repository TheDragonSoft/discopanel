<script lang="ts">
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Alert, AlertDescription } from '$lib/components/ui/alert';
	import { formatBytes } from '$lib/utils';
	import {
		serverStatusBadgeClass,
		serverStatusLabel,
		ONLINE_BADGE_CLASS
	} from '$lib/utils/status-colors';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import {
		Server,
		MemoryStick,
		Plus,
		LayoutDashboard,
		AlertCircle,
		Play,
		Square,
		RotateCw,
		Clock,
		TrendingUp,
		Users,
		ChevronRight,
		Github,
		MessageCircle,
		HelpCircle,
		BookOpen,
		Shield,
		Gauge,
		Database,
		Wifi,
		WifiOff,
		CheckCircle,
		XCircle,
		AlertTriangle,
		RefreshCw
	} from '@lucide/svelte';
	import {
		ServerStatus,
		ModLoader,
		type Server as ServerType
	} from '$lib/proto/discopanel/v1/common_pb';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { serversStore, sortServersByActivity } from '$lib/stores/servers';
	import { create } from '@bufbuild/protobuf';
	import type { OnlinePlayer } from '$lib/proto/discopanel/v1/player_pb';
	import { ListOnlinePlayersRequestSchema } from '$lib/proto/discopanel/v1/player_pb';
	import { timestampToDate } from '$lib/utils';
	import type { Timestamp } from '@bufbuild/protobuf/wkt';

	// Dashboard data
	let dashboardServers: ServerType[] = $state([]);
	let isLoading = $state(true);
	let isRefreshing = $state(false);
	let currentTime = $state(new Date());
	let onlinePlayers = $state<OnlinePlayer[]>([]);
	let actionBusyId = $state<string | null>(null);

	// Quick actions (start/stop/restart) — same RPC flow as the servers page
	async function handleServerAction(action: 'start' | 'stop' | 'restart', server: ServerType) {
		actionBusyId = server.id;
		try {
			switch (action) {
				case 'start':
					await rpcClient.server.startServer({ id: server.id });
					toast.success(`Starting ${server.name}...`);
					break;
				case 'stop':
					await rpcClient.server.stopServer({ id: server.id });
					toast.success(`Stopping ${server.name}...`);
					break;
				case 'restart':
					await rpcClient.server.restartServer({ id: server.id });
					toast.success(`Restarting ${server.name}...`);
					break;
			}
			await loadDashboardData();
		} catch (error) {
			toast.error(
				`Failed to ${action} server: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			actionBusyId = null;
		}
	}

	function getModLoaderDisplay(modLoader: ModLoader): string {
		return ModLoader[modLoader].replace('_', ' ').toLowerCase();
	}

	// Online players via tracking service
	async function loadOnlinePlayers() {
		try {
			const response = await rpcClient.player.listOnlinePlayers(
				create(ListOnlinePlayersRequestSchema, {}),
				silentCallOptions
			);
			onlinePlayers = response.players;
		} catch (error) {
			console.error('Failed to load online players:', error);
		}
	}

	// Format how long an online player has been connected
	function joinedDuration(joinedAt?: Timestamp): string {
		if (!joinedAt) return '—';
		const secs = Math.max(0, Math.floor((Date.now() - timestampToDate(joinedAt).getTime()) / 1000));
		const hours = Math.floor(secs / 3600);
		const minutes = Math.floor((secs % 3600) / 60);
		if (hours > 0) return `${hours}h ${minutes}m`;
		return `${minutes}m`;
	}
	// Load dashboard data with full stats
	async function loadDashboardData() {
		try {
			const response = await rpcClient.server.listServers({ fullStats: true });
			dashboardServers = response.servers;
			serversStore.set(response.servers);
		} catch (error) {
			console.error('Failed to load dashboard data:', error);
		}
	}

	// Refresh function for manual updates
	async function refreshDashboard() {
		isRefreshing = true;
		await loadDashboardData();
		isRefreshing = false;
	}

	let stats = $derived({
		total: dashboardServers.length,
		running: dashboardServers.filter((s) => s.status === ServerStatus.RUNNING).length,
		stopped: dashboardServers.filter((s) => s.status === ServerStatus.STOPPED).length,
		error: dashboardServers.filter(
			(s) => s.status === ServerStatus.ERROR || s.status === ServerStatus.UNHEALTHY
		).length,
		totalMemory: dashboardServers.reduce((acc, s) => acc + (s.memory || 0), 0),
		usedMemory: dashboardServers
			.filter((s) => s.status === ServerStatus.RUNNING)
			.reduce((acc, s) => acc + Number(s.memoryUsage || s.memory || 0), 0),
		totalPlayers: dashboardServers
			.filter((s) => s.status === ServerStatus.RUNNING)
			.reduce((acc, s) => acc + (s.playersOnline || 0), 0),
		totalMaxPlayers: dashboardServers.reduce((acc, s) => acc + (s.maxPlayers || 0), 0),
		avgTps: dashboardServers
			.filter((s) => s.tps && s.tps > 0)
			.reduce((acc, s, _, arr) => acc + (s.tps || 0) / arr.length, 0),
		totalDiskUsage: dashboardServers.reduce((acc, s) => acc + Number(s.diskUsage || 0), 0),
		totalDiskSize:
			dashboardServers.length > 0
				? ` / ${dashboardServers?.[0]?.diskTotal && formatBytes(Number(dashboardServers[0].diskTotal))}`
				: '',
		diskFree: dashboardServers.length > 0 ? Number(dashboardServers[0].diskFree || 0) : 0,
		avgCpu: dashboardServers
			.filter((s) => s.cpuPercent && s.cpuPercent > 0)
			.reduce((acc, s, _, arr) => acc + (s.cpuPercent || 0) / arr.length, 0)
	});

	let recentActivity = $derived(
		dashboardServers
			.filter((s) => s.lastStarted)
			.sort(
				(a, b) =>
					new Date(Number(b.lastStarted!.seconds) * 1000).getTime() -
					new Date(Number(a.lastStarted!.seconds) * 1000).getTime()
			)
			.slice(0, 5)
			.map((s) => ({
				server: s.name,
				action: s.status === ServerStatus.RUNNING ? 'Started' : 'Activity',
				time: s.lastStarted,
				status: s.status
			}))
	);

	let serversByStatus = $derived({
		healthy: dashboardServers.filter(
			(s) => s.status === ServerStatus.RUNNING && (!s.tps || s.tps >= 18)
		),
		warning: dashboardServers.filter(
			(s) => s.status === ServerStatus.RUNNING && s.tps && s.tps < 18 && s.tps >= 15
		),
		critical: dashboardServers.filter(
			(s) =>
				s.status === ServerStatus.ERROR ||
				s.status === ServerStatus.UNHEALTHY ||
				(s.status === ServerStatus.RUNNING && s.tps && s.tps < 15)
		)
	});

	onMount(() => {
		// Load dashboard data on mount
		loadDashboardData().then(() => {
			isLoading = false;
		});

		// Load online players and refresh every 30s
		loadOnlinePlayers();
		const onlineInterval = setInterval(loadOnlinePlayers, 30000);

		// Update time for relative timestamps
		const interval = setInterval(() => {
			currentTime = new Date();
		}, 1000);
		return () => {
			clearInterval(onlineInterval);
			clearInterval(interval);
		};
	});

	const formatUptime = (lastStarted?: Timestamp) => {
		if (!lastStarted) return 'Never';
		const start = new Date(Number(lastStarted.seconds) * 1000);
		const diff = currentTime.getTime() - start.getTime();
		const days = Math.floor(diff / (1000 * 60 * 60 * 24));
		const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
		const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

		if (days > 0) return `${days}d ${hours}h`;
		if (hours > 0) return `${hours}h ${minutes}m`;
		return `${minutes}m`;
	};

	const getTpsColor = (tps: number | undefined) => {
		if (!tps) return 'text-gray-400';
		if (tps >= 19) return 'text-green-500';
		if (tps >= 17) return 'text-yellow-500';
		if (tps >= 15) return 'text-orange-500';
		return 'text-red-500';
	};

	const getCpuColor = (cpu: number | undefined) => {
		if (!cpu) return 'text-gray-400';
		if (cpu <= 50) return 'text-green-500';
		if (cpu <= 70) return 'text-yellow-500';
		if (cpu <= 85) return 'text-orange-500';
		return 'text-red-500';
	};
</script>

{#if isLoading}
	<div class="flex h-full flex-1 items-center justify-center p-6">
		<div class="space-y-4 text-center">
			<div
				class="mx-auto h-12 w-12 animate-spin rounded-full border-4 border-primary/30 border-t-primary"
			></div>
			<p class="text-muted-foreground">Loading dashboard...</p>
		</div>
	</div>
{:else}
	<div class="h-full flex-1 space-y-8 bg-linear-to-br from-background to-muted/10 p-8 pt-6">
		<div class="flex flex-wrap items-center justify-between gap-4 border-b-2 border-border/50 pb-6">
			<div class="flex items-center gap-4">
				<div
					class="flex h-16 w-16 animate-in items-center justify-center rounded-2xl bg-linear-to-br from-primary/20 to-primary/10 shadow-lg duration-500 fade-in-50"
				>
					<LayoutDashboard class="h-8 w-8 text-primary" />
				</div>
				<div class="animate-in space-y-1 duration-500 slide-in-from-left-5">
					<h2
						class="bg-linear-to-r from-foreground to-foreground/70 bg-clip-text text-4xl font-bold tracking-tight text-transparent"
					>
						Dashboard
					</h2>
					<p class="text-base text-muted-foreground">
						Monitor and manage your Minecraft server infrastructure
					</p>
				</div>
			</div>
			<div class="flex animate-in items-center gap-3 duration-500 slide-in-from-right-5">
				<Button
					variant="outline"
					onclick={refreshDashboard}
					disabled={isRefreshing}
					class="flex items-center gap-2 border-2 shadow-sm transition-all hover:scale-[1.02] hover:shadow-md"
				>
					<RefreshCw class="h-4 w-4 {isRefreshing ? 'animate-spin' : ''}" />
					Refresh
				</Button>
				<Button
					href="/servers/new"
					size="default"
					class="bg-linear-to-r from-primary to-primary/80 shadow-lg transition-all hover:from-primary/90 hover:to-primary/70 hover:shadow-xl"
				>
					<Plus class="mr-2 h-4 w-4" />
					New Server
				</Button>
			</div>
		</div>

		<!-- Aggregate health strip -->
		<Card class="animate-in border-border/50 duration-500 fade-in-50 slide-in-from-bottom-2">
			<CardContent class="grid grid-cols-2 gap-4 p-5 sm:grid-cols-3 lg:grid-cols-5">
				<div class="flex items-center gap-3">
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-blue-500/20 to-blue-600/10"
					>
						<Server class="h-5 w-5 text-blue-500" />
					</div>
					<div class="min-w-0">
						<p class="text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
							Servers
						</p>
						<p class="text-2xl leading-tight font-bold">{stats.total}</p>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-green-500/20 to-green-600/10"
					>
						<CheckCircle class="h-5 w-5 text-green-500" />
					</div>
					<div class="min-w-0">
						<p class="text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
							Running
						</p>
						<p class="text-2xl leading-tight font-bold text-green-500 dark:text-green-400">
							{stats.running}
						</p>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-gray-500/20 to-gray-600/10"
					>
						<XCircle class="h-5 w-5 text-gray-400" />
					</div>
					<div class="min-w-0">
						<p class="text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
							Stopped
						</p>
						<p class="text-2xl leading-tight font-bold">{stats.stopped}</p>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-red-500/20 to-red-600/10"
					>
						<AlertTriangle class="h-5 w-5 text-red-500" />
					</div>
					<div class="min-w-0">
						<p class="text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
							Issues
						</p>
						<p
							class="text-2xl leading-tight font-bold {stats.error > 0
								? 'text-red-500 dark:text-red-400'
								: ''}"
						>
							{stats.error}
						</p>
					</div>
				</div>
				<div class="flex items-center gap-3">
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-green-500/20 to-green-600/10"
					>
						<Users class="h-5 w-5 text-green-500" />
					</div>
					<div class="min-w-0">
						<p class="text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
							Players Online
						</p>
						<p class="text-2xl leading-tight font-bold">{stats.totalPlayers}</p>
					</div>
				</div>
			</CardContent>
		</Card>

		{#if serversByStatus.critical.length > 0 || serversByStatus.warning.length > 0}
			<Alert
				class="animate-in border-orange-500/20 bg-orange-500/5 duration-500 fade-in-50 slide-in-from-top-2"
			>
				<AlertCircle class="h-4 w-4 text-orange-500" />
				<AlertDescription class="ml-2">
					{#if serversByStatus.critical.length > 0}
						<span class="font-medium text-red-500"
							>{serversByStatus.critical.length} server{serversByStatus.critical.length > 1
								? 's'
								: ''} need attention.</span
						>
					{/if}
					{#if serversByStatus.warning.length > 0}
						<span class="font-medium text-yellow-500"
							>{serversByStatus.warning.length} server{serversByStatus.warning.length > 1
								? 's'
								: ''} running slow.</span
						>
					{/if}
					<a href={resolve('/servers')} class="ml-2 text-primary hover:underline">View details →</a>
				</AlertDescription>
			</Alert>
		{/if}

		<!-- Server cards -->
		<Card
			class="animate-in border-border/50 duration-500 fade-in-50 slide-in-from-bottom-2"
			style="animation-delay: 200ms"
		>
			<CardHeader>
				<div class="flex items-center justify-between gap-2">
					<div>
						<CardTitle>Servers</CardTitle>
						<CardDescription>Live status, resources and quick actions</CardDescription>
					</div>
					<Button variant="ghost" size="sm" href="/servers">
						View All
						<ChevronRight class="ml-1 h-4 w-4" />
					</Button>
				</div>
			</CardHeader>
			<CardContent>
				{#if dashboardServers.length === 0}
					<div class="py-12 text-center">
						<div
							class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted"
						>
							<Server class="h-6 w-6 text-muted-foreground" />
						</div>
						<h3 class="mb-1 text-sm font-semibold">No servers yet</h3>
						<p class="mb-4 text-sm text-muted-foreground">
							Create your first server to get started
						</p>
						<Button href="/servers/new" size="sm">
							<Plus class="mr-2 h-4 w-4" />
							Create Server
						</Button>
					</div>
				{:else}
					<div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
						{#each sortServersByActivity([...dashboardServers]) as server (server.id)}
							<div
								class="group flex flex-col rounded-xl border border-border/60 bg-card p-4 transition-colors hover:border-primary/30 hover:bg-muted/30"
							>
								<div class="flex items-start justify-between gap-2">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm font-semibold">{server.name}</p>
										<div class="mt-1.5 flex flex-wrap items-center gap-1.5">
											<Badge
												variant="outline"
												class="text-xs {serverStatusBadgeClass(server.status)}"
											>
												{serverStatusLabel(server.status)}
											</Badge>
											<Badge variant="outline" class="text-xs">{server.mcVersion}</Badge>
											{#if server.modLoader !== ModLoader.VANILLA && server.modLoader !== ModLoader.UNSPECIFIED}
												<Badge variant="outline" class="text-xs capitalize">
													{getModLoaderDisplay(server.modLoader)}
												</Badge>
											{/if}
										</div>
									</div>
									<div class="flex shrink-0 items-center gap-0.5">
										{#if server.status === ServerStatus.STOPPED || server.status === ServerStatus.ERROR}
											<Button
												variant="ghost"
												size="icon"
												class="h-9 w-9"
												disabled={actionBusyId === server.id}
												aria-label="Start {server.name}"
												title="Start {server.name}"
												onclick={() => handleServerAction('start', server)}
											>
												<Play class="h-4 w-4 text-green-500" />
											</Button>
										{/if}
										{#if server.status === ServerStatus.RUNNING || server.status === ServerStatus.UNHEALTHY || server.status === ServerStatus.STARTING}
											<Button
												variant="ghost"
												size="icon"
												class="h-9 w-9"
												disabled={actionBusyId === server.id}
												aria-label="Stop {server.name}"
												title="Stop {server.name}"
												onclick={() => handleServerAction('stop', server)}
											>
												<Square class="h-4 w-4 text-red-500" />
											</Button>
										{/if}
										{#if server.status === ServerStatus.RUNNING || server.status === ServerStatus.UNHEALTHY}
											<Button
												variant="ghost"
												size="icon"
												class="h-9 w-9"
												disabled={actionBusyId === server.id}
												aria-label="Restart {server.name}"
												title="Restart {server.name}"
												onclick={() => handleServerAction('restart', server)}
											>
												<RotateCw class="h-4 w-4 text-yellow-500" />
											</Button>
										{/if}
										<Button
											variant="ghost"
											size="icon"
											class="h-9 w-9"
											href={resolve(`/servers/${server.id}`)}
											aria-label="Manage {server.name}"
											title="Manage {server.name}"
										>
											<ChevronRight class="h-4 w-4" />
										</Button>
									</div>
								</div>
								<div class="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-border/40 pt-3">
									<div>
										<p
											class="flex items-center gap-1 text-[10px] font-medium tracking-wider text-muted-foreground uppercase"
										>
											<Users class="h-3 w-3" />
											Players
										</p>
										<p class="mt-0.5 text-sm font-semibold">
											{#if server.status === ServerStatus.RUNNING}
												{server.playersOnline || 0}
												<span class="font-normal text-muted-foreground">/ {server.maxPlayers}</span>
											{:else}
												<span class="font-normal text-muted-foreground">—</span>
											{/if}
										</p>
									</div>
									<div>
										<p
											class="flex items-center gap-1 text-[10px] font-medium tracking-wider text-muted-foreground uppercase"
										>
											<Clock class="h-3 w-3" />
											Last Start
										</p>
										<p class="mt-0.5 text-sm font-semibold">
											{#if server.lastStarted}
												{formatUptime(server.lastStarted)}
												<span class="font-normal text-muted-foreground">ago</span>
											{:else}
												<span class="font-normal text-muted-foreground">Never</span>
											{/if}
										</p>
									</div>
									<div>
										<p
											class="flex items-center gap-1 text-[10px] font-medium tracking-wider text-muted-foreground uppercase"
										>
											<Gauge class="h-3 w-3" />
											CPU
										</p>
										<p class="mt-0.5 text-sm font-semibold {getCpuColor(server.cpuPercent)}">
											{#if server.cpuPercent !== undefined && server.cpuPercent > 0}
												{server.cpuPercent.toFixed(0)}%
											{:else}
												<span class="font-normal text-muted-foreground">—</span>
											{/if}
										</p>
									</div>
									<div>
										<p
											class="flex items-center gap-1 text-[10px] font-medium tracking-wider text-muted-foreground uppercase"
										>
											<MemoryStick class="h-3 w-3" />
											RAM
										</p>
										<p class="mt-0.5 text-sm font-semibold">
											{#if server.status === ServerStatus.RUNNING && server.memoryUsage}
												{(Number(server.memoryUsage) / 1024).toFixed(1)}
												<span class="font-normal text-muted-foreground"
													>/ {(server.memory / 1024).toFixed(0)} GB</span
												>
											{:else if server.memory}
												{(server.memory / 1024).toFixed(0)}
												<span class="font-normal text-muted-foreground">GB alloc.</span>
											{:else}
												<span class="font-normal text-muted-foreground">—</span>
											{/if}
										</p>
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</CardContent>
		</Card>

		<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
			<Card
				class="animate-in border-border/50 duration-500 fade-in-50 slide-in-from-bottom-5"
				style="animation-delay: 250ms"
			>
				<CardHeader>
					<CardTitle>Recent Activity</CardTitle>
					<CardDescription>Latest server events and actions</CardDescription>
				</CardHeader>
				<CardContent>
					{#if recentActivity.length === 0}
						<div class="py-8 text-center text-muted-foreground">
							<Clock class="mx-auto mb-2 h-8 w-8" />
							<p class="text-sm">No recent activity</p>
						</div>
					{:else}
						<div class="space-y-3">
							{#each recentActivity as activity, i (activity.server)}
								<div
									class="flex animate-in items-start gap-3 fade-in-50 slide-in-from-right-2"
									style="animation-delay: {300 + i * 50}ms"
								>
									<div class="mt-1">
										{#if activity.status === ServerStatus.RUNNING}
											<div class="h-2 w-2 animate-pulse rounded-full bg-green-500"></div>
										{:else if activity.status === ServerStatus.STOPPED}
											<div class="h-2 w-2 rounded-full bg-gray-400"></div>
										{:else}
											<div class="h-2 w-2 rounded-full bg-yellow-500"></div>
										{/if}
									</div>
									<div class="flex-1 space-y-1">
										<p class="text-sm">
											<span class="font-medium">{activity.server}</span>
											<span class="ml-1 text-muted-foreground">{activity.action}</span>
										</p>
										<p class="text-xs text-muted-foreground">
											{formatUptime(activity.time)} ago
										</p>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>

			<Card
				class="group animate-in border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-5 hover:border-primary/30 hover:shadow-lg"
				style="animation-delay: 300ms"
			>
				<CardHeader>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-3">
							<div
								class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-green-500/20 to-green-600/10"
							>
								<Users class="h-5 w-5 text-green-500" />
							</div>
							<div>
								<CardTitle class="text-base">Online Now</CardTitle>
								<CardDescription class="text-xs">Tracked via proxy connections</CardDescription>
							</div>
						</div>
						<Badge variant="outline" class={ONLINE_BADGE_CLASS}>
							{onlinePlayers.length}
						</Badge>
					</div>
				</CardHeader>
				<CardContent>
					{#if onlinePlayers.length === 0}
						<div class="py-6 text-center text-muted-foreground">
							<Users class="mx-auto mb-2 h-8 w-8 opacity-40" />
							<p class="text-sm">No players online</p>
						</div>
					{:else}
						<div class="max-h-56 scrollbar-thin space-y-2 overflow-y-auto pr-1">
							{#each onlinePlayers as op (op.playerId + op.serverId)}
								<div
									class="flex items-center justify-between gap-2 rounded-lg border bg-muted/30 px-3 py-2"
								>
									<div class="flex min-w-0 items-center gap-2">
										<img
											src="https://mc-heads.net/avatar/{op.name}/20"
											alt={op.name}
											class="h-5 w-5 rounded-sm"
											onerror={(e) => {
												const target = e.currentTarget as HTMLImageElement;
												target.style.display = 'none';
											}}
										/>
										<div class="min-w-0">
											<p class="truncate text-sm font-medium">{op.name}</p>
											<p class="truncate text-xs text-muted-foreground">
												{op.serverName || op.serverId}
											</p>
										</div>
									</div>
									<span class="shrink-0 font-mono text-xs text-muted-foreground">
										{joinedDuration(op.joinedAt)}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</CardContent>
			</Card>

			<Card
				class="group animate-in border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-5 hover:border-primary/30 hover:shadow-lg"
				style="animation-delay: 350ms"
			>
				<CardHeader>
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-3">
							<div
								class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-blue-500/20 to-blue-600/10"
							>
								<HelpCircle class="h-5 w-5 text-blue-500" />
							</div>
							<div>
								<CardTitle class="text-base">Need Help?</CardTitle>
								<CardDescription class="text-xs">Get support from our community</CardDescription>
							</div>
						</div>
					</div>
				</CardHeader>
				<CardContent class="space-y-3">
					<Button
						variant="outline"
						class="w-full justify-start gap-3 transition-all hover:border-primary/50 hover:bg-muted/50"
						onclick={() => window.open('https://discord.gg/6Z9yKTbsrP', '_blank')}
					>
						<MessageCircle class="h-4 w-4 text-[#5865F2]" />
						<span class="flex-1 text-left">Join Discord Server</span>
						<ChevronRight class="h-4 w-4 text-muted-foreground" />
					</Button>
					<Button
						variant="outline"
						class="w-full justify-start gap-3 transition-all hover:border-primary/50 hover:bg-muted/50"
						onclick={() => window.open('https://github.com/nickheyer/discopanel/issues', '_blank')}
					>
						<Github class="h-4 w-4" />
						<span class="flex-1 text-left">Report an Issue</span>
						<ChevronRight class="h-4 w-4 text-muted-foreground" />
					</Button>
					<Button
						variant="outline"
						class="w-full justify-start gap-3 transition-all hover:border-primary/50 hover:bg-muted/50"
						onclick={() => window.open('https://github.com/nickheyer/discopanel', '_blank')}
					>
						<BookOpen class="h-4 w-4 text-green-500" />
						<span class="flex-1 text-left">Documentation</span>
						<ChevronRight class="h-4 w-4 text-muted-foreground" />
					</Button>
				</CardContent>
			</Card>

			<Card
				class="animate-in border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-5 hover:border-primary/30 hover:shadow-lg"
				style="animation-delay: 400ms"
			>
				<CardHeader>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-green-500/20 to-green-600/10"
						>
							<Shield class="h-5 w-5 text-green-500" />
						</div>
						<div>
							<CardTitle class="text-base">System Health</CardTitle>
							<CardDescription class="text-xs">Overall infrastructure status</CardDescription>
						</div>
					</div>
				</CardHeader>
				<CardContent>
					<div class="space-y-3">
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">Services</span>
							<div class="flex items-center gap-1">
								<CheckCircle class="h-4 w-4 text-green-500" />
								<span class="text-sm font-medium">Operational</span>
							</div>
						</div>
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">Network</span>
							<div class="flex items-center gap-1">
								{#if dashboardServers.some((s) => s.status === ServerStatus.RUNNING)}
									<Wifi class="h-4 w-4 text-green-500" />
									<span class="text-sm font-medium">Connected</span>
								{:else}
									<WifiOff class="h-4 w-4 text-gray-400" />
									<span class="text-sm text-muted-foreground">Idle</span>
								{/if}
							</div>
						</div>
						<div class="flex items-center justify-between">
							<span class="text-sm text-muted-foreground">Storage</span>
							<div class="flex items-center gap-1">
								{#if stats.totalDiskUsage > 0}
									<Database class="h-4 w-4 text-blue-500" />
									<span class="text-sm font-medium"
										>{formatBytes(stats.totalDiskUsage)}{stats.totalDiskSize}</span
									>
									{#if stats.diskFree > 0}
										<span class="text-xs text-muted-foreground"
											>· {formatBytes(stats.diskFree)} free</span
										>
									{/if}
								{:else}
									<Database class="h-4 w-4 text-gray-400" />
									<span class="text-sm text-muted-foreground">No data</span>
								{/if}
							</div>
						</div>
					</div>
				</CardContent>
			</Card>

			<Card
				class="animate-in border-border/50 transition-all duration-500 fade-in-50 slide-in-from-bottom-5 hover:border-primary/30 hover:shadow-lg"
				style="animation-delay: 450ms"
			>
				<CardHeader>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-xl bg-linear-to-br from-purple-500/20 to-purple-600/10"
						>
							<TrendingUp class="h-5 w-5 text-purple-500" />
						</div>
						<div>
							<CardTitle class="text-base">Quick Stats</CardTitle>
							<CardDescription class="text-xs">Server performance metrics</CardDescription>
						</div>
					</div>
				</CardHeader>
				<CardContent>
					<div class="grid grid-cols-2 gap-4">
						<div class="space-y-1">
							<p class="text-xs text-muted-foreground">Uptime</p>
							<p class="text-xl font-bold">
								{stats.running > 0
									? `${((stats.running / Math.max(stats.total, 1)) * 100).toFixed(0)}%`
									: '—'}
							</p>
						</div>
						<div class="space-y-1">
							<p class="text-xs text-muted-foreground">Load</p>
							<p class="text-xl font-bold {getCpuColor(stats.avgCpu)}">
								{stats.avgCpu > 0 ? `${stats.avgCpu.toFixed(0)}%` : '—'}
							</p>
						</div>
						<div class="space-y-1">
							<p class="text-xs text-muted-foreground">Avg TPS</p>
							<p class="text-xl font-bold {getTpsColor(stats.avgTps)}">
								{stats.avgTps > 0 ? stats.avgTps.toFixed(1) : '—'}
							</p>
						</div>
						<div class="space-y-1">
							<p class="text-xs text-muted-foreground">Players</p>
							<p class="text-xl font-bold text-green-500">
								{stats.totalPlayers}
							</p>
						</div>
					</div>
				</CardContent>
			</Card>
		</div>
	</div>
{/if}
