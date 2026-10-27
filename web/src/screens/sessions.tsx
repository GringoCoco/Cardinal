import { Preferences, Session, syncDevice } from "../api";
import { AppShell } from "../components/app-shell";
import { SessionCard } from "../components/session-card";
import { Badge, Button, Card, EmptyState, Header, Screen, Section } from "../components/ui";
import { Navigate } from "../routes";

const statusLabels: Record<string, string> = {
  running: "Работает", waiting: "Ждёт решения", paused: "На паузе",
  completed: "Завершено", error: "Ошибка", stopped: "Остановлено",
};

export function SessionsScreen({ sessions, preferences, online, device, requests, navigate }: {
  sessions: Session[];
  preferences: Preferences;
  online: boolean;
  device?: string;
  requests: number;
  navigate: Navigate;
}) {
  const filtered = sessions.filter((session) =>
    preferences.providers[session.provider] !== false &&
    preferences.statuses[session.state] !== false &&
    (!preferences.directories.length || preferences.directories.includes(session.cwd)) &&
    !(preferences.hideCompleted && ["completed", "stopped"].includes(session.state)),
  );
  const working = sessions.filter((s) => s.state === "running").length;
  const waiting = sessions.filter((s) => s.state === "waiting").length;
  const idle = sessions.length - working - waiting;
  return (
    <AppShell title="Cardinal" active="sessions" requests={requests} navigate={(key) => navigate({ name: key })}
      action={<button className="header-link" onClick={() => syncDevice()}>↻</button>}>
      <div className="eyebrow">{device ?? "macbook"} · {online ? "на связи" : "нет связи"}</div>
      <h1>Ваши агенты</h1>
      <p className="lead">Обновлено сейчас · {sessions.length} сессий</p>
      <div className="stats">
        <Card><strong>{working}</strong><small>Работает</small></Card>
        <Card><strong>{waiting}</strong><small>Ждёт решения</small></Card>
        <Card><strong>{idle}</strong><small>Без действия</small></Card>
      </div>
      <button className="filter-row" onClick={() => navigate({ name: "filters" })}>
        <span>Фильтры / директории</span><span>Все директории ›</span>
      </button>
      {!online && (
        <EmptyState title="Компьютер недоступен" text="Проверьте, что локальный connector Cardinal запущен.">
          <Button variant="primary" onClick={() => syncDevice()}>Повторить подключение</Button>
          <Button onClick={() => navigate({ name: "computer" })}>Настройки подключения</Button>
        </EmptyState>
      )}
      {online && filtered.length === 0 && (
        <EmptyState title="Сессий пока нет" text="Запустите агента в нужной директории на компьютере.">
          <Card className="terminal-hint"><code>$ cd ~/project<br />$ opencode</code></Card>
          <Button variant="primary" onClick={() => syncDevice()}>Обновить</Button>
          <Button onClick={() => navigate({ name: "filters" })}>Изменить фильтры</Button>
        </EmptyState>
      )}
      <div className="session-list">
        {filtered.sort((a, b) => b.updatedAt - a.updatedAt).map((session) =>
          <SessionCard key={session.id} session={session} showTime={preferences.showTimes} onClick={() => navigate({ name: "session", id: session.id })} />,
        )}
      </div>
    </AppShell>
  );
}

export function FiltersScreen({ sessions, preferences, update, navigate }: {
  sessions: Session[];
  preferences: Preferences;
  update: (next: Preferences) => void;
  navigate: Navigate;
}) {
  const directories = [...new Set(sessions.map((s) => s.cwd).filter(Boolean))];
  const patch = (key: "providers" | "statuses", name: string) =>
    update({ ...preferences, [key]: { ...preferences[key], [name]: !preferences[key][name] } });
  const toggleDir = (dir: string) => update({
    ...preferences,
    directories: preferences.directories.includes(dir)
      ? preferences.directories.filter((value) => value !== dir)
      : [...preferences.directories, dir],
  });
  return (
    <Screen>
      <Header title="Фильтры" onBack={() => navigate({ name: "sessions" })} />
      <main className="content">
        <h1>Найдите нужную сессию</h1>
        <Section title="Провайдер">
          <div className="chips">{Object.keys(preferences.providers).map((key) =>
            <button className={preferences.providers[key] ? "selected" : ""} onClick={() => patch("providers", key)}>{key}</button>,
          )}</div>
        </Section>
        <Section title="Статус">
          <div className="chips">{Object.keys(preferences.statuses).map((key) =>
            <button className={preferences.statuses[key] ? "selected" : ""} onClick={() => patch("statuses", key)}>{statusLabels[key]}</button>,
          )}</div>
        </Section>
        <Section title="Директория">
          <div className="choice-list">
            {directories.map((dir) => <button onClick={() => toggleDir(dir)}><code>{dir.replace(/^\/Users\/[^/]+/, "~")}</code>{preferences.directories.includes(dir) && <Badge tone="green">✓</Badge>}</button>)}
            {!directories.length && <p className="muted">Директории появятся после первого события агента.</p>}
          </div>
        </Section>
        <div className="sticky-actions">
          <Button variant="primary" onClick={() => navigate({ name: "sessions" })}>Применить</Button>
          <Button onClick={() => update({ ...preferences, directories: [] })}>Сбросить</Button>
        </div>
      </main>
    </Screen>
  );
}
