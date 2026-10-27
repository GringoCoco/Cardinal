import { useEffect, useReducer, useState } from "preact/hooks";
import {
  applyEnvelope,
  AppStore,
  emptyStore,
  loadPreferences,
  me,
  Preferences,
  ready,
  savePreferences,
  syncDevice,
  updateDeviceSettings,
  wsURL,
} from "./api";
import { ErrorBanner } from "./components/ui";
import { Route } from "./routes";
import { OnboardingScreen } from "./screens/onboarding";
import { PermissionScreen, RequestsScreen } from "./screens/requests";
import { MessageScreen, OutputScreen, SessionDetailScreen, StopSessionScreen } from "./screens/session-detail";
import { ComputerScreen, DisconnectScreen, NotificationsScreen, SettingsScreen } from "./screens/settings";
import { FiltersScreen, SessionsScreen } from "./screens/sessions";

type MeState = { loading: boolean; paired: boolean; online: boolean; device?: string };

export function App() {
  const [meState, setMeState] = useState<MeState>({ loading: true, paired: false, online: false });
  const [route, setRoute] = useState<Route>({ name: "sessions" });
  const [store, dispatch] = useReducer((state: AppStore, envelope: any) => applyEnvelope(state, envelope), emptyStore);
  const [preferences, setPreferences] = useState<Preferences>(loadPreferences);

  const updatePreferences = (next: Preferences) => {
    setPreferences(next);
    savePreferences(next);
    updateDeviceSettings(next).catch(() => {});
  };

  useEffect(() => {
    ready();
    me()
      .then((value) => setMeState({ loading: false, paired: value.paired, online: Boolean(value.online), device: value.name || value.device }))
      .catch(() => setMeState({ loading: false, paired: false, online: false }));
  }, []);

  useEffect(() => {
    if (!meState.paired) return;
    let disposed = false;
    let ws: WebSocket | undefined;
    let reconnect: number | undefined;
    const refreshPresence = () => me()
      .then((value) => setMeState((current) => ({
        ...current,
        paired: value.paired,
        online: Boolean(value.online),
        device: value.name || value.device || current.device,
      })))
      .catch(() => setMeState((current) => ({ ...current, online: false })));
    const connect = () => {
      if (disposed) return;
      ws = new WebSocket(wsURL());
      ws.onopen = () => {
        refreshPresence();
        syncDevice().catch(() => {});
        updateDeviceSettings(preferences).catch(() => {});
      };
      ws.onmessage = (event) => {
        try { dispatch(JSON.parse(event.data)); } catch {}
      };
      ws.onclose = () => {
        if (!disposed) reconnect = window.setTimeout(connect, 1500);
      };
    };
    connect();
    const presenceTimer = window.setInterval(refreshPresence, 10_000);
    return () => {
      disposed = true;
      window.clearInterval(presenceTimer);
      if (reconnect) window.clearTimeout(reconnect);
      ws?.close();
    };
  }, [meState.paired]);

  if (meState.loading) return <div className="boot-screen"><span>&gt;_ CARDINAL</span><i /></div>;
  if (!meState.paired) return <OnboardingScreen preferences={preferences} update={updatePreferences} complete={(device) => setMeState({ loading: false, paired: true, online: true, device })} />;

  const sessions = Object.values(store.sessions);
  const permissions = Object.values(store.permissions);
  const session = "id" in route ? store.sessions[route.id] : undefined;
  const permission = route.name === "permission" ? store.permissions[route.id] : undefined;
  const sessionPermission = session ? permissions.find((item) => item.sessionId === session.id) : undefined;

  let screen;
  switch (route.name) {
    case "sessions":
      screen = <SessionsScreen sessions={sessions} preferences={preferences} online={meState.online} device={meState.device} requests={permissions.length} navigate={setRoute} />;
      break;
    case "filters":
      screen = <FiltersScreen sessions={sessions} preferences={preferences} update={updatePreferences} navigate={setRoute} />;
      break;
    case "session":
      screen = session ? <SessionDetailScreen session={session} permission={sessionPermission} confirmStop={preferences.confirmStop} navigate={setRoute} /> : <SessionsScreen sessions={sessions} preferences={preferences} online={meState.online} device={meState.device} requests={permissions.length} navigate={setRoute} />;
      break;
    case "output":
      screen = session ? <OutputScreen session={session} navigate={setRoute} /> : null;
      break;
    case "message":
      screen = session ? <MessageScreen session={session} navigate={setRoute} /> : null;
      break;
    case "stop":
      screen = session ? <StopSessionScreen session={session} navigate={setRoute} /> : null;
      break;
    case "requests":
      screen = <RequestsScreen permissions={permissions} decisions={store.decisions} navigate={setRoute} />;
      break;
    case "permission":
      screen = permission ? <PermissionScreen permission={permission} navigate={setRoute} /> : <RequestsScreen permissions={permissions} decisions={store.decisions} navigate={setRoute} />;
      break;
    case "settings":
      screen = <SettingsScreen preferences={preferences} update={updatePreferences} online={meState.online} device={meState.device} sessions={sessions} requests={permissions.length} navigate={setRoute} />;
      break;
    case "computer":
      screen = <ComputerScreen preferences={preferences} update={updatePreferences} online={meState.online} device={meState.device} sessions={sessions} navigate={setRoute} />;
      break;
    case "notifications":
      screen = <NotificationsScreen preferences={preferences} update={updatePreferences} navigate={setRoute} />;
      break;
    case "disconnect":
      screen = <DisconnectScreen device={meState.device} sessions={sessions} navigate={setRoute} />;
      break;
  }
  return <><ErrorBanner text={store.notice} onClose={() => dispatch({ type: "notice.clear" })} />{screen}</>;
}
