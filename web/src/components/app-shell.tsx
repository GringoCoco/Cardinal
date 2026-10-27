import { ComponentChildren } from "preact";
import { BottomNav, Header, NavKey, Screen } from "./ui";

export function AppShell({ title, active, requests, navigate, children, action }: {
  title: string;
  active: NavKey;
  requests: number;
  navigate: (key: NavKey) => void;
  children: ComponentChildren;
  action?: ComponentChildren;
}) {
  return (
    <Screen className="with-nav">
      <Header title={title} action={action} />
      <main className="content">{children}</main>
      <BottomNav active={active} requests={requests} navigate={navigate} />
    </Screen>
  );
}
