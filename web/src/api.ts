// Typed transport and the single event reducer used by every Mini App screen.

export type Envelope = { v?: number; to?: string; type: string; body?: any };

export type SessionState =
  | "running"
  | "waiting"
  | "paused"
  | "completed"
  | "error"
  | "stopped";

export type Session = {
  id: string;
  agent: string;
  provider: string;
  project: string;
  cwd: string;
  title: string;
  activity: string;
  state: SessionState;
  tool?: string;
  startedAt: number;
  updatedAt: number;
  hasOutput: boolean;
  output?: string;
  outputError?: string;
  actionError?: string;
};

export type Permission = {
  id: string;
  sessionId: string;
  agent: string;
  project: string;
  cwd: string;
  summary: string;
  createdAt: number;
};

export type Decision = Permission & { decision: "allow" | "deny" | "expired"; resolvedAt: number };

export type AppStore = {
  sessions: Record<string, Session>;
  permissions: Record<string, Permission>;
  decisions: Decision[];
  notice?: string;
};

export type Me = {
  paired: boolean;
  online?: boolean;
  device?: string;
  name?: string;
  snapshot_n?: number;
};

export type Preferences = {
  providers: Record<string, boolean>;
  statuses: Record<string, boolean>;
  directories: string[];
  notifications: { permissions: boolean; completed: boolean; errors: boolean };
  confirmStop: boolean;
  hideCompleted: boolean;
  showTimes: boolean;
};

export const emptyStore: AppStore = { sessions: {}, permissions: {}, decisions: [] };

export const defaultPreferences: Preferences = {
  providers: { opencode: true, codex: true, claude: true },
  statuses: { running: true, waiting: true, paused: true, completed: true, error: true, stopped: true },
  directories: [],
  notifications: { permissions: true, completed: true, errors: true },
  confirmStop: true,
  hideCompleted: false,
  showTimes: true,
};

export const tg = (globalThis as any).Telegram?.WebApp;
const queryInitData = new URLSearchParams(location.search).get("init_data") ?? "";
export const initData = tg?.initData || queryInitData;

export function ready() {
  try {
    tg?.ready();
    tg?.expand();
    tg?.setHeaderColor?.("#07100b");
    tg?.setBackgroundColor?.("#07100b");
  } catch {}
	// Query-вариант нужен для локального E2E; после чтения убираем подпись из
	// адресной строки, чтобы она не попала в referrer/скриншот.
	if (queryInitData) history.replaceState(null, "", location.pathname + location.hash);
}

export function wsURL(): string {
  const scheme = location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${location.host}/ws/app?init_data=${encodeURIComponent(initData)}`;
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(location.origin + path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${initData}`,
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `HTTP ${res.status}`);
  }
  if (res.status === 202 || res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const me = () => api<Me>("/api/me");

export const pair = (code: string) =>
  api<{ device: string; name: string }>("/api/pair", {
    method: "POST",
    body: JSON.stringify({ initData, code }),
  });

export const send = (type: string, body: any = {}) =>
  api<void>("/api/send", { method: "POST", body: JSON.stringify({ type, body }) });

export const resolvePermission = (id: string, allow: boolean) =>
  send("permission.resolve", { id, decision: allow ? "allow" : "deny" });

export const sessionCommand = (sessionId: string, action: "pause" | "resume" | "stop" | "output") =>
  send("session.command", { session_id: sessionId, action });

export const sendPrompt = (session: Pick<Session, "id" | "agent" | "provider" | "cwd">, text: string) =>
  send("prompt", { agent: session.provider || session.agent, text, session_id: session.id, cwd: session.cwd });

export const syncDevice = () => send("device.sync");
export const updateDeviceSettings = (preferences: Preferences) =>
  send("settings.update", { notifications: preferences.notifications });
export const unpair = () => api<{ ok: boolean }>("/api/unpair", { method: "POST", body: "{}" });

function sessionFromBody(body: any, previous?: Session): Session {
  const now = Date.now();
  return {
    id: body.session_id,
    agent: body.agent ?? previous?.agent ?? "agent",
    provider: body.provider ?? previous?.provider ?? normalizeProvider(body.agent),
    project: body.project ?? previous?.project ?? "",
    cwd: body.cwd ?? previous?.cwd ?? "",
    title: body.title ?? previous?.title ?? body.project ?? "Сессия агента",
    activity: body.activity ?? previous?.activity ?? "Агент работает",
    state: (body.state ?? previous?.state ?? "running") as SessionState,
    tool: body.tool ?? previous?.tool,
    startedAt: Number(body.started_at) || previous?.startedAt || now,
    updatedAt: Number(body.updated_at) || now,
    hasOutput: Boolean(body.has_output ?? previous?.hasOutput),
    output: previous?.output,
    outputError: previous?.outputError,
    actionError: previous?.actionError,
  };
}

function normalizeProvider(agent?: string): string {
  const value = (agent ?? "").toLowerCase();
  if (value.includes("open")) return "opencode";
  if (value.includes("codex")) return "codex";
  return "claude";
}

export function applyEnvelope(store: AppStore, env: Envelope): AppStore {
  const body = env.body ?? {};
  if (env.type === "notice.clear") return { ...store, notice: undefined };
  if ((env.type === "session.status" || env.type === "session.finished") && body.session_id) {
    const next = sessionFromBody(body, store.sessions[body.session_id]);
    return { ...store, sessions: { ...store.sessions, [next.id]: next } };
  }
  if (env.type === "permission.request" && body.id) {
    const permission: Permission = {
      id: body.id,
      sessionId: body.session_id ?? "",
      agent: body.agent ?? "agent",
      project: body.project ?? "",
      cwd: body.cwd ?? "",
      summary: body.summary ?? "Требуется разрешение",
      createdAt: Date.now(),
    };
    return { ...store, permissions: { ...store.permissions, [permission.id]: permission } };
  }
  if (env.type === "permission.resolved" && body.id) {
    const permission = store.permissions[body.id];
    const permissions = { ...store.permissions };
    delete permissions[body.id];
    const decision: Decision = {
      id: body.id,
      sessionId: body.session_id ?? permission?.sessionId ?? "",
      agent: body.agent ?? permission?.agent ?? "agent",
      project: permission?.project ?? "",
      cwd: permission?.cwd ?? "",
      summary: body.summary ?? permission?.summary ?? "Запрос обработан",
      createdAt: permission?.createdAt ?? Date.now(),
      decision: body.decision ?? "expired",
      resolvedAt: Date.now(),
    };
    return { ...store, permissions, decisions: [decision, ...store.decisions].slice(0, 20) };
  }
  if (env.type === "session.output" && body.session_id) {
    const previous = store.sessions[body.session_id];
    if (!previous) return store;
    return {
      ...store,
      sessions: {
        ...store.sessions,
        [previous.id]: { ...previous, output: body.output ?? previous.output, outputError: body.error },
      },
    };
  }
  if (env.type === "session.action.result" && body.session_id) {
    const previous = store.sessions[body.session_id];
    if (!previous) return { ...store, notice: body.error || "Команда выполнена" };
    return {
      ...store,
      notice: body.ok ? undefined : body.error,
      sessions: { ...store.sessions, [previous.id]: { ...previous, actionError: body.ok ? undefined : body.error } },
    };
  }
  if (env.type === "prompt.rejected") return { ...store, notice: body.reason ?? "Агент отклонил сообщение" };
  return store;
}

export function loadPreferences(): Preferences {
  try {
    const saved = JSON.parse(localStorage.getItem("cardinal.preferences") || "{}");
    return {
      ...defaultPreferences,
      ...saved,
      providers: { ...defaultPreferences.providers, ...(saved.providers ?? {}) },
      statuses: { ...defaultPreferences.statuses, ...(saved.statuses ?? {}) },
      notifications: { ...defaultPreferences.notifications, ...(saved.notifications ?? {}) },
    };
  } catch {
    return defaultPreferences;
  }
}

export function savePreferences(preferences: Preferences) {
  try {
    localStorage.setItem("cardinal.preferences", JSON.stringify(preferences));
  } catch {}
}
