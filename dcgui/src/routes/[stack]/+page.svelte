<script>
  import YamlEditor from "$lib/YamlEditor.svelte";
  import StackSideMenu from "$lib/StackSideMenu.svelte";
  import { page } from "$app/stores";
  import { fetchStackDoc } from "$lib/stackManager.js";

  let selectedStack = $state("");

  // React to changes in the stack parameter
  $effect(() => {
    const stackParam = $page.params.stack;
    if (stackParam) {
      selectedStack = stackParam
    }
  });

</script>

<main class="w-full h-screen overflow-hidden">
  <div class="flex flex-row w-full h-full gap-1">
    <StackSideMenu />
    <YamlEditor {selectedStack} />
  </div>
</main>
