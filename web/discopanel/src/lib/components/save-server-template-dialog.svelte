<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
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
	import { Loader2, PackagePlus } from '@lucide/svelte';
	import type { Server } from '$lib/proto/discopanel/v1/common_pb';
	import { CreateServerTemplateRequestSchema } from '$lib/proto/discopanel/v1/template_pb';

	interface Props {
		server: Server | null;
		open?: boolean;
		onSaved?: (templateName: string) => void;
	}

	let { server = null, open = $bindable(false), onSaved }: Props = $props();

	let name = $state('');
	let description = $state('');
	let includeMods = $state(true);
	let saving = $state(false);

	// Prefill the form whenever the dialog opens for a different server
	let lastServerId = $state<string | null>(null);
	$effect(() => {
		if (open && server && server.id !== lastServerId) {
			lastServerId = server.id;
			name = server.name;
			description = server.description || '';
			includeMods = true;
			saving = false;
		}
	});

	async function handleSave() {
		if (!server) return;
		if (!name.trim()) {
			toast.error('Template name is required');
			return;
		}

		saving = true;
		try {
			await rpcClient.template.createServerTemplate(
				create(CreateServerTemplateRequestSchema, {
					serverId: server.id,
					name: name.trim(),
					description: description.trim(),
					includeMods
				})
			);
			toast.success(`Template "${name.trim()}" saved`, {
				description: includeMods ? 'The mods directory was captured into the template.' : undefined
			});
			open = false;
			onSaved?.(name.trim());
		} catch (error) {
			toast.error(
				`Failed to save template: ${error instanceof Error ? error.message : 'Unknown error'}`
			);
		} finally {
			saving = false;
		}
	}
</script>

<Dialog bind:open>
	<DialogContent class="sm:max-w-md">
		<DialogHeader>
			<DialogTitle class="flex items-center gap-2">
				<PackagePlus class="h-5 w-5 text-primary" />
				Save as Template
			</DialogTitle>
			<DialogDescription class="text-sm text-muted-foreground">
				Capture "{server?.name}" configuration{server?.proxyHostname
					? ' (without proxy routing)'
					: ''} as a reusable server template.
			</DialogDescription>
		</DialogHeader>

		<div class="space-y-4 py-2">
			<div class="space-y-2">
				<Label for="template_name" class="text-sm font-medium">Template Name</Label>
				<Input id="template_name" placeholder="My Template" bind:value={name} class="h-10" />
			</div>

			<div class="space-y-2">
				<Label for="template_description" class="text-sm font-medium">
					Description
					<span class="text-xs text-muted-foreground">(Optional)</span>
				</Label>
				<Input
					id="template_description"
					placeholder="What is this template for?"
					bind:value={description}
					class="h-10"
				/>
			</div>

			<div class="flex items-start justify-between gap-4 rounded-lg bg-muted/50 p-4">
				<div class="space-y-1">
					<Label for="include_mods" class="cursor-pointer text-sm font-medium">Include Mods</Label>
					<p class="text-xs text-muted-foreground">
						Copies the server's mods directory into the template so deploys come with mods already
						installed. This can take a while and use significant disk space for large mods folders.
					</p>
				</div>
				<Checkbox
					id="include_mods"
					checked={includeMods}
					onCheckedChange={(checked) => (includeMods = checked === true)}
					class="mt-0.5 shrink-0"
				/>
			</div>
		</div>

		<DialogFooter>
			<Button variant="outline" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
			<Button onclick={handleSave} disabled={saving || !name.trim()}>
				{#if saving}
					<Loader2 class="mr-2 h-4 w-4 animate-spin" />
				{:else}
					<PackagePlus class="mr-2 h-4 w-4" />
				{/if}
				Save Template
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>
