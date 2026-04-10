const coreBaseUrl = process.env.MEZA_CORE_BASE_URL ?? "http://127.0.0.1:8080";
const operatorToken = process.env.MEZA_PANEL_OPERATOR_TOKEN ?? process.env.MEZA_OPERATOR_TOKEN ?? "";

export async function coreFetch(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }
  if (operatorToken) {
    headers.set("Authorization", `Bearer ${operatorToken}`);
  }

  return fetch(`${coreBaseUrl}${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });
}

export async function coreJson<T>(path: string, init: RequestInit = {}) {
  const response = await coreFetch(path, init);
  const text = await response.text();
  if (!response.ok) {
    throw new Error(text || `core request failed with ${response.status}`);
  }

  if (!text) {
    return {} as T;
  }

  return JSON.parse(text) as T;
}
