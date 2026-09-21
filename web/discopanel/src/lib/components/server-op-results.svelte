<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { CheckCircle2, XCircle } from '@lucide/svelte';
	import type { ServerOpResult } from '$lib/proto/discopanel/v1/admin_pb';

	let { results = [] }: { results?: ServerOpResult[] } = $props();
</script>

{#if results.length > 0}
	<div class="space-y-2">
		{#each results as result (result.serverId)}
			<div
				class="flex flex-col gap-1 rounded-lg border p-3 {result.success
					? 'border-green-500/20 bg-green-500/5'
					: 'border-red-500/20 bg-red-500/5'}"
			>
				<div class="flex items-center justify-between gap-2">
					<div class="flex items-center gap-2">
						{#if result.success}
							<CheckCircle2 class="h-4 w-4 shrink-0 text-green-600 dark:text-green-400" />
						{:else}
							<XCircle class="h-4 w-4 shrink-0 text-red-600 dark:text-red-400" />
						{/if}
						<span class="text-sm font-medium">{result.serverName || result.serverId}</span>
					</div>
					<Badge
						variant="outline"
						class="text-xs {result.success
							? 'border-green-500/20 bg-green-500/10 text-green-600 dark:text-green-400'
							: 'border-red-500/20 bg-red-500/10 text-red-600 dark:text-red-400'}"
					>
						{result.success ? 'Success' : 'Failed'}
					</Badge>
				</div>
				{#if result.message}
					<p
						class="text-xs text-muted-foreground {result.success
							? ''
							: 'text-red-600 dark:text-red-400'}"
					>
						{result.message}
					</p>
				{/if}
				{#if result.added.length > 0}
					<p class="text-xs text-muted-foreground">
						Added ({result.added.length}):
						<span class="font-mono">{result.added.join(', ')}</span>
					</p>
				{/if}
				{#if result.removed.length > 0}
					<p class="text-xs text-muted-foreground">
						Removed ({result.removed.length}):
						<span class="font-mono">{result.removed.join(', ')}</span>
					</p>
				{/if}
			</div>
		{/each}
	</div>
{/if}
