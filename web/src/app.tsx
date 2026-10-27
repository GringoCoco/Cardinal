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
  return new Date(ts).toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
}

function stateIcon(state?: string): string {
  switch (state) {
    case "started": return "🟡";
    case "ok": return "🟢";
    case "error": return "🔴";
    case "interrupted": return "⏹";
    case "allow": return "✅";
    case "deny": return "⛔";
    case "sent": return "🚀";
    default: return "•";
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
    <div class="card perm">
      <div class="row">
        <span class="agent">{item.agent}</span>
        {item.project && <span class="project">· {item.project}</span>}
        <span class="time">{fmtTime(item.time)}</span>
      </div>
      <div class="summary">{item.summary}</div>
      {done === null ? (
        <>
          <div class="buttons">
            <button class="allow" disabled={busy} onClick={() => decide(true)}>✅ Разрешить</button>
            <button class="deny" disabled={busy} onClick={() => decide(false)}>⛔ Запретить</button>
          </div>
          {err && <div class="err">{err}</div>}
        </>
      ) : (
        <div class="resolved">{done ? "✅ разрешено" : "⛔ запрещено"}</div>
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
      <h1>Cardinal</h1>
      <p>Код пары выдаёт терминал Mac:</p>
      <pre>cardinald connect</pre>
      <form onSubmit={submit}>
        <input
          placeholder="6-значный код"
          value={code}
          inputmode="numeric"
          onInput={(e) => setCode((e.target as HTMLInputElement).value)}
        />
        <button disabled={busy || code.trim().length < 4}>Привязать</button>
      </form>
      {err && <div class="err">{err}</div>}
    </div>
  );
}

const AGENTS = [
  { key: "claude", label: "Claude" },
  { key: "opencode", label: "OpenCode" },
  { key: "codex", label: "Codex" },
];

export function App() {
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
            // resolved: убрать карточку из «Ждут решения», в ленту — итог
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
      setTimeout(() => location.reload(), 3000); // relay/daemon мог умереть — перезайдём
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
    } catch (e) {
      setPromptErr((e as Error).message);
    }
  };

  const onResolve = (permId: string, allow: boolean) => {
    setFeed((prev) => prev.filter((f) => f.permId !== permId));
  };

  if (paired === null) return <div class="loading">…</div>;
  if (paired === false) return <PairScreen onPaired={() => setPaired(true)} />;

  return (
    <div class="app">
      <header>
        <span class="logo">🕊️ Cardinal</span>
        <span class={"status " + (deviceOnline ? "on" : "off")}>
          {deviceOnline ? "● Mac на связи" : "○ Mac не в сети"}
        </span>
      </header>

      {pending.length > 0 && (
        <section>
          <h2>Ждут решения</h2>
          {pending.map((p) => <PermCard item={p} onResolve={onResolve} />)}
        </section>
      )}

      <section>
        <h2>Промпт агенту</h2>
        <div class="pills">
          {AGENTS.map((a) => (
            <button class={"pill" + (agent === a.key ? " active" : "")} onClick={() => setAgent(a.key)}>
              {a.label}
            </button>
          ))}
        </div>
        <textarea
          rows={2}
          placeholder={`Что ${AGENTS.find((a) => a.key === agent)?.label} должен сделать?`}
          value={prompt}
          onInput={(e) => setPrompt((e.target as HTMLTextAreaElement).value)}
        />
        <button class="send" disabled={!prompt.trim()} onClick={sendPrompt}>Отправить</button>
        {promptErr && <div class="err">{promptErr}</div>}
      </section>

      <section>
        <h2>Лента</h2>
        {events.length === 0 && <div class="empty">Пока тихо. События агентов появятся здесь.</div>}
        {events.map((e) => (
          <div class="card event">
            <div class="row">
              <span>{stateIcon(e.state)}</span>
              <span class="agent">{e.agent}</span>
              {e.project && <span class="project">· {e.project}</span>}
              <span class="time">{fmtTime(e.time)}</span>
            </div>
            {e.summary && <div class="summary">{e.summary}</div>}
          </div>
        ))}
      </section>
    </div>
  );
}
