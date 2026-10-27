export type Route =
  | { name: "sessions" }
  | { name: "filters" }
  | { name: "session"; id: string }
  | { name: "output"; id: string }
  | { name: "message"; id: string }
  | { name: "stop"; id: string }
  | { name: "requests" }
  | { name: "permission"; id: string }
  | { name: "settings" }
  | { name: "computer" }
  | { name: "notifications" }
  | { name: "disconnect" };

export type Navigate = (route: Route) => void;
