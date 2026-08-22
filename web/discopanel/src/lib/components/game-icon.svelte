<script lang="ts">
	interface Props {
		game?: 'minecraft' | 'terraria' | string | number;
		class?: string;
		size?: number | string;
	}

	let { game = 'minecraft', class: className = 'h-6 w-6', size }: Props = $props();

	let normalizedGame = $derived.by(() => {
		if (typeof game === 'number') {
			if (game === 2) return 'terraria';
			return 'minecraft';
		}
		if (typeof game === 'string') {
			const lower = game.toLowerCase();
			if (lower.includes('terraria') || lower === '2') return 'terraria';
			return 'minecraft';
		}
		return 'minecraft';
	});
</script>

{#if normalizedGame === 'minecraft'}
	<svg
		xmlns="http://www.w3.org/2000/svg"
		viewBox="0 0 64 64"
		class={className}
		width={size}
		height={size}
		fill="none"
		aria-label="Minecraft"
	>
		<!-- Minecraft Isometric Grass Block -->
		<defs>
			<filter id="mc-drop" x="-10%" y="-10%" width="120%" height="120%">
				<feDropShadow dx="0" dy="2" stdDeviation="1.5" flood-opacity="0.25" />
			</filter>
		</defs>
		<g filter="url(#mc-drop)">
			<!-- Left Face (Dirt + Grass Fringe) -->
			<polygon points="4,20 32,36 32,60 4,44" fill="#866043" />
			<polygon points="4,28 11,32 11,40 4,36" fill="#755135" />
			<polygon points="11,36 18,40 18,48 11,44" fill="#966E4E" />
			<polygon points="18,44 25,48 25,56 18,52" fill="#6A472E" />
			<polygon points="25,38 32,42 32,50 25,46" fill="#755135" />
			<polygon points="11,48 18,52 18,58 11,54" fill="#5C3D26" />
			<polygon points="4,40 11,44 11,48 4,44" fill="#966E4E" />
			<polygon points="18,32 25,36 25,44 18,40" fill="#7A5538" />
			<polygon points="4,20 32,36 32,42 28,40 28,44 23,41 23,46 18,43 18,39 13,36 13,42 9,40 9,35 4,32" fill="#5C8E32" />
			<polygon points="4,20 13,25 13,31 9,29 9,35 4,32" fill="#6DA33D" />
			<polygon points="18,28 28,34 28,40 23,37 23,42 18,39" fill="#4E7C28" />

			<!-- Right Face (Shaded Dirt + Grass Fringe) -->
			<polygon points="32,36 60,20 60,44 32,60" fill="#5C3E28" />
			<polygon points="32,42 39,38 39,46 32,50" fill="#4D321F" />
			<polygon points="39,46 46,42 46,50 39,54" fill="#6E4D34" />
			<polygon points="46,38 53,34 53,42 46,46" fill="#442B19" />
			<polygon points="53,42 60,38 60,44 53,48" fill="#543823" />
			<polygon points="39,52 46,48 46,56 39,60" fill="#402816" />
			<polygon points="46,50 53,46 53,54 46,58" fill="#67462E" />
			<polygon points="32,36 60,20 60,31 55,34 55,29 50,32 50,38 45,35 45,41 40,38 40,43 36,41 36,45 32,43" fill="#446D23" />
			<polygon points="32,36 40,31 40,37 36,35 36,41 32,39" fill="#4E7C28" />
			<polygon points="45,29 55,23 55,29 50,32 50,35 45,32" fill="#385C1B" />

			<!-- Top Face (Grass) -->
			<polygon points="32,4 60,20 32,36 4,20" fill="#67A835" />
			<polygon points="32,4 41,9 34,13 25,8" fill="#7BC342" />
			<polygon points="41,9 50,14 43,18 34,13" fill="#5C992E" />
			<polygon points="50,14 60,20 53,24 43,18" fill="#6DB337" />
			<polygon points="25,8 34,13 27,17 18,12" fill="#56912A" />
			<polygon points="34,13 43,18 36,22 27,17" fill="#75BC3E" />
			<polygon points="43,18 53,24 46,28 36,22" fill="#538C27" />
			<polygon points="18,12 27,17 20,21 11,16" fill="#70B73B" />
			<polygon points="27,17 36,22 29,26 20,21" fill="#62A232" />
			<polygon points="36,22 46,28 39,32 29,26" fill="#7BC342" />
			<polygon points="11,16 20,21 13,25 4,20" fill="#5A962D" />
			<polygon points="20,21 29,26 22,30 13,25" fill="#73B93C" />
			<polygon points="29,26 39,32 32,36 22,30" fill="#58942C" />

			<!-- Highlight Edges -->
			<polyline points="4,20 32,4 60,20" stroke="#8CE64A" stroke-width="0.75" stroke-linecap="round" fill="none" opacity="0.6" />
			<polyline points="4,20 32,36 60,20" stroke="#2D4D14" stroke-width="0.5" fill="none" opacity="0.4" />
			<line x1="32" y1="36" x2="32" y2="60" stroke="#3A2414" stroke-width="0.75" opacity="0.4" />
		</g>
	</svg>
{:else if normalizedGame === 'terraria'}
	<svg
		xmlns="http://www.w3.org/2000/svg"
		viewBox="0 0 64 64"
		class={className}
		width={size}
		height={size}
		fill="none"
		aria-label="Terraria"
	>
		<!-- Terraria Iconic Tree -->
		<defs>
			<filter id="tr-drop" x="-10%" y="-10%" width="120%" height="120%">
				<feDropShadow dx="0" dy="2" stdDeviation="1.5" flood-opacity="0.25" />
			</filter>
		</defs>
		<g filter="url(#tr-drop)">
			<rect x="20" y="56" width="24" height="4" rx="2" fill="#5C3615" />
			<rect x="22" y="54" width="20" height="3" fill="#7A4C24" />
			<rect x="18" y="52" width="28" height="3" rx="1" fill="#4DA227" />
			<rect x="19" y="50" width="3" height="3" fill="#6BD638" />
			<rect x="26" y="51" width="3" height="2" fill="#6BD638" />
			<rect x="36" y="50" width="3" height="3" fill="#6BD638" />
			<rect x="43" y="51" width="3" height="2" fill="#6BD638" />

			<path d="M28 32 H36 V53 H28 Z" fill="#8B5325" />
			<path d="M28 32 H32 V53 H28 Z" fill="#A16733" />
			<path d="M34 32 H36 V53 H34 Z" fill="#683B17" />
			<path d="M25 45 L28 40 V48 L23 54 H21 L25 45 Z" fill="#7A471E" />
			<path d="M22 51 L25 46 L27 48 L23 53 Z" fill="#995E2B" />
			<path d="M36 41 L41 46 L43 54 H40 L36 48 V41 Z" fill="#683B17" />
			<path d="M37 43 L40 47 L39 52 L36 47 Z" fill="#8B5325" />
			<path d="M28 34 L22 26 H26 L30 32 Z" fill="#8B5325" />
			<path d="M34 34 L42 25 H38 L32 32 Z" fill="#683B17" />

			<circle cx="32" cy="18" r="16" fill="#1C5508" />
			<circle cx="21" cy="22" r="12" fill="#1C5508" />
			<circle cx="43" cy="22" r="12" fill="#154205" />

			<circle cx="32" cy="17" r="14" fill="#328812" />
			<circle cx="21" cy="21" r="10.5" fill="#328812" />
			<circle cx="43" cy="21" r="10.5" fill="#246D0B" />

			<circle cx="31" cy="15" r="11.5" fill="#4DAF1F" />
			<circle cx="20" cy="19" r="8.5" fill="#4DAF1F" />
			<circle cx="42" cy="19" r="8.5" fill="#3A9615" />

			<circle cx="30" cy="12" r="7.5" fill="#6CD82F" />
			<circle cx="19" cy="16" r="5.5" fill="#6CD82F" />
			<circle cx="41" cy="16" r="5.5" fill="#55B824" />

			<rect x="28" y="7" width="5" height="4" rx="1.5" fill="#88EE48" />
			<rect x="17" y="12" width="4" height="3" rx="1" fill="#88EE48" />
			<rect x="38" y="12" width="4" height="3" rx="1" fill="#72DA32" />
			<rect x="23" y="11" width="3" height="3" rx="1" fill="#88EE48" />
			<rect x="33" y="16" width="3" height="3" rx="1" fill="#88EE48" />
		</g>
	</svg>
{/if}
