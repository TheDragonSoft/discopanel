<script lang="ts">
	import {
		Dialog,
		DialogContent,
		DialogDescription,
		DialogHeader,
		DialogTitle
	} from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { rpcClient } from '$lib/api/rpc-client';
	import { toast } from 'svelte-sonner';
	import { Loader2, Save, ArrowLeft, FileText, FileCog, RotateCw } from '@lucide/svelte';
	import type { FileInfo } from '$lib/proto/discopanel/v1/file_pb';
	import CodeEditor from '$lib/components/ui/code-editor.svelte';

	interface Props {
		serverId: string;
		serverRunning: boolean;
		open?: boolean;
		onClose?: () => void;
	}

	let { serverId, serverRunning, open = $bindable(false), onClose }: Props = $props();

	let view = $state<'list' | 'edit'>('list');
	let configFiles = $state<FileInfo[]>([]);
	let loadingList = $state(false);
	let listLoaded = $state(false);
	let loadingFile = $state(false);
	let saving = $state(false);
	let selectedFile = $state<FileInfo | null>(null);
	let content = $state('');
	let originalContent = $state('');
	let editorValue = $state('');

	let isDirty = $derived(content !== originalContent);
	let editorLanguage = $derived(selectedFile ? getLanguage(selectedFile.name) : 'plaintext');

	// Load the well-known config file list whenever the dialog opens
	$effect(() => {
		if (open && !listLoaded) {
			loadConfigList();
		}
		if (!open) {
			resetState();
		}
	});

	function resetState() {
		view = 'list';
		selectedFile = null;
		content = '';
		originalContent = '';
		editorValue = '';
		listLoaded = false;
	}

	async function loadConfigList() {
		loadingList = true;
		try {
			const response = await rpcClient.file.listFiles({
				serverId,
				path: '',
				tree: false
			});
			configFiles = response.files
				.filter((f) => !f.isDir && isWellKnownConfig(f.name))
				.sort((a, b) => {
					// server.properties always first
					const aProp = a.name === 'server.properties' ? 0 : 1;
					const bProp = b.name === 'server.properties' ? 0 : 1;
					if (aProp !== bProp) return aProp - bProp;
					return a.name.localeCompare(b.name);
				});
			listLoaded = true;
		} catch {
			toast.error('Failed to list config files');
		} finally {
			loadingList = false;
		}
	}

	function isWellKnownConfig(name: string): boolean {
		if (name === 'server.properties') return true;
		return /\.(yml|yaml|toml|json)$/i.test(name);
	}

	async function selectFile(file: FileInfo) {
		selectedFile = file;
		loadingFile = true;
		try {
			const response = await rpcClient.file.getFile({ serverId, path: file.path });
			const text = new TextDecoder().decode(response.content);
			content = text;
			originalContent = text;
			editorValue = text;
			view = 'edit';
		} catch {
			toast.error('Failed to load file content');
			selectedFile = null;
		} finally {
			loadingFile = false;
		}
	}

	function handleEditorChange(value: string) {
		content = value;
	}

	function backToList() {
		if (isDirty && !confirm('You have unsaved changes. Discard them?')) return;
		view = 'list';
		selectedFile = null;
		content = '';
		originalContent = '';
		editorValue = '';
	}

	async function handleSave() {
		if (!selectedFile || !isDirty) return;

		if (serverRunning) {
			const confirmed = confirm(
				'The server is currently running. Some changes require a restart to take effect. Save anyway?'
			);
			if (!confirmed) return;
		}

		saving = true;
		try {
			await rpcClient.file.updateFile({
				serverId,
				path: selectedFile.path,
				content: new TextEncoder().encode(content)
			});
			toast.success(`${selectedFile.name} saved`);
			originalContent = content;
		} catch {
			toast.error('Failed to save file');
		} finally {
			saving = false;
		}
	}

	function handleClose() {
		if (isDirty && !confirm('You have unsaved changes. Are you sure you want to close?')) return;
		open = false;
		onClose?.();
	}

	function getLanguage(fileName: string): string {
		const ext = fileName.toLowerCase().split('.').pop() || '';
		const map: Record<string, string> = {
			json: 'json',
			yml: 'yaml',
			yaml: 'yaml',
			toml: 'ini',
			properties: 'ini',
			ini: 'ini',
			conf: 'ini'
		};
		return map[ext] || 'plaintext';
	}
</script>

<Dialog {open} onOpenChange={(isOpen) => !isOpen && handleClose()}>
	<DialogContent
		showCloseButton={false}
		class="flex h-[100dvh] w-full max-w-full flex-col rounded-none border-0 p-3 sm:h-[70vh] sm:max-w-3xl sm:rounded-lg sm:border sm:p-6"
	>
		<div class="absolute top-3 right-3 z-20 flex gap-1">
			{#if view === 'edit'}
				<Button
					variant="ghost"
					size="icon"
					class="h-8 w-8"
					onclick={backToList}
					title="Back to file list"
				>
					<ArrowLeft class="h-4 w-4" />
					<span class="sr-only">Back</span>
				</Button>
			{/if}
		</div>

		<DialogHeader class="shrink-0 pr-12">
			{#if view === 'edit' && selectedFile}
				<DialogTitle class="flex items-center gap-2 text-base sm:text-lg">
					<FileText class="h-4 w-4 text-muted-foreground" />
					{selectedFile.name}
					{#if isDirty}
						<span class="text-sm text-orange-500">●</span>
					{/if}
				</DialogTitle>
				<DialogDescription class="truncate text-xs">
					Quick edit — {selectedFile.path}
				</DialogDescription>
			{:else}
				<DialogTitle class="flex items-center gap-2 text-base sm:text-lg">
					<FileCog class="h-4 w-4" />
					Quick Edit Config
				</DialogTitle>
				<DialogDescription class="text-xs">
					Common configuration files in the server root. For everything else, use the Files tab.
				</DialogDescription>
			{/if}
		</DialogHeader>

		{#if view === 'list'}
			<div class="min-h-0 flex-1 overflow-y-auto">
				{#if loadingList}
					<div class="flex h-full items-center justify-center">
						<Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
					</div>
				{:else if configFiles.length === 0}
					<div class="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
						<FileCog class="h-8 w-8" />
						<p class="text-sm">No well-known config files found in the server root.</p>
					</div>
				{:else}
					<div class="flex flex-col gap-1 p-1">
						{#each configFiles as file (file.path)}
							<button
								type="button"
								class="flex items-center gap-3 rounded-md px-3 py-2.5 text-left transition-colors hover:bg-accent"
								onclick={() => selectFile(file)}
							>
								<FileText class="h-4 w-4 shrink-0 text-muted-foreground" />
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium">{file.name}</p>
									<p class="truncate font-mono text-xs text-muted-foreground">{file.path}</p>
								</div>
								{#if file.name === 'server.properties'}
									<span
										class="shrink-0 rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground"
										>server</span
									>
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{:else}
			<div class="relative my-2 min-h-0 flex-1 overflow-hidden sm:my-0">
				{#if loadingFile}
					<div class="absolute inset-0 z-10 flex items-center justify-center bg-background/80">
						<Loader2 class="h-8 w-8 animate-spin" />
					</div>
				{/if}
				{#if selectedFile}
					<CodeEditor
						value={editorValue}
						language={editorLanguage}
						height="100%"
						onChange={handleEditorChange}
					/>
				{/if}
			</div>

			<div class="flex shrink-0 flex-col justify-between gap-2 sm:flex-row sm:items-center">
				<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
					<span class="font-mono">{editorLanguage.toUpperCase()}</span>
					<span>{content.split('\n').length} lines</span>
					{#if isDirty}
						<span class="font-medium text-orange-500">● Modified</span>
					{:else}
						<span class="font-medium text-green-500">● Saved</span>
					{/if}
				</div>
				<div class="flex items-center justify-end gap-2">
					<Button variant="outline" size="sm" class="h-8" onclick={handleClose}>Close</Button>
					<Button
						onclick={handleSave}
						disabled={!isDirty || saving || loadingFile}
						variant={isDirty ? 'default' : 'secondary'}
						size="sm"
						class="h-8"
					>
						{#if saving}
							<Loader2 class="mr-1.5 h-3.5 w-3.5 animate-spin" />
						{:else}
							<Save class="mr-1.5 h-3.5 w-3.5" />
						{/if}
						Save
					</Button>
				</div>
			</div>
			{#if serverRunning}
				<p class="flex shrink-0 items-center gap-1.5 text-xs text-amber-500">
					<RotateCw class="h-3 w-3" />
					Server is running — some changes require a restart to take effect.
				</p>
			{/if}
		{/if}
	</DialogContent>
</Dialog>
