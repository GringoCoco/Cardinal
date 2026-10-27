import { useState } from "preact/hooks";
import { Decision, Permission, resolvePermission } from "../api";
import { AppShell } from "../components/app-shell";
import { PermissionCard } from "../components/permission-card";
import { Badge, Button, Card, EmptyState, ErrorBanner, Header, Screen, Section } from "../components/ui";
import { Navigate } from "../routes";

export function RequestsScreen({ permissions, decisions, navigate }: { permissions: Permission[]; decisions: Decision[]; navigate: Navigate }) {
  return (
    <AppShell title="Cardinal" active="requests" requests={permissions.length} navigate={(key) => navigate({ name: key })}>
      <h1>Запросы</h1>
      <p className="lead">{permissions.length ? `${permissions.length} команд ждут вашего решения.` : "Новых запросов нет."}</p>
      {permissions.length === 0 && <EmptyState glyph="[✓]" title="Всё решено" text="Новые команды появятся здесь, когда агент запросит разрешение." />}
      {permissions.map((permission) => <PermissionCard key={permission.id} permission={permission} onClick={() => navigate({ name: "permission", id: permission.id })} />)}
      <Section title="Последние решения">
        {decisions.length === 0 && <p className="muted">История решений пока пуста.</p>}
        {decisions.map((decision) => (
          <Card className="decision-card">
            <div><strong>{decision.project || decision.agent}</strong><code>{decision.summary}</code></div>
            <Badge tone={decision.decision === "allow" ? "green" : decision.decision === "deny" ? "red" : "neutral"}>
              {decision.decision === "allow" ? "Разрешено" : decision.decision === "deny" ? "Отклонено" : "Истекло"}
            </Badge>
          </Card>
        ))}
      </Section>
    </AppShell>
  );
}

export function PermissionScreen({ permission, navigate }: { permission: Permission; navigate: Navigate }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const decide = async (allow: boolean) => {
    setBusy(true); setError("");
    try {
      await resolvePermission(permission.id, allow);
      navigate({ name: "requests" });
    } catch (e) { setError((e as Error).message); setBusy(false); }
  };
  const command = permission.summary.includes(":") ? permission.summary.split(":").slice(1).join(":").trim() : permission.summary;
  return (
    <Screen>
      <Header title="Cardinal" onBack={() => navigate({ name: "requests" })} />
      <main className="content confirm-screen">
        <div className="eyebrow green">Требуется ваше решение</div>
        <h1>Разрешить запуск команды?</h1>
        <p className="lead">Разрешение действует только на эту команду.</p>
        <Card className="permission-summary"><div className="card-kicker"><span>{permission.agent}</span><Badge tone="amber">Ждёт</Badge></div><h3>{permission.project || "Команда агента"}</h3></Card>
        <Section title="Выполнить команду / shell">
          <pre className="command-box">{command}</pre>
          {permission.cwd && <code className="path">cwd: {permission.cwd}</code>}
        </Section>
        <Card className="warning-card"><strong>Что изменится</strong><p>Команда будет выполнена локально на вашем компьютере. Проверьте аргументы перед разрешением.</p></Card>
        <Button variant="primary" disabled={busy} onClick={() => decide(true)}>Разрешить один раз</Button>
        <Button disabled={busy} onClick={() => decide(false)}>Отклонить</Button>
        <ErrorBanner text={error} />
      </main>
    </Screen>
  );
}
