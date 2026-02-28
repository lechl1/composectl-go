<script>
  import StackSideMenu from "$lib/StackSideMenu.svelte";
  import { logout } from "$lib/auth.js";
  import { secretsState, toggleSecrets } from "$lib/secretsStore.svelte.js";

  function handleLogout() {
    logout();
  }
</script>

<div class="max-w-4xl mx-auto p-8 text-white/80">
  <div class="flex justify-between items-center mb-8">
    <h1 class="text-center text-5xl font-bold flex-1">Select a Stack</h1>
    <div class="flex gap-2">
      <button
        onclick={toggleSecrets}
        class="px-4 py-2 text-white/80 border rounded border-yellow-500/50 bg-yellow-500/20 hover:bg-yellow-500/30 transition-colors text-sm"
      >
        🔐 Secrets
      </button>
      <button
        onclick={handleLogout}
        class="px-4 py-2 text-white/80 border rounded border-red-500/50 bg-red-500/20 hover:bg-red-500/30 transition-colors text-sm"
      >
        🚪 Logout
      </button>
    </div>
  </div>
  <div class="flex">
    <StackSideMenu></StackSideMenu>
  </div>
</div>
