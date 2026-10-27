import { Permission } from "../api";
import { Card } from "./ui";

export function PermissionCard({ permission, onClick }: { permission: Permission; onClick: () => void }) {
  return (
    <Card className="permission-card" onClick={onClick}>
      <div className="card-kicker"><span>{permission.agent}</span><span>Ждёт подтверждения</span></div>
      <h3>{permission.project || "Команда агента"}</h3>
      <code>{permission.summary}</code>
      <small>Нажмите, чтобы проверить команду и принять решение ›</small>
    </Card>
  );
}
