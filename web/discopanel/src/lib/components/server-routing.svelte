<script lang="ts">
	import { onMount } from 'svelte';
	import { rpcClient } from '$lib/api/rpc-client';
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
	import { Loader2, Globe, Save, Copy, AlertCircle, CheckCircle2, XCircle } from '@lucide/svelte';
	import { copyToClipboard as copyText } from '$lib/utils/clipboard';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { ServerStatus } from '$lib/proto/discopanel/v1/common_pb';
	import type { GetServerRoutingResponse, ProxyRoute } from '$lib/proto/discopanel/v1/proxy_pb';

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

	onMount(() => {
		if (server && !initialized) {
			initialized = true;
			loadRoutingInfo();
			loadAllRoutes();
		}
	});

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
			loadRoutingInfo();
		}
	});

	$effect(() => {
		if (server && !initialized && active) {
			initialized = true;
			loadRoutingInfo();
			loadAllRoutes();
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
</script>

{#if loading}
	<div class="flex items-center justify-center py-12">
		<Loader2 class="h-8 w-8 animate-spin text-muted-foreground" />
	</div>
{:else}
	<div class="space-y-8 min-w-0 max-w-full">
		<!-- Local Reverse Proxy Section -->
		<div class="space-y-4">
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
