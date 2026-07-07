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
    if (d.version) {
      const vs = "v" + String(d.version).replace(/^v/, "");
      $("#ver").textContent = vs;
      const hv = $("#hver");
      hv.textContent = vs;
      if (d.outdated) {
        hv.classList.add("offc");
        hv.title = "доступна " + d.latest;
      }
    }
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

const BASE_VIEWS = [
  ["status", "Статус"],
  ["hits", "Срабатывания"],
  ["flows", "Соединения"],
  ["modules", "Функции"],
  ["sources", "Списки"],
  ["indicators", "Блокировки"],
];
const KINDS = ["все", "ip", "cidr", "domain", "url", "mac"];
let current = "status";
let indKind = "все";
let indQuery = "";
let groupsOn = false;

function views() {
  const v = BASE_VIEWS.slice();
  if (groupsOn) v.push(["groups", "Группы"]);
  return v;
}

function setGroupsOn(mods) {
  const on = !!(mods || []).find((m) => m.name === "grouppolicy" && m.enabled);
  if (on === groupsOn) return;
  groupsOn = on;
  if (!on && current === "groups") current = "status";
  buildTabs();
}

async function refreshGroupsTab() {
  try {
    setGroupsOn(await api("module.ls"));
  } catch (e) {}
}

function buildTabs() {
  const nav = $("#tabs");
  nav.textContent = "";
  for (const [id, label] of views()) {
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
    else if (current === "groups") await viewGroups();
  } catch (e) {
    if (e.message !== "нужен вход") msg(e.message, "bad");
  }
}

function card(k, val, cls) {
  return h("div", { class: "card" },
    h("div", { class: "k", text: k }),
    h("div", { class: "v " + (cls || ""), text: String(val), title: String(val) }),
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

function indActions(value, state, canClear = true) {
  const blockB = h("button", { class: "sm danger", text: "block", onclick: () => run(() => api("block.add", { value })) });
  const allowB = h("button", { class: "sm", text: "allow", onclick: () => run(() => api("allow.add", { value })) });
  const clearB = h("button", { class: "sm mut", text: "снять", onclick: () => run(() => api("block.rm", { value })) });
  const el = h("div", { class: "actions" });
  if (state === "block") el.append(allowB);
  else if (state === "allow") el.append(blockB);
  else el.append(blockB, allowB);
  if (state && canClear) el.append(clearB);
  return el;
}

function noteCell(i) {
  const td = h("td", { class: "note" });
  const show = () => {
    td.textContent = "";
    td.append(
      h("span", { text: i.note || "" }),
      h("button", {
        class: "sm ghost pen", text: "✎", title: "изменить причину",
        onclick: () => {
          const inp = h("input", { value: i.note || "" });
          inp.addEventListener("keydown", (e) => {
            if (e.key === "Enter") {
              i.note = inp.value.trim();
              run(() => api("ind.note", { id: String(i.id), note: i.note }));
            } else if (e.key === "Escape") show();
          });
          td.textContent = "";
          td.append(inp);
          inp.focus();
        },
      }),
    );
  };
  show();
  return td;
}

function tsFmt(sec) {
  if (!sec) return "";
  const d = new Date(sec * 1000);
  const p = (n) => String(n).padStart(2, "0");
  return p(d.getDate()) + "." + p(d.getMonth() + 1) + " " + p(d.getHours()) + ":" + p(d.getMinutes()) + ":" + p(d.getSeconds());
}

async function viewStatus() {
  const v = $("#view");
  const [d, hits, srcs, hosts, ver] = await Promise.all([
    api("status"),
    api("hits", { limit: "500" }).catch(() => []),
    api("source.ls").catch(() => []),
    api("hosts").catch(() => []),
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
  const srcList = srcs || [];
  const srcOn = srcList.filter((s) => s.enabled).length;
  const machines = new Set((hosts || []).map((x) => x.hostname)).size;

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
  cards.append(card("машин в сети", machines));
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

  v.append(h("h3", { class: "subh", text: "функции" }));
  v.append(tableEl(["", "функция", "состояние"], mods.map((m) =>
    h("tr", null,
      h("td", null, runState(m)),
      h("td", { text: m.title || m.name }),
      h("td", { class: m.health && m.health.ok === false ? "offc" : "dim", text: m.health ? m.health.detail || "" : "" }),
    ))));

  const groups = new Map();
  for (const x of hitList) {
    const key = (x.layer || "") + "|" + (x.indicator || "");
    let g = groups.get(key);
    if (!g) {
      g = { layer: x.layer || "", indicator: x.indicator || "", first: x.ts, last: x.ts, count: 0 };
      groups.set(key, g);
    }
    g.count++;
    if (x.ts < g.first) g.first = x.ts;
    if (x.ts > g.last) g.last = x.ts;
  }
  const top = [...groups.values()].sort((a, b) => b.last - a.last).slice(0, 5);
  if (top.length) {
    v.append(h("h3", { class: "subh", text: "последние срабатывания" }));
    v.append(tableEl(["диапазон", "слой", "индикатор", "раз"], top.map((g) =>
      h("tr", null,
        h("td", { text: rangeFmt(g.first, g.last) }),
        h("td", null, h("span", { class: "tag", text: g.layer })),
        h("td", { text: g.indicator }),
        h("td", { text: "×" + g.count }),
      ))));
  }

  if (srcList.length) {
    v.append(h("h3", { class: "subh", text: "источники" }));
    v.append(tableEl(["имя", "вкл", "синк"], srcList.map((s) =>
      h("tr", null,
        h("td", { text: s.name }),
        h("td", null, onoff(s.enabled)),
        h("td", { class: syncCls(s), text: syncFmt(s) }),
      ))));
  }

  const ikeys = Object.keys(inds).sort();
  if (ikeys.length) {
    v.append(h("h3", { class: "subh", text: "индикаторы по видам" }));
    v.append(tableEl(["вид", "кол-во"], ikeys.map((k) =>
      h("tr", null, h("td", { text: k || "-" }), h("td", { text: String(inds[k]) })))));
  }
}

function syncFmt(s) {
  if (!s.last_sync) return "ещё не синкался";
  return (s.last_status || "?") + " · " + (s.last_count || 0) + " · " + tsFmt(s.last_sync);
}

function syncCls(s) {
  if (!s.last_sync || /^ok/.test(s.last_status || "")) return "dim";
  return "offc";
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
      g = { layer: x.layer || "", indicator: x.indicator || "", first: x.ts, last: x.ts, count: 0, ips: new Set(), action: x.action || "" };
      groups.set(key, g);
    }
    g.count++;
    if (x.ts < g.first) g.first = x.ts;
    if (x.ts > g.last) g.last = x.ts;
    if (x.src_ip) g.ips.add(x.src_host ? x.src_host + " (" + x.src_ip + ")" : x.src_ip);
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
      { node: indActions(g.indicator, g.action), sig: g.action },
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
    const wrap = tableEl(["хост", "src mac", "src ip", "вид", "назначение", "proto", "пакетов", "байт", "посл.", "действие"], []);
    wrap.querySelector("tbody").id = "flows-tbody";
    v.append(wrap);
    tbody = wrap.querySelector("tbody");
  }
  const rows = flows.map((f) => ({
    key: (f.src_mac || "") + "|" + (f.src_ip || "") + "|" + (f.dst_ip || "") + "|" + f.port + "|" + (f.proto || ""),
    cls: f.blocked ? "hot" : "",
    cells: [
      f.src_host || "",
      macCell(f),
      f.src_ip || "",
      { tag: f.kind || "" },
      f.dst || "",
      (f.proto || "") + "/" + f.port,
      String(f.packets),
      human(f.bytes),
      tsFmt(f.last),
      { node: indActions(f.dst, f.src_blocked ? "" : f.verdict), sig: f.src_blocked ? "" : f.verdict || "" },
    ],
  }));
  syncRows(tbody, rows);
}

function macCell(f) {
  if (!f.src_mac) return "";
  const btn = f.src_blocked
    ? h("button", { class: "sm", text: "разбл", title: "разблокировать клиента", onclick: () => run(() => api("allow.add", { value: f.src_mac })) })
    : h("button", { class: "sm mut", text: "блок", title: "заблокировать клиента по mac", onclick: () => run(() => api("block.add", { value: f.src_mac })) });
  return {
    node: h("span", { class: "macw" }, f.src_mac, btn),
    sig: f.src_mac + "|" + (f.src_blocked ? 1 : 0),
  };
}

async function viewModules() {
  const mods = (await api("module.ls")) || [];
  setGroupsOn(mods);
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

  const fileI = h("input", { type: "file" });
  const nameU = h("input", { placeholder: "имя (или из файла)" });
  const adaptU = h("select", null, ...["csv", "text", "hosts"].map((a) => h("option", { value: a, text: a })));
  const mapU = h("input", { placeholder: "map csv: indicator:0,type:1" });
  v.append(h("div", { class: "bar" }, fileI, nameU, adaptU, mapU,
    h("button", {
      text: "загрузить файл",
      onclick: () => run(async () => {
        if (!fileI.files.length) throw new Error("выбери файл");
        const fd = new FormData();
        fd.append("file", fileI.files[0]);
        fd.append("name", nameU.value.trim());
        fd.append("adapter", adaptU.value);
        fd.append("map", mapU.value.trim());
        const r = await fetch("/api/upload", { method: "POST", body: fd });
        if (r.status === 401) { showLogin(); throw new Error("нужен вход"); }
        const j = await r.json().catch(() => ({ ok: false, error: "плохой ответ" }));
        if (!j.ok) throw new Error(j.error || "ошибка загрузки");
        fileI.value = ""; nameU.value = ""; mapU.value = "";
        return j.data;
      }),
    }),
  ));

  if (!srcs.length) {
    v.append(h("p", { class: "dim", text: "источников нет" }));
    return;
  }
  const rows = srcs.map((s) => {
    const tr = h("tr", null,
      h("td", { text: s.name }),
      h("td", { text: s.adapter }),
      h("td", null, onoff(s.enabled)),
      h("td", { class: syncCls(s), text: syncFmt(s) }),
      h("td", null, s.uri
        ? h("button", { class: "linkish", text: s.uri, title: "показать содержимое", onclick: () => toggleSource(tr, s) })
        : ""),
      h("td", null,
        h("div", { class: "actions" },
          h("button", {
            class: "sm " + (s.enabled ? "mut" : ""),
            text: s.enabled ? "выключить" : "включить",
            onclick: () => run(() => api(s.enabled ? "source.disable" : "source.enable", { name: s.name })),
          }),
          sourceDelBtn(s),
        ),
      ),
    );
    return tr;
  });
  v.append(tableEl(["имя", "адаптер", "вкл", "синк", "файл / ссылка", ""], rows));
}

function sourceDelBtn(s) {
  const b = h("button", { class: "sm mut", text: "удалить" });
  b.addEventListener("click", () => {
    if (b.dataset.arm) {
      run(() => api("source.rm", { id: String(s.id) }));
      return;
    }
    b.dataset.arm = "1";
    b.textContent = "точно?";
    b.classList.add("danger");
    setTimeout(() => {
      b.dataset.arm = "";
      b.textContent = "удалить";
      b.classList.remove("danger");
    }, 3000);
  });
  return b;
}

async function toggleSource(tr, s) {
  const next = tr.nextElementSibling;
  if (next && next.classList.contains("expand")) {
    next.remove();
    return;
  }
  let d;
  try {
    d = await api("source.indicators", { id: String(s.id) });
  } catch (e) {
    if (e.message !== "нужен вход") msg(e.message, "bad");
    return;
  }
  const items = d.items || [];
  const cols = tr.children.length;
  const inner = items.map((i) =>
    h("tr", null,
      h("td", { text: i.value }),
      h("td", null, h("span", { class: "tag", text: i.kind || "" })),
      h("td", null, actionTag(i.action)),
      noteCell(i),
    ),
  );
  const body = h("td", { colspan: String(cols) },
    h("p", { class: "dim", text: "содержимое «" + s.name + "»: " + (items.length < d.total ? "показано " + items.length + " из " + d.total : "записей " + d.total) }),
    items.length ? tableEl(["значение", "тип", "действие", "причина"], inner) : h("p", { class: "dim", text: "пусто (сделай синхронизацию)" }),
  );
  tr.after(h("tr", { class: "expand" }, body));
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

  const inds = (await api("list", { kind: indKind === "все" ? "all" : indKind })) || [];
  const rows = inds.map((i) => {
    const tr = h("tr", null,
      h("td", { text: i.value }),
      h("td", null, h("span", { class: "tag", text: i.kind || "" })),
      h("td", null, actionTag(i.action)),
      h("td", { class: "dim", text: i.source_id ? "фид" : "вручную" }),
      noteCell(i),
      h("td", null, indActions(i.value, i.action, i.source_id === 0)),
    );
    tr.dataset.val = (i.value || "").toLowerCase();
    return tr;
  });
  const wrap = tableEl(["значение", "тип", "действие", "откуда", "причина", ""], rows);
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

async function viewGroups() {
  const v = $("#view");
  v.textContent = "";

  v.append(h("div", { class: "warnbox" },
    h("b", null, "Опасный экспериментальный функционал. "),
    "Группа — набор машин (по MAC или имени хоста) со своими правилами (ip/cidr/домен/url), ",
    "которые действуют только на участников группы. ",
    "Глобальные правила (вкладка «Правила» и фиды) всегда приоритетнее групповых. ",
    "Одна машина может состоять только в одной группе."));

  let groups, cands;
  try {
    [groups, cands] = await Promise.all([api("group.ls"), api("group.scan").catch(() => [])]);
  } catch (e) {
    if (e.message === "нужен вход") return;
    v.append(h("p", { class: "dim", text: e.message }));
    return;
  }
  groups = groups || [];
  cands = cands || [];

  const dl = h("datalist", { id: "grp-cands" });
  const seen = new Set();
  for (const c of cands) {
    for (const val of [c.key, c.hostname]) {
      if (val && !seen.has(val)) { seen.add(val); dl.append(h("option", { value: val })); }
    }
  }
  v.append(dl);

  const nameI = h("input", { placeholder: "имя группы" });
  const noteI = h("input", { placeholder: "заметка (необязательно)" });
  v.append(h("div", { class: "bar" }, nameI, noteI,
    h("button", {
      text: "создать группу",
      onclick: () => run(async () => {
        const r = await api("group.add", { name: nameI.value.trim(), note: noteI.value.trim() });
        nameI.value = ""; noteI.value = "";
        return r;
      }),
    })));

  if (!groups.length) v.append(h("p", { class: "dim", text: "групп нет" }));
  for (const g of groups) v.append(groupCard(g));

  if (cands.length) {
    v.append(h("h3", { class: "subh", text: "машины в сети" }));
    v.append(tableEl(["имя", "вид", "адрес", "группа"], cands.map((c) =>
      h("tr", null,
        h("td", { text: c.hostname || "" }),
        h("td", null, h("span", { class: "tag", text: c.kind || "" })),
        h("td", { text: c.key || "" }),
        c.group ? h("td", { text: c.group }) : h("td", { class: "dim", text: "—" }),
      ))));
  }
}

function groupCard(g) {
  const box = h("div", { class: "grp" });
  box.append(h("div", { class: "grp-head" },
    h("b", { text: g.name }),
    g.enabled ? h("span", { class: "on", text: "включена" }) : h("span", { class: "offc", text: "выключена" }),
    g.note ? h("span", { class: "dim", text: "· " + g.note }) : null,
    h("span", { class: "spacer" }),
    h("button", {
      class: "sm " + (g.enabled ? "mut" : ""), text: g.enabled ? "выключить" : "включить",
      onclick: () => run(() => api(g.enabled ? "group.disable" : "group.enable", { ref: String(g.id) })),
    }),
    groupDelBtn(g),
  ));

  const body = h("div", { class: "grp-body" });
  const members = g.members || [];
  if (members.length) {
    body.append(tableEl(["участник", "вид", "имя", "mac", "готов", ""], members.map((m) =>
      h("tr", null,
        h("td", { text: m.value }),
        h("td", null, h("span", { class: "tag", text: m.kind })),
        h("td", { text: m.hostname || "" }),
        h("td", { text: (m.macs || []).join(", ") }),
        h("td", null, m.resolved
          ? h("span", { class: "on", text: "да" })
          : h("span", { class: "offc", text: "ждёт" })),
        h("td", null, h("button", {
          class: "sm mut", text: "убрать",
          onclick: () => run(() => api("group.member.rm", { ref: String(g.id), value: m.value })),
        })),
      ))));
  } else {
    body.append(h("p", { class: "dim", text: "нет участников" }));
  }

  const memI = h("input", { placeholder: "MAC или имя хоста", list: "grp-cands" });
  const add = () => run(async () => {
    const r = await api("group.member.add", { ref: String(g.id), value: memI.value.trim() });
    memI.value = "";
    return r;
  });
  memI.addEventListener("keydown", (e) => { if (e.key === "Enter") add(); });
  body.append(h("div", { class: "bar" }, memI,
    h("button", { class: "sm", text: "добавить участника", onclick: add })));

  const rules = g.rules || [];
  body.append(h("h4", { class: "subh", text: "правила группы" }));
  if (rules.length) {
    body.append(tableEl(["значение", "тип", "действие", "причина", ""], rules.map((r) =>
      h("tr", null,
        h("td", { text: r.value }),
        h("td", null, h("span", { class: "tag", text: r.kind || "" })),
        h("td", null, actionTag(r.action)),
        h("td", { class: "dim", text: r.note || "" }),
        h("td", null, h("button", {
          class: "sm mut", text: "убрать",
          onclick: () => run(() => api("group.rule.rm", { ref: String(g.id), value: r.value })),
        })),
      ))));
  } else {
    body.append(h("p", { class: "dim", text: "нет правил — группа пока ничего не блокирует" }));
  }

  const rval = h("input", { placeholder: "ip/cidr/домен/url" });
  const rnote = h("input", { placeholder: "причина (необязательно)" });
  const addRule = (action) => run(async () => {
    const r = await api("group.rule.add", { ref: String(g.id), value: rval.value.trim(), action, note: rnote.value.trim() });
    rval.value = ""; rnote.value = "";
    return r;
  });
  rval.addEventListener("keydown", (e) => { if (e.key === "Enter") addRule("block"); });
  body.append(h("div", { class: "bar" }, rval, rnote,
    h("button", { class: "sm danger", text: "block", onclick: () => addRule("block") }),
    h("button", { class: "sm", text: "allow", onclick: () => addRule("allow") })));

  box.append(body);
  return box;
}

function groupDelBtn(g) {
  const b = h("button", { class: "sm mut", text: "удалить" });
  b.addEventListener("click", () => {
    if (b.dataset.arm) { run(() => api("group.rm", { ref: String(g.id) })); return; }
    b.dataset.arm = "1";
    b.textContent = "точно?";
    b.classList.add("danger");
    setTimeout(() => {
      b.dataset.arm = "";
      b.textContent = "удалить";
      b.classList.remove("danger");
    }, 3000);
  });
  return b;
}

const LIVE_VIEWS = new Set(["status", "hits", "flows"]);

let autoTimer;
function startAuto() {
  stopAuto();
  const sec = parseInt($("#auto").value, 10) || 0;
  if (sec <= 0) return;
  autoTimer = setInterval(() => {
    if (document.hidden || !LIVE_VIEWS.has(current)) return;
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
  if (c && c.node) {
    const td = h("td", null, c.node);
    if (c.sig !== undefined) td.dataset.sig = String(c.sig);
    return td;
  }
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
    } else if (c && c.node && c.sig !== undefined && td.dataset.sig !== String(c.sig)) {
      td.textContent = "";
      td.append(c.node);
      td.dataset.sig = String(c.sig);
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
  refreshGroupsTab();
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
