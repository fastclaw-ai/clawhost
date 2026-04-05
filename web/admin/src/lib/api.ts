// ── Auth helpers ────────────────────────────────────────────────

const ADMIN_BASE = "/bot/api/v1/admin";

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
    const res = await fetch(`${ADMIN_BASE}/verify`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    return res.ok;
  } catch {
    return false;
  }
}

// ── Generic request ─────────────────────────────────────────────

export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

async function request<T>(
  path: string,
  options: RequestInit = {}
): Promise<ApiResponse<T>> {
  const token = getToken();
  const res = await fetch(path, {
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

/** Admin API request (prefix: /bot/api/v1/admin) */
function adminReq<T>(path: string, options: RequestInit = {}) {
  return request<T>(`${ADMIN_BASE}${path}`, options);
}

/** Bot-level API request via admin proxy (prefix: /bot/api/v1/admin/bots/:id) */
function botReq<T>(botId: string, path: string, options: RequestInit = {}) {
  return request<T>(`${ADMIN_BASE}/bots/${botId}${path}`, options);
}

// ── Error helper ────────────────────────────────────────────────

export function getErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  return "An unknown error occurred";
}

// ── Types ───────────────────────────────────────────────────────

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

export interface BotStatus {
  id: string;
  name: string;
  status: string;
  ready: boolean;
  endpoint?: string;
}

export interface BotConnect {
  id: string;
  name: string;
  status: string;
  ready: boolean;
  token: string;
  endpoint: string;
  ws_url: string;
  webchat_url: string;
}

export interface ModelProvider {
  name: string;
  baseUrl?: string;
  apiKey?: string;
  auth?: string;
  api?: string;
  models?: ModelConfig[];
}

export interface ModelConfig {
  id: string;
  name?: string;
  reasoning?: boolean;
  input?: string[];
  context_window?: number;
  max_tokens?: number;
}

export interface AgentDefaults {
  primary_model?: string;
  fallback_model?: string;
}

export interface Channel {
  channel: string;
  account?: string;
  enabled?: boolean;
  [key: string]: unknown;
}

export interface Skill {
  name: string;
}

export interface Device {
  device_id: string;
  role: string;
  platform: string;
  client_id: string;
  client_mode: string;
  status: string;
  age: string;
  connected: boolean;
  revoked: boolean;
}

// ── App APIs ────────────────────────────────────────────────────

export async function listApps() {
  return adminReq<App[]>("/apps");
}

export async function getApp(id: string) {
  return adminReq<App>(`/apps/${id}`);
}

export async function createApp(data: {
  name: string;
  url?: string;
  description?: string;
  owner_email?: string;
  bot_domain_template?: string;
}) {
  return adminReq<App>("/apps", {
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
  return adminReq<App>(`/apps/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteApp(id: string) {
  return adminReq<{ message: string }>(`/apps/${id}`, { method: "DELETE" });
}

export async function resetAppToken(id: string) {
  return adminReq<{ api_token: string }>(`/apps/${id}/reset-token`, {
    method: "POST",
  });
}

// ── Bot APIs (admin) ────────────────────────────────────────────

export async function createBot(data: {
  app_id: string;
  user_id?: string;
  name: string;
  slug?: string;
}) {
  return adminReq<Bot>("/bots", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function listBots() {
  return adminReq<Bot[]>("/bots");
}

export async function getBot(id: string) {
  return adminReq<Bot>(`/bots/${id}`);
}

export async function startBot(id: string) {
  return adminReq<Bot>(`/bots/${id}/start`, { method: "POST" });
}

export async function stopBot(id: string) {
  return adminReq<Bot>(`/bots/${id}/stop`, { method: "POST" });
}

export async function deleteBot(id: string) {
  return adminReq<{ message: string }>(`/bots/${id}`, { method: "DELETE" });
}

export async function upgradeBot(id: string) {
  return adminReq<Bot>(`/bots/${id}/upgrade`, { method: "POST" });
}

export async function restartBot(id: string) {
  return adminReq<Bot>(`/bots/${id}/restart`, { method: "POST" });
}

export async function upgradeAllBots() {
  return adminReq<{ message: string }>("/bots/upgrade", { method: "POST" });
}

export async function restartAllBots() {
  return adminReq<{ message: string }>("/bots/restart", { method: "POST" });
}

// ── Bot sub-resource APIs (via admin proxy) ─────────────────────

// Status & Connect
export async function getBotStatus(botId: string) {
  return botReq<BotStatus>(botId, "/status");
}

export async function getBotConnect(botId: string) {
  return botReq<BotConnect>(botId, "/connect");
}

export async function updateBot(
  botId: string,
  data: { name?: string; slug?: string; config?: Record<string, unknown>; expires_at?: string | null }
) {
  return botReq<Bot>(botId, "", { method: "PUT", body: JSON.stringify(data) });
}

export async function resetBotToken(botId: string) {
  return botReq<{ id: string; access_token: string; access_url: string; message: string }>(
    botId, "/reset-token", { method: "POST" }
  );
}

// Skills
export async function listSkills(botId: string) {
  return botReq<Skill[]>(botId, "/skills");
}

export async function updateSkill(botId: string, name: string, content: string) {
  return botReq<{ message: string; name: string }>(botId, `/skills/${name}`, {
    method: "PUT",
    body: JSON.stringify({ content }),
  });
}

export async function deleteSkill(botId: string, name: string) {
  return botReq<{ message: string }>(botId, `/skills/${name}`, { method: "DELETE" });
}

// Channels
export async function listChannels(botId: string) {
  return botReq<Channel[]>(botId, "/channels");
}

export async function addChannel(botId: string, data: Record<string, unknown>) {
  return botReq<unknown>(botId, "/channels", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function removeChannel(botId: string, channel: string, account?: string) {
  const query = account ? `?account=${encodeURIComponent(account)}` : "";
  return botReq<unknown>(botId, `/channels/${channel}${query}`, { method: "DELETE" });
}

// Channel Pairing
export async function listPairingRequests(botId: string, channel: string) {
  return botReq<unknown[]>(botId, `/channels/${channel}/pairing`);
}

export async function approvePairing(botId: string, channel: string, code: string) {
  return botReq<unknown>(botId, `/channels/${channel}/pairing/approve`, {
    method: "POST",
    body: JSON.stringify({ code }),
  });
}

export async function revokePairing(botId: string, channel: string, userId: string) {
  return botReq<unknown>(botId, `/channels/${channel}/pairing/revoke`, {
    method: "POST",
    body: JSON.stringify({ user_id: userId }),
  });
}

export async function listPairedUsers(botId: string, channel: string) {
  return botReq<unknown[]>(botId, `/channels/${channel}/pairing/users`);
}

// WeChat
export async function wechatLoginStart(botId: string, name?: string) {
  return botReq<{ qrcode_url: string; message: string }>(botId, "/channels/wechat/login", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export async function wechatLoginStatus(botId: string) {
  return botReq<{ status: string; connected: boolean; message: string; account_id?: string }>(
    botId, "/channels/wechat/login/status"
  );
}

export async function wechatListAccounts(botId: string) {
  return botReq<{ channel: string; accounts: unknown[] }>(botId, "/channels/wechat/accounts");
}

export async function wechatRemoveAccount(botId: string, accountId: string) {
  return botReq<unknown>(botId, `/channels/wechat/accounts/${accountId}`, { method: "DELETE" });
}

// Devices
export async function listDevices(botId: string, status?: string, clientMode?: string) {
  const params = new URLSearchParams();
  if (status) params.set("status", status);
  if (clientMode) params.set("client_mode", clientMode);
  const query = params.toString() ? `?${params.toString()}` : "";
  return botReq<{ bot_id: string; devices: Device[] }>(botId, `/devices${query}`);
}

export async function approveDevice(botId: string, requestId: string) {
  return botReq<unknown>(botId, `/devices/${requestId}/approve`, { method: "POST" });
}

export async function revokeDevice(botId: string, deviceId: string) {
  return botReq<unknown>(botId, `/devices/${deviceId}`, { method: "DELETE" });
}

// Model Providers
export async function listModelProviders(botId: string) {
  return botReq<ModelProvider[]>(botId, "/config/models");
}

export async function addModelProvider(botId: string, data: ModelProvider) {
  return botReq<ModelProvider>(botId, "/config/models", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function getModelProvider(botId: string, provider: string) {
  return botReq<ModelProvider>(botId, `/config/models/${provider}`);
}

export async function updateModelProvider(botId: string, provider: string, data: ModelProvider) {
  return botReq<ModelProvider>(botId, `/config/models/${provider}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteModelProvider(botId: string, provider: string) {
  return botReq<{ message: string }>(botId, `/config/models/${provider}`, { method: "DELETE" });
}

// Agent Defaults
export async function getAgentDefaults(botId: string) {
  return botReq<AgentDefaults>(botId, "/config/defaults");
}

export async function setAgentDefaults(botId: string, data: AgentDefaults) {
  return botReq<AgentDefaults>(botId, "/config/defaults", {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

// Raw Config
export async function getBotRawConfig(botId: string) {
  return botReq<Record<string, unknown>>(botId, "/config/raw");
}

export async function updateBotRawConfig(
  botId: string,
  data: Record<string, unknown>,
  mode: "merge" | "replace" = "merge"
) {
  return botReq<Record<string, unknown>>(botId, `/config/raw?mode=${mode}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

// Admin Config
export async function getAdminConfig() {
  return adminReq<{ bot_domain_template: string }>("/config");
}
