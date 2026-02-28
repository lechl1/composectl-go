const STORAGE_KEY = "secretsVisible";

export const secretsState = $state({
  visible: (localStorage.getItem(STORAGE_KEY) ?? "true") === "true",
});

export function toggleSecrets() {
  secretsState.visible = !secretsState.visible;
  localStorage.setItem(STORAGE_KEY, String(secretsState.visible));
}
