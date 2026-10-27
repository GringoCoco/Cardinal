import { Session } from "../api";
import { Badge, Card } from "./ui";

export const providerLabel = (provider: string) => ({ opencode: "opencode", codex: "codex", claude: "claude" }[provider] ?? provider);

export const stateMeta = (state: Session["state"]) => {
  switch (state) {
    case "running": return { label: "Работает", tone: "green" as const };
    case "waiting": return { label: "Ждёт подтверждения", tone: "amber" as const };
    case "paused": return { label: "На паузе", tone: "neutral" as const };
    case "completed": return { label: "Завершено", tone: "green" as const };
    case "error": return { label: "Ошибка", tone: "red" as const };
    default: return { label: "Без действия", tone: "neutral" as const };
  }
};

export function shortPath(path: string) {
  if (!path) return "директория не указана";
  return path.replace(/^\/Users\/[^/]+/, "~");
}

export function timeAgo(value: number) {
  const seconds = Math.max(0, Math.floor((Date.now() - value) / 1000));
  if (seconds < 60) return `${seconds} сек`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} мин`;
  return `${Math.floor(minutes / 60)} ч`;
}

export function SessionCard({ session, onClick, showTime = true }: { session: Session; onClick: () => void; showTime?: boolean }) {
  const meta = stateMeta(session.state);
  return (
    <Card className={`session-card state-${session.state}`} onClick={onClick}>
      <div className="card-kicker">
        <span>{providerLabel(session.provider)} · {session.id.slice(0, 7)}</span>
        <Badge tone={meta.tone}>{meta.label}</Badge>
      </div>
      <h3>{session.title || session.project || "Сессия агента"}</h3>
      <code>{shortPath(session.cwd)}</code>
      <p>{session.activity}</p>
      {showTime && <small>Обновлено {timeAgo(session.updatedAt)} назад</small>}
    </Card>
  );
}
