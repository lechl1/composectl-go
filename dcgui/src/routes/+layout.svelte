<script>
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import '../index.css';
  import { checkAuth } from "$lib/auth";

  onMount(async () => {
    // Fetch auth status from backend; if endpoint not available, default to false
    await checkAuth();

    // Check authentication on mount and route changes
    return page.subscribe(async () => {
      await checkAuth();
    });
  });
</script>

<slot></slot>
