<script lang="ts">
	import {
		CommandDialog,
		CommandEmpty,
		CommandGroup,
		CommandInput,
		CommandItem,
		CommandList
	} from '$lib/components/ui/command';
	import { goto } from '$app/navigation';
	import { resolve as resolvePath } from '$app/paths';
	import {
		Server as ServerIcon,
		Users,
		Package,
		Blocks,
		Puzzle,
		Settings,
		FileText
	} from '@lucide/svelte';
	import { activitySortedServers } from '$lib/stores/servers';
	import { canAccessSettings } from '$lib/stores/auth';
	import { ServerStatus } from '$lib/proto/discopanel/v1/common_pb';

	let open = $state(false);
	let query = $state('');

	export function openSearch() {
		open = true;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key.toLowerCase() === 'k' && (event.ctrlKey || event.metaKey) && !event.altKey) {
			event.preventDefault();
			open = !open;
		}
	}

	function closePalette() {
		open = false;
		query = '';
	}

	interface StaticPage {
		label: string;
		href: '/players' | '/modpacks' | '/mods' | '/modules' | '/settings' | '/docs/api';
		keywords: string;
		icon: typeof Users;
		restricted?: boolean;
	}

	const staticPages: StaticPage[] = [
		{
			label: 'Players',
			href: '/players',
			keywords: 'players online list whitelist ops',
			icon: Users
		},
		{
			label: 'Modpacks',
			href: '/modpacks',
			keywords: 'modpacks browse install curseforge modrinth',
			icon: Package
		},
		{
			label: 'Mods',
			href: '/mods',
			keywords: 'mods browse search install',
			icon: Blocks
		},
		{
			label: 'Modules',
			href: '/modules',
			keywords: 'modules proxy routing enable disable',
			icon: Puzzle,
			restricted: true
		},
		{
			label: 'Settings',
			href: '/settings',
			keywords: 'settings configuration preferences',
			icon: Settings,
			restricted: true
		},
		{
			label: 'API Docs',
			href: '/docs/api',
			keywords: 'docs api documentation endpoints',
			icon: FileText
		}
	];

	function matches(label: string, keywords: string, q: string): boolean {
		if (!q) return true;
		return label.toLowerCase().includes(q) || keywords.toLowerCase().includes(q);
	}

	let matchingServers = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return $activitySortedServers
			.filter((server) => matches(server.name, server.description ?? '', q))
			.slice(0, 8);
	});

	let matchingPages = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return staticPages
			.filter((p) => !p.restricted || $canAccessSettings)
			.filter((p) => matches(p.label, p.keywords, q))
			.slice(0, 8);
	});

	function statusDotClass(status: ServerStatus) {
		if (status === ServerStatus.RUNNING) return 'bg-green-500';
		if (status === ServerStatus.ERROR) return 'bg-red-500';
		if (
			status === ServerStatus.STARTING ||
			status === ServerStatus.STOPPING ||
			status === ServerStatus.UNHEALTHY
		)
			return 'bg-yellow-500';
		return 'bg-gray-400';
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<CommandDialog
	bind:open
	shouldFilter={false}
	title="Search DiscoPanel"
	description="Search servers and navigation"
>
	<CommandInput placeholder="Search servers, pages..." bind:value={query} />
	<CommandList>
		<CommandEmpty>No results found.</CommandEmpty>

		{#if matchingServers.length > 0}
			<CommandGroup heading="Servers">
				{#each matchingServers as server (server.id)}
					<CommandItem
						value={`server-${server.id}`}
						onSelect={() => {
							closePalette();
							goto(resolvePath('/servers/[id]', { id: server.id }));
						}}
					>
						<ServerIcon />
						<div class="flex min-w-0 flex-1 items-center gap-2">
							<span class="h-2 w-2 shrink-0 rounded-full {statusDotClass(server.status)}"></span>
							<span class="truncate">{server.name}</span>
							{#if server.description}
								<span class="hidden truncate text-xs text-muted-foreground sm:inline">
									{server.description}
								</span>
							{/if}
						</div>
					</CommandItem>
				{/each}
			</CommandGroup>
		{/if}

		{#if matchingPages.length > 0}
			<CommandGroup heading="Navigation">
				{#each matchingPages as item (item.href)}
					<CommandItem
						value={`page-${item.href}`}
						onSelect={() => {
							closePalette();
							goto(resolvePath(item.href));
						}}
					>
						<item.icon />
						<span>{item.label}</span>
					</CommandItem>
				{/each}
			</CommandGroup>
		{/if}
	</CommandList>
</CommandDialog>
