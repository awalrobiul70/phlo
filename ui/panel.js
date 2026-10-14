const runState = document.querySelector("#run-state");
const startButton = document.querySelector("#start");
const stopButton = document.querySelector("#stop");
const services = document.querySelector("#services");
const note = document.querySelector("#note");
const rootProblem = document.querySelector("#root-problem");

let stackBusy = false;
let noteSticky = false;
let latest = null;
let closing = false;

function showNote(text, sticky) {
  note.textContent = text || "";
  note.hidden = !text;
  note.classList.toggle("error", Boolean(text) && !sticky);
  noteSticky = Boolean(text) && Boolean(sticky);
}

async function api(method, url) {
  const res = await fetch(url, { method, headers: { "X-Phlo": "1" } });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function serverURL(scheme, host, port) {
  const omit = (scheme === "http" && port === 80) || (scheme === "https" && port === 443);
  return scheme + "://" + host + (omit ? "" : ":" + port);
}

function openLocal(url) {
  if (typeof window.phloOpen === "function") {
    window.phloOpen(url).catch((err) => showNote(err.message || "Could not open that link"));
    return;
  }
  window.open(url, "_blank", "noopener");
}

function paintRunButtons(status) {
  if (stackBusy) return;
  startButton.textContent = "Start";
  stopButton.textContent = "Stop";
  startButton.classList.remove("busy");
  stopButton.classList.remove("busy");
  startButton.hidden = status.running;
  startButton.disabled = Boolean(status.rootProblem);
  stopButton.hidden = !status.running;
  stopButton.disabled = false;
}

function beginStackAction(button, label) {
  stackBusy = true;
  button.disabled = true;
  button.classList.add("busy");
  button.textContent = label;
}

function port80Note(status) {
  if (!status || !status.port80Taken) return "";
  return "Another program is using port 80. Open Phlo sites with the port in the link, such as http://name." +
    (status.tld || "test") + ":" + status.httpPort + "/.";
}

function row(name, version, on, lines, stateText) {
  const item = document.createElement("li");
  item.className = "service";
  const dot = document.createElement("span");
  dot.className = on ? "dot on" : "dot";
  const body = document.createElement("div");
  const title = document.createElement("h2");
  title.textContent = name;
  if (version) {
    const ver = document.createElement("span");
    ver.className = "version";
    ver.textContent = version;
    title.append(ver);
  }
  const addrs = document.createElement("div");
  addrs.className = "addrs";
  for (const line of lines) addrs.append(line);
  body.append(title, addrs);
  const state = document.createElement("span");
  state.className = on ? "state on" : "state";
  state.textContent = stateText || (on ? "Running" : "Stopped");
  item.append(dot, body, state);
  return item;
}

function textLine(text) {
  const span = document.createElement("span");
  span.className = "addr";
  span.textContent = text;
  return span;
}

function linkLine(url, label) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "addr";
  button.textContent = label || url;
  button.addEventListener("click", () => openLocal(url));
  return button;
}

function listen(port, label) {
  const text = "127.0.0.1:" + port + (label ? " " + label : "");
  return textLine(text);
}

function webLines(status) {
  const lines = [linkLine(serverURL("http", "127.0.0.1", status.httpPort))];
  if (status.sslEnabled) lines.push(linkLine(serverURL("https", "127.0.0.1", status.sslPort)));
  if (status.hostIp && status.hostIp !== "127.0.0.1") {
    lines.push(linkLine(serverURL("http", status.hostIp, status.httpPort)));
    if (status.sslEnabled) lines.push(linkLine(serverURL("https", status.hostIp, status.sslPort)));
  }
  return lines;
}

function paintServices(status) {
  const items = [
    row("Dashboard", "", true, [linkLine(location.origin + "/", "127.0.0.1:" + status.dashboardPort)]),
    row("Nginx", status.nginx, status.running, webLines(status)),
  ];
  for (const pool of status.phpPools || []) {
    items.push(row("PHP " + pool.version, "", true, [listen(pool.port)]));
  }
  if (status.mariadbEnabled !== false) {
    items.push(row("MariaDB", status.mariadb, status.mysqlRunning, [listen(status.mysqlPort)]));
  }
  if (status.postgresEnabled) {
    items.push(row("PostgreSQL", status.postgres, status.postgresRunning, [listen(status.postgresPort)]));
  }
  if (status.mongodbEnabled) {
    items.push(row("MongoDB", status.mongodb, status.mongodbRunning, [listen(status.mongodbPort)]));
  }
  if (status.redisEnabled !== false) {
    items.push(row("Redis", status.redis, status.redisRunning, [listen(status.redisPort)]));
  }
  if (status.memcachedEnabled !== false) {
    items.push(row("Memcached", status.memcached, status.memcachedRunning, [listen(status.memcachedPort)]));
  }
  if (status.mailpitEnabled !== false) {
    const lines = [listen(status.mailpitSmtpPort, "SMTP")];
    if (status.mailpitUrl) lines.push(linkLine(status.mailpitUrl, "127.0.0.1:" + status.mailpitUiPort + " mailbox"));
    else lines.push(listen(status.mailpitUiPort, "mailbox"));
    items.push(row("Mailpit", status.mailpit, status.mailpitRunning, lines));
  }
  if (status.cronEnabled) {
    const label = status.cronicalInstalled ? "" : "Not installed";
    items.push(row("Cron", status.cronical || "", status.cronRunning, [textLine("config/custom/cronical.dat")], label));
  }
  services.replaceChildren(...items);
}

const defaultDashboardPort = 7700;

function paintDashboardPort(status) {
  const form = document.querySelector("#dash-port-form");
  const hint = document.querySelector(".port-hint");
  const portInput = document.querySelector("#dash-port");
  const moved = status.dashboardPort !== defaultDashboardPort;
  form.hidden = !moved;
  hint.hidden = !moved;
  if (!moved || document.activeElement === portInput || portInput.dataset.busy === "1" || portInput.dataset.edited === "1") return;
  portInput.value = defaultDashboardPort;
}

function paint(status) {
  latest = status;
  runState.textContent = status.running ? "Running" : "Stopped";
  runState.className = status.running ? "pill on" : "pill";
  paintRunButtons(status);
  paintDashboardPort(status);
  rootProblem.textContent = status.rootProblem || "";
  rootProblem.hidden = !status.rootProblem;
  const portNote = document.querySelector("#port80-note");
  const portText = port80Note(status);
  portNote.textContent = portText;
  portNote.hidden = !portText;
  paintServices(status);
}

async function refresh() {
  const status = await api("GET", "/api/status");
  paint(status);
  if (!noteSticky) showNote("");
  return status;
}

async function runStack(button, label, path) {
  showNote("");
  beginStackAction(button, label);
  try {
    const data = await api("POST", path);
    stackBusy = false;
    await refresh();
    if (data.warning) showNote(data.warning, true);
  } catch (err) {
    stackBusy = false;
    showNote(err.message);
    refresh().catch(() => paintRunButtons({ running: !stopButton.hidden, rootProblem: rootProblem.textContent }));
  }
}

function bindDesktop() {
  if (typeof window.phloClose !== "function") return;
  document.body.classList.add("desktop");
  document.querySelector(".chrome").hidden = false;
  document.querySelector("#min").addEventListener("click", () => window.phloMinimize());
  document.querySelector("#close").addEventListener("click", closeWindow);
  document.querySelector(".bar").addEventListener("mousedown", (event) => {
    if (event.button !== 0 || event.target.closest("button")) return;
    window.phloDrag();
  });
}

async function closeWindow() {
  if (closing) return;
  closing = true;
  const button = document.querySelector("#close");
  button.disabled = true;
  try {
    if (latest && !latest.running && !stackBusy) {
      await api("POST", "/api/shutdown");
    }
    window.phloClose();
  } catch (err) {
    closing = false;
    button.disabled = false;
    showNote(err.message);
  }
}

document.querySelector("#dash-port").addEventListener("input", (event) => {
  event.target.dataset.edited = "1";
});

document.querySelector("#dash-port-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const input = document.querySelector("#dash-port");
  const button = document.querySelector("#dash-port-save");
  const closeButton = document.querySelector("#close");
  const port = Number(input.value);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    showNote("The dashboard port must be from 1 to 65535.");
    return;
  }
  if (typeof window.phloSetDashboardPort !== "function") {
    showNote("Change the dashboard port from the Phlo window.");
    return;
  }
  input.dataset.busy = "1";
  stackBusy = true;
  button.disabled = true;
  button.textContent = "Changing";
  closeButton.disabled = true;
  try {
    const url = await new Promise((resolve, reject) => {
      window.phloPortDone = (next, err) => (err ? reject(err) : resolve(next));
      window.phloSetDashboardPort(port).catch(reject);
    });
    const next = new URL(url, location.href);
    if (next.origin !== location.origin) {
      location.href = url;
      return;
    }
    stackBusy = false;
    delete input.dataset.busy;
    button.disabled = false;
    button.textContent = "Change";
    closeButton.disabled = false;
    showNote("The dashboard is already on this port.", true);
  } catch (err) {
    stackBusy = false;
    delete input.dataset.busy;
    button.disabled = false;
    button.textContent = "Change";
    closeButton.disabled = false;
    const message = typeof err === "string" ? err : (err && err.message) || "Could not change the dashboard port.";
    showNote(message, true);
    note.classList.add("error");
  }
});

startButton.addEventListener("click", () => runStack(startButton, "Starting", "/api/services/start"));
stopButton.addEventListener("click", () => runStack(stopButton, "Stopping", "/api/services/stop"));
document.querySelector("#dashboard").addEventListener("click", () => openLocal(location.origin + "/"));

bindDesktop();
refresh().catch((err) => showNote(err.message));
setInterval(() => {
  if (stackBusy) return;
  refresh().catch((err) => showNote(err.message));
}, 2000);
