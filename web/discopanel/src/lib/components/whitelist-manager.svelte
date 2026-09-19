<script lang="ts">
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { toast } from 'svelte-sonner';
	import { debounce } from 'lodash-es';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import {
		Dialog,
		DialogContent,
		DialogDescription,
		DialogFooter,
		DialogHeader,
		DialogTitle
	} from '$lib/components/ui/dialog';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		ShieldCheck,
		Search,
		Plus,
		Trash2,
		Download,
		Upload,
		Ban,
		Gavel,
		Pencil,
		Check,
		X,
		UserX
	} from '@lucide/svelte';
	import {
		ListWhitelistEntriesRequestSchema,
		AddWhitelistEntryRequestSchema,
		RemoveWhitelistEntryRequestSchema,
		ApplyWhitelistRequestSchema,
		PullWhitelistRequestSchema,
		BanPlayerRequestSchema,
		UnbanPlayerRequestSchema
	} from '$lib/proto/discopanel/v1/admin_pb';
	import type { WhitelistEntry, ServerOpResult } from '$lib/proto/discopanel/v1/admin_pb';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { rpcClient } from '$lib/api/rpc-client';
	import { serversStore } from '$lib/stores/servers';
	import { timestampToDate } from '$lib/utils';
	import ServerOpResults from '$lib/components/server-op-results.svelte';

	let servers = $derived<Server[]>($serversStore);

	// Whitelist list state
	let entries = $state<WhitelistEntry[]>([]);
	let loading = $state(true);
	let searchInput = $state('');
	let addName = $state('');
	let addNote = $state('');
	let adding = $state(false);

	// Inline note editing
	let editingId = $state('');
	let editNote = $state('');

	// Apply dialog
	let applyOpen = $state(false);
	let applying = $state(false);
	let applySelected = $state<string[]>([]);
	let applyResults = $state<ServerOpResult[]>([]);

	// Pull dialog
	let pullOpen = $state(false);
	let pulling = $state(false);
	let pullServerId = $state('');

	// Ban/unban form
	let banName = $state('');
	let banReason = $state('');
	let banSelected = $state<string[]>([]);
	let banBusy = $state(false);
	let banResults = $state<ServerOpResult[]>([]);

	async function loadEntries() {
		loading = true;
		try {
			const response = await rpcClient.admin.listWhitelistEntries(
				create(ListWhitelistEntriesRequestSchema, { search: searchInput.trim() })
			);
			entries = response.entries;
		} catch (error) {
			console.error('Failed to load whitelist entries:', error);
		} finally {
			loading = false;
		}
	}

	const searchEntries = debounce(() => {
		loadEntries();
	}, 350);

	async function addEntry() {
		const name = addName.trim();
		if (!name) {
			toast.error('Player name is required');
			return;
		}
		adding = true;
		try {
			await rpcClient.admin.addWhitelistEntry(
				create(AddWhitelistEntryRequestSchema, { name, note: addNote.trim() })
			);
			toast.success(`${name} added to the panel whitelist`);
			addName = '';
			addNote = '';
			await loadEntries();
		} catch (error) {
			console.error('Failed to add whitelist entry:', error);
		} finally {
			adding = false;
		}
	}

	async function removeEntry(entry: WhitelistEntry) {
		try {
			await rpcClient.admin.removeWhitelistEntry(
				create(RemoveWhitelistEntryRequestSchema, { id: entry.id })
			);
			toast.success(`${entry.name} removed from the panel whitelist`);
			await loadEntries();
		} catch (error) {
			console.error('Failed to remove whitelist entry:', error);
		}
	}

	function startEditNote(entry: WhitelistEntry) {
		editingId = entry.id;
		editNote = entry.note;
	}

	async function saveNote(entry: WhitelistEntry) {
		const note = editNote.trim();
		editingId = '';
		try {
			// Note updates go through Add, which upserts the entry by name
			await rpcClient.admin.addWhitelistEntry(
				create(AddWhitelistEntryRequestSchema, { name: entry.name, note })
			);
			toast.success(`Note updated for ${entry.name}`);
			await loadEntries();
		} catch (error) {
			console.error('Failed to update note:', error);
		}
	}

	function toggleApplyServer(id: string, checked: boolean) {
		applySelected = checked ? [...applySelected, id] : applySelected.filter((s) => s !== id);
	}

	function toggleBanServer(id: string, checked: boolean) {
		banSelected = checked ? [...banSelected, id] : banSelected.filter((s) => s !== id);
	}

	async function applyWhitelist() {
		applying = true;
		applyResults = [];
		try {
			const response = await rpcClient.admin.applyWhitelist(
				create(ApplyWhitelistRequestSchema, { serverIds: applySelected })
			);
			applyResults = response.results;
			const failed = response.results.filter((r) => !r.success).length;
			if (failed > 0) {
				toast.warning(`Whitelist applied with ${failed} server ${failed === 1 ? 'failure' : 'failures'}`);
			} else {
				toast.success('Whitelist applied successfully');
			}
			await loadEntries();
		} catch (error) {
			console.error('Failed to apply whitelist:', error);
		} finally {
			applying = false;
		}
	}

	async function pullWhitelist() {
		if (!pullServerId) {
			toast.error('Select a server to pull from');
			return;
		}
		pulling = true;
		try {
			const response = await rpcClient.admin.pullWhitelist(
				create(PullWhitelistRequestSchema, { serverId: pullServerId })
			);
			toast.success(`Imported ${response.imported.length} name${response.imported.length === 1 ? '' : 's'} from the server whitelist`);
			pullOpen = false;
			await loadEntries();
		} catch (error) {
			console.error('Failed to pull whitelist:', error);
		} finally {
			pulling = false;
		}
	}

	function serverSelectionLabel(selected: string[]): string {
		if (selected.length === 0) return 'All servers';
		if (selected.length === servers.length) return 'All servers (every server selected)';
		return `${selected.length} ${selected.length === 1 ? 'server' : 'servers'} selected`;
	}

	async function banPlayer(unban = false) {
		const name = banName.trim();
		if (!name) {
			toast.error('Player name is required');
			return;
		}
		banBusy = true;
		banResults = [];
		try {
			if (unban) {
				const response = await rpcClient.admin.unbanPlayer(
					create(UnbanPlayerRequestSchema, { serverIds: banSelected, name })
				);
				banResults = response.results;
				toast.success(`Unban commands sent for ${name}`);
			} else {
				const response = await rpcClient.admin.banPlayer(
					create(BanPlayerRequestSchema, { serverIds: banSelected, name, reason: banReason.trim() })
				);
				banResults = response.results;
				toast.success(`Ban commands sent for ${name}`);
			}
		} catch (error) {
			console.error('Failed to run ban command:', error);
		} finally {
			banBusy = false;
		}
	}

	function openApplyDialog() {
		applyResults = [];
		applyOpen = true;
	}

	function openPullDialog() {
		pullServerId = '';
		pullOpen = true;
	}

	onMount(() => {
		loadEntries();
		if ($serversStore.length === 0) {
			serversStore.fetchServers(true).catch(() => {});
		}
	});
</script>

<div class="space-y-6">
	<!-- Panel whitelist -->
	<Card>
		<CardHeader class="flex flex-row flex-wrap items-center justify-between gap-3 space-y-0">
			<div class="space-y-1">
				<CardTitle class="flex items-center gap-2">
					<ShieldCheck class="h-5 w-5 text-primary" />
					Panel Whitelist
				</CardTitle>
				<CardDescription>
					Desired whitelist state stored in the panel — apply it to servers via RCON
				</CardDescription>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<Button
					variant="outline"
					size="sm"
					class="border-2"
					onclick={openPullDialog}
				>
					<Download class="mr-2 h-4 w-4" />
					Pull from server
				</Button>
				<Button size="sm" onclick={openApplyDialog}>
					<Upload class="mr-2 h-4 w-4" />
					Apply to servers
				</Button>
			</div>
		</CardHeader>
		<CardContent class="space-y-4">
			<!-- Add entry -->
			<div class="flex flex-col gap-2 sm:flex-row">
				<Input
					placeholder="Player name"
					bind:value={addName}
					onkeydown={(e) => {
						if (e.key === 'Enter') addEntry();
					}}
					class="sm:max-w-48"
				/>
				<Input
					placeholder="Note (optional)"
					bind:value={addNote}
					onkeydown={(e) => {
						if (e.key === 'Enter') addEntry();
					}}
					class="flex-1"
				/>
				<Button onclick={addEntry} disabled={adding || !addName.trim()}>
					<Plus class="mr-2 h-4 w-4" />
					Add
				</Button>
			</div>

			<!-- Search -->
			<div class="relative max-w-sm">
				<Search class="absolute top-2.5 left-2.5 h-4 w-4 text-muted-foreground" />
				<Input
					placeholder="Search whitelist..."
					bind:value={searchInput}
					oninput={searchEntries}
					class="pl-8"
				/>
			</div>

			<!-- Entries table -->
			{#if loading}
				<div class="space-y-2">
					{#each Array(3) as _, i (i)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if entries.length === 0}
				<div class="py-10 text-center">
					<UserX class="mx-auto mb-3 h-10 w-10 text-muted-foreground/50" />
					<h3 class="mb-1 text-sm font-semibold">No whitelist entries</h3>
					<p class="text-sm text-muted-foreground">
						{#if searchInput}
							No entries match your search.
						{:else}
							Add players above or pull an existing whitelist from a server.
						{/if}
					</p>
				</div>
			{:else}
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Note</TableHead>
							<TableHead>Added</TableHead>
							<TableHead class="text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each entries as entry (entry.id)}
							<TableRow>
								<TableCell class="font-medium">{entry.name}</TableCell>
								<TableCell>
									{#if editingId === entry.id}
										<div class="flex items-center gap-1">
											<Input
												bind:value={editNote}
												class="h-8 max-w-64"
												onkeydown={(e) => {
													if (e.key === 'Enter') saveNote(entry);
													if (e.key === 'Escape') editingId = '';
												}}
											/>
											<Button variant="ghost" size="sm" class="h-8 w-8 p-0" onclick={() => saveNote(entry)}>
												<Check class="h-4 w-4" />
											</Button>
											<Button variant="ghost" size="sm" class="h-8 w-8 p-0" onclick={() => (editingId = '')}>
												<X class="h-4 w-4" />
											</Button>
										</div>
									{:else}
										<button
											class="group flex items-center gap-1 text-left text-muted-foreground hover:text-foreground"
											onclick={() => startEditNote(entry)}
											title="Click to edit note"
										>
											<span class="truncate">{entry.note || '—'}</span>
											<Pencil class="h-3 w-3 shrink-0 opacity-0 transition-opacity group-hover:opacity-60" />
										</button>
									{/if}
								</TableCell>
								<TableCell class="text-muted-foreground">
									{timestampToDate(entry.createdAt).getTime() > 0
										? timestampToDate(entry.createdAt).toLocaleDateString()
										: '—'}
								</TableCell>
								<TableCell class="text-right">
									<Button
										variant="ghost"
										size="sm"
										class="h-8 w-8 p-0 text-destructive hover:text-destructive"
										onclick={() => removeEntry(entry)}
									>
										<Trash2 class="h-4 w-4" />
									</Button>
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			{/if}
		</CardContent>
	</Card>

	<!-- Bans -->
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Gavel class="h-5 w-5 text-destructive" />
				Bans
			</CardTitle>
			<CardDescription>
				Ban or unban a player on the selected servers via RCON. Leave all servers unchecked to target every server.
			</CardDescription>
		</CardHeader>
		<CardContent class="space-y-4">
			<div class="flex flex-col gap-2 sm:flex-row">
				<Input
					placeholder="Player name"
					bind:value={banName}
					class="sm:max-w-48"
				/>
				<Input
					placeholder="Reason (optional)"
					bind:value={banReason}
					class="flex-1"
				/>
				<div class="flex gap-2">
					<Button
						variant="destructive"
						onclick={() => banPlayer(false)}
						disabled={banBusy || !banName.trim()}
					>
						<Ban class="mr-2 h-4 w-4" />
						Ban
					</Button>
					<Button
						variant="outline"
						class="border-2"
						onclick={() => banPlayer(true)}
						disabled={banBusy || !banName.trim()}
					>
						<ShieldCheck class="mr-2 h-4 w-4" />
						Unban
					</Button>
				</div>
			</div>

			<div class="flex flex-wrap gap-x-6 gap-y-2">
				<Label class="w-full text-sm font-medium">Servers ({serverSelectionLabel(banSelected)})</Label>
				{#each servers as server (server.id)}
					<Label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
						<Checkbox
							checked={banSelected.includes(server.id)}
							onCheckedChange={(checked) => toggleBanServer(server.id, checked === true)}
						/>
						{server.name}
					</Label>
				{/each}
				{#if servers.length === 0}
					<p class="text-sm text-muted-foreground">No servers available.</p>
				{/if}
			</div>

			{#if banResults.length > 0}
				<ServerOpResults results={banResults} />
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Apply to servers dialog -->
<Dialog bind:open={applyOpen}>
	<DialogContent class="max-h-[92vh] overflow-y-auto rounded-2xl border-2 sm:max-w-lg">
		<DialogHeader>
			<DialogTitle>Apply whitelist to servers</DialogTitle>
			<DialogDescription>
				Converge the selected servers to the panel whitelist via RCON. Leave all servers unchecked to apply to every server.
			</DialogDescription>
		</DialogHeader>

		{#if applyResults.length === 0}
			<div class="flex flex-wrap gap-x-6 gap-y-2">
				<Label class="w-full text-sm font-medium">
					Servers ({serverSelectionLabel(applySelected)})
				</Label>
				{#each servers as server (server.id)}
					<Label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
						<Checkbox
							checked={applySelected.includes(server.id)}
							onCheckedChange={(checked) => toggleApplyServer(server.id, checked === true)}
						/>
						{server.name}
					</Label>
				{/each}
				{#if servers.length === 0}
					<p class="text-sm text-muted-foreground">No servers available.</p>
				{/if}
			</div>
			<DialogFooter>
				<Button variant="outline" class="border-2" onclick={() => (applyOpen = false)} disabled={applying}>
					Cancel
				</Button>
				<Button onclick={applyWhitelist} disabled={applying}>
					<Upload class="mr-2 h-4 w-4" />
					{applying ? 'Applying...' : 'Apply whitelist'}
				</Button>
			</DialogFooter>
		{:else}
			<ServerOpResults results={applyResults} />
			<DialogFooter>
				<Button variant="outline" class="border-2" onclick={() => (applyOpen = false)}>Close</Button>
				<Button
					variant="secondary"
					onclick={() => {
						applyResults = [];
					}}
				>
					Run again
				</Button>
			</DialogFooter>
		{/if}
	</DialogContent>
</Dialog>

<!-- Pull from server dialog -->
<Dialog bind:open={pullOpen}>
	<DialogContent class="rounded-2xl border-2 sm:max-w-md">
		<DialogHeader>
			<DialogTitle>Pull whitelist from server</DialogTitle>
			<DialogDescription>
				Read the current whitelist from a server via RCON and import it into the panel.
			</DialogDescription>
		</DialogHeader>
		<div class="space-y-2">
			<Label class="text-sm font-medium">Server</Label>
			<select
				bind:value={pullServerId}
				class="border-input bg-background flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-sm focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
			>
				<option value="" disabled>Select a server...</option>
				{#each servers as server (server.id)}
					<option value={server.id}>{server.name}</option>
				{/each}
			</select>
			<Badge variant="secondary" class="text-xs">Pulling replaces nothing — names are merged into the panel list</Badge>
		</div>
		<DialogFooter>
			<Button variant="outline" class="border-2" onclick={() => (pullOpen = false)} disabled={pulling}>
				Cancel
			</Button>
			<Button onclick={pullWhitelist} disabled={pulling || !pullServerId}>
				<Download class="mr-2 h-4 w-4" />
				{pulling ? 'Pulling...' : 'Pull whitelist'}
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>
