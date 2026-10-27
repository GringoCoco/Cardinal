import { ComponentChildren } from "preact";

export function Screen({ children, className = "" }: { children: ComponentChildren; className?: string }) {
  return <div className={`screen ${className}`}>{children}</div>;
}

export function Header({ title, onBack, action }: { title: string; onBack?: () => void; action?: ComponentChildren }) {
  return (
    <header className="app-header">
      <button className="icon-button" aria-label={onBack ? "Назад" : "Cardinal"} onClick={onBack} disabled={!onBack}>
        {onBack ? "‹" : ">_"}
      </button>
      <strong>{title}</strong>
      <div className="header-action">{action}</div>
    </header>
  );
}

export function Card({ children, className = "", onClick }: { children: ComponentChildren; className?: string; onClick?: () => void }) {
  return <article className={`panel ${className}${onClick ? " clickable" : ""}`} onClick={onClick}>{children}</article>;
}

export function Section({ title, children, className = "" }: { title?: string; children: ComponentChildren; className?: string }) {
  return (
    <section className={`section ${className}`}>
      {title && <h2 className="section-title">{title}</h2>}
      {children}
    </section>
  );
}

type ButtonProps = {
  children: ComponentChildren;
  onClick?: () => void;
  variant?: "primary" | "secondary" | "danger" | "ghost";
  disabled?: boolean;
  className?: string;
  type?: "button" | "submit";
};

export function Button({ children, onClick, variant = "secondary", disabled, className = "", type = "button" }: ButtonProps) {
  return <button type={type} className={`button ${variant} ${className}`} onClick={onClick} disabled={disabled}>{children}</button>;
}

export function Badge({ children, tone = "neutral" }: { children: ComponentChildren; tone?: "green" | "amber" | "red" | "neutral" }) {
  return <span className={`badge ${tone}`}>{children}</span>;
}

export function Toggle({ checked, onChange, label, hint }: { checked: boolean; onChange: (value: boolean) => void; label: string; hint?: string }) {
  return (
    <button className="setting-row" onClick={() => onChange(!checked)}>
      <span><strong>{label}</strong>{hint && <small>{hint}</small>}</span>
      <span className={`toggle ${checked ? "on" : ""}`}><i /></span>
    </button>
  );
}

export function EmptyState({ glyph = "[ — ]", title, text, children }: { glyph?: string; title: string; text: string; children?: ComponentChildren }) {
  return (
    <div className="empty-state">
      <div className="empty-glyph">{glyph}</div>
      <h1>{title}</h1>
      <p>{text}</p>
      {children && <div className="empty-actions">{children}</div>}
    </div>
  );
}

export type NavKey = "sessions" | "requests" | "settings";

export function BottomNav({ active, requests, navigate }: { active: NavKey; requests: number; navigate: (key: NavKey) => void }) {
  return (
    <nav className="bottom-nav">
      <button className={active === "sessions" ? "active" : ""} onClick={() => navigate("sessions")}><span>▣</span>Сессии</button>
      <button className={active === "requests" ? "active" : ""} onClick={() => navigate("requests")}><span>♢</span>Запросы{requests ? ` · ${requests}` : ""}</button>
      <button className={active === "settings" ? "active" : ""} onClick={() => navigate("settings")}><span>☷</span>Настройки</button>
    </nav>
  );
}

export function ErrorBanner({ text, onClose }: { text?: string; onClose?: () => void }) {
  if (!text) return null;
  return <div className="error-banner"><span>{text}</span>{onClose && <button onClick={onClose}>×</button>}</div>;
}
