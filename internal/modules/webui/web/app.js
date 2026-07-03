const $ = (s, r = document) => r.querySelector(s);

function h(tag, attrs, ...kids) {
  const e = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === null || v === undefined) continue;
      if (k === "class") e.className = v;
      else if (k === "text") e.textContent = v;
      else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
      else e.setAttribute(k, v);
    }
  }
  for (const kid of kids) {
    if (kid === null || kid === undefined) continue;
    e.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
  }
  return e;
}

async function api(cmd, args) {
  const r = await fetch("/api/cmd/" + cmd, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(args || {}),
  });
  if (r.status === 401) {
    showLogin();
    throw new Error("нужен вход");
  }
  const j = await r.json().catch(() => ({ ok: false, error: "плохой ответ" }));
  if (!j.ok) throw new Error(j.error || "ошибка");
  return j.data;
}

let msgTimer;
function msg(text, kind) {
  const m = $("#msg");
  m.textContent = text;
  m.className = "msg" + (kind ? " " + kind : "");
  clearTimeout(msgTimer);
  msgTimer = setTimeout(() => m.classList.add("hidden"), 4000);
}

async function run(fn) {
  try {
    const r = await fn();
    msg(typeof r === "string" ? r : "готово", "good");
    await renderView();
  } catch (e) {
    if (e.message !== "нужен вход") msg(e.message, "bad");
  }
}

function showLogin() {
  stopAuto();
  $("#app").classList.add("hidden");
  $("#login").classList.remove("hidden");
  $("#token").focus();
}
function showApp() {
  $("#login").classList.add("hidden");
  $("#app").classList.remove("hidden");
}

async function checkAuth() {
  const r = await fetch("/api/me");
  return r.status === 200;
}

async function loadVersion() {
  try {
    const r = await fetch("/api/version");
    const j = await r.json();
    if (!j.ok) return;
    const d = j.data;
    if (d.version) $("#ver").textContent = "v" + String(d.version).replace(/^v/, "");
    const u = $("#update");
    u.textContent = "";
    if (d.outdated) {
      u.append(document.createTextNode("доступна новая версия " + d.latest + ": "));
      u.append(h("a", { href: d.url, target: "_blank", rel: "noreferrer noopener", text: "релиз" }));
      u.classList.remove("hidden");
    } else {
      u.classList.add("hidden");
    }
  } catch (e) {
  }
}

$("#login-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  $("#login-err").textContent = "";
  const token = $("#token").value.trim();
  try {
    const r = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
    const j = await r.json();
    if (!j.ok) throw new Error(j.error || "ошибка входа");
    $("#token").value = "";
    showApp();
    start();
  } catch (err) {
    $("#login-err").textContent = err.message;
  }
});

$("#logout").addEventListener("click", async () => {
  await fetch("/api/logout", { method: "POST" });
  showLogin();
});

const VIEWS = [
  ["status", "Статус"],
  ["hits", "Срабатывания"],
  ["flows", "Соединения"],
  ["modules", "Функции"],
  ["sources", "Списки"],
  ["indicators", "Блокировки"],
];
const KINDS = ["ip", "cidr", "domain", "url"];
let current = "status";
let indKind = "ip";
let indQuery = "";

function buildTabs() {
  const nav = $("#tabs");
  nav.textContent = "";
  for (const [id, label] of VIEWS) {
    nav.append(
      h("button", {
        class: id === current ? "active" : "",
        text: label,
        onclick: () => {
          current = id;
          buildTabs();
          renderView();
        },
      }),
    );
  }
}

async function renderView() {
  try {
    if (current === "status") await viewStatus();
    else if (current === "hits") await viewHits();
    else if (current === "flows") await viewFlows();
    else if (current === "modules") await viewModules();
    else if (current === "sources") await viewSources();
    else if (current === "indicators") await viewIndicators();
  } catch (e) {
    if (e.message !== "нужен вход") msg(e.message, "bad");
  }
}

function card(k, val, cls) {
  return h("div", { class: "card" },
    h("div", { class: "k", text: k }),
    h("div", { class: "v " + (cls || ""), text: String(val) }),
  );
}

function tableEl(headers, rows) {
  const t = h("table", null,
    h("thead", null, h("tr", null, ...headers.map((x) => h("th", { text: x })))),
    h("tbody", null, ...rows),
  );
  return h("div", { class: "tw" }, t);
}

function actionTag(a) {
  const cls = { block: "t-block", allow: "t-allow", monitor: "t-monitor" }[a] || "";
  return h("span", { class: "tag " + cls, text: a || "" });
}

function onoff(b) {
  return b ? h("span", { class: "on", text: "вкл" }) : h("span", { class: "offc", text: "выкл" });
}

function runState(m) {
  if (!m.running) return h("span", { class: "dim", text: "нет" });
  if (m.health && m.health.ok === false) return h("span", { class: "offc", text: "сбой" });
  return h("span", { class: "on", text: "да" });
}

function indActions(value) {
  return h("div", { class: "actions" },
    h("button", { class: "sm danger", text: "block", onclick: () => run(() => api("block.add", { value })) }),
    h("button", { class: "sm", text: "allow", onclick: () => run(() => api("allow.add", { value })) }),
    h("button", { class: "sm mut", text: "снять", onclick: () => run(() => api("block.rm", { value })) }),
  );
}

function tsFmt(sec) {
  if (!sec) return "";
  const d = new Date(sec * 1000);
  const p = (n) => String(n).padStart(2, "0");
  return p(d.getDate()) + "." + p(d.getMonth() + 1) + " " + p(d.getHours()) + ":" + p(d.getMinutes()) + ":" + p(d.getSeconds());
}

async function viewStatus() {
  const v = $("#view");
  const [d, hits, srcs, ver] = await Promise.all([
    api("status"),
    api("hits", { limit: "500" }).catch(() => []),
    api("source.ls").catch(() => []),
    fetch("/api/version").then((r) => r.json()).then((j) => j.data).catch(() => null),
  ]);
  v.textContent = "";
  const br = d.bridge || {};
  const mods = d.modules || [];
  const inds = d.indicators || {};
  let total = 0;
  for (const k in inds) total += inds[k];
  const brText = !br.running ? "выключен" : br.detail || (br.up ? "поднят" : "опущен");
  const brCls = !br.running ? "dim" : br.up ? "on" : "offc";

  const hitList = hits || [];
  const layers = {};
  for (const x of hitList) layers[x.layer || "?"] = (layers[x.layer || "?"] || 0) + 1;
  const srcList = srcs || [];
  const srcOn = srcList.filter((s) => s.enabled).length;

  const ipb = mods.find((m) => m.name === "ipblock");
  const drops = ipb && ipb.health && ipb.health.metrics ? ipb.health.metrics["дропов"] : undefined;

  const cards = h("div", { class: "cards" },
    card("мост", brText, brCls),
    card("функции", mods.filter((m) => m.running).length + " / " + mods.length),
    card("индикаторы", total),
    card("срабатываний", hitList.length),
  );
  if (drops !== undefined) cards.append(card("IP-дропов", drops));
  cards.append(card("источников", srcOn + " / " + srcList.length));
  if (ver && ver.version) {
    cards.append(card("версия", "v" + String(ver.version).replace(/^v/, ""), ver.outdated ? "offc" : "dim"));
  }
  v.append(cards);

  const web = mods.find((m) => m.name === "webui");
  const wm = web && web.health ? web.health.metrics : null;
  if (wm) {
    v.append(h("p", { class: "dim" },
      "веб: " + (wm.url || "") + " · tls " + (wm.tls ? "вкл" : "выкл") +
      " · сессий " + (wm.sessions ?? 0) + " · токенов " + (wm.tokens ?? 0)));
  }

  const lkeys = Object.keys(layers).sort();
  if (lkeys.length) {
    v.append(h("h3", { class: "subh", text: "срабатывания по слоям" }));
    v.append(tableEl(["слой", "кол-во"], lkeys.map((k) =>
      h("tr", null, h("td", { text: k }), h("td", { text: String(layers[k]) })))));
  }

  const ikeys = Object.keys(inds).sort();
  if (ikeys.length) {
    v.append(h("h3", { class: "subh", text: "индикаторы по видам" }));
    v.append(tableEl(["вид", "кол-во"], ikeys.map((k) =>
      h("tr", null, h("td", { text: k || "-" }), h("td", { text: String(inds[k]) })))));
  }
}

function rangeFmt(first, last) {
  if (!first || first === last) return tsFmt(last);
  const a = tsFmt(first), b = tsFmt(last);
  if (a.slice(0, 6) === b.slice(0, 6)) return a + "–" + b.slice(6);
  return a + " – " + b;
}

function ipsShort(set) {
  const arr = [...set].sort();
  if (arr.length <= 6) return arr.join(", ");
  return arr.slice(0, 6).join(", ") + " +" + (arr.length - 6);
}

async function viewHits() {
  const hits = (await api("hits", { limit: "500" })) || [];
  const layers = {};
  const groups = new Map();
  for (const x of hits) {
    layers[x.layer || "?"] = (layers[x.layer || "?"] || 0) + 1;
    const key = (x.layer || "") + "|" + (x.indicator || "");
    let g = groups.get(key);
    if (!g) {
      g = { layer: x.layer || "", indicator: x.indicator || "", first: x.ts, last: x.ts, count: 0, ips: new Set() };
      groups.set(key, g);
    }
    g.count++;
    if (x.ts < g.first) g.first = x.ts;
    if (x.ts > g.last) g.last = x.ts;
    if (x.src_ip) g.ips.add(x.src_ip);
  }
  const arr = [...groups.values()].sort((a, b) => b.last - a.last);
  const summary = "событий " + hits.length + " · сайтов " + arr.length +
    (arr.length ? " · " + Object.keys(layers).sort().map((k) => k + " " + layers[k]).join(" · ") : "");

  let sum = document.getElementById("hits-sum");
  let tbody = document.getElementById("hits-tbody");
  if (!sum || !tbody) {
    const v = $("#view");
    v.textContent = "";
    sum = h("p", { id: "hits-sum", class: "dim" });
    v.append(sum);
    const wrap = tableEl(["диапазон", "слой", "индикатор", "раз", "источники", "действие"], []);
    wrap.querySelector("tbody").id = "hits-tbody";
    v.append(wrap);
    tbody = wrap.querySelector("tbody");
  }
  sum.textContent = summary;
  const rows = arr.map((g) => ({
    key: g.layer + "|" + g.indicator,
    cells: [
      rangeFmt(g.first, g.last),
      { tag: g.layer },
      g.indicator,
      "×" + g.count,
      ipsShort(g.ips),
      { node: indActions(g.indicator) },
    ],
  }));
  syncRows(tbody, rows);
}

function human(n) {
  n = Number(n) || 0;
  if (n >= 1073741824) return (n / 1073741824).toFixed(1) + "G";
  if (n >= 1048576) return (n / 1048576).toFixed(1) + "M";
  if (n >= 1024) return (n / 1024).toFixed(1) + "K";
  return n + "B";
}

async function viewFlows() {
  let flows;
  try {
    flows = await api("analyzer.flows", { limit: "300" });
  } catch (e) {
    if (e.message === "нужен вход") return;
    const v = $("#view");
    v.textContent = "";
    v.append(h("p", { class: "dim", text: "анализатор выключен, включи модуль analyzer на вкладке «Функции»" }));
    return;
  }
  flows = flows || [];
  let tbody = document.getElementById("flows-tbody");
  if (!tbody) {
    const v = $("#view");
    v.textContent = "";
    const wrap = tableEl(["src mac", "src ip", "вид", "назначение", "proto", "пакетов", "байт", "посл.", "действие"], []);
    wrap.querySelector("tbody").id = "flows-tbody";
    v.append(wrap);
    tbody = wrap.querySelector("tbody");
  }
  const rows = flows.map((f) => ({
    key: (f.src_mac || "") + "|" + (f.src_ip || "") + "|" + (f.dst_ip || "") + "|" + f.port + "|" + (f.proto || ""),
    cls: f.blocked ? "hot" : "",
    cells: [
      f.src_mac || "",
      f.src_ip || "",
      { tag: f.kind || "" },
      f.dst || "",
      (f.proto || "") + "/" + f.port,
      String(f.packets),
      human(f.bytes),
      tsFmt(f.last),
      { node: indActions(f.dst) },
    ],
  }));
  syncRows(tbody, rows);
}

async function viewModules() {
  const mods = (await api("module.ls")) || [];
  const v = $("#view");
  v.textContent = "";
  const rows = mods.map((m) =>
    h("tr", null,
      h("td", { text: m.name }),
      h("td", { text: m.title || "" }),
      h("td", null, onoff(m.enabled)),
      h("td", null, runState(m)),
      h("td", { class: m.health && m.health.ok === false ? "offc" : "dim", text: m.health ? m.health.detail || "" : "" }),
      h("td", null,
        h("button", {
          class: "sm " + (m.enabled ? "mut" : ""),
          text: m.enabled ? "выключить" : "включить",
          onclick: () => run(() => api(m.enabled ? "module.disable" : "module.enable", { name: m.name })),
        }),
      ),
    ),
  );
  v.append(tableEl(["имя", "функция", "вкл", "работает", "здоровье", ""], rows));
}

async function viewSources() {
  const srcs = (await api("source.ls")) || [];
  const v = $("#view");
  v.textContent = "";
  v.append(h("div", { class: "bar" },
    h("button", { text: "синхронизировать", onclick: () => run(() => api("source.sync")) }),
    h("button", { class: "ghost", text: "применить", onclick: () => run(() => api("apply")) }),
  ));
  const nameI = h("input", { placeholder: "имя" });
  const adaptI = h("select", null, ...["csv", "text", "hosts"].map((a) => h("option", { value: a, text: a })));
  const uriI = h("input", { placeholder: "uri (файл или ссылка)" });
  const mapI = h("input", { placeholder: "map csv: indicator:0,type:1" });
  v.append(h("div", { class: "bar" }, nameI, adaptI, uriI, mapI,
    h("button", {
      text: "добавить источник",
      onclick: () => run(async () => {
        const r = await api("source.add", { name: nameI.value.trim(), adapter: adaptI.value, uri: uriI.value.trim(), map: mapI.value.trim() });
        nameI.value = ""; uriI.value = ""; mapI.value = "";
        return r;
      }),
    }),
  ));
  if (!srcs.length) {
    v.append(h("p", { class: "dim", text: "источников нет" }));
    return;
  }
  const rows = srcs.map((s) =>
    h("tr", null,
      h("td", { text: s.name }),
      h("td", { text: s.adapter }),
      h("td", null, onoff(s.enabled)),
      h("td", { text: s.uri || "" }),
      h("td", null,
        h("button", {
          class: "sm " + (s.enabled ? "mut" : ""),
          text: s.enabled ? "выключить" : "включить",
          onclick: () => run(() => api(s.enabled ? "source.disable" : "source.enable", { name: s.name })),
        }),
      ),
    ),
  );
  v.append(tableEl(["имя", "адаптер", "вкл", "источник", ""], rows));
}

async function viewIndicators() {
  const v = $("#view");
  v.textContent = "";
  const sel = h("select", null, ...KINDS.map((k) => h("option", { value: k, text: k })));
  sel.value = indKind;
  sel.addEventListener("change", () => { indKind = sel.value; renderView(); });
  const q = h("input", { placeholder: "поиск", value: indQuery });
  q.addEventListener("input", () => { indQuery = q.value; filterRows(); });
  v.append(h("div", { class: "bar" }, h("span", { class: "dim", text: "вид:" }), sel, q));

  const mval = h("input", { placeholder: "значение" });
  const mnote = h("input", { placeholder: "причина (необязательно)" });
  v.append(h("div", { class: "bar" }, mval, mnote,
    h("button", {
      class: "danger", text: "block",
      onclick: () => run(async () => {
        const r = await api("block.add", { value: mval.value.trim(), note: mnote.value.trim() });
        mval.value = ""; mnote.value = "";
        return r;
      }),
    }),
    h("button", {
      text: "allow",
      onclick: () => run(async () => {
        const r = await api("allow.add", { value: mval.value.trim(), note: mnote.value.trim() });
        mval.value = ""; mnote.value = "";
        return r;
      }),
    }),
  ));

  const inds = (await api("list", { kind: indKind })) || [];
  const rows = inds.map((i) => {
    const tr = h("tr", null,
      h("td", { text: i.value }),
      h("td", null, actionTag(i.action)),
      h("td", { text: i.threat || "" }),
      h("td", { text: i.note || "" }),
      h("td", null, indActions(i.value)),
    );
    tr.dataset.val = (i.value || "").toLowerCase();
    return tr;
  });
  const wrap = tableEl(["значение", "действие", "угроза", "причина", ""], rows);
  wrap.querySelector("table").id = "ind-table";
  v.append(wrap);
  filterRows();
}

function filterRows() {
  const t = $("#ind-table");
  if (!t) return;
  const q = indQuery.toLowerCase();
  for (const tr of t.querySelectorAll("tbody tr")) {
    tr.classList.toggle("hidden", q && !(tr.dataset.val || "").includes(q));
  }
}

let autoTimer;
function startAuto() {
  stopAuto();
  const sec = parseInt($("#auto").value, 10) || 0;
  if (sec <= 0) return;
  autoTimer = setInterval(() => {
    if (document.hidden) return;
    const a = document.activeElement;
    if (a && (a.tagName === "INPUT" || a.tagName === "SELECT")) return;
    renderView();
  }, sec * 1000);
}
function stopAuto() {
  if (autoTimer) clearInterval(autoTimer);
  autoTimer = null;
}
$("#auto").addEventListener("change", () => {
  try { localStorage.setItem("chaff_auto", $("#auto").value); } catch (e) {}
  startAuto();
});
$("#refresh").addEventListener("click", () => renderView());

function syncRows(tbody, rows) {
  const existing = new Map();
  for (const tr of [...tbody.children]) existing.set(tr.dataset.k, tr);
  const seen = new Set();
  for (const r of rows) {
    seen.add(r.key);
    const tr = existing.get(r.key);
    if (!tr) {
      const nt = h("tr", { "data-k": r.key, class: r.cls || null });
      for (const c of r.cells) nt.append(cellNode(c));
      tbody.append(nt);
    } else {
      updateCells(tr, r.cells);
      if (tr.className !== (r.cls || "")) tr.className = r.cls || "";
    }
  }
  for (const [k, tr] of existing) if (!seen.has(k)) tr.remove();
}

function cellNode(c) {
  if (typeof c === "string") return h("td", { text: c });
  if (c && c.tag !== undefined) return h("td", null, h("span", { class: "tag", text: c.tag }));
  if (c && c.node) return h("td", null, c.node);
  return h("td", null);
}

function updateCells(tr, cells) {
  const tds = tr.children;
  for (let i = 0; i < cells.length; i++) {
    const c = cells[i], td = tds[i];
    if (!td) continue;
    if (typeof c === "string") {
      if (td.textContent !== c) td.textContent = c;
    } else if (c && c.tag !== undefined) {
      const s = td.firstChild;
      if (s && s.textContent !== c.tag) s.textContent = c.tag;
    }
  }
}

function start() {
  try {
    const v = localStorage.getItem("chaff_auto");
    if (v !== null) $("#auto").value = v;
  } catch (e) {}
  buildTabs();
  renderView();
  startAuto();
}

(async function () {
  loadVersion();
  if (await checkAuth()) {
    showApp();
    start();
  } else {
    showLogin();
  }
})();
