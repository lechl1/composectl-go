<script>
  import { goto } from "$app/navigation";
  import { onMount } from "svelte";

  let username = $state("");
  let password = $state("");
  let error = $state("");
  let isLoading = $state(false);

  onMount(() => {
    // Check if already authenticated
    const token = localStorage.getItem("authToken");
    if (token) {
      goto("/");
    }
  });

  async function handleLogin(e) {
    e.preventDefault();
    error = "";
    isLoading = true;

    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: {
          "Authorization": "Basic " + btoa(`${username}:${password}`)
        }
      });

      if (response.ok) {
        // Expect JSON { token: "..." } or plain text
        const token = await response.text();
        if (token) {
          localStorage.setItem("authToken", token);
          goto("/");
          return;
        }

        error = "Login succeeded but no token was returned.";
      } else {
        const errText = await response.text();
        error = errText || `Login failed: ${response.status}`;
      }
    } catch (err) {
      error = "An error occurred. Please try again.";
    } finally {
      isLoading = false;
    }
  }
</script>

<main class="flex w-full items-center justify-center min-h-screen bg-linear-to-br from-gray-900 to-gray-800">
  <div class="w-full max-w-md p-8 bg-white/10 backdrop-blur-md rounded-lg shadow-2xl border border-white/20">
    <h1 class="text-4xl font-bold text-white text-center mb-8">Login</h1>

    <form onsubmit={handleLogin} class="space-y-6">
      <div>
        <label for="username" class="block text-sm font-medium text-white/80 mb-2">
          Username
        </label>
        <input
          id="username"
          type="text"
          bind:value={username}
          required
          disabled={isLoading}
          class="w-full px-4 py-3 bg-white/5 border border-white/30 rounded-lg text-white placeholder-white/50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:opacity-50"
          placeholder="Enter your username"
        />
      </div>

      <div>
        <label for="password" class="block text-sm font-medium text-white/80 mb-2">
          Password
        </label>
        <input
          id="password"
          type="password"
          bind:value={password}
          required
          disabled={isLoading}
          class="w-full px-4 py-3 bg-white/5 border border-white/30 rounded-lg text-white placeholder-white/50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:opacity-50"
          placeholder="Enter your password"
        />
      </div>

      {#if error}
        <div class="p-3 bg-red-500/20 border border-red-500/50 rounded-lg text-red-200 text-sm">
          {error}
        </div>
      {/if}

      <button
        type="submit"
        disabled={isLoading}
        class="w-full py-3 px-4 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-800 disabled:opacity-50 text-white font-semibold rounded-lg transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 focus:ring-offset-gray-900"
      >
        {isLoading ? "Logging in..." : "Login"}
      </button>
    </form>
  </div>
</main>

