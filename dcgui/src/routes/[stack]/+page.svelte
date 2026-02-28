<script>
  import YamlEditor from "$lib/YamlEditor.svelte";
  import StackSideMenu from "$lib/StackSideMenu.svelte";
  import SecretSideMenu from "$lib/SecretSideMenu.svelte";
  import { page } from "$app/stores";
  import { secretsState } from "$lib/secretsStore.svelte.js";

  let selectedStack = $state("");

  $effect(() => {
    const stackParam = $page.params.stack;
    if (stackParam) {
      selectedStack = stackParam;
    }
  });
</script>

<main class="w-full h-screen overflow-hidden">
  <div class="flex flex-row w-full h-full gap-1">
    <StackSideMenu />
    <YamlEditor {selectedStack} />
    {#if secretsState.visible}
      <SecretSideMenu />
    {/if}
  </div>
</main>