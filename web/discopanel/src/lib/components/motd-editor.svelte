<script lang="ts">
	import { onMount } from 'svelte';
	import { create } from '@bufbuild/protobuf';
	import { toast } from 'svelte-sonner';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Badge } from '$lib/components/ui/badge';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		Loader2,
		Image as ImageIcon,
		Trash2,
		Upload,
		Save,
		TriangleAlert,
		Type
	} from '@lucide/svelte';
	import {
		GetServerMotdRequestSchema,
		UpdateServerMotdRequestSchema
	} from '$lib/proto/discopanel/v1/admin_pb';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { ServerStatus } from '$lib/proto/discopanel/v1/common_pb';
	import { rpcClient } from '$lib/api/rpc-client';

	let { server }: { server: Server } = $props();

	const ICON_MAX_BYTES = 64 * 1024; // Minecraft's server-icon.png must fit in 64KB

	// Editable fields
	let motdText = $state('');
	let maxPlayers = $state(20);
	let onlineMode = $state(true);
	let whitelistEnabled = $state(false);

	// Icon state: base64 PNG payload, '' means "remove icon"
	let iconBase64 = $state('');
	let iconPreview = $state(''); // data URL for the thumbnail ('' = no icon)
	let iconChanged = $state(false);

	// Loaded snapshot for dirty tracking
	let initial = $state({
		motd: '',
		maxPlayers: 20,
		onlineMode: true,
		whitelistEnabled: false,
		iconBase64: ''
	});

	let loading = $state(true);
	let saving = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);

	const isRunning = $derived(
		server.status === ServerStatus.RUNNING ||
			server.status === ServerStatus.STARTING ||
			server.status === ServerStatus.RESTARTING ||
			server.status === ServerStatus.UNHEALTHY
	);

	const motdDirty = $derived(motdText !== initial.motd);
	const maxPlayersDirty = $derived(maxPlayers !== initial.maxPlayers);
	const onlineModeDirty = $derived(onlineMode !== initial.onlineMode);
	const whitelistDirty = $derived(whitelistEnabled !== initial.whitelistEnabled);

	const hasChanges = $derived(
		motdDirty || maxPlayersDirty || onlineModeDirty || whitelistDirty || iconChanged
	);
	// Only the whitelist toggle is allowed while the server is running
	const canSave = $derived(hasChanges && (!isRunning || whitelistDirty));

	async function loadMotd() {
		loading = true;
		try {
			const response = await rpcClient.admin.getServerMotd(
				create(GetServerMotdRequestSchema, { serverId: server.id })
			);
			motdText = response.motd;
			maxPlayers = response.maxPlayers || 20;
			onlineMode = response.onlineMode;
			whitelistEnabled = response.whitelistEnabled;
			iconBase64 = response.icon;
			iconPreview = response.icon ? `data:image/png;base64,${response.icon}` : '';
			iconChanged = false;
			initial = {
				motd: response.motd,
				maxPlayers: response.maxPlayers || 20,
				onlineMode: response.onlineMode,
				whitelistEnabled: response.whitelistEnabled,
				iconBase64: response.icon
			};
		} catch (error) {
			console.error('Failed to load MOTD:', error);
		} finally {
			loading = false;
		}
	}

	function handleIconFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (!file) return;

		if (!file.name.toLowerCase().endsWith('.png')) {
			toast.error('The server icon must be a .png file');
			return;
		}
		if (file.size > ICON_MAX_BYTES) {
			toast.error(
				`Icon is too large (${Math.ceil(file.size / 1024)}KB). The server icon must be 64x64 pixels and under 64KB.`
			);
			return;
		}

		const reader = new FileReader();
		reader.onload = () => {
			const dataUrl = reader.result as string;
			const base64 = dataUrl.split(',')[1] ?? '';
			iconBase64 = base64;
			iconPreview = dataUrl;
			iconChanged = true;
			toast.info('Note: Minecraft expects a 64x64 pixel PNG for the server icon.');
		};
		reader.onerror = () => {
			toast.error('Failed to read the icon file');
		};
		reader.readAsDataURL(file);
	}

	function removeIcon() {
		iconBase64 = '';
		iconPreview = '';
		iconChanged = true;
	}

	async function save() {
		if (!canSave) return;
		saving = true;
		try {
			await rpcClient.admin.updateServerMotd(
				create(UpdateServerMotdRequestSchema, {
					serverId: server.id,
					...(motdDirty ? { motd: motdText } : {}),
					...(iconChanged ? { icon: iconBase64 } : {}),
					...(whitelistDirty ? { whitelistEnabled: whitelistEnabled } : {}),
					...(maxPlayersDirty ? { maxPlayers: maxPlayers } : {}),
					...(onlineModeDirty ? { onlineMode: onlineMode } : {})
				})
			);
			toast.success('Server branding saved');
			await loadMotd();
		} catch (error) {
			console.error('Failed to save MOTD:', error);
		} finally {
			saving = false;
		}
	}

	// ---------- MOTD preview (Minecraft color code parsing) ----------

	const MC_COLORS: Record<string, string> = {
		'0': '#000000',
		'1': '#0000AA',
		'2': '#00AA00',
		'3': '#00AAAA',
		'4': '#AA0000',
		'5': '#AA00AA',
		'6': '#FFAA00',
		'7': '#AAAAAA',
		'8': '#555555',
		'9': '#5555FF',
		a: '#55FF55',
		b: '#55FFFF',
		c: '#FF5555',
		d: '#FF55FF',
		e: '#FFFF55',
		f: '#FFFFFF'
	};

	interface MotdSegment {
		text: string;
		color: string;
		bold: boolean;
		italic: boolean;
		underline: boolean;
		strike: boolean;
		obfuscated: boolean;
	}

	function normalizeMotd(raw: string): string {
		// server.properties stores escaped \u00A7 sequences — render them as §
		return raw.replace(/\\u00[aA]7/g, '§').replace(/\\n/g, '\n');
	}

	function parseMotdLine(line: string): MotdSegment[] {
		const segments: MotdSegment[] = [];
		let color = '#FFFFFF';
		let bold = false;
		let italic = false;
		let underline = false;
		let strike = false;
		let obfuscated = false;
		let buffer = '';

		const push = () => {
			if (buffer.length > 0) {
				segments.push({ text: buffer, color, bold, italic, underline, strike, obfuscated });
				buffer = '';
			}
		};

		for (let i = 0; i < line.length; i++) {
			const ch = line[i];
			if (ch === '§' && i + 1 < line.length) {
				const code = line[i + 1].toLowerCase();
				push();
				if (code === 'r') {
					color = '#FFFFFF';
					bold = italic = underline = strike = obfuscated = false;
				} else if (code === 'l') {
					bold = true;
				} else if (code === 'o') {
					italic = true;
				} else if (code === 'n') {
					underline = true;
				} else if (code === 'm') {
					strike = true;
				} else if (code === 'k') {
					obfuscated = true;
				} else if (MC_COLORS[code]) {
					color = MC_COLORS[code];
					bold = italic = underline = strike = obfuscated = false;
				}
				i++;
			} else {
				buffer += ch;
			}
		}
		push();
		return segments;
	}

	const previewLines = $derived(
		normalizeMotd(motdText)
			.split('\n')
			.map((line) => parseMotdLine(line))
	);

	onMount(() => {
		loadMotd();
	});
</script>

<div class="space-y-6">
	<!-- Live preview -->
	<Card>
		<CardHeader>
			<CardTitle class="flex items-center gap-2">
				<Type class="h-5 w-5 text-primary" />
				Live Preview
			</CardTitle>
			<CardDescription>How the MOTD will appear in the Minecraft server list</CardDescription>
		</CardHeader>
		<CardContent>
			<div class="flex items-center gap-3 rounded-lg border border-border/50 bg-[#1d1d1d] p-4">
				<div
					class="flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded border border-white/10 bg-[#2a2a2a]"
				>
					{#if iconPreview}
						<img src={iconPreview} alt="Server icon" class="h-full w-full object-cover" />
					{:else}
						<ImageIcon class="h-6 w-6 text-white/30" />
					{/if}
				</div>
				<div class="min-w-0 flex-1 font-mono text-sm leading-relaxed">
					{#if loading}
						<Skeleton class="h-4 w-3/4 bg-white/10" />
					{:else if previewLines.length === 0 || previewLines.every((l) => l.length === 0)}
						<span class="text-white/40">A Minecraft Server</span>
					{:else}
						{#each previewLines as line, i (i)}
							<div class="truncate">
								{#each line as segment, j (j)}
									<span
										style="color: {segment.color}; font-weight: {segment.bold
											? 'bold'
											: 'normal'}; font-style: {segment.italic
											? 'italic'
											: 'normal'}; text-decoration: {[
											segment.underline ? 'underline' : '',
											segment.strike ? 'line-through' : ''
										]
											.filter(Boolean)
											.join(' ') || 'none'};"
										class={segment.obfuscated ? 'opacity-80 blur-[0.5px]' : ''}
									>
										{segment.text}
									</span>
								{/each}
								{#if line.length === 0}
									&nbsp;
								{/if}
							</div>
						{/each}
					{/if}
				</div>
			</div>
			<p class="mt-2 text-xs text-muted-foreground">
				Minecraft color codes like <code class="rounded bg-muted px-1 font-mono">§a</code> (green),
				<code class="rounded bg-muted px-1 font-mono">§l</code> (bold) and
				<code class="rounded bg-muted px-1 font-mono">§r</code> (reset) are supported. Use
				<code class="rounded bg-muted px-1 font-mono">&amp;</code> is not supported — use the section
				sign character directly.
			</p>
		</CardContent>
	</Card>

	<!-- Running warning banner -->
	{#if isRunning}
		<div
			class="flex items-start gap-3 rounded-xl border border-yellow-500/30 bg-yellow-500/10 p-4 text-sm text-yellow-700 dark:text-yellow-400"
		>
			<TriangleAlert class="mt-0.5 h-5 w-5 shrink-0" />
			<p>
				MOTD/icon/max-players/online-mode require the server to be stopped; whitelist can be toggled
				live.
			</p>
		</div>
	{/if}

	<!-- Editor -->
	<Card>
		<CardHeader class="flex flex-row flex-wrap items-center justify-between gap-3 space-y-0">
			<div class="space-y-1">
				<CardTitle>Server Branding</CardTitle>
				<CardDescription>MOTD, server icon and core server.properties settings</CardDescription>
			</div>
			<Button onclick={save} disabled={!canSave || saving}>
				{#if saving}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<Save class="mr-2 h-4 w-4" />
				{/if}
				Save
			</Button>
		</CardHeader>
		<CardContent class="space-y-6">
			{#if loading}
				<div class="space-y-3">
					<Skeleton class="h-24 w-full" />
					<Skeleton class="h-10 w-64" />
				</div>
			{:else}
				<!-- MOTD -->
				<div class="space-y-2">
					<Label for="motd-input">Message of the Day</Label>
					<Textarea
						id="motd-input"
						bind:value={motdText}
						disabled={isRunning}
						rows={3}
						placeholder="A Minecraft Server"
						class="font-mono"
					/>
					<p class="text-xs text-muted-foreground">
						Raw server.properties value. Newlines create a second MOTD line.
					</p>
				</div>

				<!-- Icon -->
				<div class="space-y-2">
					<Label>Server Icon</Label>
					<div class="flex items-center gap-4">
						<div
							class="flex h-16 w-16 items-center justify-center overflow-hidden rounded-lg border border-border/50 bg-muted/30"
						>
							{#if iconPreview}
								<img src={iconPreview} alt="Server icon" class="h-full w-full object-cover" />
							{:else}
								<ImageIcon class="h-6 w-6 text-muted-foreground/50" />
							{/if}
						</div>
						<div class="flex flex-wrap gap-2">
							<Button
								variant="outline"
								class="border-2"
								disabled={isRunning}
								onclick={() => fileInput?.click()}
							>
								<Upload class="mr-2 h-4 w-4" />
								Upload PNG
							</Button>
							{#if iconPreview}
								<Button
									variant="outline"
									class="border-2"
									disabled={isRunning}
									onclick={removeIcon}
								>
									<Trash2 class="mr-2 h-4 w-4" />
									Remove
								</Button>
							{/if}
						</div>
						<input
							type="file"
							accept=".png,image/png"
							class="hidden"
							bind:this={fileInput}
							onchange={handleIconFile}
						/>
					</div>
					<p class="text-xs text-muted-foreground">
						PNG only, 64x64 pixels and under 64KB (Minecraft's limit). Uploading a different size
						will still be accepted but may not display correctly.
					</p>
				</div>

				<!-- Properties -->
				<div class="grid gap-6 sm:grid-cols-2">
					<div class="space-y-2">
						<Label for="max-players-input">Max Players</Label>
						<Input
							id="max-players-input"
							type="number"
							min="1"
							max="2147483647"
							bind:value={maxPlayers}
							disabled={isRunning}
							class="max-w-40"
						/>
					</div>
					<div class="space-y-4">
						<div class="flex items-center justify-between gap-4">
							<div class="space-y-0.5">
								<Label>Online Mode</Label>
								<p class="text-xs text-muted-foreground">
									Require premium accounts (needs restart)
								</p>
							</div>
							<Switch bind:checked={onlineMode} disabled={isRunning} />
						</div>
						<div class="flex items-center justify-between gap-4">
							<div class="space-y-0.5">
								<Label>Whitelist</Label>
								<p class="text-xs text-muted-foreground">
									Can be toggled while the server is running
								</p>
							</div>
							<Switch bind:checked={whitelistEnabled} />
						</div>
					</div>
				</div>

				<!-- Dirty summary -->
				{#if hasChanges}
					<div class="flex flex-wrap items-center gap-2">
						<span class="text-xs text-muted-foreground">Unsaved changes:</span>
						{#if motdDirty}<Badge variant="secondary" class="text-xs">MOTD</Badge>{/if}
						{#if iconChanged}<Badge variant="secondary" class="text-xs">Icon</Badge>{/if}
						{#if maxPlayersDirty}<Badge variant="secondary" class="text-xs">Max players</Badge>{/if}
						{#if onlineModeDirty}<Badge variant="secondary" class="text-xs">Online mode</Badge>{/if}
						{#if whitelistDirty}<Badge variant="secondary" class="text-xs">Whitelist</Badge>{/if}
					</div>
				{/if}
			{/if}
		</CardContent>
	</Card>
</div>
