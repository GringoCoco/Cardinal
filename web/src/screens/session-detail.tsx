import { useEffect, useState } from "preact/hooks";
import { Permission, Session, sendPrompt, sessionCommand } from "../api";
import { providerLabel, shortPath, stateMeta, timeAgo } from "../components/session-card";
import { Badge, Button, Card, ErrorBanner, Header, Screen, Section } from "../components/ui";
import { Navigate } from "../routes";

function SessionHero({ session }: { session: Session }) {
  const meta = stateMeta(session.state);
  return (
    <>
      <div className="card-kicker"><span>{providerLabel(session.provider)} · {session.id.slice(0, 7)}</span><Badge tone={meta.tone}>{meta.label}</Badge></div>
      <h1>{session.title}</h1>
      <code className="path">{shortPath(session.cwd)}</code>
      <div className="session-meta"><span>Обновлено<br /><strong>{timeAgo(session.updatedAt)}</strong></span><span>Проект<br /><strong>{session.project || "—"}</strong></span><span>Агент<br /><strong>{session.provider}</strong></span></div>
    </>
  );
}

export function SessionDetailScreen({ session, permission, confirmStop, navigate }: { session: Session; permission?: Permission; confirmStop: boolean; navigate: Navigate }) {
  const run = async (action: "pause" | "resume") => {
    await sessionCommand(session.id, action);
  };
  return (
    <Screen>
      <Header title={`Cardinal · ${session.id.slice(0, 7)}`} onBack={() => navigate({ name: "sessions" })} action={<span>•••</span>} />
      <main className="content detail">
        <SessionHero session={session} />
        <ErrorBanner text={session.actionError} />
        {session.state === "paused" && <Card className="state-box"><h2>На паузе</h2><p>Активность остановлена. Агент продолжит с текущего шага после возобновления.</p></Card>}
        {session.state === "error" && <Card className="state-box danger"><h2>Не удалось продолжить</h2><p>{session.activity}</p></Card>}
        <Section title="Ход работы">
          <div className="timeline"><span className="done">✓</span><p>{session.activity}<small>{new Date(session.updatedAt).toLocaleTimeString("ru", { hour: "2-digit", minute: "2-digit" })}</small></p></div>
          {session.tool && <div className="timeline"><span>›</span><p>{session.tool}<small>Текущее действие</small></p></div>}
        </Section>
        {permission && <Card className="warning-card" onClick={() => navigate({ name: "permission", id: permission.id })}><strong>Требуется ваше решение</strong><p>{permission.summary}</p><span>Открыть запрос ›</span></Card>}
        <Section title="Последний вывод">
          <Card className="terminal-preview"><pre>{session.output || (session.hasOutput ? "Нажмите «Полный терминал», чтобы загрузить вывод." : "Вывод этой сессии пока недоступен.")}</pre></Card>
        </Section>
        <div className="button-grid">
          {session.state === "running" && <Button onClick={() => run("pause")}>Приостановить</Button>}
          {session.state === "paused" && <Button variant="primary" onClick={() => run("resume")}>Возобновить</Button>}
          {session.state === "error" && <Button variant="primary" onClick={() => run("resume")}>Повторить</Button>}
          {!["completed", "stopped"].includes(session.state) && <Button variant="primary" onClick={() => navigate({ name: "message", id: session.id })}>Написать агенту</Button>}
          <Button onClick={() => navigate({ name: "output", id: session.id })}>Полный терминал</Button>
          {!["completed", "stopped"].includes(session.state) && <Button variant="ghost" onClick={() => confirmStop ? navigate({ name: "stop", id: session.id }) : sessionCommand(session.id, "stop")}>Завершить сессию</Button>}
          {["completed", "stopped"].includes(session.state) && <Button variant="primary" onClick={() => navigate({ name: "sessions" })}>К сессиям</Button>}
        </div>
      </main>
    </Screen>
  );
}

export function OutputScreen({ session, navigate }: { session: Session; navigate: Navigate }) {
  useEffect(() => { sessionCommand(session.id, "output").catch(() => {}); }, [session.id]);
  const copy = () => navigator.clipboard?.writeText(session.output || "");
  return (
    <Screen>
      <Header title={`Терминал · ${session.id.slice(0, 7)}`} onBack={() => navigate({ name: "session", id: session.id })} />
      <main className="content">
        <div className="eyebrow">{providerLabel(session.provider)} · {session.project}</div>
        <div className="output-toolbar"><span>Последняя активность · обновлено сейчас</span><Button onClick={copy}>Копировать</Button></div>
        <pre className="terminal-output">{session.outputError || session.output || "Загружаю вывод…"}</pre>
        <Card><strong>Приватный режим</strong><p>Для задач и ошибок доступна только локальная сессия. Cardinal не сохраняет историю вывода на relay.</p></Card>
      </main>
    </Screen>
  );
}

export function MessageScreen({ session, navigate }: { session: Session; navigate: Navigate }) {
  const [text, setText] = useState("");
  const [history, setHistory] = useState<{ role: string; text: string }[]>([]);
  const [error, setError] = useState("");
  const submit = async () => {
    if (!text.trim()) return;
    try {
      await sendPrompt(session, text.trim());
      setHistory((items) => [...items, { role: "Вы", text: text.trim() }]);
      setText("");
    } catch (e) { setError((e as Error).message); }
  };
  return (
    <Screen>
      <Header title={`Сообщение · ${session.id.slice(0, 7)}`} onBack={() => navigate({ name: "session", id: session.id })} />
      <main className="content">
        <div className="eyebrow">{providerLabel(session.provider)} · {session.project}</div>
        <Section title="Сеанс / история">
          <Card className="message agent"><strong>Агент · сейчас</strong><p>{session.activity}</p></Card>
          {history.map((item) => <Card className="message user"><strong>{item.role}</strong><p>{item.text}</p></Card>)}
        </Section>
        <label className="field-label">Новое сообщение</label>
        <textarea value={text} maxLength={2000} rows={6} placeholder="Добавьте инструкцию агенту…" onInput={(e) => setText((e.target as HTMLTextAreaElement).value)} />
        <small className="char-count">{text.length} / 2000</small>
        <Button variant="primary" disabled={!text.trim()} onClick={submit}>Отправить</Button>
        <ErrorBanner text={error} />
        <p className="muted">После доставки вернитесь к сессии. При потере связи сообщение не считается доставленным.</p>
      </main>
    </Screen>
  );
}

export function StopSessionScreen({ session, navigate }: { session: Session; navigate: Navigate }) {
  const [busy, setBusy] = useState(false);
  const stop = async () => {
    setBusy(true);
    await sessionCommand(session.id, "stop");
    navigate({ name: "session", id: session.id });
  };
  return (
    <Screen>
      <Header title="Завершение сессии" onBack={() => navigate({ name: "session", id: session.id })} />
      <main className="content confirm-screen">
        <div className="eyebrow amber">Подтвердите действие</div>
        <h1>Завершить сессию?</h1>
        <p className="lead">Текущая работа агента будет прервана на компьютере. Это не успешное завершение задачи.</p>
        <Card><SessionHero session={session} /></Card>
        <Card className="warning-card"><strong>Что произойдёт</strong><ul><li>Выполнение агента будет остановлено</li><li>Изменённые файлы останутся на диске</li><li>Локальный вывод останется на компьютере</li></ul></Card>
        <Button variant="danger" disabled={busy} onClick={stop}>Завершить</Button>
        <Button onClick={() => navigate({ name: "session", id: session.id })}>Отмена</Button>
      </main>
    </Screen>
  );
}
