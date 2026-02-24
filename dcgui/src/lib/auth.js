import { goto } from "$app/navigation";
import { browser } from "$app/environment";

/**
 * Authenticated fetch wrapper that handles 401/403 redirects
 */
export async function authFetch(url, options = {}) {
  // Add authorization header if token exists
  if (browser) {
    const token = localStorage.getItem("authToken");
    if (token) {
      options.headers = {
        ...options.headers,
        "Authorization": `Bearer ${token}`,
      };
    }
  }

  try {
    const response = await fetch(url, options);

    // Check for authentication errors
    if (response.status === 401 || response.status === 403) {
      if (browser) {
        // Clear token and redirect to login
        localStorage.removeItem("authToken");
        goto("/login");
      }
      throw new Error("Unauthorized");
    }

    return response;
  } catch (error) {
    // Re-throw if it's our auth error
    if (error.message === "Unauthorized") {
      throw error;
    }
    // For network errors or other issues, also check if we should redirect
    throw error;
  }
}

/**
 * Logout function
 */
export function logout() {
  if (browser) {
    localStorage.removeItem("authToken");
    goto("/login");
  }
}

/**
 * Check if user is authenticated
 */
export function isAuthenticated() {
  if (browser) {
    return !!localStorage.getItem("authToken");
  }
  return false;
}

