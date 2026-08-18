<script lang="ts">
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { Switch } from '$lib/components/ui/switch';
  import { Select, SelectContent, SelectItem, SelectTrigger } from '$lib/components/ui/select';

  let { config = $bindable(), disabled = false } = $props();

  const difficulties = [
    { value: 0, label: 'Classic' },
    { value: 1, label: 'Expert' },
    { value: 2, label: 'Master' },
    { value: 3, label: 'Journey' }
  ];
</script>

{#if config}
<div class="space-y-6">
  <div class="grid grid-cols-2 gap-4">
    <div class="space-y-2">
      <Label>World Name</Label>
      <Input bind:value={config.worldName} {disabled} placeholder="World" />
    </div>

    <div class="space-y-2">
      <Label>Difficulty</Label>
      <Select type="single" value={config.difficulty?.toString()} onValueChange={(v) => config.difficulty = parseInt(v) || 0} {disabled}>
        <SelectTrigger>
           <span>{difficulties.find(d => d.value === config.difficulty)?.label || 'Classic'}</span>
        </SelectTrigger>
        <SelectContent>
           {#each difficulties as diff}
              <SelectItem value={diff.value.toString()}>{diff.label}</SelectItem>
           {/each}
        </SelectContent>
      </Select>
    </div>
  </div>

  <div class="grid grid-cols-2 gap-4">
    <div class="space-y-2">
      <Label>Max Players</Label>
      <Input type="number" bind:value={config.maxPlayers} min="1" max="255" {disabled} />
    </div>

    <div class="space-y-2">
      <Label>Server Password</Label>
      <Input type="password" bind:value={config.password} {disabled} placeholder="Leave blank for no password" />
    </div>
  </div>

  <div class="space-y-2">
    <Label>MOTD (Message of the Day)</Label>
    <Input bind:value={config.motd} {disabled} placeholder="Welcome to my Terraria server!" />
  </div>

  <div class="flex items-center justify-between rounded-lg bg-muted/50 p-4">
    <div class="space-y-0.5">
      <Label>Secure</Label>
      <p class="text-xs text-muted-foreground">Adds cheat protection</p>
    </div>
    <Switch bind:checked={config.secure} {disabled} />
  </div>

  <div class="flex items-center justify-between rounded-lg bg-muted/50 p-4">
    <div class="space-y-0.5">
      <Label>Spawn Protection</Label>
      <p class="text-xs text-muted-foreground">Protects the spawn area from block modification</p>
    </div>
    <Switch bind:checked={config.spawnProtection} {disabled} />
  </div>
</div>
{/if}
