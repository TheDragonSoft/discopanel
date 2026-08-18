/**
 * Modrinth v2 REST API Client for DiscoPanel
 */

export const MODRINTH_API_BASE = 'https://api.modrinth.com/v2';

export interface ModrinthSearchHit {
	project_id: string;
	project_type: 'mod' | 'resourcepack' | 'datapack' | 'shader' | 'plugin' | string;
	slug: string;
	author: string;
	title: string;
	description: string;
	categories: string[];
	display_categories?: string[];
	versions: string[];
	downloads: number;
	follows: number;
	icon_url: string | null;
	date_created: string;
	date_modified: string;
	latest_version?: string;
	license: string;
	client_side?: string;
	server_side?: string;
	gallery?: string[];
	featured_gallery?: string;
	color?: number;
}

export interface ModrinthSearchResponse {
	hits: ModrinthSearchHit[];
	offset: number;
	limit: number;
	total_hits: number;
}

export interface ModrinthProjectDetails {
	id: string;
	slug: string;
	title: string;
	description: string;
	body: string;
	project_type: string;
	client_side: string;
	server_side: string;
	game_versions: string[];
	loaders: string[];
	categories: string[];
	additional_categories?: string[];
	status: string;
	published: string;
	updated: string;
	downloads: number;
	followers: number;
	icon_url: string | null;
	gallery?: Array<{
		url: string;
		featured: boolean;
		title?: string;
		description?: string;
	}>;
	source_url?: string;
	issues_url?: string;
	wiki_url?: string;
	discord_url?: string;
	team?: string;
	versions?: string[];
}

export interface ModrinthVersionFile {
	hashes: {
		sha512?: string;
		sha1?: string;
	};
	url: string;
	filename: string;
	primary: boolean;
	size: number;
	file_type?: string | null;
}

export interface ModrinthDependency {
	version_id: string | null;
	project_id: string | null;
	file_name: string | null;
	dependency_type: 'required' | 'optional' | 'incompatible' | 'embedded';
}

export interface ModrinthVersion {
	id: string;
	name: string;
	version_number: string;
	project_id: string;
	game_versions: string[];
	loaders: string[];
	version_type: 'release' | 'beta' | 'alpha';
	featured: boolean;
	status: string;
	files: ModrinthVersionFile[];
	downloads: number;
	date_published: string;
	changelog?: string | null;
	dependencies: ModrinthDependency[];
}

export interface SearchOptions {
	query?: string;
	projectType?: 'all' | 'mod' | 'resourcepack' | 'datapack' | 'shader';
	environment?: 'all' | 'server' | 'client' | 'server_only' | 'client_only';
	gameVersion?: string;
	modLoader?: string;
	sortBy?: 'downloads' | 'relevance' | 'updated' | 'newest' | 'follows';
	offset?: number;
	limit?: number;
}

/**
 * Search Modrinth projects with structured filters
 */
export async function searchModrinth(options: SearchOptions = {}): Promise<ModrinthSearchResponse> {
	const params = new URLSearchParams();

	if (options.query?.trim()) {
		params.set('query', options.query.trim());
	}

	const facets: string[][] = [];

	// Project type facet
	if (options.projectType && options.projectType !== 'all') {
		facets.push([`project_type:${options.projectType}`]);
	} else {
		// Default to mods and resource packs
		facets.push(['project_type:mod', 'project_type:resourcepack']);
	}

	// Environment / Side facet
	if (options.environment === 'server') {
		facets.push(['server_side:required', 'server_side:optional']);
	} else if (options.environment === 'client') {
		facets.push(['client_side:required', 'client_side:optional']);
	} else if (options.environment === 'server_only') {
		facets.push(['server_side:required', 'server_side:optional']);
		facets.push(['client_side:unsupported']);
	} else if (options.environment === 'client_only') {
		facets.push(['client_side:required', 'client_side:optional']);
		facets.push(['server_side:unsupported']);
	}

	// Minecraft game version facet
	if (options.gameVersion?.trim()) {
		facets.push([`versions:${options.gameVersion.trim()}`]);
	}

	// Mod loader facet (Modrinth stores loaders in categories)
	if (options.modLoader?.trim()) {
		facets.push([`categories:${options.modLoader.trim().toLowerCase()}`]);
	}

	if (facets.length > 0) {
		params.set('facets', JSON.stringify(facets));
	}

	params.set('index', options.sortBy || 'downloads');
	params.set('offset', String(options.offset || 0));
	params.set('limit', String(options.limit || 20));

	const url = `${MODRINTH_API_BASE}/search?${params.toString()}`;
	const res = await fetch(url, {
		headers: {
			'User-Agent': 'DiscoPanel/1.0 (https://github.com/nickheyer/discopanel)'
		}
	});

	if (!res.ok) {
		throw new Error(`Modrinth search failed (${res.status}): ${res.statusText}`);
	}

	return await res.json();
}

/**
 * Get project details by ID or slug
 */
export async function getModrinthProject(slugOrId: string): Promise<ModrinthProjectDetails> {
	const url = `${MODRINTH_API_BASE}/project/${encodeURIComponent(slugOrId)}`;
	const res = await fetch(url, {
		headers: {
			'User-Agent': 'DiscoPanel/1.0 (https://github.com/nickheyer/discopanel)'
		}
	});

	if (!res.ok) {
		throw new Error(`Failed to fetch Modrinth project (${res.status}): ${res.statusText}`);
	}

	return await res.json();
}

/**
 * Get all versions for a Modrinth project with optional loader/game_version filtering
 */
export async function getModrinthVersions(
	slugOrId: string,
	filters?: { loaders?: string[]; game_versions?: string[] }
): Promise<ModrinthVersion[]> {
	const params = new URLSearchParams();

	if (filters?.loaders && filters.loaders.length > 0) {
		params.set('loaders', JSON.stringify(filters.loaders.map((l) => l.toLowerCase())));
	}

	if (filters?.game_versions && filters.game_versions.length > 0) {
		params.set('game_versions', JSON.stringify(filters.game_versions));
	}

	const qs = params.toString();
	const url = `${MODRINTH_API_BASE}/project/${encodeURIComponent(slugOrId)}/version${qs ? `?${qs}` : ''}`;
	const res = await fetch(url, {
		headers: {
			'User-Agent': 'DiscoPanel/1.0 (https://github.com/nickheyer/discopanel)'
		}
	});

	if (!res.ok) {
		throw new Error(`Failed to fetch Modrinth project versions (${res.status}): ${res.statusText}`);
	}

	return await res.json();
}

/**
 * Streams and downloads a file blob from Modrinth CDN with progress tracking
 */
export async function downloadModrinthFileBlob(
	downloadUrl: string,
	filename: string,
	onProgress?: (percent: number, loaded: number, total: number) => void
): Promise<File> {
	const res = await fetch(downloadUrl);

	if (!res.ok) {
		throw new Error(`Failed to download file from Modrinth (${res.status}): ${res.statusText}`);
	}

	const contentLengthHeader = res.headers.get('content-length');
	const total = contentLengthHeader ? parseInt(contentLengthHeader, 10) : 0;

	if (res.body && total > 0 && onProgress) {
		const reader = res.body.getReader();
		let loaded = 0;
		const chunks: Uint8Array[] = [];

		while (true) {
			const { done, value } = await reader.read();
			if (done) break;
			if (value) {
				chunks.push(value);
				loaded += value.length;
				const percent = Math.min(100, Math.round((loaded / total) * 100));
				onProgress(percent, loaded, total);
			}
		}

		const blob = new Blob(chunks as BlobPart[]);
		return new File([blob], filename, { type: 'application/java-archive' });
	}

	const blob = await res.blob();
	return new File([blob], filename, { type: 'application/java-archive' });
}

/**
 * Get multiple projects by IDs or slugs in a single batch request
 */
export async function getModrinthProjects(ids: string[]): Promise<ModrinthProjectDetails[]> {
	if (!ids || ids.length === 0) return [];
	const url = `${MODRINTH_API_BASE}/projects?ids=${encodeURIComponent(JSON.stringify(ids))}`;
	const res = await fetch(url, {
		headers: {
			'User-Agent': 'DiscoPanel/1.0 (https://github.com/nickheyer/discopanel)'
		}
	});

	if (!res.ok) {
		throw new Error(`Failed to fetch Modrinth projects (${res.status}): ${res.statusText}`);
	}

	return await res.json();
}

/**
 * Get multiple versions by IDs in a single batch request
 */
export async function getModrinthVersionsByIds(ids: string[]): Promise<ModrinthVersion[]> {
	if (!ids || ids.length === 0) return [];
	const url = `${MODRINTH_API_BASE}/versions?ids=${encodeURIComponent(JSON.stringify(ids))}`;
	const res = await fetch(url, {
		headers: {
			'User-Agent': 'DiscoPanel/1.0 (https://github.com/nickheyer/discopanel)'
		}
	});

	if (!res.ok) {
		throw new Error(`Failed to fetch Modrinth versions (${res.status}): ${res.statusText}`);
	}

	return await res.json();
}

export interface ResolvedDependency {
	projectId: string;
	projectTitle: string;
	projectSlug: string;
	projectIcon: string | null;
	dependencyType: 'required' | 'optional' | 'incompatible' | 'embedded';
	versionId: string | null;
	versionName: string | null;
	versionNumber: string | null;
	filename: string | null;
	downloadUrl: string | null;
	size: number;
	selected: boolean;
	status: 'pending' | 'downloading' | 'uploading' | 'completed' | 'error';
	errorMessage?: string;
}

/**
 * Resolves all dependencies for a selected Modrinth version, retrieving project metadata and matching compatible files
 */
export async function resolveDependenciesForVersion(
	version: ModrinthVersion,
	serverMcVersion?: string,
	serverLoader?: string
): Promise<ResolvedDependency[]> {
	if (!version.dependencies || version.dependencies.length === 0) {
		return [];
	}

	// 1. Gather all project IDs and version IDs
	const projectIdsToFetch = new Set<string>();
	const versionIdsToFetch = new Set<string>();

	for (const dep of version.dependencies) {
		if (dep.project_id) projectIdsToFetch.add(dep.project_id);
		if (dep.version_id) versionIdsToFetch.add(dep.version_id);
	}

	// 2. Batch fetch projects and pinned versions
	const [projects, pinnedVersions] = await Promise.all([
		projectIdsToFetch.size > 0 ? getModrinthProjects(Array.from(projectIdsToFetch)).catch(() => []) : [],
		versionIdsToFetch.size > 0 ? getModrinthVersionsByIds(Array.from(versionIdsToFetch)).catch(() => []) : []
	]);

	const projectMap = new Map<string, ModrinthProjectDetails>();
	for (const p of projects) {
		projectMap.set(p.id, p);
		if (p.slug) projectMap.set(p.slug, p);
	}

	const versionMap = new Map<string, ModrinthVersion>();
	for (const v of pinnedVersions) {
		versionMap.set(v.id, v);
		if (v.project_id && !projectMap.has(v.project_id)) {
			projectIdsToFetch.add(v.project_id);
		}
	}

	const results: ResolvedDependency[] = [];

	// 3. For each dependency, find the best file / version
	for (const dep of version.dependencies) {
		if (dep.dependency_type === 'incompatible') {
			continue; // Incompatible dependencies shouldn't be installed
		}

		let proj: ModrinthProjectDetails | undefined;
		if (dep.project_id) {
			proj = projectMap.get(dep.project_id);
		}

		let depVersion: ModrinthVersion | undefined;
		if (dep.version_id) {
			depVersion = versionMap.get(dep.version_id);
		}

		// If no pinned version, but we have a project, fetch compatible versions for that project
		if (!depVersion && dep.project_id) {
			try {
				const projectVersions = await getModrinthVersions(dep.project_id);
				if (projectVersions.length > 0) {
					const normalizedLoader = (serverLoader || '').toLowerCase();
					const normalizedMc = (serverMcVersion || '').trim();

					// 1. Compatible release
					const compatibleRelease = projectVersions.find((pv) => {
						const lMatch = !normalizedLoader || pv.loaders.some((l) => l.toLowerCase() === normalizedLoader);
						const vMatch = !normalizedMc || pv.game_versions.includes(normalizedMc);
						return lMatch && vMatch && pv.version_type === 'release';
					});

					// 2. Any compatible
					const anyCompatible = projectVersions.find((pv) => {
						const lMatch = !normalizedLoader || pv.loaders.some((l) => l.toLowerCase() === normalizedLoader);
						const vMatch = !normalizedMc || pv.game_versions.includes(normalizedMc);
						return lMatch && vMatch;
					});

					// 3. Fallback to newest release or first
					depVersion = compatibleRelease || anyCompatible || projectVersions.find((pv) => pv.version_type === 'release') || projectVersions[0];
				}
			} catch (e) {
				console.debug(`Could not fetch versions for dependency ${dep.project_id}:`, e);
			}
		}

		const primaryFile = depVersion?.files?.find((f) => f.primary) || depVersion?.files?.[0];
		const title = proj?.title || dep.file_name || depVersion?.name || dep.project_id || 'Unknown Dependency';
		const slug = proj?.slug || dep.project_id || '';
		const icon = proj?.icon_url || null;

		results.push({
			projectId: dep.project_id || depVersion?.project_id || '',
			projectTitle: title,
			projectSlug: slug,
			projectIcon: icon,
			dependencyType: dep.dependency_type,
			versionId: depVersion?.id || dep.version_id || null,
			versionName: depVersion?.name || null,
			versionNumber: depVersion?.version_number || null,
			filename: primaryFile?.filename || dep.file_name || null,
			downloadUrl: primaryFile?.url || null,
			size: primaryFile?.size || 0,
			selected: dep.dependency_type === 'required', // Auto-selected if required
			status: 'pending'
		});
	}

	return results;
}
