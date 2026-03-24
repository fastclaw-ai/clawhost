const API_BASE = "/bot/api/v1/admin";

function getToken(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem("admin_token") || "";
}

export function setToken(token: string) {
  localStorage.setItem("admin_token", token);
}

export function getStoredToken(): string {
  return getToken();
}

export function clearToken() {
  localStorage.removeItem("admin_token");
}

export async function verifyToken(token: string): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/verify`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    return res.ok;
  } catch {
    return false;
  }
}

async function request<T>(
  path: string,
  options: RequestInit = {}
): Promise<{ code: number; message: string; data: T }> {
  const token = getToken();
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...options.headers,
    },
  });

  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.message || `Request failed: ${res.status}`);
  }
  return json;
}

// App types
export interface App {
  id: string;
  name: string;
  url: string;
  description: string;
  owner_email: string;
  api_token: string;
  bot_domain_template: string;
  status: string;
  created_at: string;
  updated_at: string;
}

// Bot types
export interface Bot {
  id: string;
  app_id: string;
  user_id: string;
  name: string;
  slug: string;
  access_token: string;
  status: "created" | "starting" | "running" | "stopped" | "error";
  config: Record<string, unknown>;
  endpoint: string;
  expires_at: string | null;
  created_at: string;
  updated_at: string;
}

// App APIs
export async function listApps() {
  return request<App[]>("/apps");
}

export async function getApp(id: string) {
  return request<App>(`/apps/${id}`);
}

export async function createApp(data: {
  name: string;
  url?: string;
  description?: string;
  owner_email?: string;
  bot_domain_template?: string;
}) {
  return request<App>("/apps", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function updateApp(
  id: string,
  data: {
    name?: string;
    url?: string;
    description?: string;
    owner_email?: string;
    bot_domain_template?: string;
    status?: string;
  }
) {
  return request<App>(`/apps/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteApp(id: string) {
  return request<{ message: string }>(`/apps/${id}`, {
    method: "DELETE",
  });
}

export async function resetAppToken(id: string) {
  return request<{ api_token: string }>(`/apps/${id}/reset-token`, {
    method: "POST",
  });
}

// Bot APIs
export async function createBot(data: {
  app_id: string;
  user_id?: string;
  name: string;
  slug?: string;
}) {
  return request<Bot>("/bots", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function listBots() {
  return request<Bot[]>("/bots");
}

export async function startBot(id: string) {
  return request<Bot>(`/bots/${id}/start`, { method: "POST" });
}

export async function stopBot(id: string) {
  return request<Bot>(`/bots/${id}/stop`, { method: "POST" });
}

export async function deleteBot(id: string) {
  return request<{ message: string }>(`/bots/${id}`, { method: "DELETE" });
}

export async function upgradeBot(id: string) {
  return request<Bot>(`/bots/${id}/upgrade`, { method: "POST" });
}

export async function upgradeAllBots() {
  return request<{ message: string }>("/bots/upgrade", { method: "POST" });
}

export async function restartAllBots() {
  return request<{ message: string }>("/bots/restart", { method: "POST" });
}

// Config APIs
export async function getAdminConfig() {
  return request<{ bot_domain_template: string }>("/config");
}
