<script lang="ts">
	import { onMount } from 'svelte';
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
	import { toast } from 'svelte-sonner';
	import {
		Heart,
		Download,
		Search,
		ExternalLink,
		ArrowLeft,
		Blocks,
		Loader2,
		Sparkles,
		Layers,
		Package
	} from '@lucide/svelte';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import {
		searchModrinth,
		type ModrinthSearchHit,
		type ModrinthSearchResponse,
		type SearchOptions
	} from '$lib/api/modrinth';
	import UseInServerDialog from '$lib/components/modrinth/use-in-server-dialog.svelte';

	const FAVORITES_STORAGE_KEY = 'discopanel_modrinth_favorites';

	let searchParams = $state<{
		query: string;
		projectType: 'all' | 'mod' | 'resourcepack' | 'datapack' | 'shader';
		environment: 'all' | 'server' | 'client' | 'server_only' | 'client_only';
		gameVersion: string;
		modLoader: string;
		sortBy: 'downloads' | 'relevance' | 'updated' | 'newest' | 'follows';
		page: number;
		pageSize: number;
	}>({
		query: '',
		projectType: 'all',
		environment: 'all',
		gameVersion: '',
		modLoader: '',
		sortBy: 'downloads',
		page: 1,
		pageSize: 20
	});

	let searchResults = $state<ModrinthSearchResponse | null>(null);
	let favorites = $state<ModrinthSearchHit[]>([]);
	let loading = $state(false);
	let showFavorites = $state(false);

	const DEFAULT_VERSIONS = [
		'1.21.4',
		'1.21.3',
		'1.21.2',
		'1.21.1',
		'1.21',
		'1.20.6',
		'1.20.5',
		'1.20.4',
		'1.20.3',
		'1.20.2',
		'1.20.1',
		'1.20',
		'1.19.4',
		'1.19.3',
		'1.19.2',
		'1.19.1',
		'1.19',
		'1.18.2',
		'1.18.1',
		'1.18',
		'1.17.1',
		'1.16.5',
		'1.15.2',
		'1.14.4',
		'1.12.2',
		'1.8.9',
		'1.7.10'
	];

	const DEFAULT_LOADERS: Array<{ value: string; label: string }> = [
		{ value: '', label: 'All Loaders' },
		{ value: 'fabric', label: 'Fabric' },
		{ value: 'forge', label: 'Forge' },
		{ value: 'neoforge', label: 'NeoForge' },
		{ value: 'quilt', label: 'Quilt' },
		{ value: 'paper', label: 'Paper' },
		{ value: 'purpur', label: 'Purpur' },
		{ value: 'spigot', label: 'Spigot' },
		{ value: 'bukkit', label: 'Bukkit' }
	];

	// Game versions and mod loaders
	let gameVersions = $state<string[]>(DEFAULT_VERSIONS);
	let modLoaders = $state<Array<{ value: string; label: string }>>(DEFAULT_LOADERS);

	// Dialog state
	let selectedProjectForServer = $state<ModrinthSearchHit | null>(null);
	let isDialogOpen = $state(false);

	onMount(async () => {
		loadFavoritesFromStorage();
		loadMinecraftVersions();
		loadModLoaders();
		performSearch();
	});

	function loadFavoritesFromStorage() {
		try {
			const stored = localStorage.getItem(FAVORITES_STORAGE_KEY);
			if (stored) {
				favorites = JSON.parse(stored);
			}
		} catch (e) {
			console.error('Failed to load favorites from localStorage', e);
		}
	}

	function saveFavoritesToStorage() {
		try {
			localStorage.setItem(FAVORITES_STORAGE_KEY, JSON.stringify(favorites));
		} catch (e) {
			console.error('Failed to save favorites to localStorage', e);
		}
	}

	async function loadMinecraftVersions() {
		try {
			const response = await rpcClient.minecraft.getMinecraftVersions({}, { ...silentCallOptions });
			if (response?.versions?.length > 0) {
				gameVersions = response.versions.map((v) => v.id);
			}
		} catch (error) {
			console.debug('Failed to load dynamic Minecraft versions, using defaults:', error);
		}
	}

	async function loadModLoaders() {
		try {
			const response = await rpcClient.minecraft.getModLoaders({}, { ...silentCallOptions });
			const loaders = response?.modloaders || [];
			if (loaders.length > 0) {
				modLoaders = [
					{ value: '', label: 'All Loaders' },
					...loaders.map((loader) => ({
						value: loader.name.toLowerCase(),
						label: loader.displayName || loader.name
					}))
				];
			}
		} catch (error) {
			console.debug('Failed to load dynamic mod loaders, using defaults:', error);
		}
	}

	async function performSearch(resetPage = true) {
		loading = true;
		if (resetPage) {
			searchParams.page = 1;
		}

		try {
			const offset = (searchParams.page - 1) * searchParams.pageSize;
			const res = await searchModrinth({
				query: searchParams.query,
				projectType: searchParams.projectType,
				environment: searchParams.environment,
				gameVersion: searchParams.gameVersion,
				modLoader: searchParams.modLoader,
				sortBy: searchParams.sortBy,
				offset,
				limit: searchParams.pageSize
			});
			searchResults = res;
		} catch (error) {
			console.error('Search failed:', error);
			toast.error(error instanceof Error ? error.message : 'Failed to search Modrinth');
		} finally {
			loading = false;
		}
	}

	function isFavorited(projectId: string): boolean {
		return favorites.some((f) => f.project_id === projectId);
	}

	function toggleFavorite(project: ModrinthSearchHit) {
		if (isFavorited(project.project_id)) {
			favorites = favorites.filter((f) => f.project_id !== project.project_id);
			toast.success(`Removed "${project.title}" from favorites`);
		} else {
			favorites = [project, ...favorites];
			toast.success(`Added "${project.title}" to favorites`);
		}
		saveFavoritesToStorage();
	}

	function formatNumber(num: number): string {
		if (num >= 1000000) {
			return `${(num / 1000000).toFixed(1)}M`;
		} else if (num >= 1000) {
			return `${(num / 1000).toFixed(1)}K`;
		}
		return num.toString();
	}

	function openUseInServer(project: ModrinthSearchHit) {
		selectedProjectForServer = project;
		isDialogOpen = true;
	}

	let displayProjects = $derived(
		showFavorites ? favorites : (searchResults?.hits || [])
	);

	let totalPages = $derived(
		searchResults ? Math.ceil(searchResults.total_hits / searchParams.pageSize) : 1
	);
</script>

<svelte:head>
	<title>Mods & Resources - DiscoPanel</title>
</svelte:head>

<div class="h-full flex-1 space-y-8 bg-linear-to-br from-background to-muted/10 p-8 pt-6">
	<!-- Page Header -->
	<div class="flex items-center justify-between border-b-2 border-border/50 pb-6">
		<div class="flex items-center gap-4">
			<div
				class="flex h-16 w-16 items-center justify-center rounded-2xl bg-linear-to-br from-primary/20 to-primary/10 shadow-lg text-primary"
			>
				<Blocks class="h-8 w-8" />
			</div>
			<div class="space-y-1">
				<h2
					class="bg-linear-to-r from-foreground to-foreground/70 bg-clip-text text-4xl font-bold tracking-tight text-transparent"
				>
					Mods & Resources
				</h2>
				<p class="text-base text-muted-foreground">
					Browse and install mods and resource packs for your servers via Modrinth
				</p>
			</div>
		</div>

		<div class="flex items-center gap-2">
			<Button
				variant={showFavorites ? 'outline' : 'default'}
				onclick={() => {
					showFavorites = !showFavorites;
				}}
				class="shadow-md transition-all hover:scale-[1.02] hover:shadow-lg"
			>
				{#if showFavorites}
					<ArrowLeft class="mr-2 h-5 w-5" />
					Back to all
				{:else}
					<Heart class="mr-2 h-5 w-5" />
					Favorites ({favorites.length})
				{/if}
			</Button>
		</div>
	</div>

	<!-- Search & Filter Controls -->
	{#if !showFavorites}
		<div class="flex flex-col gap-4">
			<div class="flex flex-wrap gap-2">
				<!-- Search text input -->
				<Input
					placeholder="Search mods & resource packs..."
					bind:value={searchParams.query}
					onkeydown={(e) => e.key === 'Enter' && performSearch(true)}
					class="min-w-64 flex-1 h-10"
				/>

				<!-- Type selector -->
				<Select
					type="single"
					value={searchParams.projectType}
					onValueChange={(v: string | undefined) => {
						if (v) {
							searchParams.projectType = v as typeof searchParams.projectType;
							performSearch(true);
						}
					}}
					disabled={loading}
				>
					<SelectTrigger class="w-40 h-10">
						<span>
							{searchParams.projectType === 'all'
								? 'All Types'
								: searchParams.projectType === 'mod'
									? 'Mods'
									: searchParams.projectType === 'resourcepack'
										? 'Resource Packs'
										: searchParams.projectType === 'datapack'
											? 'Data Packs'
											: 'Shaders'}
						</span>
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="all">All Types</SelectItem>
						<SelectItem value="mod">Mods</SelectItem>
						<SelectItem value="resourcepack">Resource Packs</SelectItem>
						<SelectItem value="datapack">Data Packs</SelectItem>
						<SelectItem value="shader">Shaders</SelectItem>
					</SelectContent>
				</Select>

				<!-- Environment / Side selector (Server / Client) -->
				<Select
					type="single"
					value={searchParams.environment}
					onValueChange={(v: string | undefined) => {
						if (v) {
							searchParams.environment = v as typeof searchParams.environment;
							performSearch(true);
						}
					}}
					disabled={loading}
				>
					<SelectTrigger class="w-44 h-10">
						<span>
							{searchParams.environment === 'all'
								? 'All Environments'
								: searchParams.environment === 'server'
									? 'Server Mods'
									: searchParams.environment === 'client'
										? 'Client Mods'
										: searchParams.environment === 'server_only'
											? 'Server Only'
											: 'Client Only'}
						</span>
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="all">All Environments</SelectItem>
						<SelectItem value="server">Server Mods</SelectItem>
						<SelectItem value="client">Client Mods</SelectItem>
						<SelectItem value="server_only">Server Only</SelectItem>
						<SelectItem value="client_only">Client Only</SelectItem>
					</SelectContent>
				</Select>

				<!-- Game Version selector -->
				<Select
					type="single"
					value={searchParams.gameVersion}
					onValueChange={(v: string | undefined) => {
						searchParams.gameVersion = v || '';
						performSearch(true);
					}}
					disabled={loading}
				>
					<SelectTrigger class="w-36 h-10">
						<span>{searchParams.gameVersion || 'All Versions'}</span>
					</SelectTrigger>
					<SelectContent class="max-h-64">
						<SelectItem value="">All Versions</SelectItem>
						{#each gameVersions as version (version)}
							<SelectItem value={version}>{version}</SelectItem>
						{/each}
					</SelectContent>
				</Select>

				<!-- Mod Loader selector -->
				<Select
					type="single"
					value={searchParams.modLoader}
					onValueChange={(v: string | undefined) => {
						searchParams.modLoader = v || '';
						performSearch(true);
					}}
					disabled={loading}
				>
					<SelectTrigger class="w-36 h-10">
						<span>
							{searchParams.modLoader
								? modLoaders.find((l) => l.value === searchParams.modLoader)?.label ||
									searchParams.modLoader
								: 'All Loaders'}
						</span>
					</SelectTrigger>
					<SelectContent class="max-h-64">
						{#each modLoaders as loader (loader.value)}
							<SelectItem value={loader.value}>{loader.label}</SelectItem>
						{/each}
					</SelectContent>
				</Select>

				<!-- Sort By selector -->
				<Select
					type="single"
					value={searchParams.sortBy}
					onValueChange={(v: string | undefined) => {
						if (v) {
							searchParams.sortBy = v as typeof searchParams.sortBy;
							performSearch(true);
						}
					}}
					disabled={loading}
				>
					<SelectTrigger class="w-40 h-10">
						<span>
							{searchParams.sortBy === 'downloads'
								? 'Most Downloads'
								: searchParams.sortBy === 'relevance'
									? 'Relevance'
									: searchParams.sortBy === 'updated'
										? 'Recently Updated'
										: searchParams.sortBy === 'newest'
											? 'Newest'
											: 'Most Followers'}
						</span>
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="downloads">Most Downloads</SelectItem>
						<SelectItem value="relevance">Relevance</SelectItem>
						<SelectItem value="updated">Recently Updated</SelectItem>
						<SelectItem value="newest">Newest</SelectItem>
						<SelectItem value="follows">Most Followers</SelectItem>
					</SelectContent>
				</Select>

				<!-- Search action button -->
				<Button
					onclick={() => performSearch(true)}
					disabled={loading}
					class="bg-linear-to-r from-primary to-primary/80 shadow-md transition-all hover:scale-[1.02] hover:from-primary/90 hover:to-primary/70 hover:shadow-lg h-10"
				>
					{#if loading}
						<Loader2 class="mr-2 h-4 w-4 animate-spin" />
						Searching...
					{:else}
						<Search class="mr-2 h-4 w-4" />
						Search
					{/if}
				</Button>
			</div>
		</div>
	{/if}

	<!-- Results Cards Grid -->
	{#if loading}
		<div class="flex flex-col items-center justify-center py-24 text-muted-foreground space-y-4">
			<Loader2 class="h-10 w-10 animate-spin text-primary" />
			<p class="text-sm font-medium">Fetching content from Modrinth...</p>
		</div>
	{:else}
		<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
			{#each displayProjects as project (project.project_id)}
				<Card
					class="group relative flex flex-col justify-between overflow-hidden border-2 bg-linear-to-br from-card to-card/80 transition-all duration-300 hover:border-primary/50 hover:shadow-2xl"
				>
					<div
						class="absolute inset-0 bg-linear-to-br from-primary/10 via-transparent to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-100 pointer-events-none"
					></div>

					<div>
						<CardHeader class="relative pb-3">
							<div class="flex items-start gap-4">
								{#if project.icon_url}
									<img
										src={project.icon_url}
										alt={project.title}
										class="h-16 w-16 rounded-xl object-cover shadow-sm shrink-0"
									/>
								{:else}
									<div
										class="flex h-16 w-16 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-sm shrink-0"
									>
										{#if project.project_type === 'resourcepack'}
											<Layers class="h-8 w-8" />
										{:else}
											<Blocks class="h-8 w-8" />
										{/if}
									</div>
								{/if}

								<div class="min-w-0 flex-1">
									<CardTitle class="line-clamp-1 text-lg font-semibold tracking-tight">
										{project.title}
									</CardTitle>
									<p class="text-xs text-muted-foreground mt-0.5">
										by <span class="font-medium text-foreground">{project.author}</span>
									</p>
									<div class="mt-1.5 flex items-center gap-1.5 flex-wrap">
										<Badge variant="secondary" class="text-[10px] font-semibold uppercase">
											{project.project_type === 'resourcepack' ? 'Resource Pack' : project.project_type}
										</Badge>
										{#if project.project_type === 'mod'}
											{#if project.server_side === 'unsupported'}
												<Badge variant="outline" class="text-[10px] text-amber-500 border-amber-500/30">
													Client Only
												</Badge>
											{:else if project.client_side === 'unsupported'}
												<Badge variant="outline" class="text-[10px] text-blue-500 border-blue-500/30">
													Server Only
												</Badge>
											{:else if project.server_side && project.client_side}
												<Badge variant="outline" class="text-[10px] text-emerald-500 border-emerald-500/30">
													Server & Client
												</Badge>
											{/if}
										{/if}
										<Badge variant="outline" class="text-[10px]">
											modrinth
										</Badge>
										<span class="text-xs text-muted-foreground flex items-center gap-1 ml-auto">
											<Download class="h-3 w-3" />
											{formatNumber(project.downloads)}
										</span>
									</div>
								</div>

								<Button
									size="icon"
									variant={isFavorited(project.project_id) ? 'default' : 'outline'}
									onclick={() => toggleFavorite(project)}
									class="transition-transform hover:scale-110 shrink-0 h-8 w-8"
									title={isFavorited(project.project_id) ? 'Remove favorite' : 'Add favorite'}
								>
									<Heart class={`h-4 w-4 ${isFavorited(project.project_id) ? 'fill-current' : ''}`} />
								</Button>
							</div>
						</CardHeader>

						<CardContent class="relative pt-0">
							<CardDescription class="mb-4 line-clamp-2 text-sm">
								{project.description}
							</CardDescription>

							<div class="space-y-2">
								<!-- Loader tags -->
								{#if project.categories?.length > 0}
									<div class="flex flex-wrap gap-1">
										{#each project.categories.slice(0, 4) as cat (cat)}
											<Badge variant="outline" class="text-[10px] lowercase">{cat}</Badge>
										{/each}
										{#if project.categories.length > 4}
											<span class="text-[10px] text-muted-foreground">+{project.categories.length - 4}</span>
										{/if}
									</div>
								{/if}

								<!-- Versions preview -->
								{#if project.versions?.length > 0}
									<div class="text-xs text-muted-foreground truncate">
										MC: {project.versions.slice(0, 3).join(', ')}
										{#if project.versions.length > 3}
											+{project.versions.length - 3} more
										{/if}
									</div>
								{/if}
							</div>
						</CardContent>
					</div>

					<!-- Card Action Buttons -->
					<div class="relative flex items-center justify-between border-t bg-muted/20 p-4">
						<a
							href={`https://modrinth.com/${project.project_type || 'mod'}/${project.slug}`}
							target="_blank"
							rel="noopener noreferrer"
						>
							<Button variant="outline" size="sm" class="text-xs h-8">
								<ExternalLink class="mr-1 h-3 w-3" />
								View
							</Button>
						</a>

						<Button
							size="sm"
							onclick={() => openUseInServer(project)}
							class="text-xs font-semibold shadow-sm transition-all hover:scale-[1.02] hover:shadow-md h-8 bg-linear-to-r from-primary to-primary/85"
						>
							<Download class="mr-1.5 h-3.5 w-3.5" />
							Use in Server
						</Button>
					</div>
				</Card>
			{/each}
		</div>

		<!-- Pagination Controls -->
		{#if !showFavorites && searchResults && searchResults.total_hits > searchParams.pageSize}
			<div class="mt-8 flex items-center justify-center gap-3">
				<Button
					variant="outline"
					size="sm"
					disabled={searchParams.page <= 1 || loading}
					onclick={() => {
						searchParams.page = Math.max(1, searchParams.page - 1);
						performSearch(false);
					}}
				>
					Previous
				</Button>
				<span class="text-sm text-muted-foreground">
					Page {searchParams.page} of {totalPages}
				</span>
				<Button
					variant="outline"
					size="sm"
					disabled={searchParams.page >= totalPages || loading}
					onclick={() => {
						searchParams.page = searchParams.page + 1;
						performSearch(false);
					}}
				>
					Next
				</Button>
			</div>
		{/if}

		<!-- Empty States -->
		{#if displayProjects.length === 0}
			<div class="py-20 text-center space-y-3">
				<div class="flex h-12 w-12 items-center justify-center rounded-full bg-muted mx-auto text-muted-foreground">
					{#if showFavorites}
						<Heart class="h-6 w-6" />
					{:else}
						<Blocks class="h-6 w-6" />
					{/if}
				</div>
				<h3 class="font-semibold text-lg">
					{#if showFavorites}
						No favorite items yet
					{:else}
						No mods or resource packs found
					{/if}
				</h3>
				<p class="text-sm text-muted-foreground max-w-md mx-auto">
					{#if showFavorites}
						Browse mods and resource packs and click the heart icon on any card to save favorites.
					{:else if searchParams.query}
						No results found for "{searchParams.query}". Try searching with different keywords or clearing filters.
					{:else}
						Try adjusting your filters to find what you are looking for.
					{/if}
				</p>
			</div>
		{/if}
	{/if}
</div>

<!-- Floating "Use in Server" Dialog -->
<UseInServerDialog bind:open={isDialogOpen} project={selectedProjectForServer} />
