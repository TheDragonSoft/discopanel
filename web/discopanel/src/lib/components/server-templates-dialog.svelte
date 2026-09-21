<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Badge } from '$lib/components/ui/badge';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Separator } from '$lib/components/ui/separator';
	import {
		Dialog,
		DialogContent,
		DialogDescription,
		DialogFooter,
		DialogHeader,
		DialogTitle
	} from '$lib/components/ui/dialog';
	import { rpcClient } from '$lib/api/rpc-client';
	import { create } from '@bufbuild/protobuf';
	import { toast } from 'svelte-sonner';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		Loader2,
		PackagePlus,
		Pencil,
		Rocket,
		Trash2,
		Blocks,
		HardDrive,
		RefreshCcw
	} from '@lucide/svelte';
	import { serversStore } from '$lib/stores/servers';
	import { ModLoader } from '$lib/proto/discopanel/v1/common_pb';
	import type { ServerTemplate } from '$lib/proto/discopanel/v1/template_pb';
	import {
		ListServerTemplatesRequestSchema,
		DeleteServerTemplateRequestSchema,
		UpdateServerTemplateRequestSchema,
		DeployServerTemplateRequestSchema
	} from '$lib/proto/discopanel/v1/template_pb';
	import { formatBytes } from '$lib/utils';

	interface Props {
		open?: boolean;
	}

	let { open = $bindable(false) }: Props = $props();

	let templates = $state<ServerTemplate[]>([]);
	let loading = $state(false);

	// Proxy availability for the deploy dialog
	let proxyEnabled = $state(false);

	// Edit state
	let editOpen = $state(false);
	let editing = $state<ServerTemplate | null>(null);
	let editName = $state('');
	let editDescription = $state('');
	let savingEdit = $state(false);

	// Deploy state
	let deployOpen = $state(false);
	let deploying = $state<ServerTemplate | null>(null);
	let deployName = $state('');
	let deployProxyHostname = $state('');
	let deployAutoStart = $state(false);
	let deployingBusy = $state(false);

	// Load templates + proxy status whenever the dialog opens
	$effect(() => {
		if (open) {
			loadTemplates();
			loadProxyStatus();
		}
	});

	async function loadTemplates() {
		loading = true;
		try {
			const response = await rpcClient.template.listServerTemplates(
				create(ListServerTemplatesRequestSchema, {})
			);
			templates = response.templates;
		} catch (error) {
			toast.error(
				`Failed to load templates: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			loading = false;
		}
	}

	async function loadProxyStatus() {
		try {
			const status = await rpcClient.proxy.getProxyStatus({});
			proxyEnabled = status.enabled;
		} catch {
			proxyEnabled = false;
		}
	}

	function getModLoaderDisplay(modLoader: ModLoader): string {
		if (modLoader === ModLoader.UNSPECIFIED || modLoader === ModLoader.VANILLA) {
			return 'vanilla';
		}
		return ModLoader[modLoader].replace('_', ' ').toLowerCase();
	}

	function getUpdatedDisplay(template: ServerTemplate): string {
		if (!template.updatedAt) return '—';
		const date = new Date(Number(template.updatedAt.seconds) * 1000);
		const diff = Date.now() - date.getTime();
		const minutes = Math.floor(diff / (1000 * 60));
		if (minutes < 1) return 'just now';
		if (minutes < 60) return `${minutes}m ago`;
		const hours = Math.floor(minutes / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		if (days === 1) return 'yesterday';
		if (days < 7) return `${days}d ago`;
		return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
	}

	function openEdit(template: ServerTemplate) {
		editing = template;
		editName = template.name;
		editDescription = template.description || '';
		editOpen = true;
	}

	async function handleEditSave() {
		if (!editing) return;
		if (!editName.trim()) {
			toast.error('Template name is required');
			return;
		}

		savingEdit = true;
		try {
			const response = await rpcClient.template.updateServerTemplate(
				create(UpdateServerTemplateRequestSchema, {
					id: editing.id,
					name: editName.trim(),
					description: editDescription.trim()
				})
			);
			if (response.template) {
				templates = templates.map((t) => (t.id === response.template!.id ? response.template! : t));
			} else {
				await loadTemplates();
			}
			toast.success('Template updated');
			editOpen = false;
		} catch (error) {
			toast.error(
				`Failed to update template: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			savingEdit = false;
		}
	}

	async function handleDelete(template: ServerTemplate) {
		const modsWarning = template.hasMods
			? `\n\nThis template has captured mods (${formatBytes(Number(template.modsSizeBytes))}) which will be permanently removed.`
			: '';
		if (
			!confirm(`Are you sure you want to delete the template "${template.name}"?${modsWarning}`)
		) {
			return;
		}

		try {
			await rpcClient.template.deleteServerTemplate(
				create(DeleteServerTemplateRequestSchema, { id: template.id })
			);
			templates = templates.filter((t) => t.id !== template.id);
			toast.success(`Deleted template "${template.name}"`);
		} catch (error) {
			toast.error(
				`Failed to delete template: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		}
	}

	function openDeploy(template: ServerTemplate) {
		deploying = template;
		deployName = template.name;
		deployProxyHostname = '';
		deployAutoStart = false;
		deployOpen = true;
	}

	async function handleDeploy() {
		if (!deploying) return;
		if (!deployName.trim()) {
			toast.error('Server name is required');
			return;
		}

		deployingBusy = true;
		try {
			const response = await rpcClient.template.deployServerTemplate(
				create(DeployServerTemplateRequestSchema, {
					id: deploying.id,
					name: deployName.trim(),
					proxyHostname: proxyEnabled ? deployProxyHostname.trim() : '',
					autoStart: deployAutoStart
				})
			);
			toast.success(`Deploying "${deployName.trim()}" from template "${deploying.name}"...`);
			deployOpen = false;
			open = false;
			if (response.server) {
				serversStore.addServer(response.server);
				goto(resolve(`/servers/${response.server.id}`));
			}
		} catch (error) {
			toast.error(
				`Failed to deploy template: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			deployingBusy = false;
		}
	}
</script>

<Dialog bind:open>
	<DialogContent class="max-h-[85vh] overflow-hidden sm:max-w-2xl">
		<DialogHeader>
			<DialogTitle class="flex items-center gap-2">
				<Blocks class="h-5 w-5 text-primary" />
				Server Templates
			</DialogTitle>
			<DialogDescription class="text-sm text-muted-foreground">
				Reusable server configurations captured from your servers. Deploy one to spin up a new
				server with the same settings.
			</DialogDescription>
		</DialogHeader>

		<div class="max-h-[55vh] space-y-3 overflow-y-auto pr-1">
			{#if loading}
				<div class="flex h-40 items-center justify-center">
					<Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
				</div>
			{:else if templates.length === 0}
				<div class="flex h-40 flex-col items-center justify-center gap-2 text-center">
					<Blocks class="h-8 w-8 text-muted-foreground/50" />
					<p class="text-sm font-medium">No templates yet</p>
					<p class="text-xs text-muted-foreground">
						Use "Save as Template" on a server to capture its configuration here.
					</p>
				</div>
			{:else}
				{#each templates as template (template.id)}
					<div class="rounded-lg border border-border/60 bg-muted/20 p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1 space-y-1.5">
								<div class="flex items-center gap-2">
									<p class="truncate text-sm font-semibold">{template.name}</p>
									{#if template.hasMods}
										<Badge
											variant="outline"
											class="shrink-0 border-blue-500/30 bg-blue-500/10 text-[10px] text-blue-500"
										>
											<PackagePlus class="mr-1 h-2.5 w-2.5" />
											Mods {formatBytes(Number(template.modsSizeBytes))}
										</Badge>
									{/if}
								</div>
								{#if template.description}
									<p class="line-clamp-2 text-xs text-muted-foreground">{template.description}</p>
								{/if}
								<div class="flex flex-wrap items-center gap-1.5">
									<Badge variant="outline" class="text-[10px] capitalize"
										>{getModLoaderDisplay(template.modLoader)}</Badge
									>
									<Badge variant="outline" class="text-[10px]">{template.mcVersion}</Badge>
									<Badge variant="outline" class="text-[10px]">
										<HardDrive class="mr-1 h-2.5 w-2.5" />
										{(template.memory / 1024).toFixed(1)} GB
									</Badge>
								</div>
								<p class="text-[11px] text-muted-foreground/70">
									{#if template.sourceServerName}
										From "{template.sourceServerName}" •
									{/if}
									Updated {getUpdatedDisplay(template)}
								</p>
							</div>
							<div class="flex shrink-0 items-center gap-1">
								<Button size="sm" onclick={() => openDeploy(template)} disabled={deployingBusy}>
									<Rocket class="mr-1.5 h-3.5 w-3.5" />
									Deploy
								</Button>
								<Button
									variant="ghost"
									size="icon"
									title="Edit"
									class="h-8 w-8"
									onclick={() => openEdit(template)}
								>
									<Pencil class="h-3.5 w-3.5" />
								</Button>
								<Button
									variant="ghost"
									size="icon"
									title="Delete"
									class="h-8 w-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
									onclick={() => handleDelete(template)}
								>
									<Trash2 class="h-3.5 w-3.5" />
								</Button>
							</div>
						</div>
					</div>
				{/each}
			{/if}
		</div>

		<DialogFooter>
			<Button variant="outline" size="sm" onclick={() => loadTemplates()} disabled={loading}>
				<RefreshCcw class="mr-2 h-3.5 w-3.5" />
				Refresh
			</Button>
			<Button variant="outline" onclick={() => (open = false)}>Close</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>

<!-- Edit template metadata -->
<Dialog bind:open={editOpen}>
	<DialogContent class="sm:max-w-md">
		<DialogHeader>
			<DialogTitle>Edit Template</DialogTitle>
			<DialogDescription class="text-sm text-muted-foreground">
				Update the template's name and description.
			</DialogDescription>
		</DialogHeader>

		<div class="space-y-4 py-2">
			<div class="space-y-2">
				<Label for="edit_template_name" class="text-sm font-medium">Template Name</Label>
				<Input id="edit_template_name" bind:value={editName} class="h-10" />
			</div>
			<div class="space-y-2">
				<Label for="edit_template_description" class="text-sm font-medium">
					Description
					<span class="text-xs text-muted-foreground">(Optional)</span>
				</Label>
				<Input id="edit_template_description" bind:value={editDescription} class="h-10" />
			</div>
		</div>

		<DialogFooter>
			<Button variant="outline" onclick={() => (editOpen = false)} disabled={savingEdit}>
				Cancel
			</Button>
			<Button onclick={handleEditSave} disabled={savingEdit || !editName.trim()}>
				{#if savingEdit}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<Pencil class="mr-2 h-4 w-4" />
				{/if}
				Save Changes
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>

<!-- Deploy template -->
<Dialog bind:open={deployOpen}>
	<DialogContent class="sm:max-w-md">
		<DialogHeader>
			<DialogTitle class="flex items-center gap-2">
				<Rocket class="h-5 w-5 text-primary" />
				Deploy from Template
			</DialogTitle>
			<DialogDescription class="text-sm text-muted-foreground">
				Creates a new server from "{deploying?.name}"
				{#if deploying?.hasMods}
					(with mods {deploying ? formatBytes(Number(deploying.modsSizeBytes)) : ''})
				{/if}.
			</DialogDescription>
		</DialogHeader>

		<div class="space-y-4 py-2">
			<div class="space-y-2">
				<Label for="deploy_server_name" class="text-sm font-medium">New Server Name</Label>
				<Input id="deploy_server_name" bind:value={deployName} class="h-10" />
			</div>

			{#if proxyEnabled}
				<div class="space-y-2">
					<Label for="deploy_proxy_hostname" class="text-sm font-medium">
						Server Hostname
						<span class="text-xs text-muted-foreground">(Optional)</span>
					</Label>
					<Input
						id="deploy_proxy_hostname"
						placeholder="survival.example.com"
						bind:value={deployProxyHostname}
						class="h-10"
					/>
					<p class="text-xs text-muted-foreground">
						Optional hostname routing through the proxy for the new server
					</p>
				</div>
			{/if}

			<div class="flex items-start justify-between gap-4 rounded-lg bg-muted/50 p-4">
				<div class="space-y-0.5">
					<Label for="deploy_auto_start" class="cursor-pointer text-sm font-medium"
						>Auto Start</Label
					>
					<p class="text-xs text-muted-foreground">Start the server right after deployment</p>
				</div>
				<Checkbox
					id="deploy_auto_start"
					checked={deployAutoStart}
					onCheckedChange={(checked) => (deployAutoStart = checked === true)}
					class="mt-0.5 shrink-0"
				/>
			</div>
		</div>

		<Separator class="my-1" />

		<DialogFooter>
			<Button variant="outline" onclick={() => (deployOpen = false)} disabled={deployingBusy}>
				Cancel
			</Button>
			<Button onclick={handleDeploy} disabled={deployingBusy || !deployName.trim()}>
				{#if deployingBusy}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<Rocket class="mr-2 h-4 w-4" />
				{/if}
				Deploy Server
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>
