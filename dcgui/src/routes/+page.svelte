<script>
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { fetchStacks, getStackStatusEmoji, getContainerCounts } from "$lib/stackManager.js";
  import { logout, isAuthenticated } from "$lib/auth.js";

  let stacks = $state([]);

  async function loadStacks() {
    // Don't attempt to fetch stacks if the user isn't authenticated
    if (!isAuthenticated()) {
      stacks = [];
      return;
    }

    const data = await fetchStacks();
    stacks = data.sort((a, b) => a.name.localeCompare(b.name));
    console.log(stacks)
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

  function selectStack(stackName) {
    goto(`/${stackName}`);
  }

  function handleLogout() {
    logout();
  }
</script>

<div class="max-w-4xl mx-auto p-8 text-white/80">
  <div class="flex justify-between items-center mb-8">
    <h1 class="text-center text-5xl font-bold flex-1">Select a Stack</h1>
    <button
      onclick={handleLogout}
      class="px-4 py-2 text-white/80 border rounded border-red-500/50 bg-red-500/20 hover:bg-red-500/30 transition-colors text-sm"
    >
      🚪 Logout
    </button>
  </div>
  <div class="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-4">
    {#each stacks as stack (stack.name)}
      {@const counts = getContainerCounts(stack)}
      <a
        href={stack.name}
        class="flex items-center justify-between p-6 bg-white/80 text-black rounded-lg cursor-pointer transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg border-0"
      >
        <span class="font-semibold text-lg">{stack.name}</span>
        <div class="flex items-center gap-1">
          <span class="text-sm font-mono text-gray-600">
            {counts.running}/{counts.total}
          </span>
          <span class="text-2xl">
            {getStackStatusEmoji(stack)}
          </span>
        </div>
      </a>
    {:else}
      <p class="text-center text-white/60">
        No stacks available. Create a new stack to get started.
      </p>
    {/each}
  </div>
</div>

