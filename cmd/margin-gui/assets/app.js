import { Events } from "/wails/runtime.js";
import { api, $, state, hooks, toast, errText, tail, openModal, closeModal, applyTheme, setThemeMode, followsSystem } from "./core.js";
import { openReview, openReviewFile, reviewKey, renderReview, sendComments, sendFixes } from "./review.js";
import { loadWorkspace, loadRecent, renderSide, addFolder, selectWorktree, homeKey, backToReview, syncSideButtons, openSwitcher, switcherOpen, openSettings, navState } from "./nav.js";

// Agent terminal

const termThemes = {
  dark: { background: "#0a0d12", foreground: "#d7dde4", cursor: "#7aa2f7", selectionBackground: "#264f78" },
  light: {
    background: "#ffffff", foreground: "#1f2328", cursor: "#0969da", cursorAccent: "#ffffff", selectionBackground: "#b6d7ff",
    black: "#24292f", red: "#cf222e", green: "#116329", yellow: "#4d2d00", blue: "#0969da", magenta: "#8250df", cyan: "#1b7c83", white: "#6e7781",
    brightBlack: "#57606a", brightRed: "#a40e26", brightGreen: "#1a7f37", brightYellow: "#633c01", brightBlue: "#218bff", brightMagenta: "#a475f9", brightCyan: "#3192aa", brightWhite: "#8c959f",
  },
};

const term = new Terminal({
  fontFamily: getComputedStyle(document.body).getPropertyValue("--mono"),
  fontSize: 13,
  cursorBlink: true,
  allowProposedApi: true,
  scrollback: 5000,
  theme: termThemes.dark,
});
const fit = new FitAddon.FitAddon();
term.loadAddon(fit);
term.open($("#terminal"));

function b64(s) {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function fitTerminal() {
  if ($("#agent-pane").classList.contains("collapsed") || $("#reviewer").classList.contains("hidden")) return;
  try {
    fit.fit();
  } catch {}
  if (state.agentLive && state.opened) api("Agents.Resize", state.opened.path, term.cols, term.rows);
}

function setAgentLive(live) {
  const was = state.agentLive;
  if (was !== live) loadRecent();
  state.agentLive = live;
  if (was !== live && state.view) {
    const scroll = $("#station").scrollTop;
    renderReview();
    $("#station").scrollTop = scroll;
  }
  const saved = state.opened?.agent;
  const name = saved?.name || "Agent";
  $("#agent-dot").classList.toggle("live", live);
  $("#agent-label").textContent = live ? `${name} · ${tail(state.opened?.repo ?? "", 1)}` :
    saved?.failed ? `Could not resume ${name}` : `${name} not running`;
  $("#agent-btn").textContent = live ? "Stop agent" : saved?.exact && !saved.failed ? "Resume agent" :
    saved?.saved ? "Continue from saved review" : "Start agent";
  $("#agent-link").classList.toggle("hidden", live || !saved?.canLink || (saved.exact && !saved.failed) || !!state.opened?.created);
}

let agentStatusRequest = 0;
async function refreshAgent(opened) {
  if (state.opened !== opened) return null;
  const request = ++agentStatusRequest;
  const [live, saved] = await Promise.all([
    api("Agents.Running", opened.path),
    api("Agents.ResumeStatus", opened.path, opened.repo),
  ]);
  if (state.opened !== opened || request !== agentStatusRequest) return null;
  opened.agent = saved ?? {};
  setAgentLive(live);
  return live;
}

async function attachAgent(opened, restore) {
  term.reset();
  try {
    const live = await refreshAgent(opened);
    if (live === null) return;
    setAgentCollapsed(!live);
    if (live) {
      const buf = await api("Agents.Scrollback", opened.path);
      if (buf && state.opened === opened) term.write(b64(buf));
    } else if (restore && opened.agent.auto && !opened.shared) {
      await startAgent(opened, "Resume", false);
    } else {
      const action = $("#agent-btn").textContent;
      term.writeln(`\x1b[2mPress ${action} above.\x1b[0m`);
    }
  } catch (e) {
    if (state.opened === opened) toast(errText(e), true);
  } finally {
    if (state.opened === opened) $("#agent-btn").disabled = !opened.agent;
  }
}

async function toggleAgent() {
  const opened = state.opened;
  if (!opened?.agent || state.view?.path !== opened.path) return;
  if (state.agentLive) {
    try {
      await api("Agents.Stop", opened.path);
    } catch (e) {
      toast(errText(e), true);
    }
    return;
  }
  const saved = opened.agent;
  const action = saved?.exact && !saved.failed ? "Resume" : saved?.saved ? "Continue" : "Start";
  await startAgent(opened, action, true);
}

async function startAgent(opened, action, focus) {
  const button = $("#agent-btn");
  button.disabled = true;
  // A launch changes the process status while the terminal may still receive earlier events.
  ++agentStatusRequest;
  setAgentCollapsed(false);
  term.reset();
  try {
    const args = [opened.path, opened.repo];
    if (action === "Start") args.push(opened.created);
    const buf = await api("Agents." + action, ...args, term.cols, term.rows);
    if (state.opened !== opened) return;
    if (buf) term.write(b64(buf));
    opened.created = false;
    if (await refreshAgent(opened) && focus) term.focus();
  } catch (e) {
    if (state.opened === opened) {
      await refreshAgent(opened).catch(() => {});
      toast(errText(e), true);
    }
  } finally {
    if (state.opened === opened) button.disabled = false;
  }
}

function linkAgent() {
  const opened = state.opened;
  if (!opened || !opened.agent?.canLink) return;
  const form = document.createElement("form");
  form.className = "settings";
  form.innerHTML = `<section>
    <label>Conversation ID for this review <input name="id" required autocomplete="off" spellcheck="false"></label>
    <p class="error hidden" role="alert"></p>
    <button class="primary" type="submit">Link and resume</button>
  </section>`;
  const modal = openModal("Link saved conversation", form);
  const input = form.elements.id;
  input.focus();
  form.onsubmit = async (e) => {
    e.preventDefault();
    if (state.opened !== opened) return;
    const submit = form.querySelector("button");
    submit.disabled = true;
    try {
      await api("Agents.LinkConversation", opened.path, opened.repo, input.value);
      if (modal.isConnected) closeModal();
      if (state.opened === opened) await startAgent(opened, "Resume", true);
    } catch (err) {
      const message = form.querySelector("[role=alert]");
      message.textContent = errText(err);
      message.classList.remove("hidden");
      submit.disabled = false;
    }
  };
}

Events.On("agent:data", (ev) => {
  const d = ev.data;
  if (d && state.opened && d.review === state.opened.path) term.write(b64(d.data));
});

Events.On("agent:exit", async (ev) => {
  const opened = state.opened;
  if (!opened || ev.data !== opened.path) return;
  try {
    const live = await refreshAgent(opened);
    if (state.opened !== opened || live !== false) return;
    term.writeln(opened.agent?.failed ? "\r\nCould not reopen the saved conversation. You can continue from the saved review." :
      "\r\n\x1b[2m[agent exited]\x1b[0m");
  } catch (e) {
    if (state.opened === opened) toast(errText(e), true);
  }
});

term.onData((d) => {
  if (state.agentLive && state.opened) api("Agents.Write", state.opened.path, d);
});

// Layout and keys

function togglePicker() {
  $("#workspace").classList.toggle("focus");
  syncSideButtons();
  fitTerminal();
}

function showReviewer(loading = false) {
  if (loading) {
    $("#agent-btn").disabled = true;
    $("#agent-link").classList.add("hidden");
  }
  $("#welcome").classList.add("hidden");
  $("#home-pane").classList.add("hidden");
  $("#reviewer").classList.remove("hidden");
  syncSideButtons();
  fitTerminal();
}

function reviewOpened() {
  const o = state.opened;
  const w = state.worktree;
  const repoName = w?.repo?.name ?? tail(o?.repo ?? "", 1);
  $("#crumb").textContent = `${repoName} ›`;
  $("#crumb").title = `Back to ${repoName}${w?.branch ? " · " + w.branch : ""}: what else to review there (ctrl+k for anything)`;
  $("#workspace").classList.add("focus");
  syncSideButtons();
  loadRecent();
  renderSide();
}

async function crumbBack() {
  const o = state.opened;
  if (!o) return;
  const w = state.worktree;
  if (w && (w.path === o.repo || w.repo?.worktrees?.some((x) => x.path === o.repo))) {
    $("#workspace").classList.remove("focus");
    return selectWorktree(w, w.repo);
  }
  const nav = navState();
  for (const f of nav.folders) for (const r of f.repos ?? []) for (const x of r.worktrees) if (x.path === o.repo) return selectWorktree(x, r);
  toast("This review's repository is not in the sidebar; add it with ＋.");
}

async function pickReviewFile() {
  try {
    const p = await api("Reviews.PickFile");
    if (p) await openReviewFile(p);
  } catch (e) {
    toast(errText(e), true);
  }
}

(function splitter() {
  const pane = $("#agent-pane");
  let startY, startH;
  $("#splitter").addEventListener("mousedown", (e) => {
    startY = e.clientY;
    startH = pane.getBoundingClientRect().height;
    pane.classList.remove("collapsed");
    const move = (ev) => {
      const h = Math.max(80, Math.min(window.innerHeight - 160, startH + startY - ev.clientY));
      pane.style.height = h + "px";
      fitTerminal();
    };
    const up = () => {
      document.removeEventListener("mousemove", move);
      document.removeEventListener("mouseup", up);
      fitTerminal();
    };
    document.addEventListener("mousemove", move);
    document.addEventListener("mouseup", up);
  });
})();

document.addEventListener("keydown", (e) => {
  if (switcherOpen()) return;
  if (document.activeElement?.closest?.(".cm-editor")) return;
  const inTerm = document.activeElement?.closest?.("#terminal");
  const inInput = ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement?.tagName) && !inTerm;
  if (inTerm) {
    if (e.key === "Escape" && e.ctrlKey) {
      e.preventDefault();
      document.activeElement.blur();
    }
    return;
  }
  if ((e.ctrlKey || e.metaKey) && (e.key === "k" || e.key === "p")) {
    e.preventDefault();
    return openSwitcher();
  }
  if ((e.ctrlKey || e.metaKey) && e.key === ",") {
    e.preventDefault();
    return openSettings();
  }
  if (inInput) return;
  if (!e.ctrlKey && !e.metaKey && !e.altKey) {
    if (e.key === "\\") {
      e.preventDefault();
      return togglePicker();
    }
    if (e.key === "`" && state.opened && !$("#reviewer").classList.contains("hidden")) {
      e.preventDefault();
      return term.focus();
    }
  }
  const modal = document.querySelector(".modal-back");
  if (modal && !state.view) {
    if (modal.onkey?.(e) || e.key === "Escape") e.preventDefault();
    if (e.key === "Escape") closeModal();
    return;
  }
  if (!modal && homeKey(e)) return e.preventDefault();
  if (!$("#reviewer").classList.contains("hidden") && reviewKey(e)) e.preventDefault();
});

$("#jump").onclick = openSwitcher;
$("#settings-btn").onclick = openSettings;
$("#add-folder").onclick = addFolder;
$("#open-file").onclick = pickReviewFile;
$("#repo-filter").oninput = renderSide;
$("#hide-side").onclick = togglePicker;
$("#show-side").onclick = togglePicker;
$("#toggle-picker").onclick = togglePicker;
$("#crumb").onclick = crumbBack;
$("#back-review").onclick = backToReview;
$("#wt-refresh").onclick = () => state.worktree && selectWorktree(state.worktree, state.worktree.repo);
$("#agent-btn").onclick = toggleAgent;
$("#agent-link").onclick = linkAgent;
$("#send-btn").onclick = sendComments;
$("#fix-btn").onclick = sendFixes;
$("#agent-submit").onchange = (e) => api("Agents.SetSubmit", e.target.checked);

function setAgentCollapsed(collapsed) {
  $("#agent-pane").classList.toggle("collapsed", collapsed);
  $("#agent-collapse").textContent = collapsed ? "▴" : "▾";
  fitTerminal();
}
$("#agent-collapse").onclick = () => setAgentCollapsed(!$("#agent-pane").classList.contains("collapsed"));
$("#refresh").onclick = () => {
  if (state.opened?.file) return toast("Opened from a file: start it over from its repository instead.");
  if (state.opened && confirmFresh()) openReview(state.opened.repo, state.opened.arg, true);
};
window.addEventListener("resize", fitTerminal);
const themeChanged = (dark) => (term.options.theme = termThemes[dark ? "dark" : "light"]);
Object.assign(hooks, { fitTerminal, focusTerminal: () => term.focus(), attachAgent, showReviewer, reviewOpened, pickReviewFile, themeChanged });

api("Settings.Theme").then(
  (t) => setThemeMode(t.mode),
  () => applyTheme(),
);
Events.On("theme:system", () => followsSystem() && applyTheme());
window.addEventListener("focus", () => followsSystem() && applyTheme());

let freshArmed = 0;
function confirmFresh() {
  if (Date.now() - freshArmed < 3000) return true;
  freshArmed = Date.now();
  toast("Click ↺ again to throw away this review's stops and notes and start over.");
  return false;
}

api("Reviews.Recovered").then(
  (fixed) => fixed?.length && toast(`Put back ${fixed.length === 1 ? fixed[0] : `${fixed.length} files`}: a tried change was still in ${fixed.length === 1 ? "it" : "them"} from a run margin did not finish.`),
  (e) => toast(errText(e), true),
);

(async () => {
  const init = await api("Workspace.Initial");
  await loadWorkspace();
  if (init?.review) {
    const opened = await openReviewFile(init.review, init.restore);
    if (!opened && init.restore) {
      state.opened = null;
      $("#reviewer").classList.add("hidden");
      $("#welcome").classList.remove("hidden");
      $("#workspace").classList.remove("focus");
      syncSideButtons();
      toast("Could not reopen the last review. Open a review from the sidebar.", true);
    }
    return;
  }
  if (!init?.root) {
    if (navState().folders.length) $("#workspace").classList.remove("focus");
    return;
  }
  const nav = navState();
  for (const f of nav.folders) for (const r of f.repos ?? []) for (const w of r.worktrees) {
    if (w.path === init.root || r.path === init.root) {
      if (!init.target) return selectWorktree(w, r);
      state.worktree = { ...w, repo: r };
    }
  }
  if (init.target) await openReview(init.root, init.target === "." ? "" : init.target);
})();
