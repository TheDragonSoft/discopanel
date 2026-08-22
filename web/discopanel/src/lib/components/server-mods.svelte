<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Progress } from '$lib/components/ui/progress';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import {
		Loader2,
		Plus,
		Upload,
		Download,
		Trash2,
		Package,
		Blocks,
		FileText,
		ExternalLink,
		RefreshCw,
		Search,
		X
	} from '@lucide/svelte';
	import { rpcClient } from '$lib/api/rpc-client';
	import { toast } from 'svelte-sonner';
	import { ModLoader, type Server } from '$lib/proto/discopanel/v1/common_pb';
	import type { Mod } from '$lib/proto/discopanel/v1/mod_pb';
	import { formatBytes } from '$lib/utils';
	import { uploadFile, cancelUpload, type UploadProgress } from '$lib/utils/chunked-upload';

	interface Props {
		server: Server;
		active?: boolean;
	}

	let { server, active = false }: Props = $props();

	let mods = $state<Mod[]>([]);
	let searchQuery = $state('');
	let loading = $state(true);
	let uploading = $state(false);
	let uploadProgress = $state<UploadProgress | null>(null);
	let currentUploadFilename = $state('');
	let uploadAbortController = $state<AbortController | null>(null);
	let fileInput = $state<HTMLInputElement | null>(null);
	let togglingModIds = $state<Set<string>>(new Set());
	let isDragging = $state(false);
	let dragCounter = 0;

	let hasLoaded = false;
	let previousServerId = $state(server.id);

	// Filtered mods based on search query
	let filteredMods = $derived(
		mods.filter((mod) => {
			if (!searchQuery.trim()) return true;
			const q = searchQuery.toLowerCase();
			return (
				mod.displayName.toLowerCase().includes(q) ||
				mod.fileName.toLowerCase().includes(q) ||
				mod.description.toLowerCase().includes(q) ||
				mod.author.toLowerCase().includes(q)
			);
		})
	);

	let enabledCount = $derived(mods.filter((m) => m.enabled).length);

	// Reset state when server changes
	$effect(() => {
		if (server.id !== previousServerId) {
			previousServerId = server.id;
			// Reset state variables
			mods = [];
			searchQuery = '';
			loading = true;
			uploading = false;
			hasLoaded = false;
			isDragging = false;
			dragCounter = 0;
		}
	});

	$effect(() => {
		if (active && !hasLoaded) {
			hasLoaded = true;
			loadMods();
		}
	});

	async function loadMods() {
		try {
			loading = true;
			const response = await rpcClient.mod.listMods({ serverId: server.id });
			mods = response.mods;
		} catch (_e) {
			if (server.modLoader !== ModLoader.VANILLA) {
				toast.error('Failed to load mods');
			}
		} finally {
			loading = false;
		}
	}

	async function processUploadFiles(fileList: FileList | File[]) {
		const files = Array.from(fileList);
		if (files.length === 0) return;

		// Filter for valid mod extensions
		const validModExtensions = ['.jar', '.zip', '.litemod', '.disabled'];
		const validFiles = files.filter((f) => {
			const lower = f.name.toLowerCase();
			return validModExtensions.some((ext) => lower.endsWith(ext));
		});

		if (validFiles.length === 0) {
			toast.error('Only .jar, .zip, or .litemod files are supported as mods');
			return;
		}

		if (validFiles.length < files.length) {
			toast.info(
				`Uploading ${validFiles.length} mod(s) (skipped ${files.length - validFiles.length} unsupported file(s))`
			);
		}

		uploading = true;
		uploadAbortController = new AbortController();

		try {
			for (const file of validFiles) {
				currentUploadFilename = file.name;
				uploadProgress = null;

				// Use chunked upload
				const result = await uploadFile(file, {
					onProgress: (progress) => {
						uploadProgress = progress;
					},
					signal: uploadAbortController.signal
				});

				// Import the uploaded mod
				await rpcClient.mod.importUploadedMod({
					serverId: server.id,
					uploadSessionId: result.sessionId,
					displayName: '',
					description: ''
				});
			}
			toast.success(`Uploaded ${validFiles.length} mod(s)`);
			await loadMods();
		} catch (error: unknown) {
			if (error instanceof Error && error.message === 'Upload cancelled') {
				toast.info('Upload cancelled');
			} else {
				toast.error('Failed to upload mod');
			}
		} finally {
			uploading = false;
			uploadProgress = null;
			currentUploadFilename = '';
			uploadAbortController = null;
			if (fileInput) fileInput.value = '';
		}
	}

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files) {
			processUploadFiles(input.files);
		}
	}

	function handleDragEnter(e: DragEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (!canHaveMods()) return;
		dragCounter++;
		if (e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')) {
			isDragging = true;
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (!canHaveMods()) return;
		if (e.dataTransfer) {
			e.dataTransfer.dropEffect = 'copy';
		}
	}

	function handleDragLeave(e: DragEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (!canHaveMods()) return;
		dragCounter--;
		if (dragCounter <= 0) {
			isDragging = false;
			dragCounter = 0;
		}
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		e.stopPropagation();
		isDragging = false;
		dragCounter = 0;

		if (!canHaveMods() || uploading) return;

		if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
			processUploadFiles(e.dataTransfer.files);
		}
	}

	function cancelCurrentUpload() {
		if (uploadAbortController) {
			uploadAbortController.abort();
		}
		if (uploadProgress?.sessionId) {
			cancelUpload(uploadProgress.sessionId).catch(() => {});
		}
	}

	async function toggleMod(mod: Mod) {
		if (togglingModIds.has(mod.id)) return;

		const targetState = !mod.enabled;
		const newToggling = new Set(togglingModIds);
		newToggling.add(mod.id);
		togglingModIds = newToggling;

		try {
			const res = await rpcClient.mod.updateMod({
				serverId: server.id,
				modId: mod.id,
				enabled: targetState,
				displayName: mod.displayName,
				description: mod.description
			});

			// Update in-place in state for immediate smooth response
			if (res.mod) {
				const updatedMod = res.mod;
				mods = mods.map((m) => (m.id === mod.id ? updatedMod : m));
			} else {
				await loadMods();
			}

			toast.success(`Mod "${mod.displayName}" ${targetState ? 'enabled' : 'disabled'}`);
		} catch (_e) {
			toast.error(`Failed to ${targetState ? 'enable' : 'disable'} mod`);
			await loadMods();
		} finally {
			const updated = new Set(togglingModIds);
			updated.delete(mod.id);
			togglingModIds = updated;
		}
	}

	async function deleteMod(mod: Mod) {
		const confirmed = confirm(`Are you sure you want to delete "${mod.displayName}"?`);
		if (!confirmed) return;

		try {
			await rpcClient.mod.deleteMod({
				serverId: server.id,
				modId: mod.id
			});
			toast.success('Mod deleted');
			await loadMods();
		} catch (_e) {
			toast.error('Failed to delete mod');
		}
	}

	async function downloadMod(mod: Mod) {
		try {
			const response = await rpcClient.file.getFile({
				serverId: server.id,
				path: `${getModsDirectory()}/${mod.fileName}`
			});
			const blob = new Blob([new Uint8Array(response.content)]);
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = mod.fileName;
			a.click();
			URL.revokeObjectURL(url);
		} catch (_e) {
			toast.error('Failed to download mod');
		}
	}

	function getModsDirectory(): string {
		const modLoaderInfo: Record<ModLoader, string> = {
			[ModLoader.UNSPECIFIED]: 'mods',
			[ModLoader.VANILLA]: 'mods',
			[ModLoader.FORGE]: 'mods',
			[ModLoader.NEOFORGE]: 'mods',
			[ModLoader.FABRIC]: 'mods',
			[ModLoader.QUILT]: 'mods',
			[ModLoader.BUKKIT]: 'plugins',
			[ModLoader.SPIGOT]: 'plugins',
			[ModLoader.PAPER]: 'plugins',
			[ModLoader.PURPUR]: 'plugins',
			[ModLoader.SPONGE_VANILLA]: 'mods',
			[ModLoader.SPONGE_FORGE]: 'mods',
			[ModLoader.MOHIST]: 'mods',
			[ModLoader.CATSERVER]: 'mods',
			[ModLoader.ARCLIGHT]: 'mods',
			[ModLoader.AUTO_CURSEFORGE]: 'mods',
			[ModLoader.MODRINTH]: 'mods',
			[ModLoader.FOLIA]: 'plugins'
		};

		return modLoaderInfo[server.modLoader] || 'mods';
	}

	function canHaveMods(): boolean {
		const noModLoaders = [ModLoader.VANILLA, ModLoader.UNSPECIFIED];
		return !noModLoaders.includes(server.modLoader);
	}
</script>

<Card
	class="relative rounded-xl border border-border/50 bg-card shadow-sm"
	ondragenter={handleDragEnter}
	ondragover={handleDragOver}
	ondragleave={handleDragLeave}
	ondrop={handleDrop}
>
	<!-- Minimalist Drag & Drop Overlay -->
	{#if isDragging && canHaveMods()}
		<div
			class="absolute inset-0 z-50 flex items-center justify-center p-4 backdrop-blur-xs bg-background/80 dark:bg-zinc-950/80 pointer-events-none select-none transition-opacity duration-150 rounded-xl"
		>
			<div
				class="flex h-full min-h-[300px] w-full flex-col items-center justify-center rounded-xl border-2 border-dashed border-primary/60 bg-muted/20 p-6"
			>
				<Upload class="h-8 w-8 text-primary" />
				<p class="mt-3 text-base font-semibold text-foreground">Drop mods to upload</p>
				<p class="mt-1 text-xs text-muted-foreground">.jar or .zip files</p>
			</div>
		</div>
	{/if}

	<CardHeader class="p-5 sm:p-6 pb-4 border-b border-border/40">
		<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
			<div>
				<div class="flex items-center gap-2.5">
					<CardTitle class="text-lg sm:text-xl font-bold tracking-tight">Mod Management</CardTitle>
					{#if canHaveMods() && !loading && mods.length > 0}
						<Badge variant="secondary" class="text-xs font-semibold px-2 py-0.5">
							{mods.length} {mods.length === 1 ? 'mod' : 'mods'} ({enabledCount} active)
						</Badge>
					{/if}
				</div>
				<p class="mt-1 text-sm text-muted-foreground">
					{#if canHaveMods()}
						Manage mods in the <span class="font-mono text-foreground/80">{getModsDirectory()}</span> directory
					{:else}
						This server type does not support mods
					{/if}
				</p>
			</div>

			{#if canHaveMods()}
				<div class="flex items-center gap-2.5">
					<Button
						variant="outline"
						size="icon"
						onclick={loadMods}
						disabled={loading}
						title="Refresh mods list"
						class="h-9 w-9 shrink-0"
					>
						<RefreshCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
					</Button>
					<Button href="/mods" variant="outline" class="h-9 shadow-xs">
						<Plus class="mr-2 h-4 w-4" />
						Add Mods
					</Button>
					<Button onclick={() => fileInput?.click()} disabled={uploading} class="h-9 shadow-xs">
						{#if uploading}
							<Loader2 class="mr-2 h-4 w-4 animate-spin" />
						{:else}
							<Upload class="mr-2 h-4 w-4" />
						{/if}
						Upload Mods
					</Button>
					<input
						bind:this={fileInput}
						type="file"
						multiple
						accept=".jar,.zip"
						onchange={handleFileSelect}
						class="hidden"
					/>
				</div>
			{/if}
		</div>

		{#if canHaveMods() && mods.length > 3}
			<div class="relative mt-3">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					placeholder="Search installed mods by name, filename, author..."
					bind:value={searchQuery}
					class="pl-9 h-9.5 text-sm bg-background/50"
				/>
				{#if searchQuery}
					<button
						onclick={() => (searchQuery = '')}
						class="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground p-0.5"
						aria-label="Clear search"
					>
						<X class="h-4 w-4" />
					</button>
				{/if}
			</div>
		{/if}
	</CardHeader>

	{#if uploading && uploadProgress}
		<div class="px-6 py-4 bg-muted/20 border-b border-border/40">
			<div class="mb-2 flex items-center justify-between">
				<span class="text-sm font-medium text-foreground">
					Uploading: <span class="font-mono text-muted-foreground">{currentUploadFilename}</span>
				</span>
				<div class="flex items-center gap-2">
					<span class="text-sm font-semibold text-primary">
						{uploadProgress.percentComplete.toFixed(0)}%
					</span>
					<Button
						size="icon"
						variant="ghost"
						class="h-6 w-6 text-muted-foreground hover:text-foreground"
						onclick={cancelCurrentUpload}
						title="Cancel upload"
					>
						<X class="h-4 w-4" />
					</Button>
				</div>
			</div>
			<Progress value={uploadProgress.percentComplete} class="h-2" />
			<p class="mt-1.5 text-xs text-muted-foreground">
				{formatBytes(uploadProgress.bytesUploaded)} of {formatBytes(uploadProgress.totalBytes)}
			</p>
		</div>
	{/if}

	<CardContent class="p-5 sm:p-6 space-y-3">
		{#if !canHaveMods()}
			<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
				<Package class="mb-4 h-12 w-12 opacity-50" />
				<p class="text-base font-medium">This server type does not support mods</p>
			</div>
		{:else if loading && mods.length === 0}
			<div class="flex flex-col items-center justify-center py-20 gap-3 text-muted-foreground">
				<Loader2 class="h-9 w-9 animate-spin text-primary" />
				<p class="text-sm font-medium">Scanning server mods...</p>
			</div>
		{:else if mods.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-muted-foreground space-y-2">
				<div class="flex h-16 w-16 items-center justify-center rounded-2xl bg-muted/50 mb-2">
					<Package class="h-8 w-8 opacity-60" />
				</div>
				<p class="text-base font-semibold text-foreground">No mods installed</p>
				<p class="text-sm text-muted-foreground max-w-sm text-center">Browse mods on Modrinth or drop / upload mod JAR files directly.</p>
				<div class="flex flex-wrap items-center justify-center gap-3 mt-3">
					<Button href="/mods" class="shadow-xs">
						<Plus class="mr-2 h-4 w-4" />
						Add Mods
					</Button>
					<Button onclick={() => fileInput?.click()} variant="outline">
						<Upload class="mr-2 h-4 w-4" />
						Upload Mods
					</Button>
				</div>
			</div>
		{:else if filteredMods.length === 0}
			<div class="flex flex-col items-center justify-center py-16 text-muted-foreground space-y-2">
				<Search class="h-10 w-10 opacity-40 mb-1" />
				<p class="text-base font-medium">No mods match "{searchQuery}"</p>
				<Button variant="link" size="sm" onclick={() => (searchQuery = '')}>
					Clear search filter
				</Button>
			</div>
		{:else}
			<div class="space-y-3">
				{#each filteredMods as mod (mod.id)}
					<div
						class="group flex items-center justify-between gap-4 sm:gap-5 rounded-xl border p-4 sm:p-4.5 bg-card/60 transition-[border-color,background-color,box-shadow] duration-150 hover:border-primary/40 hover:bg-card/95 hover:shadow-xs [content-visibility:auto] [contain-intrinsic-size:auto_80px] {mod.enabled
							? 'border-border/60'
							: 'opacity-70 bg-muted/20 border-dashed border-border/40'}"
					>
						<!-- Left: Switch + Mod Picture + Name & Meta -->
						<div class="flex min-w-0 flex-1 items-center gap-4">
							<!-- Modern Switch Control -->
							<div class="flex shrink-0 items-center justify-center">
								<Switch
									checked={mod.enabled}
									onCheckedChange={() => toggleMod(mod)}
									disabled={togglingModIds.has(mod.id)}
									class="data-[state=checked]:bg-emerald-500 cursor-pointer"
									aria-label={mod.enabled ? `Disable ${mod.displayName}` : `Enable ${mod.displayName}`}
								/>
							</div>

							<!-- Mod Picture / Icon Thumbnail -->
							<div
								class="relative flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-border/60 bg-muted/40 shadow-xs transition-transform duration-150 group-hover:scale-105"
							>
								{#if mod.iconUrl}
									<img
										src={mod.iconUrl}
										alt={mod.displayName}
										loading="lazy"
										decoding="async"
										class="h-full w-full object-contain p-1 rounded-lg"
										onerror={(e) => {
											const target = e.currentTarget as HTMLElement;
											target.style.display = 'none';
											const fallback = target.nextElementSibling as HTMLElement;
											if (fallback) fallback.classList.remove('hidden');
										}}
									/>
									<div class="hidden flex h-full w-full items-center justify-center text-primary/70 bg-primary/5">
										<Blocks class="h-7 w-7" />
									</div>
								{:else}
									<div class="flex h-full w-full items-center justify-center text-primary/70 bg-primary/5">
										<Blocks class="h-7 w-7" />
									</div>
								{/if}
							</div>

							<!-- Mod Name & Metadata Details -->
							<div class="min-w-0 flex-1">
								<div class="flex flex-wrap items-center gap-2">
									<h4 class="font-semibold text-base tracking-tight text-foreground truncate">
										{mod.displayName}
									</h4>
									{#if mod.version}
										<Badge variant="secondary" class="text-xs font-mono font-medium px-2 py-0.5">
											{mod.version}
										</Badge>
									{/if}
									{#if mod.enabled}
										<Badge
											variant="outline"
											class="text-[11px] text-emerald-600 dark:text-emerald-400 border-emerald-500/30 bg-emerald-500/10 font-medium px-2 py-0.5"
										>
											Active
										</Badge>
									{:else}
										<Badge
											variant="outline"
											class="text-[11px] text-muted-foreground border-border bg-muted/30 font-medium px-2 py-0.5"
										>
											Disabled
										</Badge>
									{/if}
									{#if mod.author}
										<span class="text-xs text-muted-foreground hidden md:inline">
											by <span class="font-medium text-foreground/80">{mod.author}</span>
										</span>
									{/if}
								</div>

								<!-- Subtitle: filename, size, date, website link -->
								<div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
									<span class="flex items-center gap-1 font-mono text-[11px]">
										<FileText class="h-3.5 w-3.5 shrink-0" />
										{mod.fileName}
									</span>
									<span>•</span>
									<span>{formatBytes(Number(mod.fileSize))}</span>
									{#if mod.uploadedAt}
										<span>•</span>
										<span>{new Date(Number(mod.uploadedAt.seconds) * 1000).toLocaleDateString()}</span>
									{/if}
									{#if mod.website}
										<span>•</span>
										<a
											href={mod.website}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-0.5 text-primary hover:underline transition-colors"
										>
											<ExternalLink class="h-3 w-3" />
											Website
										</a>
									{/if}
								</div>

								{#if mod.description}
									<p class="mt-1 text-xs text-muted-foreground line-clamp-1 leading-normal">
										{mod.description}
									</p>
								{/if}
							</div>
						</div>

						<!-- Right: Action Buttons (Download & Delete) -->
						<div class="flex items-center gap-1 shrink-0">
							<Button
								size="icon"
								variant="ghost"
								class="h-8.5 w-8.5 text-muted-foreground hover:text-foreground hover:bg-muted"
								onclick={() => downloadMod(mod)}
								title="Download mod"
							>
								<Download class="h-4 w-4" />
							</Button>
							<Button
								size="icon"
								variant="ghost"
								class="h-8.5 w-8.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
								onclick={() => deleteMod(mod)}
								title="Delete mod"
							>
								<Trash2 class="h-4 w-4" />
							</Button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</CardContent>
</Card>
