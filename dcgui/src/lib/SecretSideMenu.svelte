<script>
  import { onMount } from "svelte";
  import { fetchSecrets, upsertSecret, deleteSecret } from "$lib/secretManager.js";
  import { isAuthenticated } from "$lib/auth.js";

  let secrets = $state([]);
  let pending = $state([]);
  let nextId = 0;

  async function loadSecrets() {
    if (!isAuthenticated()) {
      secrets = [];
      return;
    }
    secrets = await fetchSecrets();
  }

  onMount(async () => {
    if (!isAuthenticated()) return;

    await loadSecrets();
    const interval = setInterval(async () => {
      if (!isAuthenticated()) {
        clearInterval(interval);
        return;
      }
      await loadSecrets();
    }, 5000);
    return () => clearInterval(interval);
  });

  function addPending() {
    pending = [...pending, { id: nextId++, entry: "" }];
  }

  function removePending(id) {
    pending = pending.filter(p => p.id !== id);
  }

  async function savePending(id) {
    const row = pending.find(p => p.id === id);
    if (!row) return;

    const entry = (row.entry || "").trim();
    if (!entry || !isAuthenticated()) {
      removePending(id);
      return;
    }

    const match = entry.match(/^([^=:]+)[=:]([\s\S]*)$/);
    const name = match ? match[1].trim() : entry;
    const value = match ? match[2] : "";
    if (!name) {
      removePending(id);
      return;
    }

    try {
      await upsertSecret(name, value);
    } catch (err) {
      // ignore
    } finally {
      removePending(id);
      await loadSecrets();
    }
  }

  async function handleDelete(name) {
    try {
      await deleteSecret(name);
    } catch (err) {
      // ignore
    } finally {
      await loadSecrets();
    }
  }
</script>

<div class="flex flex-col shrink-0 gap-1 min-w-[18.53%]">
  <!-- Header row -->
  <div class="flex justify-end items-center">
    <button
      onclick={addPending}
      class="p-2 text-white/80 border rounded border-green-500/50 bg-green-500/20 hover:bg-green-500/30 transition-colors text-sm cursor-pointer"
    >
      ➕ Add
    </button>
  </div>

  <!-- Pending (unsaved) rows -->
  {#each pending as row (row.id)}
    <div class="w-full text-white/80 flex flex-row flex-nowrap items-stretch gap-1 border border-white/30 rounded">
      <input
        bind:value={row.entry}
        placeholder="KEY=value"
        class="flex-1 min-w-0 p-1 font-mono text-sm bg-transparent"
        onkeydown={(e) => {
          if (e.key === "Enter" && row.entry.trim()) { savePending(row.id); }
          else if (e.key === "Escape") { removePending(row.id); }
        }}
      />
      {#if row.entry.trim()}
        <button
          class="shrink-0 text-base text-gray-600 hover:text-green-400 bg-white hover:bg-white rounded-r px-2 cursor-pointer"
          onclick={() => savePending(row.id)}
          title="Save secret"
        >💾</button>
      {:else}
        <button
          class="shrink-0 text-base text-gray-600 hover:text-red-400 bg-white hover:bg-white rounded-r px-2 cursor-pointer"
          onclick={() => removePending(row.id)}
          title="Discard"
        >🗑️</button>
      {/if}
    </div>
  {/each}

  <!-- Server secrets -->
  {#each secrets as secret}
    <div class="w-full text-white/80 flex flex-row flex-nowrap items-stretch gap-1 border border-white/30 rounded">
      <div class="flex flex-col flex-1 min-w-0 overflow-hidden p-1">
        <span class="text-sm font-mono truncate">{secret.name}</span>
        <span class="text-gray-500 text-xs font-mono truncate">{secret.value}</span>
      </div>
      <button
        class="shrink-0 text-base text-gray-600 hover:text-red-400 bg-white/10 hover:bg-white/80 cursor-pointer rounded-r px-3"
        onclick={() => handleDelete(secret.name)}
        title="Delete secret"
      >🗑️</button>
    </div>
  {/each}
</div>
