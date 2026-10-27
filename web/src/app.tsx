import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import {
  FeedItem,
  groupSessions,
  me,
  pair,
  ready,
  resolve,
  send,
  toFeedItem,
  wsURL,
} from "./api";

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

function PermCard({ item }: { item: FeedItem }) {
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
    <div className={"card perm" + (done === null ? " pending" : "")}>
      <div className="perm-top">
        <span className="perm-icon">⚠</span>
        <div className="grow">
          <div className="agent">{agentLabel(item.agent)}</div>
          {item.project && <div className="project">· {item.project}</div>}
          <div className="summary">{item.summary}</div>
        </div>
        <span className="time">{fmtTime(item.time)}</span>
      </div>
      {done === null ? (
        <>
          <div className="buttons">
            <button className="allow" disabled={busy} onClick={() => decide(true)}>✅ Разрешить</button>
            <button className="deny" disabled={busy} onClick={() => decide(false)}>⛔ Запретить</button>
          </div>
          {err && <div className="err">{err}</div>}
        </>
      ) : (
        <div className="resolved">{done ? "✓ Решение принято — разрешено" : "✕ Решение принято — запрещено"}</div>
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
    <div className="pair">
      <div className="pair-core">
        <div className="orb"><div className="orb-core" /></div>
        <h1>CARDINAL</h1>
        <p className="tag">агентские события → решения, которые живут на Mac</p>
      </div>
      <form onSubmit={submit} className="pair-form">
        <input
          placeholder="КОД ПАРЫ"
          value={code}
          inputMode="numeric"
          maxLength={6}
          onInput={(e) => setCode((e.target as HTMLInputElement).value)}
        />
        <button disabled={busy || code.trim().length < 4}>Привязать</button>
      </form>
      {err && <div className="err">{err}</div>}
    </div>
  );
}

const AGENTS = [
  { key: "claude", label: "Claude Code" },
  { key: "opencode", label: "OpenCode" },
  { key: "codex", label: "Codex" },
];

function isRunning(state?: string): boolean {
  return state === "started" || state === "sent";
}

export function App() {
  const [tab, setTab] = useState<"feed" | "prompt">("feed");
  const [paired, setPaired] = useState<boolean | null>(null);
  const [deviceOnline, setDeviceOnline] = useState(false);
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [agent, setAgent] = useState("claude");
  const [prompt, setPrompt] = useState("");
  const [promptErr, setPromptErr] = useState("");
  const conn = useRef<{ online: boolean; ws: WebSocket | null }>({ online: false, ws: null });

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

  const groups = useMemo(() => groupSessions(feed), [feed]);

  const sendPrompt = async () => {
    const text = prompt.trim();
    if (!text) return;
    setPromptErr("");
    try {
      await send("prompt", { agent, text });
      setPrompt("");
      setFeed((prev) => [
        { kind: "info", id: `q-${Date.now()}`, time: Date.now(), agent, state: "sent", summary: text.slice(0, 80), sid: `q-${agent}-${Date.now()}` },
        ...prev,
      ]);
      setTab("feed");
    } catch (e) {
      setPromptErr((e as Error).message);
    }
  };

  if (paired === null) return <div className="loading">…</div>;
  if (paired === false) return <PairScreen onPaired={() => setPaired(true)} />;

  const orbPulse = deviceOnline ? "on" : "off";

  return (
    <div className="screen">
      <div className="ambient"><div className="glow g1" /><div className="glow g2" /></div>

      <header className="hud">
        <div className="logo">
          <div className="logo-mark"><div className={"orb-core " + orbPulse} /></div>
          <span className="name">CARDINAL</span>
        </div>
        <div className={"status " + orbPulse}>
          <span className="dot" />
          {deviceOnline ? "Mac на связи" : "Mac не в сети"}
        </div>
      </header>

      {tab === "feed" && (
        <main className="tab">
          {groups.length === 0 && (
            <div className="empty">
              <div className="empty-orb">◯</div>
              <p>Тихо. Ожидаю события…</p>
            </div>
          )}
          {groups.map((g) => {
            const running = isRunning(g.state);
            const pendingItem = g.items.find((i) => i.kind === "perm");
            return (
              <div className={"card session" + (running ? " live" : "")} key={g.sid}>
                <div className="row">
                  <span className="eicon">{stateIcon(g.state)}</span>
                  <span className="agent">{agentLabel(g.agent)}</span>
                  {g.project && <span className="project">· {g.project}</span>}
                  <span className="time">{fmtTime(g.items[0]?.time ?? Date.now())}</span>
                </div>
                {g.items[g.items.length - 1]?.summary && <div className="summary">{g.items[g.items.length - 1]?.summary}</div>}
                {pendingItem ? (
                  <PermCard item={pendingItem} />
                ) : running ? (
                  <div className="running">◈ генерирует…</div>
                ) : null}
              </div>
            );
          })}
        </main>
      )}

      {tab === "prompt" && (
        <main className="tab">
          <section className="section">
            <h2 className="sec-title">Промпт агенту</h2>
            <div className="pills">
              {AGENTS.map((a) => (
                <button key={a.key} className={"pill" + (agent === a.key ? " active" : "")} onClick={() => setAgent(a.key)}>
                  {a.label}
                </button>
              ))}
            </div>
            <textarea
              rows={3}
              placeholder={`Что должен делать ${agentLabel(agent)}?`}
              value={prompt}
              onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)}
            />
            <button className="send" disabled={!prompt.trim()} onClick={sendPrompt}>Отправить</button>
            {promptErr && <div className="err">{promptErr}</div>}
          </section>
        </main>
      )}

      <nav className="tabbar">
        <button className={tab === "feed" ? "tab active" : "tab"} onClick={() => setTab("feed")}>
          <span className="ticon">◎</span><span className="tlabel">События</span>
        </button>
        <button className={tab === "prompt" ? "tab active" : "tab"} onClick={() => setTab("prompt")}>
          <span className="ticon">➜</span><span className="tlabel">Промпт</span>
        </button>
      </nav>
    </div>
  );
}
