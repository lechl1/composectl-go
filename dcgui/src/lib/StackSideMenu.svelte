<script>
  import { onMount, tick } from "svelte";
  import {
    fetchStacks,
    getStackStatusEmoji,
    getContainerCounts,
    saveStack
  } from "$lib/stackManager.js";
  import { isAuthenticated } from "$lib/auth.js";

  let stacks = $state([]);

  let addMode = $state(false);
  let newName = $state("");
  let addInputRef = null;

  async function loadStacks() {
    if (!isAuthenticated()) {
      stacks = [];
      return;
    }

    stacks = await fetchStacks();
  }

  onMount(async () => {
    // Only start polling when authenticated
    if (!isAuthenticated()) {
      return;
    }

    await loadStacks();
    const interval = setInterval(async () => {
      if (!isAuthenticated()) {
        clearInterval(interval);
        return;
      }
      await loadStacks();
    }, 5000);
    return () => clearInterval(interval);
  });

  async function enterAddMode() {
    addMode = true;
    await tick();
    if (addInputRef && addInputRef.focus) addInputRef.focus();
  }

  async function cancelAdd() {
    addMode = false;
    newName = "";
  }

  async function createStackIfValid() {
    const name = (newName || "").trim();
    if (!name) {
      cancelAdd();
      return;
    }

    // Ensure authenticated before attempting to create
    if (!isAuthenticated()) {
      cancelAdd();
      return;
    }

    try {
      // saveStack does a PUT to /api/stacks/:name
      const result = await saveStack(name, "services: {}", (/*log*/) => {});
      // result is {text, success}
      if (result && result.success) {
        await loadStacks();
      } else {
        // still reload to reflect any changes or errors
        await loadStacks();
      }
    } catch (err) {
      // ignore, authFetch will redirect on 401/403; just reload stacks
      await loadStacks();
    } finally {
      cancelAdd();
    }
  }

</script>

<div class="flex flex-col justify-stretch shrink-0 gap-1">
  {#each stacks as stack}
    {@const counts = getContainerCounts(stack)}
    <a
      href={stack.name}
      class="w-full text-white/80 border-0 rounded border-1 border-white/30 gap-1 p-1 cursor-pointer flex justify-between flex-nowrap"
    >
      <span class="flex w-full justify-start">{stack.name}</span>
      <span class="flex w-full justify-end">{counts.running}/{counts.total} {getStackStatusEmoji(stack)}</span>
    </a>
  {/each}

  <!-- Add button / input -->
  {#if !addMode}
    <div class="w-full text-white/60 border-0 rounded border-1 border-white/20 gap-1 p-1 cursor-pointer flex justify-center items-center"
         on:click={enterAddMode}>
      + Add
    </div>
  {:else}
    <div class="gap-1 p-1 w-full max-w-full text-white">
      <input
        bind:this={addInputRef}
        bind:value={newName}
        placeholder="name"
        class="w-full max-w-full p-1 rounded"
        on:keydown={(e) => { if (e.key === 'Enter') { createStackIfValid(); } else if (e.key === 'Escape') { cancelAdd(); } }}
        on:blur={cancelAdd}
      />
    </div>
  {/if}
</div>
