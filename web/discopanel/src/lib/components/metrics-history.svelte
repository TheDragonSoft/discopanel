<script lang="ts">
	import { onMount } from 'svelte';
	import { rpcClient, silentCallOptions } from '$lib/api/rpc-client';
	import { create } from '@bufbuild/protobuf';
	import {
		ListMetricHistoryRequestSchema,
		ListAlertEventsRequestSchema,
		AlertState,
		type MetricSample
	} from '$lib/proto/discopanel/v1/metrics_pb';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Badge } from '$lib/components/ui/badge';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import {
		Cpu,
		MemoryStick,
		Gauge,
		Users,
		RefreshCw,
		BellRing,
		LineChart,
		Clock
	} from '@lucide/svelte';

	let { server, active }: { server: Server; active?: boolean } = $props();

	// Time ranges offered in the selector
	const RANGES = [
		{ label: 'Last hour', secs: 3600 },
		{ label: 'Last 6 hours', secs: 21600 },
		{ label: 'Last 24 hours', secs: 86400 }
	];

	let rangeSecs = $state(3600);
	let samples = $state<MetricSample[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let firingAlerts = $state(0);
	let initialized = $state(false);
	let lastRange = $state(0);
	let prevServerId = $state('');
	// svelte-ignore state_referenced_locally
	let activeRef = active;

	let refreshTimer: ReturnType<typeof setInterval> | undefined;

	function isActive(): boolean {
		return activeRef === true && document.visibilityState === 'visible';
	}

	onMount(() => {
		refreshTimer = setInterval(() => {
			if (isActive()) loadHistory(true);
		}, 60000);

		const onVisibility = () => {
			if (document.visibilityState === 'visible' && activeRef && initialized) {
				loadHistory(true);
			}
		};
		document.addEventListener('visibilitychange', onVisibility);

		return () => {
			if (refreshTimer) clearInterval(refreshTimer);
			document.removeEventListener('visibilitychange', onVisibility);
		};
	});

	$effect(() => {
		// Track tab visibility reactively (prop)
		activeRef = active;
		if (server.id !== prevServerId) {
			prevServerId = server.id;
			initialized = false;
			samples = [];
			loading = true;
		}
		const rs = rangeSecs;
		if (active && !initialized) {
			initialized = true;
			lastRange = rs;
			loadHistory();
		} else if (active && rs !== lastRange) {
			lastRange = rs;
			loadHistory();
		}
	});

	async function loadHistory(isRefresh = false) {
		if (!server?.id) return;
		if (isRefresh) {
			refreshing = true;
		} else {
			loading = true;
		}
		try {
			const historyRequest = create(ListMetricHistoryRequestSchema, {
				serverId: server.id,
				rangeSecs: rangeSecs,
				maxPoints: 200
			});
			const [historyRes, eventsRes] = await Promise.all([
				rpcClient.metric.listMetricHistory(historyRequest, silentCallOptions),
				rpcClient.metric
					.listAlertEvents(
						create(ListAlertEventsRequestSchema, { serverId: server.id, limit: 50 }),
						silentCallOptions
					)
					.catch(() => null)
			]);
			samples = historyRes.samples;
			firingAlerts = (eventsRes?.events ?? []).filter((e) => e.state === AlertState.FIRING).length;
		} catch {
			// Silent for background refreshes; first load errors surface as empty state
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	type Point = { x: number; y: number };

	type ChartSpec = {
		key: string;
		title: string;
		icon: typeof Cpu;
		values: number[];
		times: number[];
		min: number;
		max: number;
		refLine?: number;
		refLabel?: string;
		color: string;
		latest: string;
		sub?: string;
	};

	// SVG geometry (viewBox space). preserveAspectRatio="none" stretches the box;
	// strokes use vector-effect="non-scaling-stroke" to stay crisp.
	const W = 300;
	const H = 80;
	const PAD_TOP = 5;
	const PAD_BOTTOM = 5;
	const PAD_X = 2;

	function yFor(value: number, spec: ChartSpec): number {
		const span = spec.max - spec.min || 1;
		const clamped = Math.min(Math.max(value, spec.min), spec.max);
		return PAD_TOP + (1 - (clamped - spec.min) / span) * (H - PAD_TOP - PAD_BOTTOM);
	}

	function xFor(i: number, spec: ChartSpec): number {
		const n = spec.times.length;
		if (n <= 1) return PAD_X;
		const tMin = spec.times[0];
		const tMax = spec.times[n - 1];
		const span = tMax - tMin || 1;
		return PAD_X + ((spec.times[i] - tMin) / span) * (W - PAD_X * 2);
	}

	function linePath(spec: ChartSpec): string {
		if (spec.values.length === 0) return '';
		let d = '';
		for (let i = 0; i < spec.values.length; i++) {
			const x = xFor(i, spec).toFixed(2);
			const y = yFor(spec.values[i], spec).toFixed(2);
			d += (i === 0 ? 'M' : 'L') + `${x} ${y} `;
		}
		return d.trim();
	}

	function areaPath(spec: ChartSpec): string {
		if (spec.values.length < 2) return '';
		const last = spec.values.length - 1;
		return `${linePath(spec)} L${xFor(last, spec).toFixed(2)} ${H - PAD_BOTTOM} L${xFor(0, spec).toFixed(2)} ${H - PAD_BOTTOM} Z`;
	}

	function fmtTime(ms: number): string {
		return new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
	}

	function fmtMem(mb: number): string {
		if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
		return `${Math.round(mb)} MB`;
	}

	const series = $derived.by(() => {
		const times = samples.map((s) => (s.timestamp ? Number(s.timestamp.seconds) * 1000 : 0));
		const cpu = samples.map((s) => s.cpuPercent);
		const mem = samples.map((s) => s.memoryUsage);
		const tps = samples.map((s) => s.tps);
		const players = samples.map((s) => s.playersOnline);

		const maxMemTotal = Math.max(...samples.map((s) => s.memoryTotal), 0);
		const peakMem = Math.max(...mem, 1);
		const maxPlayers = server.maxPlayersSlp || server.maxPlayers || 0;
		const peakPlayers = Math.max(...players, 0);

		const last = <T>(arr: T[]): T | undefined => (arr.length > 0 ? arr[arr.length - 1] : undefined);
		const lastCpu = last(cpu);
		const lastMem = last(mem);
		const lastTps = last(tps);
		const lastPlayers = last(players);

		const specs: ChartSpec[] = [
			{
				key: 'cpu',
				title: 'CPU',
				icon: Cpu,
				values: cpu,
				times,
				min: 0,
				max: 100,
				color: '#3b82f6',
				latest: lastCpu !== undefined ? `${lastCpu.toFixed(1)}%` : '--'
			},
			{
				key: 'memory',
				title: 'Memory',
				icon: MemoryStick,
				values: mem,
				times,
				min: 0,
				max: Math.max(maxMemTotal, peakMem * 1.1, 1),
				refLine: maxMemTotal > 0 ? maxMemTotal : undefined,
				refLabel: maxMemTotal > 0 ? `Allocated ${fmtMem(maxMemTotal)}` : undefined,
				color: '#f97316',
				latest:
					lastMem !== undefined
						? maxMemTotal > 0
							? `${fmtMem(lastMem)} / ${fmtMem(maxMemTotal)}`
							: fmtMem(lastMem)
						: '--'
			},
			{
				key: 'tps',
				title: 'TPS',
				icon: Gauge,
				values: tps,
				times,
				min: 0,
				max: 20,
				refLine: 20,
				refLabel: 'Optimal 20',
				color: '#22c55e',
				latest: lastTps !== undefined ? lastTps.toFixed(1) : '--'
			},
			{
				key: 'players',
				title: 'Players Online',
				icon: Users,
				values: players,
				times,
				min: 0,
				max: Math.max(4, maxPlayers, peakPlayers),
				color: '#a855f7',
				latest:
					lastPlayers !== undefined
						? maxPlayers > 0
							? `${lastPlayers} / ${maxPlayers}`
							: `${lastPlayers}`
						: '--'
			}
		];
		return specs;
	});

	const timeRangeLabel = $derived.by(() => {
		if (samples.length === 0) return '';
		const first = samples[0].timestamp;
		const lastSample = samples[samples.length - 1].timestamp;
		if (!first || !lastSample) return '';
		return `${fmtTime(Number(first.seconds) * 1000)} – ${fmtTime(Number(lastSample.seconds) * 1000)}`;
	});
</script>

<div class="space-y-4">
	<!-- Header -->
	<div class="flex flex-wrap items-center justify-between gap-3">
		<div class="flex items-center gap-2">
			<div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
				<LineChart class="h-5 w-5 text-primary" />
			</div>
			<div>
				<h3 class="flex items-center gap-2 text-lg font-semibold">
					Metric History
					{#if firingAlerts > 0}
						<Badge variant="destructive" class="gap-1 text-xs">
							<BellRing class="h-3 w-3" />
							{firingAlerts} firing alert{firingAlerts === 1 ? '' : 's'}
						</Badge>
					{/if}
				</h3>
				<p class="text-sm text-muted-foreground">
					CPU, memory, TPS and player trends over time.
				</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<Select.Root
				type="single"
				value={rangeSecs.toString()}
				onValueChange={(v) => {
					if (v) rangeSecs = parseInt(v);
				}}
			>
				<Select.Trigger class="h-8 w-[140px] text-xs">
					{RANGES.find((r) => r.secs === rangeSecs)?.label ?? 'Last hour'}
				</Select.Trigger>
				<Select.Content>
					{#each RANGES as range (range.secs)}
						<Select.Item value={range.secs.toString()} label={range.label}>
							{range.label}
						</Select.Item>
					{/each}
				</Select.Content>
			</Select.Root>
			<Button variant="outline" size="sm" onclick={() => loadHistory(true)} disabled={refreshing}>
				{#if refreshing}
					<RefreshCw class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<RefreshCw class="mr-2 h-4 w-4" />
				{/if}
				Refresh
			</Button>
		</div>
	</div>

	<!-- Loading skeleton -->
	{#if loading}
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
			{#each Array(4) as _, i (i)}
				<div class="space-y-2 rounded-lg border border-border/50 bg-muted/10 p-3">
					<Skeleton class="h-4 w-32" />
					<Skeleton class="h-20 w-full" />
				</div>
			{/each}
		</div>
	{:else if samples.length === 0}
		<!-- Empty state -->
		<div class="flex flex-col items-center justify-center rounded-lg border border-dashed py-16">
			<LineChart class="mb-4 h-12 w-12 text-muted-foreground/50" />
			<p class="text-lg font-medium">No metric history yet</p>
			<p class="mt-1 text-sm text-muted-foreground">
				Metrics are recorded while the server is running. Check back after it has been up for a
				while.
			</p>
		</div>
	{:else}
		<!-- Charts -->
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
			{#each series as spec (spec.key)}
				{@const SpecIcon = spec.icon}
				<div class="rounded-lg border border-border/50 bg-muted/10 p-3">
					<div class="mb-2 flex items-center justify-between gap-2">
						<div class="flex items-center gap-2">
							<SpecIcon class="h-4 w-4" style="color: {spec.color}" />
							<span
								class="text-xs font-semibold tracking-wider text-muted-foreground uppercase"
								>{spec.title}</span
							>
						</div>
						<span class="font-mono text-sm font-bold" style="color: {spec.color}"
							>{spec.latest}</span
						>
					</div>
					{#if spec.values.length === 0}
						<div
							class="flex h-20 items-center justify-center text-xs text-muted-foreground"
						>
							No data in this range
						</div>
					{:else}
						<svg
							viewBox="0 0 {W} {H}"
							preserveAspectRatio="none"
							class="h-20 w-full overflow-visible"
							role="img"
							aria-label="{spec.title} history chart"
						>
							<defs>
								<linearGradient id="metric-grad-{spec.key}" x1="0" y1="0" x2="0" y2="1">
									<stop offset="0%" stop-color={spec.color} stop-opacity="0.3" />
									<stop offset="100%" stop-color={spec.color} stop-opacity="0.02" />
								</linearGradient>
							</defs>
							{#if spec.refLine !== undefined && spec.refLine > spec.min && spec.refLine < spec.max}
								{@const refY = yFor(spec.refLine, spec).toFixed(2)}
								<line
									x1="0"
									x2="{W}"
									y1={refY}
									y2={refY}
									class="text-muted-foreground/40"
									stroke="currentColor"
									stroke-width="1"
									stroke-dasharray="4 4"
									vector-effect="non-scaling-stroke"
								/>
							{/if}
							{#if areaPath(spec)}
								<path d={areaPath(spec)} fill="url(#metric-grad-{spec.key})" />
							{/if}
							{#if spec.values.length === 1}
								<circle
									cx={xFor(0, spec)}
									cy={yFor(spec.values[0], spec)}
									r="3"
									fill={spec.color}
									vector-effect="non-scaling-stroke"
								/>
							{:else}
								<path
									d={linePath(spec)}
									fill="none"
									stroke={spec.color}
									stroke-width="2"
									stroke-linejoin="round"
									stroke-linecap="round"
									vector-effect="non-scaling-stroke"
								/>
							{/if}
						</svg>
					{/if}
					<div class="mt-1.5 flex items-center justify-between text-[10px] text-muted-foreground/70">
						<span class="flex items-center gap-1">
							<Clock class="h-3 w-3" />
							{timeRangeLabel}
						</span>
						{#if spec.refLabel}
							<span class="italic">- - {spec.refLabel}</span>
						{:else}
							<span>max {spec.max % 1 === 0 ? spec.max : spec.max.toFixed(1)}</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
