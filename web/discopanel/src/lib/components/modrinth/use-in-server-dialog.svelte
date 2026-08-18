<script lang="ts">
	import { Dialog, DialogContent } from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Progress } from '$lib/components/ui/progress';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import {
		Loader2,
		Download,
		Server as ServerIcon,
		AlertCircle,
		ExternalLink,
		Package,
		X,
		Layers,
		CheckCircle2,
		ArrowRight,
		Check,
		ChevronDown,
		ChevronUp,
		Search,
		Puzzle
	} from '@lucide/svelte';
	import { rpcClient } from '$lib/api/rpc-client';
	import { serversStore } from '$lib/stores/servers';
	import { ModLoader, ServerStatus, type Server } from '$lib/proto/discopanel/v1/common_pb';
	import { uploadFile, type UploadProgress } from '$lib/utils/chunked-upload';
	import { formatBytes } from '$lib/utils';
	import { toast } from 'svelte-sonner';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		getModrinthVersions,
		downloadModrinthFileBlob,
		resolveDependenciesForVersion,
		type ModrinthSearchHit,
		type ModrinthVersion,
		type ResolvedDependency
	} from '$lib/api/modrinth';

	interface Props {
		open: boolean;
		project: ModrinthSearchHit | null;
	}

	let { open = $bindable(), project }: Props = $props();

	let servers: Server[] = $derived($serversStore);
	let selectedServerId = $state<string>('');
	let selectedServer = $derived(servers.find((s) => s.id === selectedServerId) || null);

	let versions = $state<ModrinthVersion[]>([]);
	let loadingVersions = $state(false);
	let selectedVersionId = $state<string>('');
	let selectedVersion = $derived(versions.find((v) => v.id === selectedVersionId) || null);
	let showAllVersions = $state(false);

	// Version picker state
	let isVersionPickerOpen = $state(false);
	let versionSearchQuery = $state('');

	// Dependencies state
	let dependencies = $state<ResolvedDependency[]>([]);
	let loadingDependencies = $state(false);

	// Installation state
	type InstallStep = 'idle' | 'downloading' | 'uploading' | 'saving' | 'completed';
	let installStep = $state<InstallStep>('idle');
	let installProgress = $state(0);
	let installStatusText = $state('');
	let installedItems = $state<Array<{ name: string; filename: string; isDep: boolean }>>([]);
	let errorMessage = $state('');

	// Reset state when dialog opens or project changes
	$effect(() => {
		if (open && project) {
			installStep = 'idle';
			installProgress = 0;
			installStatusText = '';
			errorMessage = '';
			installedItems = [];
			dependencies = [];
			showAllVersions = false;
			isVersionPickerOpen = false;
			versionSearchQuery = '';

			// If we have servers and none selected, auto-select first running or first available
			if (servers.length > 0 && !selectedServerId) {
				const running = servers.find((s) => s.status === ServerStatus.RUNNING);
				selectedServerId = running ? running.id : servers[0].id;
			}

			loadVersionsForProject();
		}
	});

	// Reload or filter versions when selected server changes
	$effect(() => {
		if (selectedServer && versions.length > 0) {
			autoSelectBestVersion();
		}
	});

	// When selected version changes, resolve its dependencies
	$effect(() => {
		if (selectedVersion) {
			loadDependenciesForSelectedVersion();
		} else {
			dependencies = [];
		}
	});

	async function loadVersionsForProject() {
		if (!project) return;
		loadingVersions = true;
		versions = [];
		selectedVersionId = '';
		dependencies = [];

		try {
			const projectVersions = await getModrinthVersions(project.project_id || project.slug);
			versions = projectVersions;
			autoSelectBestVersion();
		} catch (error) {
			console.error('Failed to load Modrinth versions:', error);
			toast.error('Failed to fetch versions for this project');
		} finally {
			loadingVersions = false;
		}
	}

	async function loadDependenciesForSelectedVersion() {
		if (!selectedVersion) {
			dependencies = [];
			return;
		}

		loadingDependencies = true;
		try {
			const mcVer = selectedServer?.mcVersion;
			const loader = selectedServer ? getLoaderString(selectedServer.modLoader) : undefined;
			const deps = await resolveDependenciesForVersion(selectedVersion, mcVer, loader);
			dependencies = deps;
		} catch (error) {
			console.error('Failed to resolve dependencies:', error);
			dependencies = [];
		} finally {
			loadingDependencies = false;
		}
	}

	function getLoaderString(loader: ModLoader): string {
		switch (loader) {
			case ModLoader.FABRIC:
				return 'fabric';
			case ModLoader.FORGE:
				return 'forge';
			case ModLoader.NEOFORGE:
				return 'neoforge';
			case ModLoader.QUILT:
				return 'quilt';
			case ModLoader.PAPER:
			case ModLoader.PURPUR:
			case ModLoader.FOLIA:
			case ModLoader.SPIGOT:
			case ModLoader.BUKKIT:
				return 'paper';
			default:
				return '';
		}
	}

	function isVersionCompatible(version: ModrinthVersion, server: Server | null): boolean {
		if (!server) return true;

		// Resource packs can work on any server
		if (project?.project_type === 'resourcepack') {
			if (server.mcVersion && version.game_versions?.length > 0) {
				return version.game_versions.includes(server.mcVersion);
			}
			return true;
		}

		// For mods: check loader and MC version
		const serverLoaderStr = getLoaderString(server.modLoader);
		const loaderMatches =
			!serverLoaderStr ||
			version.loaders.length === 0 ||
			version.loaders.some((l) => l.toLowerCase() === serverLoaderStr);

		const versionMatches =
			!server.mcVersion ||
			version.game_versions.length === 0 ||
			version.game_versions.includes(server.mcVersion);

		return loaderMatches && versionMatches;
	}

	let displayVersions = $derived(
		showAllVersions
			? versions
			: versions.filter((v) => isVersionCompatible(v, selectedServer))
	);

	let filteredDisplayVersions = $derived(
		displayVersions.filter((v) => {
			if (!versionSearchQuery.trim()) return true;
			const q = versionSearchQuery.toLowerCase().trim();
			const nameMatch = (v.name || '').toLowerCase().includes(q);
			const verNumberMatch = (v.version_number || '').toLowerCase().includes(q);
			const gameVerMatch = v.game_versions.some((gv) => gv.toLowerCase().includes(q));
			const loaderMatch = v.loaders.some((l) => l.toLowerCase().includes(q));
			const fileMatch = v.files.some((f) => f.filename.toLowerCase().includes(q));
			return nameMatch || verNumberMatch || gameVerMatch || loaderMatch || fileMatch;
		})
	);

	function autoSelectBestVersion() {
		if (versions.length === 0) return;

		// 1. Look for compatible release
		const compatibleReleases = versions.filter(
			(v) => isVersionCompatible(v, selectedServer) && v.version_type === 'release'
		);
		if (compatibleReleases.length > 0) {
			selectedVersionId = compatibleReleases[0].id;
			return;
		}

		// 2. Look for any compatible version
		const anyCompatible = versions.filter((v) => isVersionCompatible(v, selectedServer));
		if (anyCompatible.length > 0) {
			selectedVersionId = anyCompatible[0].id;
			return;
		}

		// 3. Fall back to newest release or first version
		const releases = versions.filter((v) => v.version_type === 'release');
		if (releases.length > 0) {
			selectedVersionId = releases[0].id;
		} else {
			selectedVersionId = versions[0].id;
		}
	}

	function getTargetDirectory(server: Server): string {
		if (project?.project_type === 'resourcepack') {
			return 'resourcepacks';
		}

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

	function isServerVanilla(server: Server): boolean {
		return server.modLoader === ModLoader.VANILLA || server.modLoader === ModLoader.UNSPECIFIED;
	}

	async function handleInstall() {
		if (!selectedServer) {
			toast.error('Please select a target server');
			return;
		}
		if (!selectedVersion) {
			toast.error('Please select a version to install');
			return;
		}

		const primaryFile =
			selectedVersion.files.find((f) => f.primary) || selectedVersion.files[0];
		if (!primaryFile || !primaryFile.url) {
			toast.error('No downloadable file found for this version');
			return;
		}

		const selectedDeps = dependencies.filter((d) => d.selected && d.downloadUrl && d.filename);
		const totalItems = 1 + selectedDeps.length;
		let currentItemIndex = 0;

		installedItems = [];
		installStep = 'downloading';
		installProgress = 5;
		errorMessage = '';

		const targetDir = getTargetDirectory(selectedServer);

		try {
			// 1. Download and install all selected dependencies first
			for (let i = 0; i < selectedDeps.length; i++) {
				const dep = selectedDeps[i];
				currentItemIndex++;

				installStatusText = `Downloading dependency (${currentItemIndex}/${totalItems}): ${dep.projectTitle}...`;

				const depFileObj = await downloadModrinthFileBlob(
					dep.downloadUrl!,
					dep.filename!,
					(percent) => {
						const baseProgress = (i / totalItems) * 100;
						const stepFraction = (1 / totalItems) * 0.45;
						installProgress = Math.round(baseProgress + percent * stepFraction);
					}
				);

				installStatusText = `Uploading dependency (${currentItemIndex}/${totalItems}): ${dep.projectTitle}...`;

				const depUploadResult = await uploadFile(depFileObj, {
					onProgress: (prog: UploadProgress) => {
						const baseProgress = (i / totalItems) * 100 + (1 / totalItems) * 45;
						const stepFraction = (1 / totalItems) * 0.45;
						installProgress = Math.round(baseProgress + prog.percentComplete * stepFraction);
					}
				});

				if (!depUploadResult.sessionId) {
					throw new Error(`Upload session failed for dependency: ${dep.projectTitle}`);
				}

				if (isServerVanilla(selectedServer)) {
					await rpcClient.file.saveUploadedFile({
						serverId: selectedServer.id,
						uploadSessionId: depUploadResult.sessionId,
						destinationPath: targetDir,
						filename: dep.filename!
					});
				} else {
					await rpcClient.mod.importUploadedMod({
						serverId: selectedServer.id,
						uploadSessionId: depUploadResult.sessionId,
						displayName: dep.projectTitle,
						description: `Dependency for ${project?.title}`
					});
				}

				installedItems = [
					...installedItems,
					{ name: dep.projectTitle, filename: dep.filename!, isDep: true }
				];
			}

			// 2. Download and install main project file
			currentItemIndex++;
			installStatusText = `Downloading ${project?.title || 'mod'} (${currentItemIndex}/${totalItems})...`;

			const mainFileObj = await downloadModrinthFileBlob(
				primaryFile.url,
				primaryFile.filename,
				(percent) => {
					const baseProgress = (selectedDeps.length / totalItems) * 100;
					const stepFraction = (1 / totalItems) * 0.45;
					installProgress = Math.round(baseProgress + percent * stepFraction);
				}
			);

			installStatusText = `Uploading ${project?.title || 'mod'} to server...`;

			const mainUploadResult = await uploadFile(mainFileObj, {
				onProgress: (prog: UploadProgress) => {
					const baseProgress = (selectedDeps.length / totalItems) * 100 + (1 / totalItems) * 45;
					const stepFraction = (1 / totalItems) * 0.45;
					installProgress = Math.round(baseProgress + prog.percentComplete * stepFraction);
				}
			});

			if (!mainUploadResult.sessionId) {
				throw new Error('Upload session failed to initialize');
			}

			installStatusText = 'Finalizing installation...';
			installProgress = 98;

			if (project?.project_type === 'resourcepack') {
				await rpcClient.file.saveUploadedFile({
					serverId: selectedServer.id,
					uploadSessionId: mainUploadResult.sessionId,
					destinationPath: targetDir,
					filename: primaryFile.filename
				});
			} else {
				if (isServerVanilla(selectedServer)) {
					await rpcClient.file.saveUploadedFile({
						serverId: selectedServer.id,
						uploadSessionId: mainUploadResult.sessionId,
						destinationPath: targetDir,
						filename: primaryFile.filename
					});
				} else {
					await rpcClient.mod.importUploadedMod({
						serverId: selectedServer.id,
						uploadSessionId: mainUploadResult.sessionId,
						displayName: project?.title || primaryFile.filename,
						description: project?.description || ''
					});
				}
			}

			installedItems = [
				...installedItems,
				{
					name: project?.title || primaryFile.filename,
					filename: primaryFile.filename,
					isDep: false
				}
			];

			installProgress = 100;
			installStep = 'completed';
			installStatusText = 'Installation complete!';
			toast.success(
				`Installed "${project?.title}"${selectedDeps.length > 0 ? ` + ${selectedDeps.length} dependencies` : ''} to "${selectedServer.name}"`
			);
		} catch (error) {
			console.error('Failed to install to server:', error);
			installStep = 'idle';
			errorMessage = error instanceof Error ? error.message : 'Installation failed';
			toast.error(`Installation failed: ${errorMessage}`);
		}
	}

	function closeDialog() {
		open = false;
	}

	function goToTargetServer() {
		if (selectedServer) {
			closeDialog();
			goto(resolve(`/servers/${selectedServer.id}`));
		}
	}
</script>

<Dialog bind:open>
	<DialogContent
		class="flex max-h-[92vh] w-[95vw] max-w-2xl flex-col gap-0 overflow-hidden rounded-2xl border-2 bg-card p-0 shadow-2xl"
		showCloseButton={false}
	>
		<!-- Header -->
		<div class="flex items-start justify-between border-b bg-muted/40 p-6">
			<div class="flex items-start gap-4 min-w-0 flex-1">
				{#if project?.icon_url}
					<img
						src={project.icon_url}
						alt={project.title}
						class="h-16 w-16 rounded-xl object-cover shadow-md shrink-0"
					/>
				{:else}
					<div
						class="flex h-16 w-16 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-md shrink-0"
					>
						<Package class="h-8 w-8" />
					</div>
				{/if}
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2 flex-wrap">
						<h2 class="truncate text-xl font-bold tracking-tight">{project?.title || 'Use in Server'}</h2>
						<Badge variant="secondary" class="text-xs uppercase font-semibold">
							{project?.project_type === 'resourcepack' ? 'Resource Pack' : 'Mod'}
						</Badge>
						<Badge variant="outline" class="text-xs">Modrinth</Badge>
					</div>
					<p class="text-sm text-muted-foreground mt-1">by <span class="font-medium text-foreground">{project?.author}</span></p>
					<p class="text-xs text-muted-foreground mt-1 line-clamp-1">{project?.description}</p>
				</div>
			</div>
			<Button
				variant="ghost"
				size="icon"
				onclick={closeDialog}
				class="h-8 w-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
			>
				<X class="h-4 w-4" />
			</Button>
		</div>

		<!-- Body Content -->
		<div class="flex-1 overflow-y-auto p-6 space-y-5">
			{#if installStep === 'completed'}
				<!-- Success Screen -->
				<div class="flex flex-col items-center justify-center py-6 text-center space-y-4">
					<div class="flex h-16 w-16 items-center justify-center rounded-full bg-green-500/10 text-green-500 ring-8 ring-green-500/5">
						<CheckCircle2 class="h-10 w-10" />
					</div>
					<div class="space-y-1">
						<h3 class="text-2xl font-bold tracking-tight">Successfully Installed!</h3>
						<p class="text-sm text-muted-foreground max-w-md">
							Files have been added to <span class="font-semibold text-foreground">{selectedServer?.name}</span> in the <code class="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{getTargetDirectory(selectedServer || ({} as Server))}/</code> directory.
						</p>
					</div>

					<!-- Installed Files List -->
					<div class="w-full max-w-md rounded-xl border bg-muted/20 p-3 space-y-1.5 text-left text-xs">
						<div class="font-semibold text-muted-foreground mb-2 flex items-center justify-between">
							<span>Installed Items ({installedItems.length}):</span>
							<span class="font-mono text-[11px] text-primary">/{getTargetDirectory(selectedServer || ({} as Server))}/</span>
						</div>
						{#each installedItems as item (item.filename)}
							<div class="flex items-center justify-between gap-2 py-1 border-b border-border/40 last:border-b-0">
								<div class="flex items-center gap-2 truncate">
									{#if item.isDep}
										<Puzzle class="h-3.5 w-3.5 text-primary shrink-0" />
									{:else}
										<Package class="h-3.5 w-3.5 text-green-500 shrink-0" />
									{/if}
									<span class="font-medium truncate">{item.name}</span>
									{#if item.isDep}
										<Badge variant="outline" class="text-[9px] uppercase px-1 py-0 font-bold">Dependency</Badge>
									{/if}
								</div>
								<span class="font-mono text-muted-foreground text-[11px] truncate max-w-36 shrink-0">{item.filename}</span>
							</div>
						{/each}
					</div>

					{#if selectedServer?.status === ServerStatus.RUNNING}
						<Alert class="mt-2 text-left border-amber-500/30 bg-amber-500/10 text-amber-500">
							<AlertCircle class="h-4 w-4 text-amber-500" />
							<AlertTitle>Server is Running</AlertTitle>
							<AlertDescription class="text-xs text-amber-500/90">
								The server is currently running. You may need to restart the server for the new files to take effect.
							</AlertDescription>
						</Alert>
					{/if}

					<div class="flex items-center gap-3 pt-4">
						<Button variant="outline" onclick={closeDialog}>
							Done
						</Button>
						<Button onclick={goToTargetServer} class="gap-2">
							View Server
							<ArrowRight class="h-4 w-4" />
						</Button>
					</div>
				</div>
			{:else}
				<!-- Configuration & Install Form -->
				{#if servers.length === 0}
					<div class="flex flex-col items-center justify-center py-10 text-center">
						<ServerIcon class="h-12 w-12 text-muted-foreground/50 mb-3" />
						<h4 class="font-semibold text-lg">No Servers Found</h4>
						<p class="text-sm text-muted-foreground max-w-sm mt-1 mb-4">
							You need at least one Minecraft server configured in DiscoPanel to install this {project?.project_type === 'resourcepack' ? 'resource pack' : 'mod'}.
						</p>
						<Button onclick={() => { closeDialog(); goto(resolve('/servers/new')); }}>
							Create a Server
						</Button>
					</div>
				{:else}
					<!-- Server Selection -->
					<div class="space-y-2">
						<Label class="text-sm font-semibold flex items-center gap-2">
							<ServerIcon class="h-4 w-4 text-primary" />
							Target Server
						</Label>
						<Select
							type="single"
							value={selectedServerId}
							onValueChange={(v: string | undefined) => { if (v) selectedServerId = v; }}
							disabled={installStep !== 'idle'}
						>
							<SelectTrigger class="w-full h-12">
								{#if selectedServer}
									<div class="flex items-center gap-2 truncate">
										<div
											class="h-2.5 w-2.5 rounded-full shrink-0 {selectedServer.status === ServerStatus.RUNNING
												? 'bg-green-500'
												: selectedServer.status === ServerStatus.ERROR
													? 'bg-red-500'
													: 'bg-zinc-400'}"
										></div>
										<span class="font-medium truncate">{selectedServer.name}</span>
										<Badge variant="outline" class="text-xs ml-auto shrink-0">
											{selectedServer.mcVersion || 'Vanilla'} &bull; {selectedServer.modLoader || 'Vanilla'}
										</Badge>
									</div>
								{:else}
									<span class="text-muted-foreground">Select a server...</span>
								{/if}
							</SelectTrigger>
							<SelectContent>
								{#each servers as srv (srv.id)}
									<SelectItem value={srv.id}>
										<div class="flex items-center gap-2">
											<div
												class="h-2 w-2 rounded-full shrink-0 {srv.status === ServerStatus.RUNNING
													? 'bg-green-500'
													: srv.status === ServerStatus.ERROR
														? 'bg-red-500'
														: 'bg-zinc-400'}"
											></div>
											<span class="font-medium">{srv.name}</span>
											<span class="text-xs text-muted-foreground ml-2">
												({srv.mcVersion || 'Latest'} - {srv.modLoader || 'Vanilla'})
											</span>
										</div>
									</SelectItem>
								{/each}
							</SelectContent>
						</Select>
					</div>

					<!-- Server Compatibility Info -->
					{#if project?.project_type === 'mod' && project?.server_side === 'unsupported'}
						<Alert class="border-red-500/30 bg-red-500/10 text-red-500 py-3">
							<AlertCircle class="h-4 w-4 text-red-500" />
							<AlertTitle class="text-sm font-semibold">Client-Only Mod Warning</AlertTitle>
							<AlertDescription class="text-xs text-red-500/90 mt-0.5">
								This mod is marked as Client-Only on Modrinth (unsupported on dedicated servers). It may cause the server to crash if installed.
							</AlertDescription>
						</Alert>
					{/if}

					{#if selectedServer}
						{#if isServerVanilla(selectedServer) && project?.project_type === 'mod'}
							<Alert class="border-amber-500/30 bg-amber-500/10 text-amber-500 py-3">
								<AlertCircle class="h-4 w-4 text-amber-500" />
								<AlertTitle class="text-sm font-semibold">Vanilla Server Warning</AlertTitle>
								<AlertDescription class="text-xs text-amber-500/90 mt-0.5">
									"{selectedServer.name}" is a Vanilla server. Mod JAR files typically require a modloader (Fabric, Forge, NeoForge, etc.) to run.
								</AlertDescription>
							</Alert>
						{:else if selectedVersion && !isVersionCompatible(selectedVersion, selectedServer)}
							<Alert class="border-amber-500/30 bg-amber-500/10 text-amber-500 py-3">
								<AlertCircle class="h-4 w-4 text-amber-500" />
								<AlertTitle class="text-sm font-semibold">Compatibility Notice</AlertTitle>
								<AlertDescription class="text-xs text-amber-500/90 mt-0.5">
									Selected version does not explicitly declare support for {selectedServer.modLoader} on MC {selectedServer.mcVersion}. It may still work, but proceed with caution.
								</AlertDescription>
							</Alert>
						{/if}
					{/if}

					<!-- Version Selection -->
					<div class="space-y-2">
						<div class="flex items-center justify-between">
							<Label class="text-sm font-semibold flex items-center gap-2">
								<Layers class="h-4 w-4 text-primary" />
								Version / File
							</Label>
							<div class="flex items-center gap-2">
								<span class="text-xs text-muted-foreground">Show all versions</span>
								<Switch
									checked={showAllVersions}
									onCheckedChange={(c) => (showAllVersions = c)}
									disabled={installStep !== 'idle' || loadingVersions}
								/>
							</div>
						</div>

						{#if loadingVersions}
							<div class="flex h-12 w-full items-center justify-center rounded-lg border bg-muted/20">
								<Loader2 class="h-5 w-5 animate-spin text-muted-foreground" />
								<span class="text-xs text-muted-foreground ml-2">Loading Modrinth versions...</span>
							</div>
						{:else if displayVersions.length === 0}
							<div class="rounded-lg border border-dashed p-4 text-center">
								<p class="text-xs text-muted-foreground">
									No compatible versions found for {selectedServer?.mcVersion || 'this server'}.
								</p>
								<Button
									variant="link"
									size="sm"
									class="text-xs text-primary mt-1"
									onclick={() => (showAllVersions = true)}
								>
									Show all {versions.length} versions
								</Button>
							</div>
						{:else}
							<!-- Custom Smooth Version Selector -->
							<div class="relative">
								<!-- Selected Version Trigger Card -->
								<button
									type="button"
									onclick={() => { if (installStep === 'idle') isVersionPickerOpen = !isVersionPickerOpen; }}
									disabled={installStep !== 'idle'}
									class="flex w-full items-center justify-between gap-3 rounded-xl border-2 bg-card p-3 text-left transition-all hover:border-primary/40 hover:bg-muted/40 focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:pointer-events-none disabled:opacity-50 shadow-sm"
								>
									{#if selectedVersion}
										<div class="flex items-center gap-3 min-w-0 flex-1">
											<Badge
												variant={selectedVersion.version_type === 'release' ? 'default' : 'secondary'}
												class="text-[10px] uppercase font-bold shrink-0 {selectedVersion.version_type === 'release' ? 'bg-emerald-600 hover:bg-emerald-600 text-white' : selectedVersion.version_type === 'beta' ? 'bg-amber-600 hover:bg-amber-600 text-white' : 'bg-rose-600 hover:bg-rose-600 text-white'}"
											>
												{selectedVersion.version_type}
											</Badge>
											<div class="min-w-0 flex-1">
												<div class="flex items-center gap-2">
													<span class="font-semibold text-foreground truncate text-sm">
														{selectedVersion.name || selectedVersion.version_number}
													</span>
												</div>
												<div class="flex items-center gap-2 text-xs text-muted-foreground truncate mt-0.5">
													<span>MC: {selectedVersion.game_versions.slice(0, 3).join(', ')}{selectedVersion.game_versions.length > 3 ? ` (+${selectedVersion.game_versions.length - 3})` : ''}</span>
													{#if selectedVersion.loaders.length > 0}
														<span>&bull;</span>
														<span class="capitalize">{selectedVersion.loaders.join(', ')}</span>
													{/if}
													{#if selectedVersion.files[0]}
														<span>&bull;</span>
														<span>{formatBytes(selectedVersion.files[0].size)}</span>
													{/if}
												</div>
											</div>
										</div>
									{:else}
										<span class="text-muted-foreground text-sm">Select version to install...</span>
									{/if}

									<div class="flex items-center gap-2 shrink-0 text-muted-foreground">
										<Badge variant="outline" class="text-xs font-normal">
											{displayVersions.length} {displayVersions.length === 1 ? 'version' : 'versions'}
										</Badge>
										{#if isVersionPickerOpen}
											<ChevronUp class="h-4 w-4 text-foreground" />
										{:else}
											<ChevronDown class="h-4 w-4 text-foreground" />
										{/if}
									</div>
								</button>

								<!-- Smooth Scrollable Version List -->
								{#if isVersionPickerOpen}
									<div
										class="mt-2 rounded-xl border-2 bg-card/98 backdrop-blur-md shadow-2xl overflow-hidden animate-in fade-in-0 zoom-in-95 duration-150 transition-all"
									>
										<!-- Quick search filter -->
										<div class="p-2.5 border-b bg-muted/30">
											<div class="relative">
												<Search class="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
												<Input
													placeholder="Search versions (e.g. 1.20, beta, 0.9)..."
													bind:value={versionSearchQuery}
													class="h-8.5 pl-8 text-xs bg-background rounded-lg"
												/>
											</div>
										</div>

										<!-- Buttery-smooth scrollable list -->
										<div
											class="max-h-60 overflow-y-auto overscroll-contain p-1.5 space-y-1 focus:outline-none [scrollbar-width:thin] [scrollbar-color:rgba(155,155,155,0.4)_transparent]"
										>
											{#if filteredDisplayVersions.length === 0}
												<div class="py-6 text-center text-xs text-muted-foreground">
													No versions match "{versionSearchQuery}"
												</div>
											{:else}
												{#each filteredDisplayVersions as ver (ver.id)}
													<button
														type="button"
														onclick={() => {
															selectedVersionId = ver.id;
															isVersionPickerOpen = false;
														}}
														class="w-full flex items-center justify-between gap-3 p-2.5 rounded-lg text-left transition-all hover:bg-muted/70 {ver.id === selectedVersionId ? 'bg-primary/10 border border-primary/40 text-foreground' : 'text-muted-foreground'}"
													>
														<div class="flex items-center gap-2.5 min-w-0 flex-1">
															<Badge
																variant={ver.version_type === 'release' ? 'default' : 'secondary'}
																class="text-[9px] uppercase font-bold shrink-0 {ver.version_type === 'release' ? 'bg-emerald-600 hover:bg-emerald-600 text-white' : ver.version_type === 'beta' ? 'bg-amber-600 hover:bg-amber-600 text-white' : 'bg-rose-600 hover:bg-rose-600 text-white'}"
															>
																{ver.version_type}
															</Badge>
															<div class="min-w-0 flex-1">
																<div class="flex items-center gap-2">
																	<span class="font-medium text-foreground truncate text-xs">
																		{ver.name || ver.version_number}
																	</span>
																</div>
																<div class="flex items-center gap-2 text-[10px] text-muted-foreground truncate mt-0.5">
																	<span>MC: {ver.game_versions.slice(0, 3).join(', ')}{ver.game_versions.length > 3 ? ` (+${ver.game_versions.length - 3})` : ''}</span>
																	{#if ver.loaders.length > 0}
																		<span>&bull;</span>
																		<span class="capitalize">{ver.loaders.join(', ')}</span>
																	{/if}
																</div>
															</div>
														</div>

														<div class="flex items-center gap-2 shrink-0">
															{#if ver.files[0]}
																<span class="text-[11px] text-muted-foreground font-mono">
																	{formatBytes(ver.files[0].size)}
																</span>
															{/if}
															{#if ver.id === selectedVersionId}
																<Check class="h-4 w-4 text-primary shrink-0" />
															{/if}
														</div>
													</button>
												{/each}
											{/if}
										</div>
									</div>
								{/if}
							</div>
						{/if}
					</div>

					<!-- Dependencies Section -->
					{#if loadingDependencies}
						<div class="rounded-xl border bg-muted/20 p-3.5 flex items-center gap-2.5 text-xs text-muted-foreground">
							<Loader2 class="h-4 w-4 animate-spin text-primary" />
							<span>Resolving mod dependencies for this version...</span>
						</div>
					{:else if dependencies.length > 0}
						<div class="space-y-2.5 rounded-xl border-2 bg-muted/15 p-4">
							<div class="flex items-center justify-between">
								<Label class="text-sm font-semibold flex items-center gap-2">
									<Puzzle class="h-4 w-4 text-primary" />
									Dependencies ({dependencies.length})
								</Label>
								<Badge variant="secondary" class="text-[11px] font-medium">
									{dependencies.filter((d) => d.selected).length} selected for auto-install
								</Badge>
							</div>

							<p class="text-xs text-muted-foreground">
								This mod requires or recommends the following dependencies. They will be automatically downloaded and installed:
							</p>

							<div class="space-y-2 max-h-52 overflow-y-auto overscroll-contain p-1 focus:outline-none [scrollbar-width:thin]">
								{#each dependencies as dep (dep.projectId || dep.projectTitle)}
									<button
										type="button"
										onclick={() => {
											if (installStep === 'idle') {
												dep.selected = !dep.selected;
											}
										}}
										disabled={installStep !== 'idle'}
										class="w-full flex items-center justify-between gap-3 p-3 rounded-xl border-2 text-left cursor-pointer select-none transition-all duration-150 ease-out hover:border-primary/80 hover:bg-primary/10 hover:shadow-md hover:shadow-primary/5 disabled:pointer-events-none disabled:opacity-50 {dep.selected ? 'border-primary/70 bg-primary/10 shadow-xs ring-1 ring-primary/40' : 'border-border/60 bg-card/60 opacity-80 hover:opacity-100 hover:border-primary/60'}"
									>
										<div class="flex items-center gap-3 min-w-0 flex-1">
											{#if dep.projectIcon}
												<img src={dep.projectIcon} alt={dep.projectTitle} class="h-8 w-8 rounded-lg object-cover shrink-0 shadow-xs" />
											{:else}
												<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
													<Puzzle class="h-4 w-4" />
												</div>
											{/if}

											<div class="min-w-0 flex-1">
												<div class="flex items-center gap-2">
													<span class="font-semibold text-foreground text-xs sm:text-sm truncate">{dep.projectTitle}</span>
													<Badge
														variant={dep.dependencyType === 'required' ? 'destructive' : 'secondary'}
														class="text-[9px] uppercase font-bold shrink-0 {dep.dependencyType === 'required' ? 'bg-amber-500/20 text-amber-500 border border-amber-500/30' : ''}"
													>
														{dep.dependencyType}
													</Badge>
												</div>
												<div class="flex items-center gap-2 text-[11px] text-muted-foreground truncate mt-0.5">
													{#if dep.filename}
														<span class="font-mono truncate">{dep.filename}</span>
														{#if dep.size > 0}
															<span>&bull;</span>
															<span>{formatBytes(dep.size)}</span>
														{/if}
													{:else}
														<span>Auto-matched for server</span>
													{/if}
												</div>
											</div>
										</div>

										<div class="flex items-center gap-2 shrink-0 pointer-events-none">
											<Checkbox
												checked={dep.selected}
												tabindex={-1}
											/>
										</div>
									</button>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Install Destination Info -->
					{#if selectedServer && selectedVersion}
						<div class="rounded-xl border bg-muted/20 p-4 space-y-2 text-xs">
							<div class="flex items-center justify-between text-muted-foreground">
								<span>Target Directory:</span>
								<span class="font-mono text-foreground font-semibold">/{getTargetDirectory(selectedServer)}/</span>
							</div>
							{#if selectedVersion.files[0]}
								<div class="flex items-center justify-between text-muted-foreground">
									<span>File Name:</span>
									<span class="font-mono text-foreground font-medium truncate max-w-xs">{selectedVersion.files[0].filename}</span>
								</div>
								<div class="flex items-center justify-between text-muted-foreground">
									<span>File Size:</span>
									<span class="text-foreground">{formatBytes(selectedVersion.files[0].size)}</span>
								</div>
							{/if}
							{#if selectedVersion.loaders.length > 0}
								<div class="flex items-center justify-between text-muted-foreground">
									<span>Supported Loaders:</span>
									<span class="text-foreground capitalize">{selectedVersion.loaders.join(', ')}</span>
								</div>
							{/if}
						</div>
					{/if}

					<!-- Progress Bar (During Install) -->
					{#if installStep !== 'idle'}
						<div class="space-y-2 rounded-xl border bg-muted/30 p-4">
							<div class="flex items-center justify-between text-xs font-medium">
								<span class="flex items-center gap-2 text-foreground">
									<Loader2 class="h-3.5 w-3.5 animate-spin text-primary" />
									{installStatusText}
								</span>
								<span class="text-muted-foreground">{installProgress}%</span>
							</div>
							<Progress value={installProgress} class="h-2" />
						</div>
					{/if}

					{#if errorMessage}
						<Alert variant="destructive" class="py-2.5">
							<AlertCircle class="h-4 w-4" />
							<AlertDescription class="text-xs">{errorMessage}</AlertDescription>
						</Alert>
					{/if}
				{/if}
			{/if}
		</div>

		<!-- Footer -->
		{#if installStep !== 'completed'}
			<div class="flex items-center justify-between border-t bg-muted/30 p-4 px-6">
				{#if project?.slug}
					<a
						href={`https://modrinth.com/${project.project_type || 'mod'}/${project.slug}`}
						target="_blank"
						rel="noopener noreferrer"
						class="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors"
					>
						<ExternalLink class="h-3 w-3" />
						View on Modrinth
					</a>
				{:else}
					<div></div>
				{/if}

				<div class="flex items-center gap-2">
					<Button
						variant="outline"
						onclick={closeDialog}
						disabled={installStep !== 'idle'}
					>
						Cancel
					</Button>
					<Button
						onclick={handleInstall}
						disabled={!selectedServer || !selectedVersion || installStep !== 'idle' || loadingVersions}
						class="bg-linear-to-r from-primary to-primary/85 shadow-md hover:shadow-lg font-semibold min-w-32"
					>
						{#if installStep !== 'idle'}
							<Loader2 class="mr-2 h-4 w-4 animate-spin" />
							Installing...
						{:else}
							<Download class="mr-2 h-4 w-4" />
							Install {dependencies.filter((d) => d.selected).length > 0 ? `(${1 + dependencies.filter((d) => d.selected).length} items)` : 'to Server'}
						{/if}
					</Button>
				</div>
			</div>
		{/if}
	</DialogContent>
</Dialog>
