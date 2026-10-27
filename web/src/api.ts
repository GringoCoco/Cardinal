// Cardinal Mini App: сессии, лента, карточки разрешений, промпты агентам.
// Транспорт — relay: REST /api/* + WS /ws/app. Авторизация — Telegram initData.

export type Envelope = {
  v?: number;
  to?: string;
  type: string;
  body?: any;
};

export type FeedItem = {
  kind: "session" | "perm" | "info";
  id: string;
  time: number;
  agent?: string;
  state?: string;
  project?: string;
  summary?: string;
  permId?: string;
  sid?: string;
};

export type SessionGroup = {
  sid: string;
  agent?: string;
  project?: string;
  state?: string;
  items: FeedItem[];
};

type Me = { paired: boolean; online?: boolean; device?: string; name?: string };

export const tg = (globalThis as any).Telegram?.WebApp;
export const initData = tg?.initData ?? "";

export function ready() {
  try {
    tg?.ready();
    tg?.expand();
  } catch {}
}

export function apiBase(): string {
  return location.origin;
}

export function wsURL(): string {
  const scheme = location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${location.host}/ws/app?init_data=${encodeURIComponent(initData)}`;
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(apiBase() + path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${initData}`,
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`HTTP ${res.status}${text ? `: ${text}` : ""}`);
  }
  return res.json() as Promise<T>;
}

export function me(): Promise<Me> {
  return api<Me>("/api/me");
}

export function pair(code: string): Promise<{ device: string; name: string }> {
  return api("/api/pair", {
    method: "POST",
    body: JSON.stringify({ initData, code }),
  });
}

export function send(type: string, body: any): Promise<void> {
  return api("/api/send", {
    method: "POST",
    body: JSON.stringify({ type, body }),
  });
}

export function resolve(permId: string, allow: boolean): Promise<void> {
  return send("permission.resolve", { id: permId, decision: allow ? "allow" : "deny" });
}

export function toFeedItem(env: Envelope): FeedItem | null {
  const b = env.body ?? {};
  const time = Date.now();
  switch (env.type) {
    case "session.status":
      return {
        kind: "session",
        id: b.session_id ?? `s-${time}-${Math.random()}`,
        time,
        agent: b.agent,
        state: b.state,
        project: b.project,
        sid: b.session_id,
      };
    case "session.finished":
      return {
        kind: "session",
        id: b.session_id ?? `f-${time}-${Math.random()}`,
        time,
        agent: b.agent,
        state: b.state,
        project: b.project,
        sid: b.session_id,
      };
    case "permission.request":
      return {
        kind: "perm",
        id: `p-${b.id}`,
        time,
        agent: b.agent,
        project: b.project,
        summary: b.summary,
        permId: b.id,
        sid: b.session_id,
      };
    case "permission.resolved":
      return {
        kind: "info",
        id: `r-${time}-${Math.random()}`,
        time,
        agent: b.agent,
        state: b.decision,
        permId: b.id,
        summary: b.summary,
        sid: b.id,
      };
    case "prompt.accepted":
      return { kind: "info", id: `pa-${time}-${Math.random()}`, time, agent: b.agent, state: "sent", summary: `принято · ${b.cwd ?? ""}`, sid: b.sid };
    case "prompt.rejected":
      return { kind: "info", id: `pr-${time}-${Math.random()}`, time, agent: b.agent, state: "error", summary: b.reason, sid: b.sid };
    default:
      return null;
  }
}

export function groupSessions(feed: FeedItem[]): SessionGroup[] {
  const map = new Map<string, SessionGroup>();
  for (const it of feed) {
    const sid = it.sid ?? it.permId ?? it.id;
    if (!sid) continue;
    let g = map.get(sid);
    if (!g) {
      g = { sid, agent: it.agent, project: it.project, state: it.state, items: [] };
      map.set(sid, g);
    }
    g.items.push(it);
    g.state = it.state;
    g.agent = g.agent ?? it.agent;
    g.project = g.project ?? it.project;
  }
  const list = [...map.values()];
  list.sort((a, b) => (b.items[0]?.time ?? 0) - (a.items[0]?.time ?? 0));
  return list;
}
