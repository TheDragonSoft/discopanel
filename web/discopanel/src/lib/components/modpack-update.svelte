<script lang="ts">
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { toast } from 'svelte-sonner';
	import {
		Loader2,
		RefreshCw,
		AlertCircle,
		TriangleAlert,
		PackageSearch,
		PackageCheck,
		History,
		Undo2,
		Download,
		Clock
	} from '@lucide/svelte';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import {
		CheckModpackUpdateRequestSchema,
		ListModpackVersionsRequestSchema,
		UpdateServerModpackRequestSchema,
		RollbackModpackUpdateRequestSchema,
		GetModpackUpdateSettingsRequestSchema,
		SetModpackUpdateSettingsRequestSchema,
		ModpackUpdateMode,
		type ModpackUpdateCheck,
		type ModpackVersionInfo,
		type ModpackUpdateSettings,
		type UpdateServerModpackResponse
	} from '$lib/proto/discopanel/v1/modpack_update_pb';
	import { timestampToDate, formatTimeAgo } from '$lib/utils';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Badge } from '$lib/components/ui/badge';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Switch } from '$lib/components/ui/switch';
	import { Separator } from '$lib/components/ui/separator';
	import { Alert, AlertDescription } from '$lib/components/ui/alert';
	import {
		Dialog,
		DialogContent,
		DialogDescription,
		DialogFooter,
		DialogHeader,
		DialogTitle
	} from '$lib/components/ui/dialog';
	import {
		Select,
		SelectContent,
		SelectItem,
		SelectTrigger
	} from '$lib/components/ui/select';
	import {
		RadioGroup,
		RadioGroupItem
	} from '$lib/components/ui/radio-group';
	import {
		Collapsible,
		CollapsibleContent,
		CollapsibleTrigger
	} from '$lib/components/ui/collapsible';

	interface Props {
		server: Server;
	}

	let { server }: Props = $props();

	// --- Update check ---
	let check = $state<ModpackUpdateCheck | null>(null);
	let checking = $state(false);
	let checkFailed = $state(false);

	// --- Versions list ---
	let showVersions = $state(false);
	let versionsLoaded = $state(false);
	let versionsLoading = $state(false);
	let versions = $state<ModpackVersionInfo[]>([]);
	// Radio selection; LATEST_VALUE means "latest release"
	const LATEST_VALUE = '__latest__';
	let selectedVersionId = $state(LATEST_VALUE);

	// --- Update flow ---
	let updateOpen = $state(false);
	let createBackup = $state(true);
	let updating = $state(false);
	let lastUpdate = $state<UpdateServerModpackResponse | null>(null);

	// --- Rollback ---
	let rollingBack = $state(false);

	// --- Automatic update settings ---
	let settings = $state<ModpackUpdateSettings | null>(null);
	let settingsEnabled = $state(false);
	let settingsInterval = $state(24);
	let settingsMode = $state<'notify' | 'apply'>('notify');
	let savingSettings = $state(false);

	let settingsDirty = $derived(
		!settings ||
			settingsEnabled !== settings.enabled ||
			settingsInterval !== settings.intervalHours ||
			(settingsMode === 'apply' ? ModpackUpdateMode.APPLY : ModpackUpdateMode.NOTIFY) !==
				settings.mode
	);

	// Selected version label for the dialog / buttons
	let selectedVersion = $derived(
		selectedVersionId === LATEST_VALUE
			? null
			: (versions.find((v) => v.versionId === selectedVersionId) ?? null)
	);
	let targetLabel = $derived(
		selectedVersion?.versionName || check?.latestVersion || 'latest version'
	);

	// Reset when the server changes (route param change reuses the component)
	let previousServerId = $state(server.id);
	$effect(() => {
		if (server.id !== previousServerId) {
			previousServerId = server.id;
			resetState();
			runCheck();
			loadSettings();
		}
	});

	onMount(() => {
		runCheck();
		loadSettings();
	});

	function resetState() {
		check = null;
		checkFailed = false;
		showVersions = false;
		versionsLoaded = false;
		versions = [];
		selectedVersionId = LATEST_VALUE;
		lastUpdate = null;
		updateOpen = false;
		settings = null;
		settingsEnabled = false;
		settingsInterval = 24;
		settingsMode = 'notify';
	}

	async function runCheck() {
		checking = true;
		checkFailed = false;
		try {
			const response = await rpcClient.modpackUpdate.checkModpackUpdate(
				create(CheckModpackUpdateRequestSchema, { serverId: server.id }),
				silentCallOptions
			);
			check = response.check ?? null;
		} catch (error) {
			check = null;
			checkFailed = true;
			console.debug('Failed to check modpack update:', error);
		} finally {
			checking = false;
		}
	}

	async function loadVersions() {
		versionsLoading = true;
		try {
			const response = await rpcClient.modpackUpdate.listModpackVersions(
				create(ListModpackVersionsRequestSchema, { serverId: server.id }),
				silentCallOptions
			);
			versions = response.versions;
			versionsLoaded = true;
			// Drop a selected version that no longer exists
			if (
				selectedVersionId !== LATEST_VALUE &&
				!versions.some((v) => v.versionId === selectedVersionId)
			) {
				selectedVersionId = LATEST_VALUE;
			}
		} catch (error) {
			toast.error(
				`Failed to list modpack versions: ${
					error instanceof Error ? error.message : 'Unknown error'
				}`
			);
		} finally {
			versionsLoading = false;
		}
	}

	// Load versions lazily the first time the list is expanded
	$effect(() => {
		if (showVersions && !versionsLoaded && !versionsLoading) {
			loadVersions();
		}
	});

	async function performUpdate() {
		updating = true;
		try {
			const response = await rpcClient.modpackUpdate.updateServerModpack(
				create(UpdateServerModpackRequestSchema, {
					serverId: server.id,
					targetVersionId: selectedVersionId === LATEST_VALUE ? '' : selectedVersionId,
					createBackup
				}),
				silentCallOptions
			);
			lastUpdate = response;
			updateOpen = false;

			if (response.status === 'updated') {
				toast.success('Modpack updated', {
					description: response.message || `Updated to ${targetLabel}`
				});
			} else {
				toast.error('Modpack update failed', {
					description: response.message || undefined
				});
			}

			// Refresh the check snapshot and versions afterwards
			runCheck();
			if (versionsLoaded) loadVersions();
		} catch (error) {
			toast.error(
				`Failed to update modpack: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			updating = false;
		}
	}

	async function rollback() {
		const confirmed = confirm(
			'Restore the pre-update backup for this server?\n\nThe modpack will be reverted to the state captured before the last update. The server must be stopped.'
		);
		if (!confirmed) return;

		rollingBack = true;
		try {
			const response = await rpcClient.modpackUpdate.rollbackModpackUpdate(
				create(RollbackModpackUpdateRequestSchema, { serverId: server.id }),
				silentCallOptions
			);
			if (response.status === 'restored') {
				toast.success('Rollback complete', {
					description: response.message || 'Pre-update backup restored.'
				});
			} else {
				toast.error('Rollback failed', { description: response.message || undefined });
			}
			runCheck();
		} catch (error) {
			// Surface backend errors, e.g. FailedPrecondition "server must be stopped"
			toast.error(
				`Rollback failed: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			rollingBack = false;
		}
	}

	function applySettings(loaded?: ModpackUpdateSettings) {
		if (!loaded) return;
		settings = loaded;
		settingsEnabled = loaded.enabled;
		settingsInterval = loaded.intervalHours > 0 ? loaded.intervalHours : 24;
		settingsMode = loaded.mode === ModpackUpdateMode.APPLY ? 'apply' : 'notify';
	}

	async function loadSettings() {
		try {
			const response = await rpcClient.modpackUpdate.getModpackUpdateSettings(
				create(GetModpackUpdateSettingsRequestSchema, { serverId: server.id }),
				silentCallOptions
			);
			applySettings(response.settings);
		} catch (error) {
			console.debug('Failed to load modpack update settings:', error);
		}
	}

	function handleIntervalInput(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const value = Math.floor(Number(input.value));
		if (Number.isNaN(value) || value < 1) {
			input.value = '1';
			settingsInterval = 1;
		} else {
			settingsInterval = value;
		}
	}

	async function saveSettings() {
		savingSettings = true;
		try {
			const response = await rpcClient.modpackUpdate.setModpackUpdateSettings(
				create(SetModpackUpdateSettingsRequestSchema, {
					serverId: server.id,
					enabled: settingsEnabled,
					intervalHours: settingsInterval,
					mode: settingsMode === 'apply' ? ModpackUpdateMode.APPLY : ModpackUpdateMode.NOTIFY
				}),
				silentCallOptions
			);
			applySettings(response.settings);
			toast.success('Automatic update settings saved');
		} catch (error) {
			toast.error(
				`Failed to save settings: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			savingSettings = false;
		}
	}
</script>

<div class="space-y-6">
	<!-- Update check -->
	<div class="space-y-4">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div>
				<h4 class="text-sm font-semibold">Modpack Updates</h4>
				<p class="text-xs text-muted-foreground">
					Check for modpack updates and apply them in place — the world is preserved.
				</p>
			</div>
			<Button variant="outline" size="sm" onclick={runCheck} disabled={checking}>
				{#if checking}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<RefreshCw class="mr-2 h-4 w-4" />
				{/if}
				Check for updates
			</Button>
		</div>

		{#if checking && !check}
			<div class="flex h-24 items-center justify-center rounded-lg bg-muted/50">
				<Loader2 class="h-5 w-5 animate-spin text-muted-foreground" />
			</div>
		{:else if checkFailed}
			<Alert>
				<AlertCircle class="h-4 w-4" />
				<AlertDescription class="text-sm">
					Failed to check for updates. Make sure this server was created from a modpack and that
					the indexer is reachable.
				</AlertDescription>
			</Alert>
		{:else if check}
			<div class="space-y-3 rounded-lg bg-muted/50 p-4">
				<div class="flex flex-wrap items-center gap-2">
					<PackageSearch class="h-4 w-4 text-primary" />
					<span class="text-sm font-semibold">{check.modpackName || 'Modpack'}</span>
					<Badge variant="secondary" class="font-mono text-[10px]">
						{check.currentVersion || 'unknown'}
					</Badge>
				</div>

				{#if check.updateAvailable}
					<Alert class="border-warning/50 bg-warning/10">
						<TriangleAlert class="text-warning h-4 w-4" />
						<AlertDescription class="text-sm font-medium">
							Update available: {check.currentVersion || 'unknown'} → {check.latestVersion}
						</AlertDescription>
					</Alert>
				{/if}

				{#if check.error}
					<Alert>
						<AlertCircle class="h-4 w-4" />
						<AlertDescription class="text-sm">
							Can't check this modpack: {check.error}
							{#if check.error.toLowerCase().includes('manual')}
								Manual uploads can't be checked — upload a new pack to update it.
							{/if}
						</AlertDescription>
					</Alert>
				{/if}
			</div>
		{/if}

		<div class="flex flex-wrap items-center gap-2">
			<Button size="sm" onclick={() => (updateOpen = true)} disabled={checking || checkFailed}>
				{#if updating}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<Download class="mr-2 h-4 w-4" />
				{/if}
				Update Modpack
			</Button>
			<Button
				variant="outline"
				size="sm"
				onclick={rollback}
				disabled={rollingBack}
			>
				{#if rollingBack}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<Undo2 class="mr-2 h-4 w-4" />
				{/if}
				Rollback
			</Button>

			<Collapsible bind:open={showVersions}>
				<CollapsibleTrigger>
					{#snippet child({ props })}
						<Button {...props} variant="ghost" size="sm">
							<History class="mr-2 h-4 w-4" />
							{showVersions ? 'Hide versions' : 'List versions'}
						</Button>
					{/snippet}
				</CollapsibleTrigger>
				<CollapsibleContent class="mt-3">
					{#if versionsLoading}
						<div class="flex h-16 items-center justify-center rounded-lg bg-muted/50">
							<Loader2 class="h-4 w-4 animate-spin text-muted-foreground" />
						</div>
					{:else if versions.length === 0}
						<p class="rounded-lg bg-muted/50 p-4 text-sm text-muted-foreground">
							No versions available.
						</p>
					{:else}
						<RadioGroup bind:value={selectedVersionId} class="gap-2">
							<label
								class="flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50"
								for="version-latest"
							>
								<RadioGroupItem id="version-latest" value={LATEST_VALUE} />
								<div class="flex-1">
									<p class="text-sm font-medium">Latest ({check?.latestVersion || 'latest'})</p>
									<p class="text-xs text-muted-foreground">Most recent release of this modpack</p>
								</div>
							</label>
							{#each versions as version (version.versionId)}
								<label
									class="flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50"
									for="version-{version.versionId}"
								>
									<RadioGroupItem id="version-{version.versionId}" value={version.versionId} />
									<div class="flex-1">
										<p class="flex items-center gap-2 text-sm font-medium">
											{version.versionName}
											{#if version.isCurrent}
												<Badge variant="secondary" class="text-[10px]">current</Badge>
											{/if}
										</p>
										{#if version.releasedAt}
											<p class="text-xs text-muted-foreground">
												Released {timestampToDate(version.releasedAt).toLocaleDateString(undefined, {
													month: 'short',
													day: 'numeric',
													year: 'numeric'
												})}
											</p>
										{/if}
									</div>
								</label>
							{/each}
						</RadioGroup>
					{/if}
				</CollapsibleContent>
			</Collapsible>
		</div>

		{#if lastUpdate}
			<Alert class={lastUpdate.status === 'updated' ? '' : 'border-destructive/50'}>
				{#if lastUpdate.status === 'updated'}
					<PackageCheck class="h-4 w-4" />
				{:else}
					<AlertCircle class="text-destructive h-4 w-4" />
				{/if}
				<AlertDescription class="text-sm">
					{#if lastUpdate.status === 'updated'}
						Update to {targetLabel} finished. {lastUpdate.message}
					{:else}
						Update to {targetLabel} failed. {lastUpdate.message}
					{/if}
					{#if lastUpdate.backupFilename}
						{#if lastUpdate.status === 'updated'}
							Pre-update backup saved as <span class="font-mono text-xs"
								>{lastUpdate.backupFilename}</span
							>.
						{:else}
							A pre-update backup was saved as <span class="font-mono text-xs"
								>{lastUpdate.backupFilename}</span
							> — you can roll back to it with the Rollback button above.
						{/if}
					{/if}
				</AlertDescription>
			</Alert>
		{/if}
	</div>

	<Separator class="my-2" />

	<!-- Automatic updates -->
	<div class="space-y-4">
		<div>
			<h4 class="flex items-center gap-2 text-sm font-semibold">
				<Clock class="h-4 w-4 text-muted-foreground" />
				Automatic Updates
			</h4>
			<p class="text-xs text-muted-foreground">
				Periodically check for new modpack versions and notify or update automatically.
			</p>
		</div>

		<div class="flex items-center justify-between rounded-lg bg-muted/50 p-4">
			<div class="space-y-0.5">
				<Label for="modpack_update_enabled" class="cursor-pointer text-sm font-medium"
					>Enable Automatic Updates</Label
				>
				<p class="text-xs text-muted-foreground">
					Run scheduled checks for this server's modpack
				</p>
			</div>
			<Switch
				id="modpack_update_enabled"
				checked={settingsEnabled}
				onCheckedChange={(checked) => (settingsEnabled = checked)}
			/>
		</div>

		{#if settingsEnabled}
			<div class="grid gap-4 md:grid-cols-2">
				<div class="space-y-2">
					<Label for="modpack_update_interval" class="text-sm font-medium"
						>Check Interval (hours)</Label
					>
					<Input
						id="modpack_update_interval"
						type="number"
						min="1"
						step="1"
						bind:value={settingsInterval}
						oninput={handleIntervalInput}
						class="h-10"
					/>
					<p class="text-xs text-muted-foreground">How often to check for new versions</p>
				</div>

				<div class="space-y-2">
					<Label for="modpack_update_mode" class="text-sm font-medium">Update Mode</Label>
					<Select
						type="single"
						value={settingsMode}
						onValueChange={(value: string) => (settingsMode = value === 'apply' ? 'apply' : 'notify')}
					>
						<SelectTrigger id="modpack_update_mode" class="h-10">
							<span>
								{settingsMode === 'apply' ? 'Auto-apply updates' : 'Notify only'}
							</span>
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="notify">Notify only</SelectItem>
							<SelectItem value="apply">Auto-apply updates</SelectItem>
						</SelectContent>
					</Select>
					{#if settingsMode === 'apply'}
						<Alert class="border-warning/50 bg-warning/10">
							<TriangleAlert class="text-warning h-4 w-4" />
							<AlertDescription class="text-xs">
								New versions will be installed automatically: the server is backed up, stopped,
								updated and restarted without confirmation.
							</AlertDescription>
						</Alert>
					{/if}
				</div>
			</div>
		{/if}

		{#if settings}
			<div class="space-y-1 rounded-lg bg-muted/30 p-4 text-xs text-muted-foreground">
				<p>
					Last check:
					{#if settings.lastCheck && Number(settings.lastCheck.seconds) > 0}
						{formatTimeAgo(timestampToDate(settings.lastCheck))}
					{:else}
						Never
					{/if}
				</p>
				{#if settings.lastResult}
					<p>Last result: {settings.lastResult}</p>
				{/if}
			</div>
		{/if}

		<div class="flex justify-end">
			<Button
				onclick={saveSettings}
				disabled={!settingsDirty || savingSettings}
				size="sm"
				class="min-w-[120px]"
			>
				{#if savingSettings}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<PackageCheck class="mr-2 h-4 w-4" />
				{/if}
				Save Settings
			</Button>
		</div>
	</div>
</div>

<!-- Update flow dialog -->
<Dialog bind:open={updateOpen}>
	<DialogContent class="sm:max-w-md">
		<DialogHeader>
			<DialogTitle class="flex items-center gap-2">
				<Download class="h-5 w-5 text-primary" />
				Update Modpack
			</DialogTitle>
			<DialogDescription class="text-sm text-muted-foreground">
				Update "{server.name}" to <span class="font-medium">{targetLabel}</span>.
			</DialogDescription>
		</DialogHeader>

		<div class="space-y-4 py-2">
			<Alert class="border-warning/50 bg-warning/10">
				<TriangleAlert class="text-warning h-4 w-4" />
				<AlertDescription class="text-sm">
					The server will be stopped and restarted if it is currently running.
				</AlertDescription>
			</Alert>

			<div class="flex items-start justify-between gap-4 rounded-lg bg-muted/50 p-4">
				<div class="space-y-1">
					<Label for="create_backup" class="cursor-pointer text-sm font-medium"
						>Create pre-update backup</Label
					>
					<p class="text-xs text-muted-foreground">
						Recommended: captures the current mods and config so the update can be rolled back.
					</p>
				</div>
				<Checkbox
					id="create_backup"
					checked={createBackup}
					onCheckedChange={(checked) => (createBackup = checked === true)}
					class="mt-0.5 shrink-0"
				/>
			</div>
		</div>

		<DialogFooter>
			<Button variant="outline" onclick={() => (updateOpen = false)} disabled={updating}>
				Cancel
			</Button>
			<Button onclick={performUpdate} disabled={updating}>
				{#if updating}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<Download class="mr-2 h-4 w-4" />
				{/if}
				Update to {targetLabel}
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>
