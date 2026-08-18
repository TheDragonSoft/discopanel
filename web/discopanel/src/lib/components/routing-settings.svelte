<script lang="ts">
	import { onMount } from 'svelte';
	import { rpcClient } from '$lib/api/rpc-client';
	import type { ProxyListener } from '$lib/proto/discopanel/v1/common_pb';
	import type { ProxyListenerWithCount, ProxyRoute } from '$lib/proto/discopanel/v1/proxy_pb';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { Badge } from '$lib/components/ui/badge';
	import { Alert, AlertDescription } from '$lib/components/ui/alert';
	import { toast } from 'svelte-sonner';
	import {
		Save,
		Plus,
		Trash2,
		Loader2,
		AlertCircle,
		Server,
		Activity,
		CheckCircle2,
		XCircle,
		Network,
		Info,
		Edit,
		Star,
		Radio,
		ExternalLink,
		ShieldCheck,
		Key,
		Copy,
		RotateCw,
		Check
	} from '@lucide/svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { copyToClipboard as copyText } from '$lib/utils/clipboard';

	let loading = $state(true);
	let saving = $state(false);
	let proxyEnabled = $state(false);
	let baseURL = $state('');
	let listenersWithCount = $state<ProxyListenerWithCount[]>([]);
	let editingListener = $state<ProxyListener | null>(null);
	let newListener = $state<Partial<ProxyListener>>({
		port: 25565,
		name: '',
		description: '',
		enabled: true,
		isDefault: false
	});
	let portError = $state('');
	let activeRoutes = $state<ProxyRoute[]>([]);

	// Playit.gg Account Linking state
	let playitLinked = $state(false);
	let playitNotice = $state('');
	let playitLoading = $state(false);
	let linkSession = $state<{ sessionId: string; claimUrl: string; claimCode: string } | null>(null);
	let linkingModalOpen = $state(false);
	let manualSecretModalOpen = $state(false);
	let manualSecret = $state('');
	let linkChecking = $state(false);
	let linkCheckInterval: ReturnType<typeof setInterval> | null = null;

	onMount(() => {
		loadAll();
		return () => {
			if (linkCheckInterval) clearInterval(linkCheckInterval);
		};
	});

	async function loadAll() {
		loading = true;
		try {
			await Promise.all([loadProxyConfig(), loadListeners(), loadActiveRoutes(), loadPlayitConfig()]);
		} finally {
			loading = false;
		}
	}

	async function loadPlayitConfig() {
		try {
			playitLoading = true;
			const res = await rpcClient.tunnel.getPlayitAccountConfig({});
			playitLinked = res.isLinked;
			playitNotice = res.notice;
		} catch {
			// fallback
		} finally {
			playitLoading = false;
		}
	}

	async function startPlayitLinking() {
		playitLoading = true;
		try {
			const res = await rpcClient.tunnel.startAccountLinkSession({});
			linkSession = {
				sessionId: res.sessionId,
				claimUrl: res.claimUrl,
				claimCode: res.claimCode
			};
			linkingModalOpen = true;
			startLinkStatusPolling(res.sessionId);
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to start Playit account linking session';
			toast.error(msg);
		} finally {
			playitLoading = false;
		}
	}

	function startLinkStatusPolling(sessionId: string) {
		if (linkCheckInterval) clearInterval(linkCheckInterval);
		linkCheckInterval = setInterval(async () => {
			if (!linkingModalOpen) {
				if (linkCheckInterval) clearInterval(linkCheckInterval);
				return;
			}
			linkChecking = true;
			try {
				const res = await rpcClient.tunnel.checkAccountLinkStatus({ sessionId });
				if (res.isLinked) {
					if (linkCheckInterval) clearInterval(linkCheckInterval);
					linkingModalOpen = false;
					toast.success('Playit.gg account successfully linked fleet-wide!');
					await loadPlayitConfig();
				}
			} catch {
				// keep polling until user closes
			} finally {
				linkChecking = false;
			}
		}, 3000);
	}

	async function unlinkPlayit() {
		if (!confirm('Are you sure you want to unlink your Playit.gg account? Tunnels will revert to guest claim mode.')) return;
		try {
			await rpcClient.tunnel.unlinkPlayitAccount({});
			toast.success('Playit.gg account unlinked');
			await loadPlayitConfig();
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to unlink account';
			toast.error(msg);
		}
	}

	async function saveManualSecret() {
		if (!manualSecret.trim()) {
			toast.error('Secret key cannot be empty');
			return;
		}
		try {
			await rpcClient.tunnel.setPlayitAccountSecret({ secretKey: manualSecret.trim() });
			toast.success('Playit account secret saved');
			manualSecretModalOpen = false;
			manualSecret = '';
			await loadPlayitConfig();
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to save secret key';
			toast.error(msg);
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

	async function loadProxyConfig() {
		try {
			const status = await rpcClient.proxy.getProxyStatus({});
			proxyEnabled = status.enabled;
			baseURL = status.baseUrl || '';
		} catch (_e) {
			toast.error('Failed to load proxy configuration');
		}
	}

	async function loadListeners() {
		try {
			const response = await rpcClient.proxy.getProxyListeners({});
			listenersWithCount = response.listeners;
			// Set default port for new listener
			if (listenersWithCount.length > 0) {
				const usedPorts = new Set(listenersWithCount.map((lwc) => lwc.listener?.port || 0));
				let nextPort = 25565;
				while (usedPorts.has(nextPort)) {
					nextPort++;
				}
				newListener.port = nextPort;
			}
		} catch (_e) {
			toast.error('Failed to load proxy listeners');
		}
	}

	async function loadActiveRoutes() {
		try {
			const response = await rpcClient.proxy.getProxyRoutes({});
			activeRoutes = response.routes;
		} catch (error) {
			console.error('Failed to load active routes:', error);
		}
	}

	function validatePort(port: number): boolean {
		portError = '';

		if (!port || port < 1 || port > 65535) {
			portError = 'Port must be between 1 and 65535';
			return false;
		}

		// Check if port is already used by another listener
		const existingListener = listenersWithCount.find(
			(lwc) => lwc.listener?.port === port && lwc.listener?.id !== editingListener?.id
		);
		if (existingListener) {
			portError = `Port ${port} is already used by listener "${existingListener.listener?.name}"`;
			return false;
		}

		return true;
	}

	async function saveProxyConfig() {
		saving = true;
		try {
			await rpcClient.proxy.updateProxyConfig({
				enabled: proxyEnabled,
				baseUrl: baseURL
			});

			toast.success('Proxy configuration saved');
			await loadAll();
		} catch (_e) {
			toast.error('Failed to save proxy configuration');
		} finally {
			saving = false;
		}
	}

	async function createListener() {
		if (!newListener.name) {
			toast.error('Listener name is required');
			return;
		}

		if (!validatePort(newListener.port!)) {
			return;
		}

		try {
			await rpcClient.proxy.createProxyListener({
				port: newListener.port!,
				name: newListener.name,
				description: newListener.description || '',
				enabled: newListener.enabled,
				isDefault: newListener.isDefault
			});

			toast.success(`Listener "${newListener.name}" created`);

			// Reset form
			newListener = {
				port: 25565,
				name: '',
				description: '',
				enabled: true,
				isDefault: false
			};

			await loadListeners();
		} catch (error: unknown) {
			toast.error(error instanceof Error ? error.message : 'Failed to create listener');
		}
	}

	async function updateListener(listener: ProxyListener) {
		try {
			await rpcClient.proxy.updateProxyListener({
				id: listener.id,
				name: listener.name,
				description: listener.description,
				enabled: listener.enabled,
				isDefault: listener.isDefault
			});

			toast.success(`Listener "${listener.name}" updated`);
			editingListener = null;
			await loadListeners();
		} catch (_e) {
			toast.error('Failed to update listener');
		}
	}

	async function deleteListener(listenerWithCount: ProxyListenerWithCount) {
		const listener = listenerWithCount.listener;
		if (!listener) return;

		// Check server count from the response
		if (listenerWithCount.serverCount > 0) {
			toast.error(
				`Cannot delete: ${listenerWithCount.serverCount} servers are using this listener`
			);
			return;
		}

		if (confirm(`Delete listener "${listener.name}" on port ${listener.port}?`)) {
			try {
				await rpcClient.proxy.deleteProxyListener({ id: listener.id });
				toast.success(`Listener "${listener.name}" deleted`);
				await loadListeners();
			} catch (error: unknown) {
				toast.error(error instanceof Error ? error.message : 'Failed to delete listener');
			}
		}
	}

	async function setDefaultListener(listener: ProxyListener) {
		listener.isDefault = true;
		await updateListener(listener);
	}

	function getListenerStatus(
		listener: ProxyListener | undefined,
		serverCount: number
	): 'active' | 'inactive' | 'disabled' {
		if (!listener || !listener.enabled) return 'disabled';
		if (!proxyEnabled) return 'inactive';
		return serverCount > 0 ? 'active' : 'inactive';
	}

	function getStatusColor(status: string): string {
		switch (status) {
			case 'active':
				return 'text-green-500';
			case 'inactive':
				return 'text-yellow-500';
			case 'disabled':
				return 'text-gray-500';
			default:
				return 'text-gray-500';
		}
	}

	function getStatusIcon(status: string) {
		switch (status) {
			case 'active':
				return CheckCircle2;
			case 'disabled':
				return XCircle;
			default:
				return AlertCircle;
		}
	}
</script>

<div class="space-y-6">
	<!-- Playit.gg Account Linking Card -->
	<Card class="border-border/70 bg-gradient-to-br from-card via-card/95 to-background shadow-md">
		<CardHeader>
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-3">
					<Radio class="h-5 w-5 text-emerald-500" />
					<div>
						<CardTitle>Playit.gg WAN Account Linking</CardTitle>
						<CardDescription>
							Auto-link all server exposure tunnels to your personal Playit.gg account
						</CardDescription>
					</div>
				</div>
				{#if playitLinked}
					<Badge variant="default" class="gap-1.5 border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
						<ShieldCheck class="h-3.5 w-3.5" />
						Account Linked (Fleet Active)
					</Badge>
				{:else}
					<Badge variant="outline" class="gap-1 text-xs text-muted-foreground">
						Not Linked (Guest Mode)
					</Badge>
				{/if}
			</div>
		</CardHeader>
		<CardContent class="space-y-4">
			{#if playitLinked}
				<div class="rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-4">
					<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1">
							<p class="text-sm font-semibold text-foreground">Playit.gg Fleet Auto-Link Active</p>
							<p class="text-xs text-muted-foreground">
								All tunnels created across your servers will automatically inherit your account credentials without manual guest claiming.
							</p>
						</div>
						<div class="flex items-center gap-2">
							<Button variant="outline" size="sm" class="gap-1.5 text-xs text-destructive hover:bg-destructive/10" onclick={unlinkPlayit}>
								<Trash2 class="h-3.5 w-3.5" />
								Unlink Account
							</Button>
						</div>
					</div>
				</div>
			{:else}
				<div class="rounded-lg border border-border/70 bg-muted/30 p-4">
					<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
						<div class="space-y-1">
							<p class="text-sm font-semibold text-foreground">1-Click Auto-Link Playit.gg Account</p>
							<p class="text-xs text-muted-foreground">
								Click the button below to authorize DiscoPanel. We will automatically retrieve and store your secret key to link all future server tunnels.
							</p>
						</div>
						<div class="flex items-center gap-2">
							<Button
								class="gap-1.5 bg-emerald-600 text-white hover:bg-emerald-700 dark:bg-emerald-500 dark:text-black dark:hover:bg-emerald-400"
								disabled={playitLoading}
								onclick={startPlayitLinking}
							>
								{#if playitLoading}
									<Loader2 class="h-4 w-4 animate-spin" />
								{:else}
									<ExternalLink class="h-4 w-4" />
								{/if}
								Link Account on Playit.gg
							</Button>
							<Button
								variant="outline"
								size="sm"
								class="text-xs"
								onclick={() => (manualSecretModalOpen = true)}
							>
								<Key class="mr-1 h-3.5 w-3.5" />
								Manual Key
							</Button>
						</div>
					</div>
				</div>
			{/if}
		</CardContent>
	</Card>

	<!-- Global Proxy Configuration -->
	<Card>
		<CardHeader>
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-3">
					<Network class="h-5 w-5 text-primary" />
					<div>
						<CardTitle>Proxy Configuration</CardTitle>
						<CardDescription>Global proxy settings and base domain configuration</CardDescription>
					</div>
				</div>
				<Switch
					checked={proxyEnabled}
					onCheckedChange={(checked) => (proxyEnabled = checked)}
					disabled={loading || saving}
				/>
			</div>
		</CardHeader>
		<CardContent class="space-y-4">
			<div class="space-y-2">
				<Label for="base-url">Base Domain</Label>
				<Input
					id="base-url"
					type="text"
					bind:value={baseURL}
					placeholder="minecraft.example.com"
					disabled={saving || !proxyEnabled}
				/>
				<p class="text-xs text-muted-foreground">
					Optional base domain that will be appended to server hostnames (e.g., "survival" becomes
					"survival.minecraft.example.com")
				</p>
			</div>

			<div class="flex justify-end">
				<Button onclick={saveProxyConfig} disabled={saving}>
					{#if saving}
						<Loader2 class="mr-2 h-4 w-4 animate-spin" />
					{:else}
						<Save class="mr-2 h-4 w-4" />
					{/if}
					Save Configuration
				</Button>
			</div>
		</CardContent>
	</Card>

	{#if loading}
		<Card>
			<CardContent class="flex items-center justify-center py-12">
				<Loader2 class="h-8 w-8 animate-spin text-muted-foreground" />
			</CardContent>
		</Card>
	{:else if !proxyEnabled}
		<Alert>
			<Info class="h-4 w-4" />
			<AlertDescription>
				Enable the proxy system to allow servers to use custom hostnames instead of direct port
				connections.
			</AlertDescription>
		</Alert>
	{:else}
		<!-- Proxy Listeners -->
		<Card>
			<CardHeader>
				<div class="flex items-center justify-between">
					<div>
						<CardTitle>Proxy Listeners</CardTitle>
						<CardDescription>Configure individual proxy listening ports</CardDescription>
					</div>
					<Badge variant="outline" class="gap-1">
						<Server class="h-3 w-3" />
						{listenersWithCount.length}
						{listenersWithCount.length === 1 ? 'Listener' : 'Listeners'}
					</Badge>
				</div>
			</CardHeader>
			<CardContent class="space-y-4">
				<!-- Existing Listeners -->
				{#if listenersWithCount.length > 0}
					<div class="space-y-3">
						{#each listenersWithCount as lwc (lwc.listener?.id)}
							{@const listener = lwc.listener}
							{@const status = getListenerStatus(listener, lwc.serverCount)}
							{@const StatusIcon = getStatusIcon(status)}
							{#if listener}
								<div class="rounded-lg border bg-card p-4">
									{#if editingListener?.id === listener.id}
										<!-- Edit Mode -->
										<div class="space-y-3">
											<div class="grid grid-cols-2 gap-3">
												<div class="space-y-2">
													<Label>Name</Label>
													<Input bind:value={editingListener.name} placeholder="Listener name" />
												</div>
												<div class="space-y-2">
													<Label>Port</Label>
													<Input type="number" value={listener.port} disabled class="bg-muted" />
												</div>
											</div>
											<div class="space-y-2">
												<Label>Description</Label>
												<Input
													bind:value={editingListener.description}
													placeholder="Optional description"
												/>
											</div>
											<div class="flex items-center justify-between">
												<div class="flex items-center gap-4">
													<div class="flex items-center gap-2">
														<Switch
															checked={editingListener?.enabled ?? false}
															onCheckedChange={(checked) => {
																if (editingListener) editingListener.enabled = checked;
															}}
														/>
														<Label>Enabled</Label>
													</div>
													<div class="flex items-center gap-2">
														<Switch
															checked={editingListener?.isDefault ?? false}
															onCheckedChange={(checked) => {
																if (editingListener) editingListener.isDefault = checked;
															}}
														/>
														<Label>Default</Label>
													</div>
												</div>
												<div class="flex gap-2">
													<Button
														variant="outline"
														size="sm"
														onclick={() => (editingListener = null)}
													>
														Cancel
													</Button>
													<Button size="sm" onclick={() => updateListener(editingListener!)}>
														Save
													</Button>
												</div>
											</div>
										</div>
									{:else}
										<!-- View Mode -->
										<div class="flex items-start justify-between">
											<div class="space-y-2">
												<div class="flex items-center gap-3">
													<StatusIcon class="h-4 w-4 {getStatusColor(status)}" />
													<span class="font-semibold">{listener.name}</span>
													<Badge variant="secondary" class="font-mono">:{listener.port}</Badge>
													{#if listener.isDefault}
														<Badge variant="default" class="gap-1">
															<Star class="h-3 w-3" />
															Default
														</Badge>
													{/if}
													{#if !listener.enabled}
														<Badge variant="outline">Disabled</Badge>
													{/if}
												</div>

												{#if listener.description}
													<p class="text-sm text-muted-foreground">{listener.description}</p>
												{/if}

												{#if lwc.serverCount > 0}
													<p class="text-xs text-muted-foreground">
														{lwc.serverCount}
														{lwc.serverCount === 1 ? 'server' : 'servers'} using this listener
													</p>
												{:else}
													<p class="text-xs text-muted-foreground">
														No servers using this listener
													</p>
												{/if}
											</div>

											<div class="flex gap-2">
												{#if !listener.isDefault}
													<Button
														variant="ghost"
														size="icon"
														class="h-8 w-8"
														onclick={() => setDefaultListener(listener)}
														title="Set as default"
													>
														<Star class="h-4 w-4" />
													</Button>
												{/if}
												<Button
													variant="ghost"
													size="icon"
													class="h-8 w-8"
													onclick={() => (editingListener = { ...listener })}
												>
													<Edit class="h-4 w-4" />
												</Button>
												{#if listenersWithCount.length > 1 && lwc.serverCount === 0}
													<Button
														variant="ghost"
														size="icon"
														class="h-8 w-8"
														onclick={() => deleteListener(lwc)}
													>
														<Trash2 class="h-4 w-4" />
													</Button>
												{/if}
											</div>
										</div>
									{/if}
								</div>
							{/if}
						{/each}
					</div>
				{/if}

				<!-- Add New Listener -->
				<div class="border-t pt-4">
					<h4 class="mb-3 font-medium">Add New Listener</h4>
					<div class="space-y-3">
						<div class="grid grid-cols-2 gap-3">
							<div class="space-y-2">
								<Label>Name</Label>
								<Input bind:value={newListener.name} placeholder="e.g., Secondary, Development" />
							</div>
							<div class="space-y-2">
								<Label>Port</Label>
								<Input
									type="number"
									bind:value={newListener.port}
									oninput={(e) => validatePort(Number(e.currentTarget.value))}
									class={portError ? 'border-destructive' : ''}
								/>
								{#if portError}
									<p class="text-xs text-destructive">{portError}</p>
								{/if}
							</div>
						</div>
						<div class="space-y-2">
							<Label>Description (Optional)</Label>
							<Input
								bind:value={newListener.description}
								placeholder="Optional description for this listener"
							/>
						</div>
						<div class="flex items-center justify-between">
							<div class="flex items-center gap-4">
								<div class="flex items-center gap-2">
									<Switch
										checked={newListener.enabled}
										onCheckedChange={(checked) => (newListener.enabled = checked)}
									/>
									<Label>Enabled</Label>
								</div>
								{#if listenersWithCount.length === 0}
									<div class="flex items-center gap-2">
										<Switch
											checked={newListener.isDefault}
											onCheckedChange={(checked) => (newListener.isDefault = checked)}
										/>
										<Label>Set as Default</Label>
									</div>
								{/if}
							</div>
							<Button onclick={createListener} disabled={!newListener.name || !!portError}>
								<Plus class="mr-2 h-4 w-4" />
								Add Listener
							</Button>
						</div>
					</div>
				</div>
			</CardContent>
		</Card>

		<!-- Active Routes -->
		{#if activeRoutes.length > 0}
			<Card>
				<CardHeader>
					<CardTitle>Active Routes</CardTitle>
					<CardDescription>Servers currently using proxy routing</CardDescription>
				</CardHeader>
				<CardContent>
					<div class="space-y-2">
						{#each activeRoutes as route (route.serverId)}
							<div class="flex items-center justify-between rounded-lg bg-muted/50 p-3">
								<div class="flex items-center gap-3">
									<Activity class="h-4 w-4 {route.active ? 'text-green-500' : 'text-gray-500'}" />
									<div>
										<p class="font-mono text-sm">{route.hostname}</p>
										<p class="text-xs text-muted-foreground">
											Server: {route.serverId.slice(0, 8)}...
										</p>
									</div>
								</div>
								<Badge variant={route.active ? 'default' : 'outline'}>
									{route.active ? 'Active' : 'Inactive'}
								</Badge>
							</div>
						{/each}
					</div>
				</CardContent>
			</Card>
		{/if}
	{/if}
</div>

<!-- Playit Account Linking Modal -->
<Dialog.Root bind:open={linkingModalOpen}>
	<Dialog.Content class="max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2">
				<Radio class="h-5 w-5 text-emerald-500" />
				Authorize Playit.gg Account
			</Dialog.Title>
			<Dialog.Description>
				Follow the steps below to link DiscoPanel to your Playit.gg account.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4 py-2">
			{#if linkSession?.claimUrl}
				<div class="space-y-2">
					<Label class="text-xs font-semibold text-muted-foreground uppercase">Step 1: Open Claim Link</Label>
					<div class="rounded-lg border border-border/80 bg-muted/40 p-3">
						<p class="mb-2 text-xs text-muted-foreground">
							Click the button below to authorize DiscoPanel on Playit.gg. Keep this dialog open.
						</p>
						<div class="flex items-center gap-2">
							<Button
								class="w-full gap-1.5 bg-emerald-600 text-white hover:bg-emerald-700 dark:bg-emerald-500 dark:text-black dark:hover:bg-emerald-400"
								onclick={() => window.open(linkSession?.claimUrl, '_blank')}
							>
								<ExternalLink class="h-4 w-4" />
								Authorize on Playit.gg
							</Button>
							<Button
								variant="outline"
								size="icon"
								onclick={() => copyToClipboard(linkSession?.claimUrl || '')}
							>
								<Copy class="h-4 w-4" />
							</Button>
						</div>
					</div>
				</div>

				<div class="space-y-2">
					<Label class="text-xs font-semibold text-muted-foreground uppercase">Step 2: Awaiting Confirmation</Label>
					<div class="flex items-center gap-3 rounded-lg border border-primary/20 bg-primary/5 p-3">
						<Loader2 class="h-5 w-5 animate-spin text-primary" />
						<div class="text-xs">
							<p class="font-medium text-foreground">Listening for claim approval...</p>
							<p class="text-muted-foreground">This modal will close automatically once authorized.</p>
						</div>
					</div>
				</div>
			{:else}
				<div class="flex items-center justify-center py-6">
					<Loader2 class="h-8 w-8 animate-spin text-muted-foreground" />
				</div>
			{/if}
		</div>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => (linkingModalOpen = false)}>
				Cancel
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<!-- Manual Secret Key Dialog -->
<Dialog.Root bind:open={manualSecretModalOpen}>
	<Dialog.Content class="max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2">
				<Key class="h-5 w-5 text-primary" />
				Set Playit Secret Key
			</Dialog.Title>
			<Dialog.Description>
				Paste your existing Playit.gg account or agent secret key.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-3 py-2">
			<div class="space-y-1.5">
				<Label for="manual-secret" class="text-xs">Secret Key</Label>
				<Input
					id="manual-secret"
					type="password"
					placeholder="playit secret key..."
					bind:value={manualSecret}
				/>
				<p class="text-xs text-muted-foreground">
					This key is securely stored in DiscoPanel settings and used for all server tunnels.
				</p>
			</div>
		</div>

		<Dialog.Footer class="gap-2">
			<Button variant="outline" onclick={() => (manualSecretModalOpen = false)}>
				Cancel
			</Button>
			<Button onclick={saveManualSecret}>
				Save Key
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
