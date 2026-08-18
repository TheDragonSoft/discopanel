<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { toast } from 'svelte-sonner';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Alert, AlertDescription } from '$lib/components/ui/alert';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		Loader2,
		Globe,
		Save,
		Copy,
		AlertCircle,
		CheckCircle2,
		XCircle,
		ExternalLink,
		Radio,
		Sparkles,
		Play,
		Square,
		RotateCw,
		Trash2,
		Terminal,
		Gamepad2,
		Smartphone,
		MapPin,
		Plus,
		Check,
		ShieldCheck
	} from '@lucide/svelte';
	import { copyToClipboard as copyText } from '$lib/utils/clipboard';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { ServerStatus } from '$lib/proto/discopanel/v1/common_pb';
	import type { GetServerRoutingResponse, ProxyRoute } from '$lib/proto/discopanel/v1/proxy_pb';
	import {
		TunnelProvider,
		TunnelProtocol,
		TunnelStatus,
		type Tunnel
	} from '$lib/proto/discopanel/v1/tunnel_pb';

	let {
		server,
		active,
		router: routingInfo = $bindable(null)
	}: { server: Server; active?: boolean; router?: GetServerRoutingResponse | null } = $props();

	// Reverse Proxy state
	let loading = $state(true);
	let saving = $state(false);
	let hostname = $state('');
	let originalHostname = $state('');
	let hasChanges = $derived(hostname !== originalHostname);
	let allRoutes = $state<ProxyRoute[]>([]);
	let hostnameError = $state('');
	let initialized = $state(false);
	let previousServerId = $state(server.id);

	// Tunnel (Playit.gg) state
	let tunnels = $state<Tunnel[]>([]);
	let hasGlobalAccount = $state(false);
	let tunnelsLoading = $state(true);
	let creatingTunnel = $state(false);
	let tunnelActionLoading = $state<Record<string, boolean>>({});
	let customPresetOpen = $state(false);
	let customPort = $state<number>(server.port || 25565);
	let customProtocol = $state<'tcp' | 'udp' | 'both'>('tcp');
	let customName = $state('');

	// Tunnel logs modal state
	let logsModalOpen = $state(false);
	let activeLogTunnel = $state<Tunnel | null>(null);
	let tunnelLogs = $state<string[]>([]);
	let logsLoading = $state(false);

	let pollInterval: ReturnType<typeof setInterval> | null = null;

	onMount(() => {
		if (server && !initialized) {
			initialized = true;
			loadRoutingInfo();
			loadAllRoutes();
			loadTunnels();
		}
		startPolling();
	});

	onDestroy(() => {
		stopPolling();
	});

	function startPolling() {
		stopPolling();
		pollInterval = setInterval(() => {
			if (active) {
				loadTunnels(true);
			}
		}, 4000);
	}

	function stopPolling() {
		if (pollInterval) {
			clearInterval(pollInterval);
			pollInterval = null;
		}
	}

	// Reset state when server changes
	$effect(() => {
		if (server.id !== previousServerId) {
			previousServerId = server.id;
			loading = true;
			saving = false;
			routingInfo = null;
			hostname = '';
			originalHostname = '';
			allRoutes = [];
			hostnameError = '';
			initialized = false;
			tunnels = [];
			loadRoutingInfo();
			loadTunnels();
		}
	});

	$effect(() => {
		if (server && !initialized && active) {
			initialized = true;
			loadRoutingInfo();
			loadAllRoutes();
			loadTunnels();
		}
	});

	async function loadRoutingInfo() {
		try {
			loading = true;
			const response = await rpcClient.proxy.getServerRouting({ serverId: server.id });
			routingInfo = response;
			hostname = response.proxyHostname || '';
			originalHostname = hostname;
		} catch (_e) {
			toast.error('Failed to load routing information');
		} finally {
			loading = false;
		}
	}

	async function loadAllRoutes() {
		try {
			const response = await rpcClient.proxy.getProxyRoutes({});
			allRoutes = response.routes;
		} catch (_e) {
			// Not critical
		}
	}

	async function loadTunnels(silent = false) {
		try {
			if (!silent) tunnelsLoading = true;
			const callOpts = silent ? silentCallOptions : undefined;
			const res = await rpcClient.tunnel.getServerTunnels({ serverId: server.id }, callOpts);
			tunnels = res.tunnels;
			hasGlobalAccount = res.hasGlobalAccount;
		} catch (_e) {
			if (!silent) {
				toast.error('Failed to load exposure tunnels');
			}
		} finally {
			if (!silent) tunnelsLoading = false;
		}
	}

	async function createPresetTunnel(presetType: 'java' | 'bedrock' | 'webmap' | 'custom') {
		creatingTunnel = true;
		try {
			let port = server.port || 25565;
			let protocol = TunnelProtocol.BOTH;
			let name = `${server.name} Java Tunnel`;

			if (presetType === 'bedrock') {
				port = 19132;
				protocol = TunnelProtocol.UDP;
				name = `${server.name} Bedrock Tunnel`;
			} else if (presetType === 'webmap') {
				port = 8100;
				protocol = TunnelProtocol.TCP;
				name = `${server.name} Web Map`;
			} else if (presetType === 'custom') {
				port = customPort || 25565;
				protocol =
					customProtocol === 'udp'
						? TunnelProtocol.UDP
						: customProtocol === 'both'
							? TunnelProtocol.BOTH
							: TunnelProtocol.TCP;
				name = customName || `${server.name} Custom (${port})`;
			}

			await rpcClient.tunnel.createTunnel({
				serverId: server.id,
				name,
				provider: TunnelProvider.PLAYIT,
				protocol,
				targetPort: port,
				autoStart: true,
				followServerLifecycle: true
			});

			toast.success('Tunnel created and started!');
			customPresetOpen = false;
			await loadTunnels();
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to create tunnel';
			toast.error(msg);
		} finally {
			creatingTunnel = false;
		}
	}

	async function startTunnel(tunnel: Tunnel) {
		tunnelActionLoading[tunnel.id] = true;
		try {
			await rpcClient.tunnel.startTunnel({ id: tunnel.id });
			toast.success('Tunnel starting...');
			await loadTunnels(true);
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to start tunnel';
			toast.error(msg);
		} finally {
			tunnelActionLoading[tunnel.id] = false;
		}
	}

	async function stopTunnel(tunnel: Tunnel) {
		tunnelActionLoading[tunnel.id] = true;
		try {
			await rpcClient.tunnel.stopTunnel({ id: tunnel.id });
			toast.success('Tunnel stopped');
			await loadTunnels(true);
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to stop tunnel';
			toast.error(msg);
		} finally {
			tunnelActionLoading[tunnel.id] = false;
		}
	}

	async function restartTunnel(tunnel: Tunnel) {
		tunnelActionLoading[tunnel.id] = true;
		try {
			await rpcClient.tunnel.restartTunnel({ id: tunnel.id });
			toast.success('Tunnel restarting...');
			await loadTunnels(true);
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to restart tunnel';
			toast.error(msg);
		} finally {
			tunnelActionLoading[tunnel.id] = false;
		}
	}

	async function deleteTunnel(tunnel: Tunnel) {
		if (!confirm(`Are you sure you want to delete tunnel "${tunnel.name}"?`)) return;
		tunnelActionLoading[tunnel.id] = true;
		try {
			await rpcClient.tunnel.deleteTunnel({ id: tunnel.id });
			toast.success('Tunnel removed');
			await loadTunnels(true);
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to delete tunnel';
			toast.error(msg);
		} finally {
			tunnelActionLoading[tunnel.id] = false;
		}
	}

	async function viewLogs(tunnel: Tunnel) {
		activeLogTunnel = tunnel;
		logsModalOpen = true;
		logsLoading = true;
		try {
			const res = await rpcClient.tunnel.getTunnelLogs({ id: tunnel.id, tail: 100 });
			tunnelLogs = res.logs;
		} catch {
			tunnelLogs = ['Failed to load logs.'];
		} finally {
			logsLoading = false;
		}
	}

	function validateHostname(value: string) {
		if (!value) {
			hostnameError = '';
			return true;
		}

		const hostnameRegex =
			/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$/i;
		if (!hostnameRegex.test(value)) {
			hostnameError = 'Invalid hostname format';
			return false;
		}

		const conflict = allRoutes.find(
			(route) =>
				route.hostname.toLowerCase() === value.toLowerCase() && route.serverId !== server.id
		);
		if (conflict) {
			hostnameError = 'Hostname already in use by another server';
			return false;
		}

		hostnameError = '';
		return true;
	}

	async function saveRouting() {
		if (!validateHostname(hostname)) return;

		saving = true;
		try {
			await rpcClient.proxy.updateServerRouting({
				serverId: server.id,
				proxyHostname: hostname
			});
			toast.success('Routing configuration saved');
			originalHostname = hostname;
			await loadRoutingInfo();
			await loadAllRoutes();
		} catch (error: unknown) {
			if (error instanceof Error && error.message.includes('Conflict')) {
				hostnameError = 'Hostname already in use by another server';
			} else {
				toast.error('Failed to save routing configuration');
			}
		} finally {
			saving = false;
		}
	}

	async function copyToClipboard(text: string) {
		const success = await copyText(text);
		if (success) {
			toast.success('Copied to clipboard');
		} else {
			toast.error('Failed to copy to clipboard');
		}
	}

	function getFullHostname() {
		if (hostname) return hostname;
		if (routingInfo?.suggestedHostname) return routingInfo.suggestedHostname;
		return `${server.name.toLowerCase().replace(/\s+/g, '-')}.minecraft.local`;
	}

	function getConnectionString() {
		const host = getFullHostname();
		const port = routingInfo?.listenPort || 25565;
		return port === 25565 ? host : `${host}:${port}`;
	}

	function getTunnelPublicEndpoint(t: Tunnel): string {
		if (!t.publicAddress) return 'Generating public link...';
		if (t.publicPort && t.publicPort !== 25565 && t.publicPort !== 19132) {
			return `${t.publicAddress}:${t.publicPort}`;
		}
		return t.publicAddress;
	}
</script>

{#if loading && tunnelsLoading}
	<div class="flex items-center justify-center py-12">
		<Loader2 class="h-8 w-8 animate-spin text-muted-foreground" />
	</div>
{:else}
	<div class="space-y-8 min-w-0 max-w-full">
		<!-- WAN Internet Exposure Section (Playit.gg) -->
		<div class="space-y-4">
			<div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<h3 class="flex items-center gap-2 text-lg font-semibold tracking-tight">
						<Radio class="h-5 w-5 text-emerald-500" />
						Internet WAN Exposure (Playit.gg)
					</h3>
					<p class="text-sm text-muted-foreground">
						Expose your Minecraft server securely to players anywhere without port forwarding.
					</p>
				</div>
				<div class="flex items-center gap-2">
					{#if hasGlobalAccount}
						<Button
							size="sm"
							variant="outline"
							disabled={tunnelsLoading}
							class="gap-1.5 border-border/70 text-xs"
							onclick={() => loadTunnels(false)}
						>
							<RotateCw class="h-3.5 w-3.5 {tunnelsLoading ? 'animate-spin' : ''}" />
							<span>Sync from Playit.gg</span>
						</Button>
						<Badge variant="default" class="gap-1 border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
							<ShieldCheck class="h-3.5 w-3.5" />
							Account Linked (Fleet Mode)
						</Badge>
					{:else}
						<Badge variant="outline" class="gap-1 text-xs text-muted-foreground">
							Account Setup Required
						</Badge>
					{/if}
				</div>
			</div>

			<!-- Notice banner when no account is linked -->
			{#if !hasGlobalAccount}
				<div class="rounded-lg border border-border/80 bg-muted/40 p-4">
					<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1">
							<p class="text-sm font-semibold text-foreground">Playit.gg Account Linking Required</p>
							<p class="text-xs text-muted-foreground">
								To expose this server to the internet, link your Playit.gg account secret key in Settings.
							</p>
						</div>
						<Button
							size="sm"
							class="gap-1.5 bg-emerald-600 text-white hover:bg-emerald-700 dark:bg-emerald-500 dark:text-black dark:hover:bg-emerald-400"
							onclick={() => goto(resolve('/settings'))}
						>
							<Radio class="h-4 w-4" />
							Link Account in Settings
						</Button>
					</div>
				</div>
			{/if}

			<!-- Active Tunnels List -->
			{#if tunnels.length > 0}
				<div class="grid gap-4">
					{#each tunnels as t (t.id)}
						<Card class="relative overflow-hidden border-border/70 bg-gradient-to-br from-card via-card/95 to-background shadow-md">
							<div class="absolute top-0 left-0 h-full w-1.5 {t.status === TunnelStatus.RUNNING ? 'bg-emerald-500' : t.status === TunnelStatus.CLAIM_PENDING ? 'bg-amber-500' : t.status === TunnelStatus.ERROR ? 'bg-destructive' : 'bg-muted-foreground/30'}"></div>
							<CardHeader class="pb-3">
								<div class="flex flex-wrap items-center justify-between gap-2">
									<div class="flex items-center gap-2.5">
										<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
											{#if t.targetPort === 19132}
												<Smartphone class="h-5 w-5" />
											{:else if t.targetPort === 8100 || t.targetPort === 8123}
												<MapPin class="h-5 w-5" />
											{:else}
												<Gamepad2 class="h-5 w-5" />
											{/if}
										</div>
										<div>
											<CardTitle class="text-base font-semibold">{t.name}</CardTitle>
											<CardDescription class="text-xs">
												Target: <code class="font-mono">{t.targetHost}:{t.targetPort}</code> ({t.protocol === TunnelProtocol.UDP ? 'UDP' : t.protocol === TunnelProtocol.BOTH ? 'TCP+UDP' : 'TCP'})
											</CardDescription>
										</div>
									</div>

									<div class="flex items-center gap-2">
										{#if t.status === TunnelStatus.RUNNING}
											<Badge variant="default" class="gap-1.5 bg-emerald-500/15 text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400">
												<span class="h-2 w-2 rounded-full bg-emerald-500 animate-pulse"></span>
												Live Online
											</Badge>
										{:else if t.status === TunnelStatus.CLAIM_PENDING}
											<Badge variant="secondary" class="gap-1.5 border-amber-500/30 bg-amber-500/15 text-amber-600 dark:text-amber-400">
												<AlertCircle class="h-3.5 w-3.5 animate-bounce" />
												Claim Required
											</Badge>
										{:else if t.status === TunnelStatus.STARTING}
											<Badge variant="secondary" class="gap-1.5 bg-blue-500/15 text-blue-600 dark:text-blue-400">
												<Loader2 class="h-3.5 w-3.5 animate-spin" />
												Starting...
											</Badge>
										{:else if t.status === TunnelStatus.STOPPED}
											<Badge variant="outline" class="text-xs text-muted-foreground">
												Stopped
											</Badge>
										{:else}
											<Badge variant="destructive" class="text-xs">
												Error
											</Badge>
										{/if}
									</div>
								</div>
							</CardHeader>

							<CardContent class="space-y-4 pt-1">
								<!-- Claim Pending Notice (Guest Mode) -->
								{#if t.status === TunnelStatus.CLAIM_PENDING && t.claimUrl}
									<div class="rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
										<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
											<div class="space-y-1">
												<p class="text-sm font-semibold text-amber-700 dark:text-amber-300">
													Action Required: Link to Playit.gg
												</p>
												<p class="text-xs text-amber-600/90 dark:text-amber-400/90">
													Click the button to claim this tunnel. Once claimed, your join link will activate immediately.
												</p>
											</div>
											<div class="flex items-center gap-2">
												<Button
													size="sm"
													class="gap-1.5 bg-amber-600 text-white hover:bg-amber-700 dark:bg-amber-500 dark:text-black dark:hover:bg-amber-400"
													onclick={() => window.open(t.claimUrl, '_blank')}
												>
													<ExternalLink class="h-4 w-4" />
													Claim on Playit.gg
												</Button>
												<Button
													size="sm"
													variant="outline"
													onclick={() => copyToClipboard(t.claimUrl)}
												>
													<Copy class="h-4 w-4" />
												</Button>
											</div>
										</div>
									</div>
								{/if}

								<!-- Running Connection Box -->
								{#if t.status === TunnelStatus.RUNNING}
									<div class="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4 shadow-sm">
										<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
											<div class="space-y-1">
												<div class="flex items-center gap-1.5">
													<span class="inline-block h-2 w-2 rounded-full bg-emerald-500 animate-pulse"></span>
													<p class="text-xs font-semibold text-emerald-600 dark:text-emerald-400 uppercase tracking-wider">
														Public WAN Join Address
													</p>
												</div>
												<p class="font-mono text-xl font-bold tracking-tight text-foreground">
													{getTunnelPublicEndpoint(t)}
												</p>
											</div>
											{#if t.publicAddress}
												<div class="flex items-center gap-2">
													<Button
														size="sm"
														class="gap-1.5 bg-emerald-600 text-white hover:bg-emerald-700 dark:bg-emerald-500 dark:text-black dark:hover:bg-emerald-400 shadow-sm"
														onclick={() => copyToClipboard(getTunnelPublicEndpoint(t))}
													>
														<Copy class="h-4 w-4" />
														Copy Address
													</Button>
												</div>
											{/if}
										</div>
									</div>
								{/if}

								<!-- Toolbar -->
								<div class="flex flex-wrap items-center justify-between gap-2 border-t border-border/40 pt-3">
									<div class="flex items-center gap-1">
										<Button
											variant="ghost"
											size="sm"
											class="h-8 gap-1 text-xs"
											onclick={() => viewLogs(t)}
										>
											<Terminal class="h-3.5 w-3.5" />
											Logs
										</Button>
									</div>

									<div class="flex items-center gap-1.5">
										{#if t.status === TunnelStatus.STOPPED}
											<Button
												size="sm"
												variant="outline"
												class="h-8 gap-1 text-xs"
												disabled={tunnelActionLoading[t.id]}
												onclick={() => startTunnel(t)}
											>
												<Play class="h-3.5 w-3.5 text-emerald-500" />
												Start
											</Button>
										{:else}
											<Button
												size="sm"
												variant="outline"
												class="h-8 gap-1 text-xs"
												disabled={tunnelActionLoading[t.id]}
												onclick={() => restartTunnel(t)}
											>
												<RotateCw class="h-3.5 w-3.5" />
												Restart
											</Button>
											<Button
												size="sm"
												variant="outline"
												class="h-8 gap-1 text-xs text-destructive hover:bg-destructive/10"
												disabled={tunnelActionLoading[t.id]}
												onclick={() => stopTunnel(t)}
											>
												<Square class="h-3.5 w-3.5" />
												Stop
											</Button>
										{/if}

										<Button
											size="icon"
											variant="ghost"
											class="h-8 w-8 text-muted-foreground hover:text-destructive"
											disabled={tunnelActionLoading[t.id]}
											onclick={() => deleteTunnel(t)}
										>
											<Trash2 class="h-4 w-4" />
										</Button>
									</div>
								</div>
							</CardContent>
						</Card>
					{/each}
				</div>
			{/if}

			<!-- 1-Click Exposure Presets Card -->
			<Card class="border-border/60 bg-muted/20">
				<CardHeader class="pb-3">
					<CardTitle class="text-sm font-semibold">
						{tunnels.length === 0 ? '1-Click Exposure Presets' : 'Add Another Exposure Preset'}
					</CardTitle>
					<CardDescription class="text-xs">
						Instantly launch a dedicated Playit.gg tunnel with optimized routing presets for your server.
					</CardDescription>
				</CardHeader>
				<CardContent class="space-y-3">
					<div class="grid grid-cols-1 gap-2.5 sm:grid-cols-3">
						<Button
							variant="outline"
							class="h-auto flex-col items-start gap-1 p-3 text-left hover:border-primary hover:bg-primary/5"
							disabled={creatingTunnel}
							onclick={() => createPresetTunnel('java')}
						>
							<div class="flex items-center gap-2 font-medium text-foreground">
								<Gamepad2 class="h-4 w-4 text-emerald-500" />
								<span>Minecraft Java</span>
							</div>
							<p class="text-xs text-muted-foreground">Port {server.port || 25565} (TCP+UDP)</p>
						</Button>

						<Button
							variant="outline"
							class="h-auto flex-col items-start gap-1 p-3 text-left hover:border-primary hover:bg-primary/5"
							disabled={creatingTunnel}
							onclick={() => createPresetTunnel('bedrock')}
						>
							<div class="flex items-center gap-2 font-medium text-foreground">
								<Smartphone class="h-4 w-4 text-blue-500" />
								<span>Bedrock / Geyser</span>
							</div>
							<p class="text-xs text-muted-foreground">Port 19132 (UDP)</p>
						</Button>

						<Button
							variant="outline"
							class="h-auto flex-col items-start gap-1 p-3 text-left hover:border-primary hover:bg-primary/5"
							disabled={creatingTunnel}
							onclick={() => createPresetTunnel('webmap')}
						>
							<div class="flex items-center gap-2 font-medium text-foreground">
								<MapPin class="h-4 w-4 text-purple-500" />
								<span>Web Map (BlueMap/Dynmap)</span>
							</div>
							<p class="text-xs text-muted-foreground">Port 8100 (TCP)</p>
						</Button>
					</div>

					<div class="pt-1">
						{#if !customPresetOpen}
							<Button
								variant="ghost"
								size="sm"
								class="gap-1.5 text-xs text-muted-foreground"
								onclick={() => (customPresetOpen = true)}
							>
								<Plus class="h-3.5 w-3.5" />
								Custom Port & Protocol...
							</Button>
						{:else}
							<div class="rounded-lg border border-border/60 bg-background p-3.5 space-y-3">
								<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
									<div class="space-y-1">
										<Label class="text-xs">Tunnel Name</Label>
										<Input
											placeholder="My Custom Tunnel"
											bind:value={customName}
											class="h-8 text-xs"
										/>
									</div>
									<div class="space-y-1">
										<Label class="text-xs">Target Port</Label>
										<Input
											type="number"
											placeholder="25565"
											bind:value={customPort}
											class="h-8 text-xs"
										/>
									</div>
									<div class="space-y-1">
										<Label class="text-xs">Protocol</Label>
										<select
											bind:value={customProtocol}
											class="h-8 w-full rounded-md border border-input bg-background px-2 text-xs"
										>
											<option value="tcp">TCP</option>
											<option value="udp">UDP</option>
											<option value="both">TCP + UDP</option>
										</select>
									</div>
								</div>
								<div class="flex justify-end gap-2">
									<Button
										variant="ghost"
										size="sm"
										class="h-7 text-xs"
										onclick={() => (customPresetOpen = false)}
									>
										Cancel
									</Button>
									<Button
										size="sm"
										class="h-7 text-xs"
										disabled={creatingTunnel}
										onclick={() => createPresetTunnel('custom')}
									>
										{#if creatingTunnel}
											<Loader2 class="mr-1 h-3.5 w-3.5 animate-spin" />
										{/if}
										Create Tunnel
									</Button>
								</div>
							</div>
						{/if}
					</div>
				</CardContent>
			</Card>
		</div>

		<!-- Local Reverse Proxy Section -->
		<div class="space-y-4 pt-4 border-t border-border/50">
			<div>
				<h3 class="flex items-center gap-2 text-lg font-semibold tracking-tight">
					<Globe class="h-5 w-5 text-blue-500" />
					Local Reverse Proxy & DNS Routing
				</h3>
				<p class="text-sm text-muted-foreground">
					Route custom domain names or internal LAN proxy traffic to this server instance.
				</p>
			</div>

			{#if !routingInfo?.proxyEnabled}
				<Alert>
					<AlertCircle class="h-4 w-4" />
					<AlertDescription>
						Proxy routing is not enabled in DiscoPanel configuration. Enable it to use custom local hostnames.
					</AlertDescription>
				</Alert>
			{:else}
				<div class="space-y-4">
					<!-- Status Card -->
					<Card>
						<CardHeader>
							<CardTitle class="flex items-center gap-2 text-base">
								<Globe class="h-4 w-4" />
								Current Proxy Status
							</CardTitle>
							<CardDescription>Domain routing state through the DiscoPanel reverse proxy</CardDescription>
						</CardHeader>
						<CardContent class="space-y-4">
							{#if routingInfo.currentRoute || routingInfo.proxyHostname}
								<div class="flex items-center gap-2">
									<Badge variant="default" class="gap-1 bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">
										<CheckCircle2 class="h-3 w-3" />
										Active Proxy Route
									</Badge>
									<span class="text-sm text-muted-foreground">
										Players can connect using the configured hostname
									</span>
								</div>
							{:else if server.status === ServerStatus.RUNNING}
								<div class="flex items-center gap-2">
									<Badge variant="secondary" class="gap-1">
										<AlertCircle class="h-3 w-3" />
										No Custom Hostname Configured
									</Badge>
									<span class="text-sm text-muted-foreground">
										Configure a hostname below to enable proxy routing
									</span>
								</div>
							{:else}
								<div class="flex items-center gap-2">
									<Badge variant="outline" class="gap-1">
										<XCircle class="h-3 w-3" />
										Server Offline
									</Badge>
									<span class="text-sm text-muted-foreground">
										Start the server to activate reverse proxy routing
									</span>
								</div>
							{/if}

							<div class="rounded-lg bg-muted p-4">
								<div class="flex items-center justify-between">
									<div>
										<p class="mb-1 text-sm font-medium">Local / Proxy Address</p>
										<p class="font-mono text-lg">{getConnectionString()}</p>
									</div>
									<Button
										variant="outline"
										size="icon"
										onclick={() => copyToClipboard(getConnectionString())}
									>
										<Copy class="h-4 w-4" />
									</Button>
								</div>
							</div>
						</CardContent>
					</Card>

					<!-- Configuration Card -->
					<Card>
						<CardHeader>
							<CardTitle class="text-base">Hostname Configuration</CardTitle>
							<CardDescription>
								Set a custom hostname for players to connect to your server through the reverse proxy
							</CardDescription>
						</CardHeader>
						<CardContent class="space-y-4">
							<div class="space-y-2">
								<Label for="hostname">Custom Hostname</Label>
								<Input
									id="hostname"
									type="text"
									bind:value={hostname}
									placeholder={routingInfo.suggestedHostname || 'minecraft.example.com'}
									oninput={(e) => validateHostname(e.currentTarget.value)}
									class={hostnameError ? 'border-destructive' : ''}
								/>
								{#if hostnameError}
									<p class="text-sm text-destructive">{hostnameError}</p>
								{:else if hostname}
									<p class="text-sm text-muted-foreground">
										Players will connect using: <span class="font-mono">{getConnectionString()}</span>
									</p>
								{:else}
									<p class="text-sm text-muted-foreground">
										Leave empty to use the default hostname based on your server name
									</p>
								{/if}
							</div>

							{#if routingInfo.baseUrl}
								<Alert>
									<AlertDescription>
										<p class="mb-1 font-medium">DNS Configuration Required</p>
										<p class="text-sm">
											Make sure to add a DNS record pointing <code class="font-mono">{getFullHostname()}</code> to your server's IP address.
										</p>
									</AlertDescription>
								</Alert>
							{/if}

							<div class="flex justify-end gap-2">
								<Button
									variant="outline"
									onclick={() => {
										hostname = originalHostname;
										hostnameError = '';
									}}
									disabled={!hasChanges || saving}
								>
									Cancel
								</Button>
								<Button onclick={saveRouting} disabled={!hasChanges || saving || !!hostnameError}>
									{#if saving}
										<Loader2 class="mr-2 h-4 w-4 animate-spin" />
									{:else}
										<Save class="mr-2 h-4 w-4" />
									{/if}
									Save Changes
								</Button>
							</div>
						</CardContent>
					</Card>
				</div>
			{/if}
		</div>
	</div>
{/if}

<!-- Tunnel Logs Dialog -->
<Dialog.Root bind:open={logsModalOpen}>
	<Dialog.Content class="max-w-3xl">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2">
				<Terminal class="h-5 w-5 text-primary" />
				Tunnel Logs: {activeLogTunnel?.name}
			</Dialog.Title>
			<Dialog.Description>
				Recent container output from Playit.gg agent
			</Dialog.Description>
		</Dialog.Header>

		<div class="mt-2 h-80 overflow-y-auto rounded-lg bg-black/90 p-4 font-mono text-xs text-green-400">
			{#if logsLoading}
				<div class="flex h-full items-center justify-center">
					<Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
				</div>
			{:else if tunnelLogs.length === 0}
				<p class="text-muted-foreground">No logs recorded yet.</p>
			{:else}
				{#each tunnelLogs as logLine, idx (idx)}
					<div class="leading-relaxed whitespace-pre-wrap break-all">{logLine}</div>
				{/each}
			{/if}
		</div>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (logsModalOpen = false)}>Close</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
