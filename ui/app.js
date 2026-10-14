const banner = document.querySelector("#banner");
const rootProblem = document.querySelector("#root-problem");
const hostsWarning = document.querySelector("#hosts-warning");
const sitesEl = document.querySelector("#sites");
const runtimesEl = document.querySelector("#runtimes");
const runState = document.querySelector("#run-state");
const startButton = document.querySelector("#start");
const stopButton = document.querySelector("#stop");
let stackBusy = false;
let refreshing = false;
let liveKeySeen = "";
const defaultSelect = document.querySelector("#default-php");
const nginxSelect = document.querySelector("#nginx-version");
const mariadbSelect = document.querySelector("#mariadb-version");
const redisSelect = document.querySelector("#redis-version");
const memcachedSelect = document.querySelector("#memcached-version");
const mailpitSelect = document.querySelector("#mailpit-version");
const postgresSelect = document.querySelector("#postgres-version");
const mongodbSelect = document.querySelector("#mongodb-version");
const toolchainsEl = document.querySelector("#toolchains");
const phpDialog = document.querySelector("#php-dialog");
const hostDialog = document.querySelector("#host-dialog");
const phpError = document.querySelector("#php-error");
const hostError = document.querySelector("#host-error");
const phpTarget = document.querySelector("#php-target");
const phpVersions = document.querySelector("#php-versions");
const customHost = document.querySelector("#custom-host");
const hostPreview = document.querySelector("#host-preview");

let state = { status: null, sites: [], runtime: null };
let activeAccount = "";
let currentView = "dashboard";

async function api(method, url, body) {
  const opts = { method, headers: { "X-Phlo": "1" } };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(url, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function href(host) {
  return serverURL("http", host, state.status.httpPort);
}

// projectURLs are the links on the Projects page. The hosts file stays a name without a port.
function projectURLs(host) {
  const status = state.status;
  const urls = [serverURL("http", host, status.httpPort)];
  if (status.sslEnabled) urls.push(serverURL("https", host, status.sslPort));
  return urls;
}

function hostLinks(host) {
  const wrap = el("span", undefined, "host-links");
  for (const url of projectURLs(host)) {
    const link = el("a", url.replace(/\/$/, ""), "host");
    link.href = url;
    link.target = "_blank";
    link.rel = "noopener";
    wrap.append(link);
  }
  return wrap;
}

function serverURL(scheme, ip, port) {
  const standard = scheme === "https" ? 443 : 80;
  return port === standard ? `${scheme}://${ip}/` : `${scheme}://${ip}:${port}/`;
}

// webAddresses are the addresses Nginx serves projects on: 127.0.0.1, then the Settings host IP.
function webAddresses() {
  const ip = state.status.hostIp;
  return ip && ip !== "127.0.0.1" ? ["127.0.0.1", ip] : ["127.0.0.1"];
}

function linkList(parent, prefix, urls) {
  parent.replaceChildren();
  if (prefix) parent.append(document.createTextNode(prefix));
  urls.forEach((url, index) => {
    if (index) parent.append(document.createTextNode(" · "));
    const link = el("a", url, "host");
    link.href = url;
    link.target = "_blank";
    link.rel = "noopener";
    parent.append(link);
  });
}

function el(tag, text, className) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function showBanner(message, tone) {
  if (message && state.status && message === state.status.rootProblem) {
    message = "Move the Phlo folder first, as the message above explains.";
  }
  banner.textContent = message || "";
  banner.className = message ? `banner ${tone || "info"}` : "banner";
}

function showView(name) {
  currentView = name;
  for (const button of document.querySelectorAll(".nav-btn")) {
    button.classList.toggle("active", button.dataset.view === name);
  }
  for (const view of document.querySelectorAll(".view")) {
    view.hidden = view.id !== "view-" + name;
  }
}

async function loadRelease() {
  const data = await api("GET", "/api/release");
  const version = document.querySelector("#about-version");
  const title = document.querySelector("#about-title");
  const notes = document.querySelector("#about-notes");
  const box = document.querySelector("#about-release");
  const button = document.querySelector("#about-update");
  const note = document.querySelector("#update-note");
  const noteText = document.querySelector("#update-note-text");
  const noteButton = document.querySelector("#update-open");
  if (version) version.textContent = "Phlo " + data.version;
  if (title) title.textContent = data.title || "";
  if (notes) {
    notes.replaceChildren(...(data.notes || []).map((line) => el("li", line)));
  }
  if (box) box.textContent = data.message || "";
  const canUpdate = Boolean(data.canUpdate);
  if (button) {
    button.hidden = !canUpdate;
    button.disabled = false;
    button.onclick = () => installUpdate(button);
  }
  if (note && noteText) {
    note.hidden = Boolean(data.upToDate);
    noteText.textContent = data.upToDate ? "" : (data.message || "");
  }
  if (noteButton) {
    noteButton.hidden = !canUpdate;
    noteButton.disabled = false;
    noteButton.onclick = () => installUpdate(noteButton);
  }
}

async function installUpdate(button) {
  if (button) button.disabled = true;
  showBanner("Updating Phlo. Your sites and databases stay where they are.", "info");
  try {
    const data = await api("POST", "/api/update");
    showBanner(data.warning || "Phlo is restarting.", "info");
    await waitForRestart();
  } catch (err) {
    if (button) button.disabled = false;
    showBanner(err.message, "error");
  }
}

async function waitForRestart() {
  let sawDown = false;
  for (let i = 0; i < 40; i++) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    try {
      const res = await fetch("/api/health", { cache: "no-store" });
      if (res.ok && sawDown) {
        location.reload();
        return;
      }
    } catch {
      sawDown = true;
    }
  }
}

async function refresh() {
  refreshing = true;
  try {
    const [status, sites, runtime] = await Promise.all([
      api("GET", "/api/status"),
      api("GET", "/api/sites"),
      api("GET", "/api/runtime"),
    ]);
    state = { status, sites, runtime };
    liveKeySeen = liveKey(status);
    applyStatus(status);
    renderSettings();
    renderRuntimes();
    renderSites();
  } finally {
    refreshing = false;
  }
}

function liveKey(status) {
  return [
    status.running,
    status.mysqlRunning,
    status.redisRunning,
    status.memcachedRunning,
    status.mailpitRunning,
    status.postgresRunning,
    status.mongodbRunning,
    status.cronRunning,
    status.cronicalInstalled ? "1" : "0",
    status.port80Taken ? "1" : "0",
    status.shareHost || "",
    status.shareUrl || "",
    status.ngrokTokenSet ? "1" : "0",
    status.rootProblem || "",
    status.hostsWarning || "",
  ].join("\n");
}

function applyStatus(status) {
  runState.textContent = status.running ? "Running" : "Stopped";
  runState.className = status.running ? "pill on" : "pill";
  paintRunButtons(status);
  rootProblem.textContent = status.rootProblem || "";
  hostsWarning.textContent = status.hostsWarning || "";
  document.querySelector("#port80-note").textContent = port80Note(status);
  renderVCRuntime(status.vcRuntime);
  renderDashboard();
  paintServiceRunState(status);
}

function paintServiceRunState(status) {
  document.querySelector("#db-state").textContent = serviceLabel(status.mysqlRunning, status.mariadbEnabled, "MariaDB");
  document.querySelector("#redis-state").textContent = serviceLabel(status.redisRunning, status.redisEnabled, "Redis");
  document.querySelector("#memcached-state").textContent = serviceLabel(status.memcachedRunning, status.memcachedEnabled, "Memcached");
  document.querySelector("#mailpit-state").textContent = serviceLabel(status.mailpitRunning, status.mailpitEnabled, "Mailpit");
  renderMailpitMeta();
  document.querySelector("#postgres-state").textContent = serviceLabel(status.postgresRunning, status.postgresEnabled, "PostgreSQL");
  document.querySelector("#mongodb-state").textContent = serviceLabel(status.mongodbRunning, status.mongodbEnabled, "MongoDB");
  const cronState = document.querySelector("#cron-state");
  if (cronState) cronState.textContent = cronLabel(status);
  paintShares();
}

async function followStack() {
  if (stackBusy || refreshing || !state.status) return;
  try {
    const status = await api("GET", "/api/status");
    if (stackBusy || refreshing) return;
    const key = liveKey(status);
    if (key === liveKeySeen) return;
    liveKeySeen = key;
    state.status = status;
    applyStatus(status);
  } catch {
    // The next poll tries again. A failed check leaves the page as it is.
  }
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

function fillEnv(id, lines) {
  document.querySelector(id).textContent = lines.join("\n");
}

function serviceLabel(running, enabled, name) {
  if (!enabled) return name + " is off";
  return running ? name + " is running" : name + " is stopped";
}

function port80Note(status) {
  if (!status || !status.port80Taken) return "";
  return "Another program is using port 80. Open Phlo sites with the port in the link, such as http://name." +
    status.tld + ":" + status.httpPort + "/.";
}

function cronLabel(status) {
  if (!status.cronEnabled) return "Cronical is off";
  if (!status.cronicalInstalled) return "Cronical is not installed";
  return status.cronRunning ? "Cronical is running" : "Cronical is stopped";
}

function renderVCRuntime(check) {
  const box = document.querySelector("#vc-runtime");
  box.hidden = !check || check.ok;
  if (box.hidden) return;
  const versions = (check.blocked || []).join(", ");
  const have = check.have ? `This PC has version ${check.have}; PHP needs ${check.need} or newer.` : "It is not installed on this PC.";
  document.querySelector("#vc-runtime-text").textContent =
    `PHP ${versions} cannot start without the Microsoft Visual C++ runtime. ${have} ` +
    "The button downloads Microsoft's installer, checks Microsoft's signature, and asks Windows for administrator approval.";
}

function toolOf(kind) {
  return (state.runtime.tools || []).find((item) => item.kind === kind);
}

function phpExtOf(name) {
  return (state.runtime.phpExtensions || []).filter((item) => item.name === name);
}

function renderDashboard() {
  const status = state.status;
  document.querySelector("#dash-stack").textContent = status.running
    ? "Stack is running on " + webAddresses().join(" and ")
    : "Stack is stopped";
  document.querySelector("#dash-nginx-state").textContent = status.running ? "Nginx is running" : "Nginx is stopped";
  document.querySelector("#dash-nginx-meta").textContent = status.nginx
    ? `${status.nginx} — HTTP ${status.httpPort}` + (status.sslEnabled ? ` — SSL on · HTTPS ${status.sslPort}` : " — SSL off")
    : "Not installed";
  const urls = webAddresses().map((ip) => serverURL("http", ip, status.httpPort));
  if (status.sslEnabled) urls.push(...webAddresses().map((ip) => serverURL("https", ip, status.sslPort)));
  linkList(document.querySelector("#dash-nginx-ports"), "", urls);

  const showMariaDB = status.mariadbEnabled !== false;
  const showRedis = status.redisEnabled !== false;
  const showMemcached = status.memcachedEnabled !== false;
  const showMailpit = status.mailpitEnabled !== false;
  document.querySelector("#dash-panel-mariadb").hidden = !showMariaDB;
  document.querySelector("#dash-panel-redis").hidden = !showRedis;
  document.querySelector("#dash-panel-memcached").hidden = !showMemcached;
  document.querySelector("#dash-panel-mailpit").hidden = !showMailpit;

  document.querySelector("#dash-db-state").textContent = status.mysqlRunning ? "MariaDB is running" : "MariaDB is stopped";
  document.querySelector("#dash-db-meta").textContent =
    `127.0.0.1:${status.mysqlPort} — ${status.mariadb || "—"} — data ${status.dataDir}`;
  fillEnv("#dash-db-env", [
    "DB_CONNECTION=mysql",
    "DB_HOST=127.0.0.1",
    "DB_PORT=" + status.mysqlPort,
    "DB_DATABASE=",
    "DB_USERNAME=root",
    "DB_PASSWORD=",
  ]);
  const pma = document.querySelector("#dash-pma-row");
  pma.replaceChildren();
  if (status.phpMyAdmin) {
    const link = el("a", href(status.phpMyAdmin).replace(/\/$/, ""), "host");
    link.href = href(status.phpMyAdmin);
    link.target = "_blank";
    link.rel = "noopener";
    pma.append(link, el("span", " — root, empty password", "hint"));
  }

  document.querySelector("#dash-panel-postgres").hidden = !status.postgresEnabled;
  document.querySelector("#dash-postgres-state").textContent = status.postgresRunning ? "PostgreSQL is running" : "PostgreSQL is stopped";
  document.querySelector("#dash-postgres-meta").textContent =
    `127.0.0.1:${status.postgresPort} — ${status.postgres || "—"} — data ${status.postgresData}`;
  fillEnv("#dash-postgres-env", [
    "DB_CONNECTION=pgsql",
    "DB_HOST=127.0.0.1",
    "DB_PORT=" + status.postgresPort,
    "DB_DATABASE=postgres",
    "DB_USERNAME=postgres",
    "DB_PASSWORD=",
  ]);

  document.querySelector("#dash-panel-mongodb").hidden = !status.mongodbEnabled;
  document.querySelector("#dash-mongodb-state").textContent = status.mongodbRunning ? "MongoDB is running" : "MongoDB is stopped";
  const mongoPHP = phpExtOf("mongodb").filter((item) => item.installed).map((item) => item.php);
  document.querySelector("#dash-mongodb-meta").textContent =
    `127.0.0.1:${status.mongodbPort} — ${status.mongodb || "—"} — data ${status.mongodbData}. ` +
    (mongoPHP.length
      ? `PHP mongodb extension loaded for PHP ${mongoPHP.join(", ")}.`
      : "PHP cannot use MongoDB yet: add the mongodb extension in Settings → MongoDB.");
  fillEnv("#dash-mongodb-env", [
    `MONGODB_URI=mongodb://127.0.0.1:${status.mongodbPort}`,
    "MONGODB_DATABASE=",
  ]);

  document.querySelector("#dash-redis-state").textContent = status.redisRunning ? "Redis is running" : "Redis is stopped";
  document.querySelector("#dash-redis-meta").textContent =
    `127.0.0.1:${status.redisPort} — ${status.redis || "—"}`;
  fillEnv("#dash-redis-env", [
    "REDIS_CLIENT=predis",
    "REDIS_HOST=127.0.0.1",
    "REDIS_PASSWORD=null",
    "REDIS_PORT=" + status.redisPort,
  ]);

  document.querySelector("#dash-memcached-state").textContent = status.memcachedRunning ? "Memcached is running" : "Memcached is stopped";
  document.querySelector("#dash-memcached-meta").textContent =
    `127.0.0.1:${status.memcachedPort} — ${status.memcached || "—"}`;
  fillEnv("#dash-memcached-env", [
    "MEMCACHED_HOST=127.0.0.1",
    "MEMCACHED_PORT=" + status.memcachedPort,
    "MEMCACHED_USERNAME=",
    "MEMCACHED_PASSWORD=",
  ]);

  document.querySelector("#dash-mailpit-state").textContent = status.mailpitRunning ? "Mailpit is running" : "Mailpit is stopped";
  const mailMeta = document.querySelector("#dash-mailpit-meta");
  mailMeta.replaceChildren();
  mailMeta.append(document.createTextNode(`smtp 127.0.0.1:${status.mailpitSmtpPort} — ${status.mailpit || "—"}`));
  if (status.mailpitRunning && status.mailpitUrl) {
    const open = el("a", "Open mailbox", "host");
    open.href = status.mailpitUrl;
    open.target = "_blank";
    open.rel = "noopener";
    mailMeta.append(document.createTextNode(" — "), open);
  }
  fillEnv("#dash-mailpit-env", [
    "MAIL_MAILER=smtp",
    "MAIL_SCHEME=null",
    "MAIL_HOST=127.0.0.1",
    "MAIL_PORT=" + status.mailpitSmtpPort,
    "MAIL_USERNAME=null",
    "MAIL_PASSWORD=null",
    "MAIL_ENCRYPTION=null",
  ]);
}

function renderSettings() {
  const status = state.status;
  defaultSelect.replaceChildren();
  for (const item of state.runtime.php) {
    const option = el("option", item.version + (item.installed ? " installed" : " not installed"));
    option.value = item.version;
    if (item.version === status.defaultPhp) option.selected = true;
    defaultSelect.append(option);
  }
  fillVersionSelect(nginxSelect, state.runtime.nginxVersions, status.nginx);
  fillVersionSelect(mariadbSelect, state.runtime.mariadbVersions, status.mariadb);
  fillVersionSelect(redisSelect, state.runtime.redisVersions, status.redis);
  fillVersionSelect(memcachedSelect, state.runtime.memcachedVersions, status.memcached);
  fillVersionSelect(mailpitSelect, state.runtime.mailpitVersions, status.mailpit);

  document.querySelector("#mariadb-enabled").checked = status.mariadbEnabled;
  document.querySelector("#redis-enabled").checked = status.redisEnabled;
  document.querySelector("#memcached-enabled").checked = status.memcachedEnabled;
  document.querySelector("#mailpit-enabled").checked = status.mailpitEnabled;
  document.querySelector("#ssl-enabled").checked = status.sslEnabled;
  linkList(document.querySelector("#http-state"), "HTTP — ", webAddresses().map((ip) => serverURL("http", ip, status.httpPort)));
  const sslState = document.querySelector("#ssl-state");
  if (status.sslEnabled) {
    linkList(sslState, "SSL on — ", webAddresses().map((ip) => serverURL("https", ip, status.sslPort)));
  } else {
    sslState.textContent = "SSL is off";
  }
  renderHostIP();

  document.querySelector("#http-port").value = status.httpPort;
  document.querySelector("#ssl-port").value = status.sslPort;
  document.querySelector("#mysql-port").value = status.mysqlPort;
  document.querySelector("#redis-port").value = status.redisPort;
  document.querySelector("#memcached-port").value = status.memcachedPort;
  document.querySelector("#mailpit-smtp-port").value = status.mailpitSmtpPort;
  document.querySelector("#mailpit-ui-port").value = status.mailpitUiPort;

  document.querySelector("#db-state").textContent = serviceLabel(status.mysqlRunning, status.mariadbEnabled, "MariaDB");
  document.querySelector("#db-meta").textContent = `127.0.0.1:${status.mysqlPort} — MySQL user root with an empty password.`;
  renderPHPMyAdmin();

  document.querySelector("#redis-state").textContent = serviceLabel(status.redisRunning, status.redisEnabled, "Redis");
  document.querySelector("#redis-meta").textContent = "127.0.0.1:" + status.redisPort;
  document.querySelector("#memcached-state").textContent = serviceLabel(status.memcachedRunning, status.memcachedEnabled, "Memcached");
  document.querySelector("#memcached-meta").textContent = "127.0.0.1:" + status.memcachedPort;
  document.querySelector("#mailpit-state").textContent = serviceLabel(status.mailpitRunning, status.mailpitEnabled, "Mailpit");
  renderMailpitMeta();

  fillVersionSelect(postgresSelect, toolOf("postgres")?.versions, status.postgres);
  fillVersionSelect(mongodbSelect, toolOf("mongodb")?.versions, status.mongodb);
  document.querySelector("#postgres-enabled").checked = status.postgresEnabled;
  document.querySelector("#mongodb-enabled").checked = status.mongodbEnabled;
  document.querySelector("#postgres-port").value = status.postgresPort;
  document.querySelector("#mongodb-port").value = status.mongodbPort;
  document.querySelector("#postgres-state").textContent = serviceLabel(status.postgresRunning, status.postgresEnabled, "PostgreSQL");
  document.querySelector("#mongodb-state").textContent = serviceLabel(status.mongodbRunning, status.mongodbEnabled, "MongoDB");
  const missing = (kind) => (toolOf(kind)?.selected.installed ? "" : " Not installed — download it under Runtimes.");
  document.querySelector("#postgres-meta").textContent =
    `127.0.0.1:${status.postgresPort} — user postgres with no password. The first start runs initdb.` + missing("postgres");
  document.querySelector("#mongodb-meta").textContent = `127.0.0.1:${status.mongodbPort} — no user or password.` + missing("mongodb");
  renderMongosh();
  renderPHPExtensions("mongodb", document.querySelector("#mongodb-php"));
  renderPHPExtensions("pgsql", document.querySelector("#postgres-php"));
  renderCron();
  renderDBTools();
  renderNgrokToken();
  renderToolchains();
  renderFolders();
}

function renderHostIP() {
  const status = state.status;
  const input = document.querySelector("#host-ip");
  if (document.activeElement !== input) input.value = status.hostIp;
  document.querySelector("#host-ips").replaceChildren(
    ...(status.hostIps || []).map((ip) => {
      const option = document.createElement("option");
      option.value = ip;
      return option;
    }),
  );
  const network = (status.hostIps || []).filter((ip) => ip !== "127.0.0.1");
  const hint = document.querySelector("#host-ip-hint");
  if (status.hostIp === "127.0.0.1") {
    hint.textContent =
      "Projects open only on this PC. To open them from a phone or another PC on your network, enter this PC's address" +
      (network.length ? ` (this PC has ${network.join(", ")})` : "") +
      " and Save IP.";
    return;
  }
  const example = state.sites[0]?.host || `inventory.${status.tld}`;
  const firewall = status.root.includes("\\")
    ? " Windows asks once whether Nginx may accept network connections; allow it on Private networks."
    : "";
  hint.textContent =
    `Projects answer on ${status.hostIp} and 127.0.0.1, and the hosts file points your project names at ${status.hostIp}. ` +
    "phpMyAdmin, the dashboard, and the databases stay on 127.0.0.1." +
    firewall +
    ` Another device needs the names in its own hosts file, such as ${status.hostIp} ${example}. Enter 127.0.0.1 to go back.`;
}

// Folders Settings can move: the status field that holds each one, and the database that uses it.
const folderKinds = {
  home: { field: "homeDir" },
  mariadb: { field: "dataDir", label: "MariaDB" },
  postgres: { field: "postgresData", label: "PostgreSQL" },
  mongodb: { field: "mongodbData", label: "MongoDB" },
};

function renderFolders() {
  const status = state.status;
  for (const [kind, { field }] of Object.entries(folderKinds)) {
    const input = document.querySelector(`#${kind}-dir`);
    if (document.activeElement !== input) input.value = status[field];
  }
  const windows = status.root.includes("\\");
  const sep = windows ? "\\" : "/";
  document.querySelector("#home-dir-hint").textContent =
    `A name such as www is a folder inside Phlo; a full path such as ${windows ? "D:\\www" : "/srv/www"} works too. ` +
    "The old folder stays where it is.";
  for (const [kind, { label }] of Object.entries(folderKinds)) {
    if (!label) continue;
    const full = windows ? `D:\\databases\\${kind}` : `/srv/databases/${kind}`;
    document.querySelector(`#${kind}-dir-hint`).textContent =
      `A name such as databases${sep}${kind} is a folder inside Phlo; a full path such as ${full} works too. ` +
      `If ${label} is running, Change restarts it from the new folder. The old folder stays where it is.`;
  }
  document.querySelector("#add-project-hint").textContent =
    `Creates ${status.homeDir}${sep}name${sep}public_html and serves name.${status.tld}. Links include the port when it is not 80 or 443. Log opens that project's log.`;
}

async function moveFolder(kind, button) {
  const { field, label } = folderKinds[kind];
  const before = state.status[field];
  const copy = document.querySelector(`#${kind}-copy`).checked;
  button.disabled = true;
  showBanner(copy ? "Copying… a large folder can take a few minutes. Keep this page open." : "Switching folders…", "info");
  try {
    const data = await api("PUT", `/api/folders/${kind}`, { path: document.querySelector(`#${kind}-dir`).value.trim(), copy });
    await refresh();
    const after = state.status[field];
    let message = label ? `${label} now uses ${after}.` : `Projects now use ${after}.`;
    if (after !== before) {
      message += ` ${before} was left as it is.`;
      if (!label) message += " Close any Terminal window that is still open in the old folder.";
    }
    showBanner(data.warning ? `${message} ${data.warning}` : message, data.warning ? "warning" : "info");
  } catch (err) {
    try {
      await refresh();
    } catch (_) {
      /* keep the error */
    }
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
}

function renderPHPExtensions(name, list) {
  list.replaceChildren();
  const rows = phpExtOf(name);
  if (!rows.length) {
    list.append(el("li", name === "pgsql" ? "Install a PHP version first." : "Install a PHP version first, then add the extension to it here."));
    return;
  }
  for (const item of rows) {
    const row = el("li");
    const label = el("span", `PHP ${item.php} (${item.build})`);
    row.append(label);
    if (item.installed) {
      row.append(el("span", item.bundled ? `${item.label} loaded` : `${name} extension loaded`, "state on"));
    } else if (item.bundled) {
      row.append(el("span", item.missing, "state warn"));
    } else if (item.downloadable) {
      const button = el("button", `Add ${name} ${item.version}`, "secondary mini");
      button.type = "button";
      button.addEventListener("click", () =>
        download("phpext-" + name, item.php, button, `the ${name} ${item.version} extension for PHP ${item.php}`));
      row.append(button);
    } else {
      row.append(el("span", `No ${item.label} build for PHP ${item.build}. Use PHP 8.1 or newer for this site.`, "state"));
    }
    list.append(row);
  }
}

function renderMongosh() {
  const row = document.querySelector("#mongosh-row");
  row.replaceChildren();
  const shell = toolOf("mongosh");
  if (!shell) return;
  if (shell.selected.installed) {
    row.append(document.createTextNode(`mongosh ${shell.selected.version} opens with the Terminal button.`));
    return;
  }
  row.append(document.createTextNode("The Terminal button needs mongosh. "));
  const next = (shell.versions || []).find((item) => item.downloadable);
  if (next) {
    const button = el("button", `Download mongosh ${next.version}`, "secondary mini");
    button.type = "button";
    button.addEventListener("click", () => download("mongosh", next.version, button));
    row.append(button);
  }
}

function renderToolchains() {
  toolchainsEl.replaceChildren();
  for (const tool of (state.runtime.tools || []).filter((item) => item.toolchain)) {
    toolchainsEl.append(toolchainRow(tool));
  }
}

function toolchainRow(tool) {
  const row = el("div", undefined, "toolchain");
  const head = el("div", undefined, "toolchain-head");
  const dot = el("span", "", tool.selected.installed ? "dot on" : "dot");
  dot.setAttribute("aria-hidden", "true");
  head.append(dot, el("strong", tool.label));
  const sw = el("label", undefined, "switch");
  const box = document.createElement("input");
  box.type = "checkbox";
  box.checked = tool.onPath;
  box.disabled = !tool.selected.installed;
  box.setAttribute("aria-label", `${tool.label} on PATH`);
  box.addEventListener("change", () => {
    putSettings({ toolPath: { [tool.kind]: box.checked } }, `${tool.label} is ${box.checked ? "on" : "off"} the Terminal PATH.`);
  });
  sw.append(box, el("span", "On PATH"));
  head.append(sw);

  const controls = el("div", undefined, "row");
  const select = document.createElement("select");
  select.setAttribute("aria-label", `${tool.label} version`);
  const use = el("button", "Use", "secondary");
  use.type = "button";
  use.addEventListener("click", () => putSettings({ tools: { [tool.kind]: select.value } }, `${tool.label} ${select.value} is in use.`));
  controls.append(select, use);
  const next = (tool.versions || []).find((item) => item.downloadable && !item.installed);
  if (next) {
    const button = el("button", `Download ${next.version}`, "secondary");
    button.type = "button";
    button.addEventListener("click", () => download(tool.kind, next.version, button));
    controls.append(button);
  }
  fillVersionSelect(select, tool.versions, tool.selected.version);

  const path = el("code", tool.path || toolHint(tool), "doc-root");
  row.append(head, controls, path);
  if (tool.note) row.append(el("p", tool.note, "hint tool-note"));
  return row;
}

function toolHint(tool) {
  if ((tool.versions || []).length > 0) return "Not installed";
  if (tool.kind === "gcc") return "No Phlo build for this system";
  return `Not in the manifest for this system. Drop a build into runtime/${tool.kind}/{version}/.`;
}

async function checkDocker() {
  const line = document.querySelector("#docker-engine");
  line.textContent = "Checking the Docker engine…";
  try {
    const info = await api("GET", "/api/docker");
    if (!info.installed) {
      line.textContent = "Docker CLI is not installed. Download it above.";
    } else if (info.reachable) {
      line.textContent = `Engine reachable — server ${info.server}, client ${info.client}.`;
    } else {
      line.replaceChildren();
      line.append(el("strong", info.hint || "The Docker engine did not answer."));
      const details = el("details", undefined, "engine-details");
      details.append(el("summary", "Details"), el("code", info.error, "doc-root"));
      line.append(document.createTextNode(" Phlo includes the docker and compose commands only; the engine comes from Docker Desktop or dockerd. "), details);
    }
  } catch (err) {
    line.textContent = err.message;
  }
}

function fillVersionSelect(select, items, selected) {
  select.replaceChildren();
  const installed = (items || []).filter((item) => item.installed);
  if (installed.length === 0) {
    const option = el("option", "Not installed");
    option.value = "";
    select.append(option);
    select.disabled = true;
    const button = select.parentElement.querySelector("button");
    if (button) button.disabled = true;
    return;
  }
  for (const item of installed) {
    const option = el("option", item.version);
    option.value = item.version;
    if (item.version === selected) option.selected = true;
    select.append(option);
  }
  select.disabled = false;
  const button = select.parentElement.querySelector("button");
  if (button) button.disabled = false;
}

function renderPHPMyAdmin() {
  const row = document.querySelector("#pma-row");
  row.replaceChildren();
  const host = state.status.phpMyAdmin;
  if (!host) {
    row.append(el("span", "phpMyAdmin is not installed. Download it under Runtimes.", "hint"));
    return;
  }
  const link = el("a", href(host).replace(/\/$/, ""), "host");
  link.href = href(host);
  link.target = "_blank";
  link.rel = "noopener";
  row.append(link, el("span", " — root, empty password", "hint"));
}

function renderMailpitMeta() {
  const row = document.querySelector("#mailpit-meta");
  row.replaceChildren();
  const status = state.status;
  row.append(document.createTextNode("smtp 127.0.0.1:" + status.mailpitSmtpPort));
  if (status.mailpitRunning && status.mailpitUrl) {
    const open = el("a", "Open mailbox", "host");
    open.href = status.mailpitUrl;
    open.target = "_blank";
    open.rel = "noopener";
    row.append(document.createTextNode(" — "), open);
  }
}

function renderRuntimes() {
  runtimesEl.replaceChildren();
  for (const item of state.runtime.nginxVersions || []) {
    runtimesEl.append(componentRow("Nginx", item.version, item.installed, item.downloadable, "nginx", item.version));
  }
  for (const item of state.runtime.mariadbVersions || []) {
    runtimesEl.append(componentRow("MariaDB", item.version, item.installed, item.downloadable, "mariadb", item.version, "mariadb"));
  }
  const pma = state.runtime.phpmyadmin;
  if (pma) {
    runtimesEl.append(componentRow("phpMyAdmin", pma.version, pma.installed, pma.downloadable, "phpmyadmin", ""));
  }
  const composer = state.runtime.composer;
  runtimesEl.append(componentRow("Composer", composer.version, composer.installed, composer.downloadable, "composer", ""));
  for (const item of state.runtime.redisVersions || []) {
    runtimesEl.append(componentRow("Redis", item.version, item.installed, item.downloadable, "redis", item.version));
  }
  for (const item of state.runtime.memcachedVersions || []) {
    runtimesEl.append(componentRow("Memcached", item.version, item.installed, item.downloadable, "memcached", item.version));
  }
  for (const item of state.runtime.mailpitVersions || []) {
    runtimesEl.append(componentRow("Mailpit", item.version, item.installed, item.downloadable, "mailpit", item.version));
  }
  for (const item of state.runtime.cronicalVersions || []) {
    runtimesEl.append(componentRow("Cronical", item.version, item.installed, item.downloadable, "cronical", item.version, "cronical"));
  }
  for (const item of state.runtime.heidisqlVersions || []) {
    runtimesEl.append(componentRow("HeidiSQL", item.version, item.installed, item.downloadable, "heidisql", item.version));
  }
  for (const item of state.runtime.sqlyogVersions || []) {
    runtimesEl.append(componentRow("SQLyog", item.version, item.installed, item.downloadable, "sqlyog", item.version));
  }
  for (const item of state.runtime.ngrokVersions || []) {
    runtimesEl.append(componentRow("ngrok", item.version, item.installed, item.downloadable, "ngrok", item.version));
  }
  for (const tool of state.runtime.tools || []) {
    const settings = configFiles[tool.kind] ? tool.kind : "";
    for (const item of tool.versions || []) {
      runtimesEl.append(componentRow(tool.label, item.version, item.installed, item.downloadable, tool.kind, item.version, settings));
    }
  }
  for (const item of state.runtime.php) {
    runtimesEl.append(componentRow("PHP", item.patch || item.version, item.installed, item.downloadable, "php", item.version, "php"));
  }
}

// configFiles are the settings files the Runtimes editors open, by API kind.
const configFiles = { php: "php.ini", mariadb: "my.ini", postgres: "postgresql.conf", mongodb: "mongod.conf", cronical: "cronical.dat" };

function componentRow(label, version, installed, downloadable, component, phpVersion, settings) {
  const row = el("div", undefined, "runtime");
  const dot = el("span", "", installed ? "dot on" : "dot");
  dot.setAttribute("aria-hidden", "true");
  const copy = el("div", undefined, "runtime-copy");
  copy.append(el("strong", label), el("span", version || "—"));
  const actions = el("span", undefined, "runtime-actions");
  row.append(dot, copy, el("span", installed ? "Installed" : "Not installed", installed ? "state on" : "state"), actions);
  if (downloadable && !installed) {
    const button = el("button", "Download", "secondary");
    button.type = "button";
    button.addEventListener("click", () => download(component, phpVersion, button));
    actions.append(button);
  }
  if (installed && settings) {
    const button = el("button", `Edit ${configFiles[settings]}`, "secondary");
    button.type = "button";
    button.addEventListener("click", () => openConfig(settings, settings === "php" ? phpVersion : "", button));
    actions.append(button);
  }
  return row;
}

const configDialog = document.querySelector("#config-dialog");
const configText = document.querySelector("#config-text");
const configError = document.querySelector("#config-error");
let configTarget = null;

async function openConfig(kind, version, button) {
  button.disabled = true;
  showBanner("");
  try {
    const query = version ? `?version=${encodeURIComponent(version)}` : "";
    const file = await api("GET", `/api/config-files/${kind}${query}`);
    configTarget = { kind, version: file.version || "" };
    document.querySelector("#config-title").textContent = file.title;
    document.querySelector("#config-hint").textContent = file.hint;
    document.querySelector("#config-file").textContent = file.file;
    document.querySelector("#config-generated").textContent = file.generated;
    configText.value = file.custom;
    configText.placeholder = "Example:\n" + file.example;
    configError.textContent = "";
    configDialog.showModal();
    configText.focus();
  } catch (err) {
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
}

document.querySelector("#config-cancel").addEventListener("click", () => configDialog.close());
document.querySelector("#config-save").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.textContent = "Saving…";
  configError.textContent = "";
  try {
    const data = await api("PUT", `/api/config-files/${configTarget.kind}`, { version: configTarget.version, content: configText.value });
    configDialog.close();
    await refresh();
    showBanner(data.message, "info");
  } catch (err) {
    configError.textContent = err.message;
  } finally {
    button.disabled = false;
    button.textContent = "Save";
  }
});

function renderSites() {
  sitesEl.replaceChildren();
  if (state.sites.length === 0) {
    sitesEl.append(el("p", "No sites yet. Add an account to create name/public_html in the project folder.", "empty"));
    return;
  }
  for (const site of state.sites) {
    const card = el("article", undefined, "card");
    const head = el("div");
    head.append(el("p", "Account", "eyebrow"), el("h3", site.account));
    card.append(head);
    const roots = el("ul", undefined, "roots");
    if (site.publicHtml) {
      roots.append(rootLine(site.account, "public_html", site.publicHtml.host, site.publicHtml.php, site.publicHtml.phpSource, site.publicHtml.documentRoot, site.publicHtml.phpPath));
    }
    for (const sub of site.subdomains) {
      roots.append(rootLine(site.account, sub.name, sub.host, sub.php, sub.phpSource, sub.documentRoot, sub.phpPath));
    }
    card.append(roots);
    const actions = el("div", undefined, "card-tools");
    actions.append(actionButton("Change PHP", "php", site.account));
    actions.append(actionButton("Custom host", "host", site.account));
    const open = el("a", "Open", "open-link");
    open.href = site.publicHtml ? href(site.host) : href(site.subdomains[0]?.host || site.host);
    open.target = "_blank";
    open.rel = "noopener";
    actions.append(open);
    card.append(actions);
    const subRow = el("div", undefined, "row sub-add");
    const input = document.createElement("input");
    input.setAttribute("aria-label", "Subdomain name");
    input.placeholder = "shop";
    const add = el("button", "Add subdomain", "secondary");
    add.type = "button";
    add.addEventListener("click", () => addSubdomain(site.account, input));
    subRow.append(input, add);
    card.append(subRow);
    sitesEl.append(card);
  }
  paintShares();
}

function rootLine(account, target, host, php, source, path, phpPath) {
  const item = el("li", undefined, "root");
  const badge = el("span", `PHP ${php}`, source === "override" ? "badge override" : "badge");
  const tools = el("span", undefined, "root-tools");
  const folder = el("button", "Folder", "secondary mini");
  folder.type = "button";
  folder.addEventListener("click", () => openTerminal({ kind: "folder", account, target }));
  const log = el("button", "Log", "secondary mini");
  log.type = "button";
  log.title = "Open this project's log";
  log.addEventListener("click", () => openTerminal({ kind: "log", account, target }));
  const terminal = el("button", "Terminal", "secondary mini");
  terminal.type = "button";
  terminal.addEventListener("click", () => openTerminal({ kind: "site", account, target }));
  const laravel = el("button", "Laravel", "secondary mini");
  laravel.type = "button";
  laravel.addEventListener("click", () => openTerminal({ kind: "laravel", account, target }));
  const share = el("button", "Share", "secondary mini");
  share.type = "button";
  share.dataset.shareHost = host;
  share.addEventListener("click", () => shareHost(host, share));
  tools.append(folder, log, terminal, laravel, share);
  const phpRow = el("div", undefined, "php-path");
  const phpCode = el("code", phpPath || "PHP is not installed");
  const copy = el("button", "Copy", "quiet mini");
  copy.type = "button";
  copy.disabled = !phpPath;
  copy.addEventListener("click", () => copyText(copy, phpPath, phpCode));
  phpRow.append(phpCode, copy);
  const shared = el("span", "", "source");
  shared.dataset.shareUrl = host;
  item.append(hostLinks(host), badge, el("span", source, "source"), tools, shared, el("code", path, "doc-root"), phpRow);
  return item;
}

function paintShares() {
  const status = state.status;
  if (!status) return;
  document.querySelectorAll("[data-share-host]").forEach((button) => {
    const active = status.shareHost === button.dataset.shareHost && status.shareUrl;
    button.textContent = active ? "Stop share" : "Share";
  });
  document.querySelectorAll("[data-share-url]").forEach((node) => {
    const active = status.shareHost === node.dataset.shareUrl && status.shareUrl;
    node.textContent = active ? status.shareUrl : "";
  });
}

async function shareHost(host, button) {
  const active = state.status.shareHost === host && state.status.shareUrl;
  button.disabled = true;
  showBanner("");
  try {
    const data = active
      ? await api("POST", "/api/share/stop")
      : await api("POST", "/api/share", { host });
    await refresh();
    showBanner(data.warning || (active ? "Share stopped." : "Sharing " + host + "."), "info");
  } catch (err) {
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
}

function renderCron() {
  const windows = (state.runtime.cronicalVersions || []).length > 0;
  document.querySelector("#cron-windows").hidden = !windows;
  document.querySelector("#cron-linux").hidden = windows;
  if (!windows) return;
  document.querySelector("#cron-enabled").checked = state.status.cronEnabled;
  document.querySelector("#cron-state").textContent = cronLabel(state.status);
  document.querySelector("#download-cronical").hidden = Boolean(state.status.cronicalInstalled);
}

function renderDBTools() {
  const heidi = state.runtime.heidisql || {};
  const sqlyog = state.runtime.sqlyog || {};
  const box = document.querySelector("#db-gui");
  const windows = Boolean(heidi.downloadable || sqlyog.downloadable);
  box.hidden = !windows;
  document.querySelector("#db-gui-meta").hidden = !windows;
  if (!windows) return;
  const heidiBtn = document.querySelector("#open-heidisql");
  heidiBtn.hidden = !heidi.downloadable;
  heidiBtn.dataset.mode = heidi.installed ? "open" : "download";
  heidiBtn.textContent = heidi.installed ? "HeidiSQL" : "Download HeidiSQL";
  const sqlBtn = document.querySelector("#open-sqlyog");
  sqlBtn.hidden = !sqlyog.downloadable;
  if (state.status.sqlyogInstalled) {
    sqlBtn.dataset.mode = "open";
    sqlBtn.textContent = "SQLyog";
  } else if (sqlyog.installed) {
    sqlBtn.dataset.mode = "installer";
    sqlBtn.textContent = "Open SQLyog installer";
  } else {
    sqlBtn.dataset.mode = "download";
    sqlBtn.textContent = "Download SQLyog";
  }
  document.querySelector("#db-gui-meta").textContent =
    `HeidiSQL and SQLyog use 127.0.0.1:${state.status.mysqlPort} as root with an empty password.`;
}

function renderNgrokToken() {
  const stateEl = document.querySelector("#ngrok-token-state");
  if (document.activeElement && document.activeElement.id === "ngrok-token") return;
  stateEl.textContent = state.status.ngrokTokenSet
    ? "A token is saved. Phlo does not show it."
    : "No token yet.";
}

async function openDatabaseTool(button, kind) {
  button.disabled = true;
  showBanner("");
  try {
    if (button.dataset.mode === "download") {
      const item = kind === "heidisql" ? state.runtime.heidisql : state.runtime.sqlyog;
      await download(kind, item.version, button);
      return;
    }
    const path = kind === "heidisql"
      ? "/api/heidisql"
      : button.dataset.mode === "installer" ? "/api/sqlyog/installer" : "/api/sqlyog";
    const data = await api("POST", path);
    showBanner(data.warning || (kind === "heidisql" ? "HeidiSQL is opening." : "SQLyog is opening."), "info");
  } catch (err) {
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
}

async function openTerminal(body) {
  showBanner("");
  try {
    await api("POST", "/api/terminal", body);
    const message = body.kind === "laravel"
      ? "Laravel install started."
      : body.kind === "folder"
        ? "Folder opened."
        : body.kind === "log" || body.kind === "cron-log"
          ? "Log opened."
          : "Terminal opened.";
    showBanner(message, "info");
  } catch (err) {
    showBanner(err.message, "error");
  }
}

function actionButton(label, action, account) {
  const button = el("button", label, "secondary");
  button.type = "button";
  button.addEventListener("click", () => {
    activeAccount = account;
    if (action === "php") openPhp();
    if (action === "host") openHost();
  });
  return button;
}

function siteByAccount(account) {
  return state.sites.find((item) => item.account === account);
}

function openPhp() {
  const site = siteByAccount(activeAccount);
  phpError.textContent = "";
  document.querySelector("#php-site").textContent = site.account;
  phpTarget.replaceChildren();
  if (site.publicHtml) phpTarget.append(new Option("Public HTML", "public_html"));
  for (const sub of site.subdomains) phpTarget.append(new Option(sub.name, sub.name));
  fillPhpChoices();
  phpDialog.showModal();
}

function currentChoice() {
  const site = siteByAccount(activeAccount);
  if (phpTarget.value === "public_html") {
    return site.publicHtml.phpSource === "default" ? "default" : site.publicHtml.php;
  }
  const sub = site.subdomains.find((item) => item.name === phpTarget.value);
  return sub && sub.phpSource === "default" ? "default" : sub.php;
}

function fillPhpChoices() {
  phpVersions.replaceChildren();
  const selected = currentChoice();
  phpVersions.append(radio("default", `Use Phlo default (${state.status.defaultPhp})`, selected === "default"));
  for (const item of state.runtime.php) {
    const note = item.installed ? "installed" : "not installed";
    phpVersions.append(radio(item.version, `${item.version} ${note}`, selected === item.version));
  }
}

function radio(value, label, checked) {
  const row = el("label", undefined, "choice");
  const input = document.createElement("input");
  input.type = "radio";
  input.name = "php-choice";
  input.value = value;
  input.checked = checked;
  row.append(input, document.createTextNode(" " + label));
  return row;
}

function selectedPhp() {
  const input = phpVersions.querySelector("input:checked");
  return input ? input.value : "default";
}

function openHost() {
  const site = siteByAccount(activeAccount);
  hostError.textContent = "";
  document.querySelector("#host-folder").textContent = site.publicHtml ? site.publicHtml.documentRoot : `${site.account}/public_html`;
  document.querySelector("#host-default").textContent = site.defaultHost;
  customHost.value = site.customHost || "";
  renderPreview();
  hostDialog.showModal();
  customHost.focus();
}

function renderPreview() {
  const site = siteByAccount(activeAccount);
  const parent = customHost.value.trim().toLowerCase() || site.defaultHost;
  hostPreview.replaceChildren();
  hostPreview.append(el("li", parent));
  for (const sub of site.subdomains) {
    hostPreview.append(el("li", `${sub.name.toLowerCase()}.${parent}`));
  }
}

async function download(component, version, button, label) {
  button.disabled = true;
  showBanner(`Downloading ${label || component + " " + version}…`, "info");
  try {
    const choice = phpDialog.open ? selectedPhp() : "";
    const data = await api("POST", "/api/runtime/download", { component, version });
    await refresh();
    if (phpDialog.open) {
      phpError.textContent = "";
      fillPhpChoices();
      const input = [...phpVersions.querySelectorAll("input")].find((item) => item.value === choice);
      if (input) input.checked = true;
    }
    showBanner(data.warning || "Download finished.", "info");
  } catch (err) {
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
}

async function addSubdomain(account, input) {
  showBanner("");
  try {
    const data = await api("POST", `/api/sites/${account}/subdomains`, { name: input.value.trim() });
    await refresh();
    showBanner(data.warning || "", "warning");
  } catch (err) {
    showBanner(err.message, "error");
  }
}

async function putSettings(body, message) {
  showBanner("");
  try {
    await api("PUT", "/api/settings", body);
    await refresh();
    if (message) showBanner(message, "info");
  } catch (err) {
    try {
      await refresh();
    } catch (_) {
      /* keep prior banner */
    }
    showBanner(err.message, "error");
  }
}

for (const button of document.querySelectorAll(".nav-btn")) {
  button.addEventListener("click", () => {
    showView(button.dataset.view);
    if (button.dataset.view === "dashboard" || button.dataset.view === "settings") {
      refresh().catch((err) => showBanner(err.message, "error"));
    }
    if (button.dataset.view === "about") {
      loadRelease().catch((err) => {
        const box = document.querySelector("#about-release");
        if (box) box.textContent = err.message;
      });
    }
    if (button.dataset.view === "settings" && toolOf("docker")?.selected.installed) {
      checkDocker();
    }
  });
}

document.querySelector("#add-site-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  showBanner("");
  try {
    const data = await api("POST", "/api/sites", { account: document.querySelector("#account-name").value.trim() });
    document.querySelector("#account-name").value = "";
    await refresh();
    showBanner(data.warning || "", "warning");
  } catch (err) {
    showBanner(err.message, "error");
  }
});

startButton.addEventListener("click", async () => {
  showBanner("");
  beginStackAction(startButton, "Starting");
  try {
    const data = await api("POST", "/api/services/start");
    stackBusy = false;
    await refresh();
    showBanner(data.warning || "", "warning");
  } catch (err) {
    stackBusy = false;
    showBanner(err.message, "error");
    refresh().catch(() => paintRunButtons(state.status || { running: false }));
  }
});

stopButton.addEventListener("click", async () => {
  showBanner("");
  beginStackAction(stopButton, "Stopping");
  try {
    await api("POST", "/api/services/stop");
    stackBusy = false;
    await refresh();
  } catch (err) {
    stackBusy = false;
    showBanner(err.message, "error");
    refresh().catch(() => paintRunButtons(state.status || { running: false }));
  }
});

document.querySelector("#save-default").addEventListener("click", () => {
  putSettings({ defaultPhp: defaultSelect.value }, "Default PHP saved.");
});
for (const kind of Object.keys(folderKinds)) {
  document.querySelector(`#save-${kind}-dir`).addEventListener("click", (event) => moveFolder(kind, event.currentTarget));
}
document.querySelector("#save-nginx").addEventListener("click", () => {
  putSettings({ nginx: nginxSelect.value }, `Nginx ${nginxSelect.value} is in use.`);
});
document.querySelector("#save-mariadb").addEventListener("click", () => {
  putSettings({ mariadb: mariadbSelect.value }, `MariaDB ${mariadbSelect.value} is in use.`);
});
document.querySelector("#save-redis").addEventListener("click", () => {
  putSettings({ redis: redisSelect.value }, `Redis ${redisSelect.value} is in use.`);
});
document.querySelector("#save-memcached").addEventListener("click", () => {
  putSettings({ memcached: memcachedSelect.value }, `Memcached ${memcachedSelect.value} is in use.`);
});
document.querySelector("#save-mailpit").addEventListener("click", () => {
  putSettings({ mailpit: mailpitSelect.value }, `Mailpit ${mailpitSelect.value} is in use.`);
});

document.querySelector("#save-http-port").addEventListener("click", () => {
  putSettings({ httpPort: Number(document.querySelector("#http-port").value) }, "HTTP port saved.");
});
document.querySelector("#save-ssl-port").addEventListener("click", () => {
  putSettings({ sslPort: Number(document.querySelector("#ssl-port").value) }, "SSL port saved.");
});
document.querySelector("#save-host-ip").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  const value = document.querySelector("#host-ip").value.trim() || "127.0.0.1";
  button.disabled = true;
  showBanner("");
  try {
    const data = await api("PUT", "/api/settings", { hostIp: value });
    await refresh();
    const ip = state.status.hostIp;
    const message = ip === "127.0.0.1"
      ? "Host IP is 127.0.0.1. Projects open only on this PC."
      : `Host IP saved. Projects now answer on ${ip} and 127.0.0.1.`;
    showBanner(data.warning ? `${message} ${data.warning}` : message, data.warning ? "warning" : "info");
  } catch (err) {
    try {
      await refresh();
    } catch (_) {
      /* keep the error */
    }
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
});
document.querySelector("#ssl-enabled").addEventListener("change", (event) => {
  putSettings({ sslEnabled: event.target.checked }, event.target.checked ? "SSL enabled." : "SSL disabled.");
});
document.querySelector("#save-mysql-port").addEventListener("click", () => {
  putSettings({ mysqlPort: Number(document.querySelector("#mysql-port").value) }, "MariaDB port saved.");
});
document.querySelector("#save-redis-port").addEventListener("click", () => {
  putSettings({ redisPort: Number(document.querySelector("#redis-port").value) }, "Redis port saved.");
});
document.querySelector("#save-memcached-port").addEventListener("click", () => {
  putSettings({ memcachedPort: Number(document.querySelector("#memcached-port").value) }, "Memcached port saved.");
});
document.querySelector("#save-mailpit-smtp").addEventListener("click", () => {
  putSettings({ mailpitSmtpPort: Number(document.querySelector("#mailpit-smtp-port").value) }, "Mailpit SMTP port saved.");
});
document.querySelector("#save-mailpit-ui").addEventListener("click", () => {
  putSettings({ mailpitUiPort: Number(document.querySelector("#mailpit-ui-port").value) }, "Mailpit UI port saved.");
});

document.querySelector("#mariadb-enabled").addEventListener("change", (event) => {
  putSettings({ mariadbEnabled: event.target.checked }, event.target.checked ? "MariaDB enabled." : "MariaDB disabled.");
});
document.querySelector("#redis-enabled").addEventListener("change", (event) => {
  putSettings({ redisEnabled: event.target.checked }, event.target.checked ? "Redis enabled." : "Redis disabled.");
});
document.querySelector("#memcached-enabled").addEventListener("change", (event) => {
  putSettings({ memcachedEnabled: event.target.checked }, event.target.checked ? "Memcached enabled." : "Memcached disabled.");
});
document.querySelector("#mailpit-enabled").addEventListener("change", (event) => {
  putSettings({ mailpitEnabled: event.target.checked }, event.target.checked ? "Mailpit enabled." : "Mailpit disabled.");
});
document.querySelector("#cron-enabled").addEventListener("change", (event) => {
  putSettings({ cronEnabled: event.target.checked }, event.target.checked ? "Cron enabled." : "Cron disabled.");
});
document.querySelector("#download-cronical").addEventListener("click", (event) => {
  const version = (state.runtime.cronical && state.runtime.cronical.version) || "";
  download("cronical", version, event.currentTarget, "Cronical " + version);
});
document.querySelector("#edit-cron").addEventListener("click", (event) => openConfig("cronical", "", event.currentTarget));
document.querySelector("#open-cron-log").addEventListener("click", () => openTerminal({ kind: "cron-log" }));
document.querySelector("#run-cron-test").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  showBanner("");
  try {
    const data = await api("POST", "/api/cronical/test");
    showBanner(data.warning || "Test wrote to the job log.", "info");
  } catch (err) {
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
});
document.querySelector("#save-ngrok-token").addEventListener("click", async () => {
  const input = document.querySelector("#ngrok-token");
  showBanner("");
  try {
    const data = await api("PUT", "/api/ngrok/token", { token: input.value });
    input.value = "";
    await refresh();
    showBanner(data.warning || "Authtoken saved.", "info");
  } catch (err) {
    showBanner(err.message, "error");
  }
});
document.querySelector("#open-heidisql").addEventListener("click", (event) => openDatabaseTool(event.currentTarget, "heidisql"));
document.querySelector("#open-sqlyog").addEventListener("click", (event) => openDatabaseTool(event.currentTarget, "sqlyog"));

document.querySelector("#postgres-enabled").addEventListener("change", (event) => {
  putSettings({ postgresEnabled: event.target.checked }, event.target.checked ? "PostgreSQL enabled." : "PostgreSQL disabled.");
});
document.querySelector("#mongodb-enabled").addEventListener("change", (event) => {
  putSettings({ mongodbEnabled: event.target.checked }, event.target.checked ? "MongoDB enabled." : "MongoDB disabled.");
});
document.querySelector("#save-postgres").addEventListener("click", () => {
  putSettings({ tools: { postgres: postgresSelect.value } }, `PostgreSQL ${postgresSelect.value} is in use.`);
});
document.querySelector("#save-mongodb").addEventListener("click", () => {
  putSettings({ tools: { mongodb: mongodbSelect.value } }, `MongoDB ${mongodbSelect.value} is in use.`);
});
document.querySelector("#save-postgres-port").addEventListener("click", () => {
  putSettings({ postgresPort: Number(document.querySelector("#postgres-port").value) }, "PostgreSQL port saved.");
});
document.querySelector("#save-mongodb-port").addEventListener("click", () => {
  putSettings({ mongodbPort: Number(document.querySelector("#mongodb-port").value) }, "MongoDB port saved.");
});
document.querySelector("#postgres-terminal").addEventListener("click", () => openTerminal({ kind: "psql" }));
document.querySelector("#mongodb-terminal").addEventListener("click", () => openTerminal({ kind: "mongosh" }));
document.querySelector("#docker-check").addEventListener("click", checkDocker);

document.querySelector("#vc-runtime-install").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  showBanner("Downloading the Visual C++ runtime from Microsoft. Approve the Windows administrator prompt when it appears…", "info");
  try {
    const result = await api("POST", "/api/vcruntime");
    await refresh();
    showBanner(result.warning || "The Visual C++ runtime is installed.", result.warning ? "error" : "info");
  } catch (err) {
    showBanner(err.message, "error");
  } finally {
    button.disabled = false;
  }
});

document.querySelector("#db-terminal").addEventListener("click", () => openTerminal({ kind: "mysql" }));

for (const button of document.querySelectorAll("[data-copy]")) {
  button.addEventListener("click", () => copyEnv(button));
}

async function copyEnv(button) {
  const node = document.querySelector(button.dataset.copy);
  await copyText(button, node.textContent, node);
}

async function copyText(button, text, fallbackNode) {
  let copied = false;
  try {
    await navigator.clipboard.writeText(text);
    copied = true;
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.left = "-9999px";
    document.body.append(area);
    area.select();
    try {
      copied = document.execCommand("copy");
    } catch {
      copied = false;
    }
    area.remove();
  }
  if (!copied) {
    const range = document.createRange();
    range.selectNodeContents(fallbackNode || button);
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    showBanner("Select the text and copy it.", "error");
    return;
  }
  const previous = button.textContent;
  button.textContent = "Copied";
  setTimeout(() => {
    button.textContent = previous;
  }, 1200);
}

phpTarget.addEventListener("change", fillPhpChoices);
customHost.addEventListener("input", renderPreview);

document.querySelector("#php-cancel").addEventListener("click", () => phpDialog.close());
document.querySelector("#host-cancel").addEventListener("click", () => hostDialog.close());

document.querySelector("#php-apply").addEventListener("click", async () => {
  phpError.textContent = "";
  const choice = selectedPhp();
  try {
    const data = await api("PUT", `/api/sites/${activeAccount}/php`, {
      target: phpTarget.value,
      version: choice === "default" ? "" : choice,
    });
    phpDialog.close();
    await refresh();
    showBanner(data.warning || "", "warning");
  } catch (err) {
    phpError.textContent = err.message;
  }
});

document.querySelector("#php-default").addEventListener("click", async () => {
  phpError.textContent = "";
  try {
    await api("PUT", `/api/sites/${activeAccount}/php`, { target: phpTarget.value, version: "" });
    phpDialog.close();
    await refresh();
  } catch (err) {
    phpError.textContent = err.message;
  }
});

document.querySelector("#php-download").addEventListener("click", async () => {
  const choice = selectedPhp();
  const version = choice === "default" ? state.status.defaultPhp : choice;
  const button = document.querySelector("#php-download");
  await download("php", version, button);
});

document.querySelector("#host-save").addEventListener("click", async () => {
  hostError.textContent = "";
  try {
    const data = await api("PUT", `/api/sites/${activeAccount}/host`, { host: customHost.value.trim() });
    hostDialog.close();
    await refresh();
    showBanner(data.warning || "", "warning");
  } catch (err) {
    hostError.textContent = err.message;
  }
});

document.querySelector("#host-reset").addEventListener("click", async () => {
  hostError.textContent = "";
  try {
    const data = await api("PUT", `/api/sites/${activeAccount}/host`, { host: "" });
    hostDialog.close();
    await refresh();
    showBanner(data.warning || "", "warning");
  } catch (err) {
    hostError.textContent = err.message;
  }
});

showView("dashboard");
refresh().catch((err) => {
  showBanner(err.message, "error");
});
loadRelease().catch(() => {});
setInterval(followStack, 2000);
setInterval(() => {
  loadRelease().catch(() => {});
}, 10 * 60 * 1000);
