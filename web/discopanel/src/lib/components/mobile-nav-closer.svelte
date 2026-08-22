<script lang="ts">
	import { page } from '$app/state';
	import { useSidebar } from '$lib/components/ui/sidebar/context.svelte.js';

	const sidebar = useSidebar();

	$effect(() => {
		page.url.pathname;
		sidebar.setOpenMobile(false);
	});
</script>
