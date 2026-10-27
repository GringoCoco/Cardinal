import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import {
  FeedItem,
  initData,
  me,
  pair,
  ready,
  resolve,
  send,
  toFeedItem,
  wsURL,
} from "./api";

type Conn = { online: boolean; ws: WebSocket | null };

function fmtTime(ts: number): string {
  const d = new Date(ts);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  const ss = String(d.getSeconds()).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}

function stateIcon(state?: string): string {
  switch (state) {
    case "started": return "◈";
    case "ok": return "●";
    case "error": return "✕";
    case "interrupted": return "⏹";
    case "allow": return "✓";
    case "deny": return "✕";
    case "sent": return "➜";
    default: return "•";
  }
}

function agentLabel(agent?: string): string {
  switch (agent) {
    case "claude": return "Claude Code";
    case "opencode": return "OpenCode";
    case "codex": return "Codex";
    default: return agent ?? "—";
  }
}

function PermCard({ item, onResolve }: { item: FeedItem; onResolve: (id: string, allow: boolean) => void }) {
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<null | boolean>(null);
  const [err, setErr] = useState("");
  const decide = async (allow: boolean) => {
    setBusy(true);
    setErr("");
    try {
      await resolve(item.permId!, allow);
      setDone(allow);
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  };
  return (
    <div class={"card perm" + (done === null ? " pending" : "")}>
      <div class="perm-top">
        <span class="perm-icon">{item.agent === "opencode" || item.agent === "claude" ? "⚠" : "◈"}</span>
        <div class="grow">
          <div class="agent">{agentLabel(item.agent)}</div>
          {item.project && <div class="project">· {item.project}</div>}
          <div class="summary">{item.summary}</div>
        </div>
        <span class="time">{fmtTime(item.time)}</span>
      </div>
      {done === null ? (
        <>
          <div class="buttons">
            <button class="allow" disabled={busy} onClick={() => decide(true)}>✅ Разрешить</button>
            <button class="deny" disabled={busy} onClick={() => decide(false)}>⛔ Запретить</button>
          </div>
          {err && <div class="err">{err}</div>}
        </>
      ) : (
        <div class="resolved">{done ? "✓ Решение принято — разрешено" : "✕ Решение принято — запрещено"}</div>
      )}
    </div>
  );
}

function PairScreen({ onPaired }: { onPaired: () => void }) {
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await pair(code.trim());
      onPaired();
    } catch (ex) {
      setErr((ex as Error).message);
      setBusy(false);
    }
  };
  return (
    <div class="pair">
      <div class="pair-core">
        <div class="orb"><div class="orb-core" /></div>
        <h1>CARDINAL</h1>
        <p class="tag">агентские события → решения, которые живут на Mac</p>
      </div>
      <form onSubmit={submit} class="pair-form">
        <input
          placeholder="КОД ПАРЫ"
          value={code}
          inputMode="numeric"
          maxLength={6}
          onInput={(e) => setCode((e.target as HTMLInputElement).value)}
        />
        <button disabled={busy || code.trim().length < 4}>Привязать</button>
      </form>
      {err && <div class="err">{err}</div>}
    </div>
  );
}

const AGENTS = [
  { key: "claude", label: "Claude Code" },
  { key: "opencode", label: "OpenCode" },
  { key: "codex", label: "Codex" },
];

export function App() {
  const [tab, setTab] = useState<"feed" | "prompt">("feed");
  const [paired, setPaired] = useState<boolean | null>(null); // null = проверяем
  const [deviceOnline, setDeviceOnline] = useState(false);
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [agent, setAgent] = useState("claude");
  const [prompt, setPrompt] = useState("");
  const [promptErr, setPromptErr] = useState("");
  const conn = useRef<Conn>({ online: false, ws: null });

  useEffect(() => {
    ready();
    me()
      .then((m) => {
        setPaired(m.paired);
        setDeviceOnline(!!m.online);
      })
      .catch(() => setPaired(false));
  }, []);

  useEffect(() => {
    if (paired !== true) return;
    let closed = false;
    const ws = new WebSocket(wsURL());
    conn.current.ws = ws;
    ws.onopen = () => {
      conn.current.online = true;
      setDeviceOnline(true);
    };
    ws.onmessage = (ev) => {
      try {
        const env = JSON.parse(ev.data);
        const item = toFeedItem(env);
        if (item) {
          setFeed((prev) => {
            let next = prev;
            if (item.kind === "info" && item.permId) {
              next = prev.filter((f) => f.permId !== item.permId);
            }
            return [item, ...next].slice(0, 50);
          });
        }
      } catch {}
    };
    ws.onclose = () => {
      if (closed) return;
      conn.current.online = false;
      setDeviceOnline(false);
      setTimeout(() => location.reload(), 3000);
    };
    return () => {
      closed = true;
      ws.close();
    };
  }, [paired]);

  const pending = useMemo(() => feed.filter((f) => f.kind === "perm"), [feed]);
  const events = useMemo(() => feed.filter((f) => f.kind !== "perm"), [feed]);

  const sendPrompt = async () => {
    const text = prompt.trim();
    if (!text) return;
    setPromptErr("");
    try {
      await send("prompt", { agent, text });
      setPrompt("");
      setFeed((prev) => [
        { kind: "info", id: `q-${Date.now()}`, time: Date.now(), agent, state: "sent", summary: text.slice(0, 80) },
        ...prev,
      ]);
      setTab("feed");
    } catch (e) {
      setPromptErr((e as Error).message);
    }
  };

  const onResolve = (permId: string, allow: boolean) => {
    setFeed((prev) => prev.filter((f) => f.permId !== permId));
  };

  if (paired === null) return <div class="loading">…</div>;
  if (paired === false) return <PairScreen onPaired={() => setPaired(true)} />;

  const orbPulse = deviceOnline ? "on" : "off";

  return (
    <div class="screen">
      <div class="ambient"><div class="glow g1" /><div class="glow g2" /></div>

      <header class="hud">
        <div class="logo">
          <div class="logo-mark"><div class={"orb-core " + orbPulse} /></div>
          <span class="name">CARDINAL</span>
        </div>
        <div class={"status " + orbPulse}>
          <span class="dot" />
          {deviceOnline ? "Mac на связи" : "Mac не в сети"}
        </div>
      </header>

      {tab === "feed" && (
        <main class="tab">
          {pending.length > 0 && (
            <section class="section pending-wrap">
              <h2 class="sec-title">Ждут решения</h2>
              {pending.map((p) => <PermCard key={p.permId ?? p.id} item={p} onResolve={onResolve} />)}
            </section>
          )}
          <section class="section">
            <h2 class="sec-title">События агентов</h2>
            {events.length === 0 && (
              <div class="empty">
                <div class="empty-orb">◯</div>
                <p>Тихо. Ожидаю события…</p>
              </div>
            )}
            {events.map((e) => (
              <div class="card event" key={e.id}>
                <div class="row">
                  <span class="eicon">{stateIcon(e.state)}</span>
                  <span class="agent">{agentLabel(e.agent)}</span>
                  {e.project && <span class="project">· {e.project}</span>}
                  <span class="time">{fmtTime(e.time)}</span>
                </div>
                {e.summary && <div class="summary">{e.summary}</div>}
              </div>
            ))}
          </section>
        </main>
      )}

      {tab === "prompt" && (
        <main class="tab">
          <section class="section">
            <h2 class="sec-title">Промпт агенту</h2>
            <div class="pills">
              {AGENTS.map((a) => (
                <button key={a.key} class={"pill" + (agent === a.key ? " active" : "")} onClick={() => setAgent(a.key)}>
                  {a.label}
                </button>
              ))}
            </div>
            <textarea
              rows={3}
              placeholder={`Что должен сделать ${agentLabel(agent)}?`}
              value={prompt}
              onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)}
            />
            <button class="send" disabled={!prompt.trim()} onClick={sendPrompt}>Отправить</button>
            {promptErr && <div class="err">{promptErr}</div>}
          </section>
        </main>
      )}

      <nav class="tabbar">
        <button class={tab === "feed" ? "tab active" : "tab"} onClick={() => setTab("feed")}>
          <span class="ticon">◎</span><span class="tlabel">События</span>
        </button>
        <button class={tab === "prompt" ? "tab active" : "tab"} onClick={() => setTab("prompt")}>
          <span class="ticon">➜</span><span class="tlabel">Промпт</span>
        </button>
      </nav>
    </div>
  );
}
