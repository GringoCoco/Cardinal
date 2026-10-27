/**
 * Cardinal — плагин OpenCode.
 *
 * Переносит события агента в daemon Cardinal (localhost), который доставляет
 * их в Telegram: пуши о завершении/вопросах и кнопки Allow/Deny для
 * permission-запросов. Решение приходит из Telegram даже если Mac за NAT,
 * а телефон в другой сети.
 *
 * Правило MauMau: никогда не блокировать агента. Любой сбой доставки —
 * и OpenCode просто спросит разрешение в TUI как обычно.
 *
 * Опции (opencode.json → plugins):
 *   { "package": ".../plugins/opencode/cardinal", "options": {
 *       "daemon": "http://127.0.0.1:9723",   // адрес daemon'а
 *       "agent": "opencode"                  // имя в пуше (coucou_agent)
 *   } }
 */
import { Plugin } from "@opencode/plugin"
import { appendFileSync } from "node:fs"

interface CardinalOptions {
  daemon?: string
  agent?: string
}

const DECISION_TIMEOUT_MS = 330_000 // daemon ждёт кнопку (до decision_wait сек) + запас

export default Plugin.define({
  id: "cardinal",
  async setup(ctx) {
    const options = (ctx.options ?? {}) as CardinalOptions
    const daemonURL = (options.daemon ?? "http://127.0.0.1:9723").replace(/\/+$/, "")
    const agentName = options.agent ?? "opencode"
    const dir = ctx.location.directory
    const debugFile =
      process.env.CARDINAL_DEBUG === "1" ? (process.env.CARDINAL_DEBUG_FILE ?? "/tmp/cardinal-plugin.log") : null

    const log = (what: string, data: unknown) => {
      if (!debugFile) return
      try {
        appendFileSync(debugFile, `${new Date().toISOString()} ${what} ${JSON.stringify(data)}\n`)
      } catch {}
    }

    log("setup", { daemonURL, agentName, dir })

    async function postHook(payload: Record<string, unknown>): Promise<any> {
      try {
        const res = await fetch(daemonURL + "/hook", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify(payload),
          signal: AbortSignal.timeout(DECISION_TIMEOUT_MS),
        })
        if (!res.ok) {
          log("daemon ответил", res.status)
          return null
        }
        return await res.json()
      } catch (e) {
        log("postHook-fail", { url: daemonURL, error: String(e) })
        return null
      }
    }

    // ── Permission: вопрос в Telegram, ответ — кнопкой ────────────────────
    // Хук срабатывает и на allow, и на ask; вмешиваемся только в ask —
    // иначе пуш на каждое автоматическое разрешение.
    await ctx.permission.hook("evaluate", async (event) => {
      if (event.effect !== "ask") return
      const first = event.resources[0] ?? ""
      const resp = await postHook({
        hook_event_name: "PermissionRequest",
        session_id: event.sessionID,
        coucou_agent: agentName,
        cwd: dir,
        tool_name: event.action,
        tool_input:
          event.action === "shell" ? { command: first } : { path: first },
      })
      const behavior: unknown = resp?.hookSpecificOutput?.decision?.behavior
      if (behavior === "allow") {
        log("perm-allow", { action: event.action })
        event.effect = "allow"
      } else if (behavior === "deny") {
        log("perm-deny", { action: event.action })
        event.effect = "deny"
        event.message = "⛔ Отклонено из Telegram (Cardinal)"
      } else {
        log("perm-no-decision", { action: event.action, resp })
      }
    })

    // ── Статусы сессий: start / finished / error ─────────────────────────
    const controller = new AbortController()
    void (async () => {
      try {
        for await (const event of ctx.event.subscribe({ signal: controller.signal })) {
          log("event", { type: event.type, event: event as unknown as Record<string, unknown> })
          if (debugFile) {
            // отладка: копия каждого события в daemon, видна в его логе
            void postHook({
              hook_event_name: "DebugEvent",
              session_id: String(
                (event as any).properties?.sessionID ??
                  (event as any).properties?.info?.id ??
                  (event as any).properties?.id ??
                  "",
              ),
              coucou_agent: agentName,
              cwd: dir,
              message: event.type,
            })
          }
          const canonical =
            event.type === "session.created"
              ? "SessionStart"
              : event.type === "session.execution.succeeded"
                ? "Stop"
                : event.type === "session.execution.failed"
                  ? "StopFailure"
                  : event.type === "session.execution.interrupted"
                    ? "StopInterrupted"
                    : null
          if (!canonical) continue
          const sessionID: string = ((event as any).data?.sessionID as string) ?? ((event as any).durable?.aggregateID as string) ?? ""
          void postHook({
            hook_event_name: canonical,
            session_id: sessionID,
            coucou_agent: agentName,
            cwd: dir,
          })
        }
      } catch (e) {
        log("subscribe-loop-crashed", { error: String(e) })
      }
    })()

    return () => controller.abort()
  },
})
