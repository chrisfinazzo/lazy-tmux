// Managed by lazy-tmux: pi-extension-v1
import { execFileSync } from "node:child_process";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  if (!process.env.TMUX || !process.env.TMUX_PANE) return;
  const tmux = (args: string[]) => execFileSync("tmux", args, {
    timeout: 2000, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"],
  });
  let startedAt = "";
  try {
    startedAt = execFileSync("ps", ["-p", String(process.pid), "-o", "lstart="], {
      timeout: 2000, encoding: "utf8", env: { ...process.env, LC_ALL: "C", TZ: "UTC" },
    }).trim().replace(/\s+/g, " ");
  } catch { /* Without ownership proof lazy-tmux must leave the pane unbound. */ }
  let running = false;
  let compacting = false;
  let settledState = "idle";
  let compactionSignal: AbortSignal | undefined;
  let onCompactionAbort: (() => void) | undefined;
  const clearCompaction = () => {
    if (compactionSignal && onCompactionAbort) {
      compactionSignal.removeEventListener("abort", onCompactionAbort);
    }
    compactionSignal = undefined;
    onCompactionAbort = undefined;
    compacting = false;
  };
  const prompts: string[] = [];
  const state = () => {
    if (prompts.length) {
      return ["confirm", "select"].includes(prompts[prompts.length - 1])
        ? "awaiting_decision" : "awaiting_input";
    }
    return running || compacting ? "working" : settledState;
  };
  const publish = (_event: unknown, ctx: ExtensionContext) => {
    if (!process.env.TMUX || !process.env.TMUX_PANE || !startedAt) return;
    // Noninteractive and --no-session clients must not inherit an old binding.
    const interactive = ctx.hasUI && (ctx.mode === undefined || ctx.mode === "tui");
    const file = interactive ? ctx.sessionManager.getSessionFile() : undefined;
    try {
      tmux(["set-option", "-p", "-t", process.env.TMUX_PANE,
        "@lazy_tmux_pi_binding", JSON.stringify({ version: 1, pid: process.pid, started_at: startedAt, file: file ?? "",
          session_id: file ? ctx.sessionManager.getSessionId() : "", state: file ? state() : "" })]);
      if (file && process.env.LAZY_TMUX_RESTORE_TOKEN) {
        tmux(["set-option", "-p", "-t", process.env.TMUX_PANE,
          "@lazy_tmux_restore_ack", process.env.LAZY_TMUX_RESTORE_TOKEN]);
      }
    } catch { /* Never disrupt a conversation when tmux is unavailable. */ }
  };
  const resetSession = (_event: unknown, ctx: ExtensionContext) => {
    running = !ctx.isIdle();
    clearCompaction();
    prompts.length = 0;
    settledState = "idle";
    publish(_event, ctx);
  };
  pi.on("session_start", resetSession);
  // Unknown names are inert: upstream emits switch/fork separately, newer Pi
  // emits session_start with a reason. Optional prompt events are not guessed
  // from terminal output on clients that do not expose them.
  const onLifecycle = pi.on.bind(pi) as (
    event: string, handler: (event: any, ctx: ExtensionContext) => void
  ) => unknown;
  onLifecycle("session_switch", resetSession);
  onLifecycle("session_fork", resetSession);
  const start = (_event: unknown, ctx: ExtensionContext) => {
    running = true;
    settledState = "awaiting_input";
    publish(_event, ctx);
  };
  pi.on("before_agent_start", start);
  pi.on("agent_start", start);
  pi.on("agent_end", (event, ctx) => {
    running = false;
    clearCompaction();
    prompts.length = 0;
    const last = [...event.messages].reverse().find(message => message.role === "assistant");
    settledState = last?.stopReason === "error" ? "error" : "awaiting_input";
    publish(event, ctx);
  });
  onLifecycle("agent_settled", (event, ctx) => {
    running = false;
    clearCompaction();
    prompts.length = 0;
    if (event.aborted) settledState = "awaiting_input";
    publish(event, ctx);
  });
  pi.on("session_before_compact", (event, ctx) => {
    clearCompaction();
    compacting = true;
    compactionSignal = event.signal;
    onCompactionAbort = () => {
      clearCompaction();
      running = !ctx.isIdle();
      settledState = "awaiting_input";
      publish(event, ctx);
    };
    if (compactionSignal?.aborted) { onCompactionAbort(); return; }
    compactionSignal?.addEventListener("abort", onCompactionAbort, { once: true });
    publish(event, ctx);
  });
  pi.on("session_compact", (event, ctx) => {
    clearCompaction();
    running = !ctx.isIdle();
    settledState = "awaiting_input";
    publish(event, ctx);
  });
  onLifecycle("session_compact_failed", (event, ctx) => {
    clearCompaction();
    running = !ctx.isIdle();
    settledState = event.aborted ? "awaiting_input" : "error";
    publish(event, ctx);
  });
  onLifecycle("ui_prompt_start", (event, ctx) => {
    prompts.push(event.kind);
    publish(event, ctx);
  });
  onLifecycle("ui_prompt_end", (event, ctx) => {
    const index = prompts.lastIndexOf(event.kind);
    if (index >= 0) prompts.splice(index, 1);
    publish(event, ctx);
  });
  pi.on("session_shutdown", () => {
    if (!process.env.TMUX || !process.env.TMUX_PANE || !startedAt) return;
    try {
      const binding = JSON.parse(tmux(["show-option", "-p", "-v", "-t",
        process.env.TMUX_PANE, "@lazy_tmux_pi_binding"]));
      if (binding.pid === process.pid) {
        tmux(["set-option", "-pu", "-t", process.env.TMUX_PANE, "@lazy_tmux_pi_binding"]);
      }
    } catch { /* The pane may already be gone. */ }
  });
}
