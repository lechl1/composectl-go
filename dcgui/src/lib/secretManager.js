import { authFetch } from "./auth.js";

// Parse KEY=value lines from `dc secret ls` output into [{name, value}]
export function parseSecretList(text) {
  return (text || "")
    .split("\n")
    .map(line => line.trim())
    .filter(line => line && !line.startsWith("#") && line.includes("="))
    .map(line => {
      const eq = line.indexOf("=");
      return { name: line.slice(0, eq), value: line.slice(eq + 1) };
    });
}

export async function fetchSecrets() {
  try {
    const response = await authFetch("/api/secrets");
    if (!response.ok) return [];
    const text = await response.text();
    return parseSecretList(text).sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }));
  } catch (err) {
    console.error("fetchSecrets error", err);
    return [];
  }
}

export async function upsertSecret(name, value) {
  const response = await authFetch(`/api/secrets/${encodeURIComponent(name)}`, {
    method: "PUT",
    body: value,
  });
  if (!response.ok) {
    throw await response.text();
  }
  return await response.text();
}

export async function deleteSecret(name) {
  const response = await authFetch(`/api/secrets/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
  if (!response.ok) {
    throw await response.text();
  }
}