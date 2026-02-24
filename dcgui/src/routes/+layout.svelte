<script>
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { page } from "$app/stores";
  import { isAuthenticated } from "$lib/auth.js";
  import '../index.css';

  onMount(() => {
    // Check authentication on mount and route changes
    const unsubscribe = page.subscribe(($page) => {
      // Allow login page without authentication
      if ($page.url.pathname === '/login') {
        return;
      }

      // Redirect to login if not authenticated
      if (!isAuthenticated()) {
        goto('/login');
      }
    });

    return unsubscribe;
  });
</script>

<slot />
