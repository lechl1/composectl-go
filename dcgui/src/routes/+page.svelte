<script>
  import StackSideMenu from "$lib/StackSideMenu.svelte";
  import { logout } from "$lib/auth.js";

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
  <div class="flex">
    <StackSideMenu></StackSideMenu>
  </div>
</div>
