<script lang="ts">
	import { onMount } from 'svelte';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { toast } from 'svelte-sonner';
	import { create } from '@bufbuild/protobuf';
	import {
		ListAlertRulesRequestSchema,
		ListAlertEventsRequestSchema,
		CreateAlertRuleRequestSchema,
		UpdateAlertRuleRequestSchema,
		DeleteAlertRuleRequestSchema,
		TestAlertRuleRequestSchema,
		AlertMetric,
		AlertComparator,
		AlertState,
		type AlertRule,
		type AlertEvent
	} from '$lib/proto/discopanel/v1/metrics_pb';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Select from '$lib/components/ui/select';
	import * as Dialog from '$lib/components/ui/dialog';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import {
		Bell,
		Plus,
		Pencil,
		Trash2,
		RefreshCw,
		Loader2,
		FlaskConical,
		BellRing,
		BellOff,
		AlertTriangle
	} from '@lucide/svelte';

	let loading = $state(true);
	let rules = $state<AlertRule[]>([]);
	let events = $state<AlertEvent[]>([]);
	let servers = $state<Server[]>([]);

	// Dialog state
	let showRuleDialog = $state(false);
	let saving = $state(false);
	let testing = $state(false);
	let selectedRule = $state<AlertRule | null>(null);

	// Form state
	let formName = $state('');
	let formDescription = $state('');
	let formServerId = $state('');
	let formMetric = $state<AlertMetric>(AlertMetric.CPU_PERCENT);
	let formComparator = $state<AlertComparator>(AlertComparator.ABOVE);
	let formThreshold = $state(80);
	let formDurationMins = $state(5);
	let formCooldownMins = $state(5);
	let formEnabled = $state(true);

	let eventsTimer: ReturnType<typeof setInterval> | undefined;

	onMount(() => {
		loadAll();
		eventsTimer = setInterval(() => {
			if (document.visibilityState === 'visible') {
				loadEvents(true);
			}
		}, 60000);
		return () => {
			if (eventsTimer) clearInterval(eventsTimer);
		};
	});

	async function loadAll() {
		await Promise.all([loadRules(), loadEvents(), loadServers()]);
	}

	async function loadRules() {
		try {
			const response = await rpcClient.metric.listAlertRules(
				create(ListAlertRulesRequestSchema, { serverId: '' }),
				silentCallOptions
			);
			rules = response.rules;
		} catch {
			toast.error('Failed to load alert rules');
		} finally {
			loading = false;
		}
	}

	async function loadEvents(isRefresh = false) {
		try {
			const response = await rpcClient.metric.listAlertEvents(
				create(ListAlertEventsRequestSchema, { serverId: '', limit: 50 }),
				silentCallOptions
			);
			events = response.events;
		} catch {
			if (!isRefresh) toast.error('Failed to load alert events');
		}
	}

	async function loadServers() {
		try {
			const response = await rpcClient.server.listServers({ fullStats: false }, silentCallOptions);
			servers = response.servers;
		} catch {
			// Server selector just stays empty; "All servers" still works
		}
	}

	const serverNames = $derived.by(() => {
		const map = new Map<string, string>();
		for (const s of servers) {
			map.set(s.id, s.name);
		}
		return map;
	});

	function serverLabel(serverId: string): string {
		if (!serverId) return 'All servers';
		return serverNames.get(serverId) ?? serverId;
	}

	const METRIC_LABELS: Record<AlertMetric, string> = {
		[AlertMetric.UNSPECIFIED]: 'Unknown',
		[AlertMetric.CPU_PERCENT]: 'CPU %',
		[AlertMetric.MEMORY_PERCENT]: 'Memory %',
		[AlertMetric.TPS]: 'TPS',
		[AlertMetric.PLAYERS_ONLINE]: 'Players online',
		[AlertMetric.DISK_PERCENT]: 'Disk %'
	};

	const COMPARATOR_LABELS: Record<AlertComparator, string> = {
		[AlertComparator.UNSPECIFIED]: 'Unknown',
		[AlertComparator.BELOW]: 'below',
		[AlertComparator.ABOVE]: 'above'
	};

	function metricLabel(metric: AlertMetric): string {
		return METRIC_LABELS[metric] ?? 'Unknown';
	}

	function formatDuration(secs: number): string {
		if (secs <= 0) return '';
		if (secs < 60) return `${secs}s`;
		const mins = secs / 60;
		const display = Number.isInteger(mins) ? mins.toString() : mins.toFixed(1);
		return `${display}m`;
	}

	function conditionLabel(
		metric: AlertMetric,
		comparator: AlertComparator,
		threshold: number,
		durationSecs: number
	): string {
		const duration = formatDuration(durationSecs);
		return `${metricLabel(metric)} ${COMPARATOR_LABELS[comparator] ?? '?'} ${threshold}${duration ? ` for ${duration}` : ''}`;
	}

	function formatCooldown(secs: number): string {
		if (secs <= 0) return 'default (5m)';
		return formatDuration(secs);
	}

	function resetForm() {
		formName = '';
		formDescription = '';
		formServerId = '';
		formMetric = AlertMetric.CPU_PERCENT;
		formComparator = AlertComparator.ABOVE;
		formThreshold = 80;
		formDurationMins = 5;
		formCooldownMins = 5;
		formEnabled = true;
		selectedRule = null;
	}

	function openCreateDialog() {
		resetForm();
		showRuleDialog = true;
	}

	function openEditDialog(rule: AlertRule) {
		resetForm();
		selectedRule = rule;
		formName = rule.name;
		formDescription = rule.description;
		formServerId = rule.serverId;
		formMetric = rule.metric;
		formComparator = rule.comparator;
		formThreshold = rule.threshold;
		formDurationMins = Math.round((rule.durationSecs / 60) * 10) / 10;
		formCooldownMins = Math.round((rule.cooldownSecs / 60) * 10) / 10;
		formEnabled = rule.enabled;
		showRuleDialog = true;
	}

	function closeDialog() {
		showRuleDialog = false;
		resetForm();
	}

	async function saveRule() {
		if (!formName.trim()) {
			toast.error('Rule name is required');
			return;
		}
		saving = true;
		try {
			if (selectedRule) {
				await rpcClient.metric.updateAlertRule(
					create(UpdateAlertRuleRequestSchema, {
						id: selectedRule.id,
						name: formName.trim(),
						description: formDescription,
						serverId: formServerId,
						metric: formMetric,
						comparator: formComparator,
						threshold: formThreshold,
						durationSecs: Math.round(formDurationMins * 60),
						cooldownSecs: Math.round(formCooldownMins * 60),
						enabled: formEnabled
					})
				);
				toast.success('Alert rule updated');
			} else {
				await rpcClient.metric.createAlertRule(
					create(CreateAlertRuleRequestSchema, {
						name: formName.trim(),
						description: formDescription,
						serverId: formServerId,
						metric: formMetric,
						comparator: formComparator,
						threshold: formThreshold,
						durationSecs: Math.round(formDurationMins * 60),
						cooldownSecs: Math.round(formCooldownMins * 60),
						enabled: formEnabled
					})
				);
				toast.success('Alert rule created');
			}
			closeDialog();
			await loadRules();
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Failed to save alert rule');
		} finally {
			saving = false;
		}
	}

	async function toggleRule(rule: AlertRule, checked: boolean) {
		try {
			await rpcClient.metric.updateAlertRule(
				create(UpdateAlertRuleRequestSchema, { id: rule.id, enabled: checked })
			);
			toast.success(`Rule "${rule.name}" ${checked ? 'enabled' : 'disabled'}`);
			await loadRules();
		} catch {
			toast.error('Failed to update rule');
		}
	}

	async function deleteRule(rule: AlertRule) {
		if (!confirm(`Delete alert rule "${rule.name}"? This cannot be undone.`)) return;
		try {
			await rpcClient.metric.deleteAlertRule(create(DeleteAlertRuleRequestSchema, { id: rule.id }));
			toast.success('Alert rule deleted');
			await loadRules();
		} catch {
			toast.error('Failed to delete alert rule');
		}
	}

	async function testRule() {
		// TestAlertRule evaluates against a specific server's live metrics
		const testServerId = formServerId || servers[0]?.id;
		if (!testServerId) {
			toast.error('Add a server first to test rules against');
			return;
		}
		testing = true;
		try {
			const response = await rpcClient.metric.testAlertRule(
				create(TestAlertRuleRequestSchema, {
					serverId: testServerId,
					metric: formMetric,
					comparator: formComparator,
					threshold: formThreshold
				})
			);
			const target = formServerId ? serverLabel(formServerId) : serverLabel(testServerId);
			if (response.wouldFire) {
				toast.warning(
					`Would fire now on "${target}" (current ${metricLabel(formMetric)}: ${response.currentValue.toFixed(1)})`
				);
			} else {
				toast.info(
					`Would not fire on "${target}" (current ${metricLabel(formMetric)}: ${response.currentValue.toFixed(1)})`
				);
			}
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Failed to test rule');
		} finally {
			testing = false;
		}
	}

	function eventBadge(state: AlertState): { label: string; class: string } {
		switch (state) {
			case AlertState.FIRING:
				return {
					label: 'Firing',
					class: 'border-red-500/30 bg-red-500/10 text-red-500'
				};
			case AlertState.RESOLVED:
				return {
					label: 'Resolved',
					class: 'border-green-500/30 bg-green-500/10 text-green-500'
				};
			default:
				return { label: 'Unknown', class: '' };
		}
	}
</script>

<div class="space-y-6">
	<!-- Alert Rules -->
	<div class="space-y-4">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div class="flex items-center gap-2">
				<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
					<Bell class="h-5 w-5 text-primary" />
				</div>
				<div>
					<h3 class="text-lg font-semibold">Alert Rules</h3>
					<p class="text-sm text-muted-foreground">
						Get notified when a server's metrics cross a threshold.
					</p>
				</div>
			</div>
			<div class="flex gap-2">
				<Button variant="outline" size="sm" onclick={() => loadRules()} disabled={loading}>
					<RefreshCw class="mr-2 h-4 w-4" />
					Refresh
				</Button>
				<Button size="sm" onclick={openCreateDialog}>
					<Plus class="mr-2 h-4 w-4" />
					New Rule
				</Button>
			</div>
		</div>

		{#if loading}
			<div class="space-y-2">
				{#each Array(3) as _, i (i)}
					<Skeleton class="h-12 w-full" />
				{/each}
			</div>
		{:else if rules.length === 0}
			<div class="flex flex-col items-center justify-center rounded-lg border border-dashed py-12">
				<BellOff class="mb-3 h-10 w-10 text-muted-foreground/50" />
				<p class="font-medium">No alert rules yet</p>
				<p class="mt-1 mb-4 text-sm text-muted-foreground">
					Create a rule to get alerted when something needs attention.
				</p>
				<Button size="sm" onclick={openCreateDialog}>
					<Plus class="mr-2 h-4 w-4" />
					New Rule
				</Button>
			</div>
		{:else}
			<div class="rounded-lg border border-border/50">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Server</TableHead>
							<TableHead>Condition</TableHead>
							<TableHead>Cooldown</TableHead>
							<TableHead>Enabled</TableHead>
							<TableHead class="w-[100px] text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each rules as rule (rule.id)}
							<TableRow>
								<TableCell>
									<div class="font-medium">{rule.name}</div>
									{#if rule.description}
										<div class="max-w-xs truncate text-xs text-muted-foreground">
											{rule.description}
										</div>
									{/if}
								</TableCell>
								<TableCell class="text-sm">{serverLabel(rule.serverId)}</TableCell>
								<TableCell>
									<Badge variant="outline" class="font-mono text-xs">
										{conditionLabel(rule.metric, rule.comparator, rule.threshold, rule.durationSecs)}
									</Badge>
								</TableCell>
								<TableCell class="text-xs text-muted-foreground">
									{formatCooldown(rule.cooldownSecs)}
								</TableCell>
								<TableCell>
									<Switch
										checked={rule.enabled}
										onCheckedChange={(checked) => toggleRule(rule, checked)}
									/>
								</TableCell>
								<TableCell class="text-right">
									<div class="flex justify-end gap-1">
										<Button
											variant="ghost"
											size="icon"
											class="h-8 w-8"
											onclick={() => openEditDialog(rule)}
											title="Edit"
										>
											<Pencil class="h-4 w-4" />
										</Button>
										<Button
											variant="ghost"
											size="icon"
											class="h-8 w-8 text-destructive hover:text-destructive"
											onclick={() => deleteRule(rule)}
											title="Delete"
										>
											<Trash2 class="h-4 w-4" />
										</Button>
									</div>
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
	</div>

	<!-- Recent Alert Events -->
	<div class="space-y-4">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
					<AlertTriangle class="h-5 w-5 text-primary" />
				</div>
				<div>
					<h3 class="text-lg font-semibold">Recent Alert Events</h3>
					<p class="text-sm text-muted-foreground">Last 50 firings and resolutions.</p>
				</div>
			</div>
			<Button variant="ghost" size="sm" onclick={() => loadEvents(true)}>
				<RefreshCw class="h-4 w-4" />
			</Button>
		</div>

		{#if events.length === 0}
			<div class="flex flex-col items-center justify-center rounded-lg border border-dashed py-10">
				<Bell class="mb-3 h-8 w-8 text-muted-foreground/50" />
				<p class="text-sm text-muted-foreground">
					No alert events yet. Events appear here when rules fire.
				</p>
			</div>
		{:else}
			<div class="rounded-lg border border-border/50">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead class="w-[160px]">Time</TableHead>
							<TableHead>Rule</TableHead>
							<TableHead>Server</TableHead>
							<TableHead>Message</TableHead>
							<TableHead class="w-[100px]">State</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{#each events as event (event.id)}
							{@const badge = eventBadge(event.state)}
							<TableRow>
								<TableCell class="text-xs text-muted-foreground">
									{event.createdAt
										? new Date(Number(event.createdAt.seconds) * 1000).toLocaleString()
										: '--'}
								</TableCell>
								<TableCell class="text-sm font-medium">{event.ruleName}</TableCell>
								<TableCell class="text-sm">{serverLabel(event.serverId)}</TableCell>
								<TableCell class="max-w-md truncate text-sm text-muted-foreground">
									{event.message}
								</TableCell>
								<TableCell>
									<Badge variant="outline" class="{badge.class} text-xs">
										{#if event.state === AlertState.FIRING}
											<BellRing class="mr-1 h-3 w-3" />
										{:else}
											<BellOff class="mr-1 h-3 w-3" />
										{/if}
										{badge.label}
									</Badge>
								</TableCell>
							</TableRow>
						{/each}
					</TableBody>
				</Table>
			</div>
		{/if}
	</div>
</div>

<!-- Create/Edit Rule Dialog -->
<Dialog.Root bind:open={showRuleDialog}>
	<Dialog.Content class="max-h-[85vh] max-w-lg overflow-y-auto">
		<Dialog.Header>
			<Dialog.Title>{selectedRule ? 'Edit Alert Rule' : 'New Alert Rule'}</Dialog.Title>
			<Dialog.Description>
				Fire an alert when a metric crosses the threshold for the given duration.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4">
			<div class="space-y-2">
				<Label for="ruleName">Name *</Label>
				<Input id="ruleName" bind:value={formName} placeholder="High CPU usage" />
			</div>

			<div class="space-y-2">
				<Label for="ruleDescription">Description</Label>
				<Input
					id="ruleDescription"
					bind:value={formDescription}
					placeholder="Warns when the server is overloaded"
				/>
			</div>

			<div class="space-y-2">
				<Label>Server</Label>
				<Select.Root
					type="single"
					value={formServerId}
					onValueChange={(v) => {
						formServerId = v ?? '';
					}}
				>
					<Select.Trigger class="w-full">
						{formServerId ? serverLabel(formServerId) : 'All servers'}
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="" label="All servers">All servers</Select.Item>
						{#each servers as server (server.id)}
							<Select.Item value={server.id} label={server.name}>{server.name}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
				<p class="text-xs text-muted-foreground">
					"All servers" applies the rule to every server.
				</p>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div class="space-y-2">
					<Label>Metric</Label>
					<Select.Root
						type="single"
						value={formMetric.toString()}
						onValueChange={(v) => {
							if (v) formMetric = parseInt(v) as AlertMetric;
						}}
					>
						<Select.Trigger class="w-full">
							{metricLabel(formMetric)}
						</Select.Trigger>
						<Select.Content>
							{#each [AlertMetric.CPU_PERCENT, AlertMetric.MEMORY_PERCENT, AlertMetric.TPS, AlertMetric.PLAYERS_ONLINE, AlertMetric.DISK_PERCENT] as metric (metric)}
								<Select.Item value={metric.toString()} label={metricLabel(metric)}>
									{metricLabel(metric)}
								</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="space-y-2">
					<Label>Comparator</Label>
					<Select.Root
						type="single"
						value={formComparator.toString()}
						onValueChange={(v) => {
							if (v) formComparator = parseInt(v) as AlertComparator;
						}}
					>
						<Select.Trigger class="w-full">
							{COMPARATOR_LABELS[formComparator]}
						</Select.Trigger>
						<Select.Content>
							<Select.Item
								value={AlertComparator.ABOVE.toString()}
								label="Above (metric > threshold)"
								>Above (metric &gt; threshold)</Select.Item
							>
							<Select.Item
								value={AlertComparator.BELOW.toString()}
								label="Below (metric < threshold)"
								>Below (metric &lt; threshold)</Select.Item
							>
						</Select.Content>
					</Select.Root>
				</div>
			</div>

			<div class="space-y-2">
				<Label for="ruleThreshold">Threshold *</Label>
				<Input id="ruleThreshold" type="number" bind:value={formThreshold} step="any" />
				<p class="text-xs text-muted-foreground">
					{conditionLabel(
						formMetric,
						formComparator,
						formThreshold,
						Math.round(formDurationMins * 60)
					)}
				</p>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div class="space-y-2">
					<Label for="ruleDuration">Duration (mins)</Label>
					<Input
						id="ruleDuration"
						type="number"
						bind:value={formDurationMins}
						min="0"
						step="0.5"
					/>
					<p class="text-xs text-muted-foreground">
						Condition must hold this long before firing. 0 = immediate.
					</p>
				</div>
				<div class="space-y-2">
					<Label for="ruleCooldown">Cooldown (mins)</Label>
					<Input
						id="ruleCooldown"
						type="number"
						bind:value={formCooldownMins}
						min="0"
						step="0.5"
					/>
					<p class="text-xs text-muted-foreground">
						Minimum time between repeat firings. 0 = default (5m).
					</p>
				</div>
			</div>

			<label class="flex cursor-pointer items-center gap-3 rounded-lg border p-3">
				<Checkbox
					checked={formEnabled}
					onCheckedChange={(checked) => (formEnabled = checked === true)}
				/>
				<div>
					<span class="text-sm font-medium">Enabled</span>
					<p class="text-xs text-muted-foreground">Rule is evaluated as soon as it is saved.</p>
				</div>
			</label>
		</div>

		<Dialog.Footer class="gap-2 sm:gap-2">
			<Button
				variant="outline"
				onclick={testRule}
				disabled={saving || testing || servers.length === 0}
				class="mr-auto"
			>
				{#if testing}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<FlaskConical class="mr-2 h-4 w-4" />
				{/if}
				Test
			</Button>
			<Button variant="ghost" onclick={closeDialog}>Cancel</Button>
			<Button onclick={saveRule} disabled={!formName.trim() || saving} class="min-w-[100px]">
				{#if saving}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
					Saving...
				{:else}
					{selectedRule ? 'Save Changes' : 'Create Rule'}
				{/if}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
