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
  const publish = (_event: unknown, ctx: ExtensionContext) => {
    if (!process.env.TMUX || !process.env.TMUX_PANE || !startedAt) return;
    // Noninteractive and --no-session clients must not inherit an old binding.
    const file = ctx.hasUI ? ctx.sessionManager.getSessionFile() : undefined;
    try {
      tmux(["set-option", "-p", "-t", process.env.TMUX_PANE,
        "@lazy_tmux_pi_binding", JSON.stringify({ version: 1, pid: process.pid, started_at: startedAt, file: file ?? "" })]);
      if (file && process.env.LAZY_TMUX_RESTORE_TOKEN) {
        tmux(["set-option", "-p", "-t", process.env.TMUX_PANE,
          "@lazy_tmux_restore_ack", process.env.LAZY_TMUX_RESTORE_TOKEN]);
      }
    } catch { /* Never disrupt a conversation when tmux is unavailable. */ }
  };
  pi.on("session_start", publish);
  // Upstream Pi emits switch/fork separately; newer Pi emits session_start
  // with reason=resume/fork. Unknown event names are inert on either version.
  const onLifecycle = pi.on.bind(pi) as (event: string, handler: typeof publish) => unknown;
  onLifecycle("session_switch", publish);
  onLifecycle("session_fork", publish);
  pi.on("agent_end", publish);
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
