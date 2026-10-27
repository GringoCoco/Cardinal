import { useState } from "preact/hooks";
import { Preferences, Session, syncDevice, tg, unpair } from "../api";
import { AppShell } from "../components/app-shell";
import { Badge, Button, Card, Header, Screen, Section, Toggle } from "../components/ui";
import { Navigate } from "../routes";

export function SettingsScreen({ preferences, update, online, device, sessions, requests, navigate }: {
  preferences: Preferences;
  update: (next: Preferences) => void;
  online: boolean;
  device?: string;
  sessions: Session[];
  requests: number;
  navigate: Navigate;
}) {
  return (
    <AppShell title="Cardinal" active="settings" requests={requests} navigate={(key) => navigate({ name: key })}>
      <h1>Настройки</h1>
      <Card className="device-card"><div><strong>Cardinal</strong><code>{device ?? "macbook"} · {online ? "на связи" : "нет связи"}</code><small>Коннектор · {sessions.length} сессий</small></div><Badge tone={online ? "green" : "red"}>{online ? "online" : "offline"}</Badge></Card>
      <div className="settings-menu">
        <button onClick={() => navigate({ name: "computer" })}><span><strong>Компьютер и провайдеры</strong><small>Подключение и источники сессий</small></span><b>›</b></button>
        <button onClick={() => navigate({ name: "notifications" })}><span><strong>Уведомления</strong><small>Запросы, завершения и ошибки</small></span><b>›</b></button>
      </div>
      <Section title="Предпочтения">
        <Toggle checked={preferences.confirmStop} onChange={(value) => update({ ...preferences, confirmStop: value })} label="Подтверждать остановку" hint="Защита от случайного завершения" />
        <Toggle checked={preferences.hideCompleted} onChange={(value) => update({ ...preferences, hideCompleted: value })} label="Скрывать завершённые" hint="В списке сессий" />
        <Toggle checked={preferences.showTimes} onChange={(value) => update({ ...preferences, showTimes: value })} label="Показывать время вывода" hint="В карточках и терминале" />
      </Section>
      <p className="version">Cardinal / версия 0.2.0<br />Ваши агенты работают на компьютере. Телефон — окно в их работу.</p>
    </AppShell>
  );
}

export function ComputerScreen({ preferences, update, online, device, sessions, navigate }: {
  preferences: Preferences;
  update: (next: Preferences) => void;
  online: boolean;
  device?: string;
  sessions: Session[];
  navigate: Navigate;
}) {
  const toggle = (provider: string) => update({ ...preferences, providers: { ...preferences.providers, [provider]: !preferences.providers[provider] } });
  return (
    <Screen>
      <Header title="Компьютер и провайдеры" onBack={() => navigate({ name: "settings" })} />
      <main className="content">
        <h1>{device ?? "macbook"}</h1>
        <Card className="device-card"><div><strong>{online ? "Коннектор подключён" : "Нет связи"}</strong><small>Защищённый канал · сессии обнаружены автоматически</small></div><Badge tone={online ? "green" : "red"}>{online ? "✓" : "×"}</Badge></Card>
        <Button onClick={() => syncDevice()}>Переподключить</Button>
        <Section title="Источники">
          {Object.keys(preferences.providers).map((provider) => {
            const count = sessions.filter((session) => session.provider === provider).length;
            return <Toggle checked={preferences.providers[provider]} onChange={() => toggle(provider)} label={provider} hint={`Доступен · ${count} сессий`} />;
          })}
        </Section>
        <Button variant="primary" onClick={() => navigate({ name: "filters" })}>Управление провайдерами</Button>
        <Button variant="ghost" onClick={() => navigate({ name: "disconnect" })}>Отключить компьютер</Button>
      </main>
    </Screen>
  );
}

export function NotificationsScreen({ preferences, update, navigate }: { preferences: Preferences; update: (next: Preferences) => void; navigate: Navigate }) {
  const set = (key: keyof Preferences["notifications"], value: boolean) => update({ ...preferences, notifications: { ...preferences.notifications, [key]: value } });
  return (
    <Screen>
      <Header title="Уведомления" onBack={() => navigate({ name: "settings" })} />
      <main className="content">
        <h1>Не пропустите важное</h1>
        <p className="lead">Настройки сохраняются сразу, отдельно для этого телефона.</p>
        <Toggle checked={preferences.notifications.permissions} onChange={(v) => set("permissions", v)} label="Запросы подтверждения" hint="Когда агенту нужно ваше решение" />
        <Toggle checked={preferences.notifications.completed} onChange={(v) => set("completed", v)} label="Завершение задачи" hint="Итог работы и результаты тестов" />
        <Toggle checked={preferences.notifications.errors} onChange={(v) => set("errors", v)} label="Ошибки сессий" hint="Когда работа не может продолжаться" />
        <Card className="warning-card"><strong>Системное разрешение</strong><p>События по-прежнему видны внутри Cardinal, но оповещения на телефон зависят от Telegram.</p><Button onClick={() => tg?.openLink?.("app-settings:")}>Открыть настройки телефона</Button></Card>
      </main>
    </Screen>
  );
}

export function DisconnectScreen({ device, sessions, navigate }: { device?: string; sessions: Session[]; navigate: Navigate }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const disconnect = async () => {
    setBusy(true);
    try { await unpair(); location.reload(); } catch (e) { setError((e as Error).message); setBusy(false); }
  };
  return (
    <Screen>
      <Header title="Отключение" onBack={() => navigate({ name: "computer" })} />
      <main className="content confirm-screen">
        <div className="eyebrow amber">Подтвердите действие</div>
        <h1>Отключить компьютер?</h1>
        <Card><strong>{device ?? "macbook"}</strong><p>{sessions.length} сессий обнаружено</p></Card>
        <Card className="warning-card"><strong>Мониторинг будет прекращён</strong><p>Вы перестанете получать новые запросы и уведомления с этого компьютера. Агенты продолжат работать локально.</p></Card>
        <Button variant="danger" disabled={busy} onClick={disconnect}>Отключить</Button>
        <Button onClick={() => navigate({ name: "computer" })}>Отмена</Button>
        {error && <p className="error-text">{error}</p>}
      </main>
    </Screen>
  );
}
