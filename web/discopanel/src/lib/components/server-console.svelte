<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import { rpcClient } from '$lib/api/rpc-client';
	import { create } from '@bufbuild/protobuf';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { ServerStatus } from '$lib/proto/discopanel/v1/common_pb';
	import type { LogEntry } from '$lib/proto/discopanel/v1/server_pb';
	import {
		GetServerLogsRequestSchema,
		ClearServerLogsRequestSchema,
		SendCommandRequestSchema,
		UploadToMCLogsRequestSchema
	} from '$lib/proto/discopanel/v1/server_pb';
	import { ResizablePaneGroup, ResizablePane, ResizableHandle } from '$lib/components/ui/resizable';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { toast } from 'svelte-sonner';
	import {
		Terminal,
		Send,
		Loader2,
		Download,
		Upload,
		Trash2,
		RefreshCw,
		Wifi,
		WifiOff
	} from '@lucide/svelte';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import AnsiToHtml from 'ansi-to-html';
	import { getStringForEnum } from '$lib/utils';
	import { wsClient } from '$lib/stores/websocket.svelte';

	// Create ansi-to-html converter with proper options
	const ansiConverter = new AnsiToHtml({
		fg: '#e8e8e8',
		bg: '#000000',
		newline: false,
		escapeXML: true,
		stream: true
	});

	let { server, active = false }: { server: Server; active?: boolean } = $props();

	interface ProcessedLogEntry {
		id: string;
		level: string;
		html: string;
		message: string;
	}

	let logEntries = $state<ProcessedLogEntry[]>([]);
	let command = $state('');
	let loading = $state(false);
	let autoScroll = $state(true);
	let scrollAreaRef = $state<HTMLDivElement | null>(null);
	let tailLines = $state(500);
	const MAX_LOG_ENTRIES = 5000;

	let appendSeq = 0;
	function processLog(entry: LogEntry, idx?: number): ProcessedLogEntry {
		const s = entry.timestamp?.seconds ? Number(entry.timestamp.seconds) : 0;
		const n = entry.timestamp?.nanos ?? 0;
		const lvl = entry.level || 'info';
		const seq = idx !== undefined ? idx : ++appendSeq;
		return {
			id: `${s}_${n}_${lvl}_${seq}`,
			level: lvl,
			html: ansiConverter.toHtml(entry.message || ''),
			message: entry.message || ''
		};
	}

	// Ws state
	let wsConnectionState = $derived(wsClient.state.connectionState);

	// Cleanup functions for handlers
	let cleanupHandlers: (() => void)[] = [];

	// Track previous server ID
	let previousServerId = server.id;

	onDestroy(() => {
		untrack(() => cleanupWebSocket());
	});

	// Start/stop polling based on active prop
	$effect(() => {
		if (active) {
			untrack(() => setupWebSocket());
		} else {
			untrack(() => cleanupWebSocket());
		}
	});

	// Handle server changes
	$effect(() => {
		const currentServerId = server.id;
		if (currentServerId !== previousServerId) {
			const oldServerId = previousServerId;
			previousServerId = currentServerId;

			untrack(() => {
				// Unsubscribe from old server
				wsClient.unsubscribe(oldServerId);
				logEntries = [];
				command = '';

				// Subscribe to new server
				if (active) {
					wsClient.subscribe(currentServerId, tailLines);
				}
			});
		}
	});

	function setupWebSocket() {
		// Clean up any existing handlers
		cleanupWebSocket();

		// Connect WebSocket
		wsClient.connect();

		// Register handlers
		const unsubLogs = wsClient.onLogs((serverId, logs) => {
			if (serverId === server.id) {
				const processed = logs.map((l, i) => processLog(l, i));
				logEntries = processed.length > MAX_LOG_ENTRIES ? processed.slice(-MAX_LOG_ENTRIES) : processed;
			}
		});

		const unsubLogEntry = wsClient.onLogEntry((serverId, logs) => {
			if (serverId === server.id && logs.length > 0) {
				const newItems = logs.map((l) => processLog(l));
				const combined = [...logEntries, ...newItems];
				logEntries =
					combined.length > MAX_LOG_ENTRIES ? combined.slice(-MAX_LOG_ENTRIES) : combined;
			}
		});

		const unsubCommandResult = wsClient.onCommandResult((result) => {
			if (result.serverId === server.id) {
				loading = false;
				if (result.success) {
					toast.success('Command sent successfully');
				} else {
					toast.error(result.error || 'Failed to execute command');
				}
			}
		});

		cleanupHandlers = [unsubLogs, unsubLogEntry, unsubCommandResult];

		// Subscribe to server logs
		wsClient.subscribe(server.id, tailLines);
	}

	function cleanupWebSocket() {
		// Unsubscribe from current server
		wsClient.unsubscribe(server.id);

		// Clean up handlers
		cleanupHandlers.forEach((cleanup) => cleanup());
		cleanupHandlers = [];
	}

	// Handle auto-scrolling
	$effect(() => {
		if (logEntries.length > 0 && autoScroll && scrollAreaRef) {
			// Use a microtask to ensure DOM has updated
			queueMicrotask(() => {
				if (scrollAreaRef) {
					scrollAreaRef.scrollTop = scrollAreaRef.scrollHeight;
				}
			});
		}
	});

	function handleScroll() {
		if (!scrollAreaRef) return;

		const { scrollTop, scrollHeight, clientHeight } = scrollAreaRef;
		const isAtBottom = scrollHeight - scrollTop - clientHeight < 5;

		if (isAtBottom && !autoScroll) {
			autoScroll = true;
		} else if (!isAtBottom && autoScroll) {
			autoScroll = false;
		}
	}

	async function fetchLogs() {
		if (loading) return;

		// Don't try to fetch logs if server is not running
		if (server.status === ServerStatus.STOPPED) {
			return;
		}

		loading = true;
		try {
			const request = create(GetServerLogsRequestSchema, {
				id: server.id,
				tail: tailLines
			});
			const response = await rpcClient.server.getServerLogs(request);
			const logs = response.logs || [];
			const processed = logs.map((l, i) => processLog(l, i));
			logEntries = processed.length > MAX_LOG_ENTRIES ? processed.slice(-MAX_LOG_ENTRIES) : processed;
		} catch (error) {
			console.error('Failed to fetch logs:', error);
		} finally {
			loading = false;
		}
	}

	async function sendCommand() {
		if (!command.trim()) return;

		loading = true;
		const cmdToSend = command;
		command = '';

		// Use WebSocket if connected, otherwise fallback to RPC
		if (wsClient.isReady) {
			wsClient.sendCommand(server.id, cmdToSend);
		} else {
			try {
				const request = create(SendCommandRequestSchema, {
					id: server.id,
					command: cmdToSend
				});
				const response = await rpcClient.server.sendCommand(request);
				if (!response.success) {
					toast.error(response.error || 'Failed to execute command');
				}
			} catch (error) {
				console.error(
					'Failed to send command: ' + (error instanceof Error ? error.message : 'Unknown error')
				);
			} finally {
				loading = false;
			}
		}
	}

	async function clearLogs() {
		const request = create(ClearServerLogsRequestSchema, {
			id: server.id
		});
		await rpcClient.server.clearServerLogs(request);
		logEntries = [];
		toast.success('Console cleared');
	}

	let uploading = $state(false);

	async function uploadToMCLogs() {
		if (uploading) return;
		uploading = true;
		try {
			const request = create(UploadToMCLogsRequestSchema, { id: server.id });
			const response = await rpcClient.server.uploadToMCLogs(request);
			await navigator.clipboard.writeText(response.url);
			toast.success('mclo.gs URL copied to clipboard');
		} catch (error) {
			toast.error(
				'Failed to upload to mclo.gs: ' + (error instanceof Error ? error.message : 'Unknown error')
			);
		} finally {
			uploading = false;
		}
	}

	function downloadLogs() {
		const logText = logEntries.map((entry) => entry.message).join('\n');
		const blob = new Blob([logText], { type: 'text/plain' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `${server.name}-logs-${new Date().toISOString()}.txt`;
		document.body.appendChild(a);
		a.click();
		document.body.removeChild(a);
		URL.revokeObjectURL(url);
		toast.success('Logs downloaded');
	}

	function handleTailChange() {
		// Re-subscribe with new tail count
		if (wsClient.isReady) {
			wsClient.unsubscribe(server.id);
			wsClient.subscribe(server.id, tailLines);
		} else {
			fetchLogs();
		}
	}

	function getConnectionColor() {
		switch (wsConnectionState) {
			case 'authenticated':
				return 'text-green-500';
			case 'connected':
				return 'text-yellow-500';
			case 'connecting':
				return 'text-yellow-500';
			default:
				return 'text-zinc-500';
		}
	}
</script>

<ResizablePaneGroup
	direction="vertical"
	class="h-full max-h-[800px] min-h-[320px] sm:min-h-[400px] w-full overflow-hidden rounded-lg border bg-black"
>
	<ResizablePane defaultSize={75} minSize={30}>
		<div class="flex h-full flex-col">
			<div class="flex items-center justify-between border-b border-zinc-800 bg-zinc-900 px-3 sm:px-4 py-2">
				<div class="flex items-center gap-2">
					<Terminal class="h-4 w-4 text-green-500 shrink-0" />
					<span class="font-mono text-xs sm:text-sm text-green-500 font-semibold truncate">Console</span>
					<Badge
						variant={server.status === ServerStatus.RUNNING ||
						server.status === ServerStatus.UNHEALTHY
							? 'default'
							: 'secondary'}
						class="text-[10px] sm:text-xs px-1.5 py-0"
					>
						{getStringForEnum(ServerStatus, server.status)?.toLowerCase()}
					</Badge>
					{#if wsConnectionState === 'authenticated'}
						<Wifi class="h-3 w-3 shrink-0 {getConnectionColor()}" />
					{:else}
						<WifiOff class="h-3 w-3 shrink-0 {getConnectionColor()}" />
					{/if}
				</div>
				<div class="flex items-center gap-1">
					<Tooltip.Root>
						<Tooltip.Trigger>
							<Button
								size="sm"
								variant="ghost"
								onclick={fetchLogs}
								disabled={loading}
								class="h-8 w-8 sm:h-7 sm:w-7 p-0 text-zinc-400 hover:text-white"
							>
								{#if loading}
									<Loader2 class="h-3.5 w-3.5 animate-spin" />
								{:else}
									<RefreshCw class="h-3.5 w-3.5" />
								{/if}
							</Button>
						</Tooltip.Trigger>
						<Tooltip.Content>Refresh logs</Tooltip.Content>
					</Tooltip.Root>
					<Tooltip.Root>
						<Tooltip.Trigger>
							<Button
								size="sm"
								variant="ghost"
								onclick={uploadToMCLogs}
								disabled={uploading}
								class="h-8 w-8 sm:h-7 sm:w-7 p-0 text-zinc-400 hover:text-white"
							>
								{#if uploading}
									<Loader2 class="h-3.5 w-3.5 animate-spin" />
								{:else}
									<Upload class="h-3.5 w-3.5" />
								{/if}
							</Button>
						</Tooltip.Trigger>
						<Tooltip.Content>Upload to mclo.gs</Tooltip.Content>
					</Tooltip.Root>
					<Tooltip.Root>
						<Tooltip.Trigger>
							<Button
								size="sm"
								variant="ghost"
								onclick={downloadLogs}
								disabled={logEntries.length === 0}
								class="h-8 w-8 sm:h-7 sm:w-7 p-0 text-zinc-400 hover:text-white"
							>
								<Download class="h-3.5 w-3.5" />
							</Button>
						</Tooltip.Trigger>
						<Tooltip.Content>Download logs</Tooltip.Content>
					</Tooltip.Root>
					<Tooltip.Root>
						<Tooltip.Trigger>
							<Button
								size="sm"
								variant="ghost"
								onclick={clearLogs}
								disabled={logEntries.length === 0}
								class="h-8 w-8 sm:h-7 sm:w-7 p-0 text-zinc-400 hover:text-white"
							>
								<Trash2 class="h-3.5 w-3.5" />
							</Button>
						</Tooltip.Trigger>
						<Tooltip.Content>Clear console</Tooltip.Content>
					</Tooltip.Root>
				</div>
			</div>
			<div
				class="custom-scrollbar min-h-0 flex-1 overflow-x-auto overflow-y-auto bg-black px-4 py-2 [overscroll-behavior-y:contain]"
				bind:this={scrollAreaRef}
				onscroll={handleScroll}
			>
				<div class="font-mono text-xs text-zinc-300">
					{#if logEntries.length === 0}
						<div class="py-8 text-center text-zinc-500">
							No logs available. {[
								ServerStatus.RUNNING,
								ServerStatus.STARTING,
								ServerStatus.UNHEALTHY
							].includes(server.status)
								? 'Try refreshing the page.'
								: 'Start the server to see output.'}
						</div>
					{:else}
						{#each logEntries as entry (entry.id)}
							<div class="log-line break-all whitespace-pre-wrap select-text" data-type={entry.level}>
								<!-- eslint-disable-next-line svelte/no-at-html-tags -->
								{@html entry.html}
							</div>
						{/each}
					{/if}
				</div>
			</div>
		</div>
	</ResizablePane>

	<ResizableHandle class="bg-zinc-800 hover:bg-zinc-700" />

	<div class="flex flex-col bg-zinc-950 shrink-0">
		<!-- Quick Command Chips for Mobile -->
		<div class="flex items-center gap-1.5 px-3 pt-2 pb-1 overflow-x-auto scrollbar-none border-t border-zinc-900">
			{#each ['/list', '/tps', '/help', '/save-all', 'Clear'] as quickCmd}
				<Button
					variant="outline"
					size="sm"
					class="h-6.5 px-2 text-[11px] bg-zinc-900 border-zinc-700 text-zinc-300 hover:text-white hover:bg-zinc-800 shrink-0"
					onclick={() => {
						if (quickCmd === 'Clear') {
							clearLogs();
						} else {
							command = quickCmd;
							sendCommand();
						}
					}}
					disabled={server.status !== ServerStatus.RUNNING && server.status !== ServerStatus.UNHEALTHY}
				>
					{quickCmd}
				</Button>
			{/each}
		</div>

		<div class="flex shrink-0 gap-2 p-3 pt-2">
			<div class="flex flex-1 items-center gap-2 rounded-md bg-zinc-900/60 px-2.5 py-1 border border-zinc-800 focus-within:border-primary/60">
				<span class="font-mono text-sm text-green-500 select-none">$</span>
				<input
					type="text"
					placeholder={server.status === ServerStatus.RUNNING ||
					server.status === ServerStatus.UNHEALTHY
						? 'Enter command...'
						: 'Server must be running'}
					bind:value={command}
					disabled={server.status !== ServerStatus.RUNNING &&
						server.status !== ServerStatus.UNHEALTHY}
					onkeydown={(e) => e.key === 'Enter' && sendCommand()}
					autocapitalize="none"
					autocorrect="off"
					autocomplete="off"
					spellcheck="false"
					enterkeyhint="send"
					class="flex-1 bg-transparent font-mono text-sm text-white outline-none placeholder:text-zinc-600"
				/>
			</div>
			<Button
				onclick={sendCommand}
				disabled={server.status === ServerStatus.STOPPED || !command.trim()}
				size="sm"
				class="h-9 w-9 p-0 bg-zinc-800 text-white hover:bg-zinc-700 shrink-0"
			>
				<Send class="h-3.5 w-3.5" />
			</Button>
		</div>

		<div class="flex shrink-0 items-center justify-between px-3 pb-2 text-xs text-zinc-500">
			<div class="flex items-center gap-4">
				<label class="flex items-center gap-2 cursor-pointer py-0.5">
					<input type="checkbox" bind:checked={autoScroll} class="h-3.5 w-3.5 rounded" />
					<span>Auto-scroll</span>
				</label>
				<div class="flex items-center gap-1.5">
					<span>Tail:</span>
					<select
						bind:value={tailLines}
						onchange={handleTailChange}
						class="rounded border border-zinc-800 bg-zinc-900 px-2 py-0.5 text-xs text-zinc-300"
					>
						<option value={100}>100</option>
						<option value={500}>500</option>
						<option value={1000}>1000</option>
						<option value={2000}>2000</option>
					</select>
				</div>
			</div>
			<div class="font-mono text-[11px] text-zinc-400">
				{logEntries.length} lines
			</div>
		</div>
	</div>
</ResizablePaneGroup>

<style>
	.custom-scrollbar {
		scrollbar-width: thin;
		scrollbar-color: hsl(var(--muted-foreground) / 0.3) transparent;
	}

	.custom-scrollbar::-webkit-scrollbar {
		width: 12px;
	}

	.custom-scrollbar::-webkit-scrollbar-track {
		background: transparent;
	}

	.custom-scrollbar::-webkit-scrollbar-thumb {
		background-color: hsl(var(--muted-foreground) / 0.3);
		border-radius: 6px;
		border: 3px solid transparent;
		background-clip: content-box;
	}

	.custom-scrollbar::-webkit-scrollbar-thumb:hover {
		background-color: hsl(var(--muted-foreground) / 0.5);
	}

	.log-line {
		padding: 0.125rem 0;
		line-height: 1.4;
	}

	.log-line:hover {
		background-color: rgba(39, 39, 42, 0.5);
	}

	/* Visually distinguish command inputs */
	.log-line[data-type='command'] {
		color: #4ade80;
		font-weight: 500;
	}

	.log-line[data-type='command']::before {
		content: '$ ';
		color: #22c55e;
		font-weight: bold;
	}

	/* Style command output differently */
	.log-line[data-type='command_output'] {
		opacity: 0.9;
		padding-left: 1rem;
	}
</style>
