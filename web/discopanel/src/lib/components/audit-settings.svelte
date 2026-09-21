<script lang="ts">
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { TimestampSchema } from '@bufbuild/protobuf/wkt';
	import { toast } from 'svelte-sonner';
	import { debounce } from 'lodash-es';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Tooltip, TooltipContent, TooltipTrigger } from '$lib/components/ui/tooltip';
	import {
		AlertDialog,
		AlertDialogAction,
		AlertDialogCancel,
		AlertDialogContent,
		AlertDialogDescription,
		AlertDialogFooter,
		AlertDialogHeader,
		AlertDialogTitle
	} from '$lib/components/ui/alert-dialog';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { ScrollText, ChevronLeft, ChevronRight, Trash2, RefreshCw, FileX } from '@lucide/svelte';
	import {
		ListAuditEntriesRequestSchema,
		ClearAuditEntriesRequestSchema
	} from '$lib/proto/discopanel/v1/audit_pb';
	import type { AuditEntry } from '$lib/proto/discopanel/v1/audit_pb';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { timestampToDate } from '$lib/utils';
	import { ERROR_BADGE_CLASS, ONLINE_BADGE_CLASS } from '$lib/utils/status-colors';

	const PAGE_SIZE = 100;

	const RESOURCES = [
		'servers',
		'server_config',
		'mods',
		'modpacks',
		'modules',
		'module_templates',
		'files',
		'tasks',
		'proxy',
		'users',
		'roles',
		'settings',
		'support',
		'uploads',
		'players'
	];

	const ACTIONS = ['read', 'create', 'update', 'delete', 'start', 'stop', 'restart', 'command'];

	let entries = $state<AuditEntry[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let refreshing = $state(false);
	let offset = $state(0);

	// Filters
	let usernameFilter = $state('');
	let resourceFilter = $state('');
	let actionFilter = $state('');

	// Clear dialog
	let clearOpen = $state(false);
	let clearBeforeDate = $state('');
	let clearing = $state(false);

	async function loadEntries(silent = false) {
		if (silent) {
			refreshing = true;
		} else {
			loading = true;
		}
		try {
			const response = await rpcClient.audit.listAuditEntries(
				create(ListAuditEntriesRequestSchema, {
					username: usernameFilter.trim(),
					resource: resourceFilter,
					action: actionFilter,
					limit: PAGE_SIZE,
					offset
				}),
				silent ? silentCallOptions : undefined
			);
			entries = response.entries;
			total = response.total;
		} catch (error) {
			console.error('Failed to load audit entries:', error);
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	const searchUsername = debounce(() => {
		offset = 0;
		loadEntries();
	}, 350);

	function applyFilters() {
		offset = 0;
		loadEntries();
	}

	function previousPage() {
		offset = Math.max(0, offset - PAGE_SIZE);
		loadEntries(true);
	}

	function nextPage() {
		offset += PAGE_SIZE;
		loadEntries(true);
	}

	function shortProcedure(procedure: string): string {
		const parts = procedure.split('/');
		return parts[parts.length - 1] || procedure;
	}

	async function clearEntries() {
		clearing = true;
		try {
			const request = create(ClearAuditEntriesRequestSchema, {});
			if (clearBeforeDate) {
				const date = new Date(clearBeforeDate);
				if (!Number.isNaN(date.getTime())) {
					request.before = create(TimestampSchema, {
						seconds: BigInt(Math.floor(date.getTime() / 1000)),
						nanos: (date.getTime() % 1000) * 1_000_000
					});
				}
			}
			const response = await rpcClient.audit.clearAuditEntries(request);
			toast.success(
				`Deleted ${response.deleted} audit ${Number(response.deleted) === 1 ? 'entry' : 'entries'}`
			);
			clearOpen = false;
			offset = 0;
			await loadEntries();
		} catch (error) {
			console.error('Failed to clear audit entries:', error);
		} finally {
			clearing = false;
		}
	}

	onMount(() => {
		loadEntries();
	});
</script>

<div class="space-y-6">
	<Card>
		<CardHeader class="flex flex-row flex-wrap items-center justify-between gap-3 space-y-0">
			<div class="space-y-1">
				<CardTitle class="flex items-center gap-2">
					<ScrollText class="h-5 w-5 text-primary" />
					Audit Log
				</CardTitle>
				<CardDescription>
					Trail of mutating panel operations — who did what, when and whether it succeeded
				</CardDescription>
			</div>
			<div class="flex items-center gap-2">
				<Button
					variant="outline"
					size="sm"
					class="border-2"
					onclick={() => loadEntries(true)}
					disabled={refreshing}
				>
					<RefreshCw class={`mr-2 h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
					Refresh
				</Button>
				<Button variant="destructive" size="sm" onclick={() => (clearOpen = true)}>
					<Trash2 class="mr-2 h-4 w-4" />
					Clear entries
				</Button>
			</div>
		</CardHeader>
		<CardContent class="space-y-4">
			<!-- Filters -->
			<div class="flex flex-col gap-2 sm:flex-row">
				<Input
					placeholder="Filter by username..."
					bind:value={usernameFilter}
					oninput={searchUsername}
					class="sm:max-w-48"
				/>
				<Select
					type="single"
					value={resourceFilter}
					onValueChange={(v: string | undefined) => {
						resourceFilter = v || '';
						applyFilters();
					}}
				>
					<SelectTrigger class="sm:w-48">
						<span>{resourceFilter || 'All resources'}</span>
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="">All resources</SelectItem>
						{#each RESOURCES as resource (resource)}
							<SelectItem value={resource}>{resource}</SelectItem>
						{/each}
					</SelectContent>
				</Select>
				<Select
					type="single"
					value={actionFilter}
					onValueChange={(v: string | undefined) => {
						actionFilter = v || '';
						applyFilters();
					}}
				>
					<SelectTrigger class="sm:w-44">
						<span>{actionFilter || 'All actions'}</span>
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="">All actions</SelectItem>
						{#each ACTIONS as action (action)}
							<SelectItem value={action}>{action}</SelectItem>
						{/each}
					</SelectContent>
				</Select>
			</div>

			<!-- Table -->
			{#if loading}
				<div class="space-y-2">
					{#each Array(6) as _, i (i)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if entries.length === 0}
				<div class="py-12 text-center">
					<FileX class="mx-auto mb-3 h-10 w-10 text-muted-foreground/50" />
					<h3 class="mb-1 text-sm font-semibold">No audit entries</h3>
					<p class="text-sm text-muted-foreground">
						{#if usernameFilter || resourceFilter || actionFilter}
							No entries match your filters.
						{:else}
							Panel operations will be recorded here as they happen.
						{/if}
					</p>
				</div>
			{:else}
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Time</TableHead>
							<TableHead>Username</TableHead>
							<TableHead>Action</TableHead>
							<TableHead>Procedure</TableHead>
							<TableHead>Object ID</TableHead>
							<TableHead>Status</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each entries as entry (entry.id)}
							<TableRow>
								<TableCell class="text-muted-foreground">
									{timestampToDate(entry.createdAt).getTime() > 0
										? timestampToDate(entry.createdAt).toLocaleString()
										: '—'}
								</TableCell>
								<TableCell class="font-medium">{entry.username || '—'}</TableCell>
								<TableCell>
									<Badge variant="secondary" class="font-mono text-xs">
										{entry.resource}/{entry.action}
									</Badge>
								</TableCell>
								<TableCell class="font-mono text-xs text-muted-foreground">
									{shortProcedure(entry.procedure)}
								</TableCell>
								<TableCell class="max-w-40 truncate font-mono text-xs text-muted-foreground">
									{entry.objectId || '—'}
								</TableCell>
								<TableCell>
									{#if entry.status === 'error'}
										<Tooltip>
											<TooltipTrigger>
												<Badge variant="outline" class="cursor-help text-xs {ERROR_BADGE_CLASS}">
													Error
												</Badge>
											</TooltipTrigger>
											<TooltipContent class="max-w-72 break-words">
												{entry.detail || 'Unknown error'}
											</TooltipContent>
										</Tooltip>
									{:else}
										<Badge variant="outline" class="text-xs {ONLINE_BADGE_CLASS}">OK</Badge>
									{/if}
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>

				{#if total > PAGE_SIZE || offset > 0}
					<div class="flex items-center justify-center gap-2">
						<Button variant="outline" size="sm" disabled={offset === 0} onclick={previousPage}>
							<ChevronLeft class="mr-1 h-4 w-4" />
							Previous
						</Button>
						<span class="text-sm text-muted-foreground">
							{offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
						</span>
						<Button
							variant="outline"
							size="sm"
							disabled={offset + PAGE_SIZE >= total}
							onclick={nextPage}
						>
							Next
							<ChevronRight class="ml-1 h-4 w-4" />
						</Button>
					</div>
				{/if}
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Clear entries dialog -->
<AlertDialog bind:open={clearOpen}>
	<AlertDialogContent class="rounded-2xl border-2">
		<AlertDialogHeader>
			<AlertDialogTitle>Clear audit entries?</AlertDialogTitle>
			<AlertDialogDescription>
				This permanently deletes audit entries and cannot be undone. Optionally pick a date to only
				delete entries recorded before it — leave it empty to clear everything.
			</AlertDialogDescription>
		</AlertDialogHeader>
		<Input type="date" bind:value={clearBeforeDate} class="max-w-48" />
		<AlertDialogFooter>
			<AlertDialogCancel disabled={clearing}>Cancel</AlertDialogCancel>
			<AlertDialogAction
				class="bg-destructive text-white hover:bg-destructive/90"
				disabled={clearing}
				onclick={(e) => {
					e.preventDefault();
					clearEntries();
				}}
			>
				{clearing ? 'Clearing...' : 'Clear entries'}
			</AlertDialogAction>
		</AlertDialogFooter>
	</AlertDialogContent>
</AlertDialog>
