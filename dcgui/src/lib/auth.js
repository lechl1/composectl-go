import { goto } from "$app/navigation";
import { browser } from "$app/environment";

function gotoRoot() {
  const loginHref = (typeof window !== 'undefined' && window.location && window.location.origin)
      ? `${window.location.origin}/login`
      : '/login';
  goto(loginHref);
}

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
        localStorage.removeItem("authToken");
        gotoRoot();
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
 * Check authentication status by calling /api/auth/status with bearer token (if present)
 * Returns the fetch Response or null on error. Performs redirects similar to previous inline logic.
 */
export async function checkAuth() {
  if (!browser) {
    return false;
  }
  const headers = {};
  const token = localStorage.getItem("authToken");
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch('/api/auth/status', { headers });

  // If the response is not 2xx, and we're on the login page, redirect to /
  if (!res.ok && browser && window.location && window.location.pathname.indexOf('/login') >= 0) {
    return gotoRoot();
  }

  return true;
}

/**
 * Logout function
 */
export function logout() {
  if (browser) {
    localStorage.removeItem("authToken");
    return gotoRoot();
  }
}

/**
 * Check if user is authenticated
 */
export async function isAuthenticated() {
  return await checkAuth();
}
