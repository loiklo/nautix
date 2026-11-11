// Entry point: routing, login, WebSocket synchronisation and refresh triggers.
import { api, h, hooks, pendingWrites, refresh, statusKey, store } from './core.js';
import grid from './grid.js';
import summary from './summary.js';
import journal from './journal.js';
import manage from './manage.js';
import admin from './admin.js';
import pwa, { initPWA } from './pwa.js';

const ROUTES = [
  { hash: '', label: 'Suivi', view: grid },
  { hash: 'synthese', label: 'Synthèse', view: summary },
  { hash: 'journal', label: 'Journal', view: journal },
  { hash: 'gestion', label: 'Gestion', view: manage },
  { hash: 'pwa', label: 'PWA', view: pwa },
  { hash: 'admin', label: 'Admin', view: admin, adminOnly: true },
];

const main = document.getElementById('main');
let loggedIn = false;
let renderedRoute = null;

function currentRoute() {
  const hash = location.hash.replace(/^#\/?/, '');
  const route = ROUTES.find((r) => r.hash === hash) || ROUTES[0];
  return route.adminOnly && !store.me?.is_admin ? ROUTES[0] : route;
}

// ---- rendering ----

function render() {
  if (!loggedIn || !store.me) return;
  const route = currentRoute();

  // Redrawing must not lose what the user is typing, nor the scroll position.
  const sameRoute = route === renderedRoute;
  const kept = new Map();
  let focusedKey = null;
  if (sameRoute) {
    for (const el of main.querySelectorAll('[data-key]')) {
      kept.set(el.dataset.key, el.type === 'checkbox' ? el.checked : el.value);
    }
    focusedKey = document.activeElement?.dataset?.key ?? null;
  }
  const { scrollTop, scrollLeft } = main;

  main.replaceChildren();
  route.view.render(main);
  renderedRoute = route;

  for (const el of main.querySelectorAll('[data-key]')) {
    if (!kept.has(el.dataset.key)) continue;
    if (el.type === 'checkbox') el.checked = kept.get(el.dataset.key);
    else el.value = kept.get(el.dataset.key);
    if (el.dataset.key === focusedKey) el.focus({ preventScroll: true });
  }
  if (sameRoute) main.scrollTo(scrollLeft, scrollTop);

  document.getElementById('tabs').replaceChildren(...ROUTES
    .filter((r) => !r.adminOnly || store.me.is_admin)
    .map((r) => h('a', { href: `#/${r.hash}`, class: r === route ? 'active' : '' }, r.label)));
  document.getElementById('me').textContent = store.me.name;
}

hooks.render = render;
hooks.load = async () => {
  // Before the first state load the admin flag is unknown: skip the view data.
  if (loggedIn && store.me) await currentRoute().view.load?.();
};

// Safe refresh for event handlers: errors (offline...) are ignored here, the
// next trigger will retry. A 401 is handled globally.
const sync = () => {
  if (loggedIn) refresh().catch(() => {});
};

window.addEventListener('hashchange', () => {
  main.scrollTo(0, 0);
  render(); // show the new tab at once, then bring its data up to date
  sync();
});

// ---- login ----

function showLogin() {
  loggedIn = false;
  store.me = null;
  renderedRoute = null;
  closeSocket();
  document.body.classList.add('anon');

  const error = h('p', { class: 'error' });
  const form = h('form', {
    class: 'login',
    onsubmit: async (event) => {
      event.preventDefault();
      error.textContent = '';
      try {
        await api('POST', '/api/login', { name: form.elements.name.value, password: form.elements.password.value });
      } catch (e) {
        error.textContent = e.message;
        return;
      }
      start();
    },
  },
  h('h2', {}, 'Connexion'),
  h('input', { type: 'text', name: 'name', placeholder: 'Nom', required: true, autocomplete: 'username', autocapitalize: 'words' }),
  h('input', { type: 'password', name: 'password', placeholder: 'Mot de passe', required: true, autocomplete: 'current-password' }),
  h('button', { type: 'submit', class: 'primary' }, 'Se connecter'),
  error);
  main.replaceChildren(form);
}

window.addEventListener('nautix:unauthorized', showLogin);

document.getElementById('logout').addEventListener('click', async (event) => {
  event.preventDefault();
  await api('POST', '/api/logout').catch(() => {});
  showLogin();
});

// ---- WebSocket ----

let socket = null;
let retryTimer = null;
let retryDelay = 500;

function setOnline(online) {
  const el = document.getElementById('conn');
  el.textContent = online ? '● synchronisé' : '○ hors ligne';
  el.className = online ? 'online' : 'offline';
}

function closeSocket() {
  clearTimeout(retryTimer);
  retryTimer = null;
  if (socket) {
    socket.onclose = null;
    socket.close();
    socket = null;
  }
  setOnline(false);
}

function connect() {
  if (!loggedIn || socket) return;
  clearTimeout(retryTimer);
  retryTimer = null;

  const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`);
  socket = ws;
  ws.onopen = () => {
    retryDelay = 500;
    setOnline(true);
    sync(); // catch up on anything missed while disconnected
  };
  ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    if (message.type === 'status') onStatus(message);
    else sync();
  };
  ws.onclose = () => {
    if (socket !== ws) return;
    socket = null;
    setOnline(false);
    retryTimer = setTimeout(reconnect, retryDelay);
    retryDelay = Math.min(retryDelay * 2, 10000);
  };
}

// The state reload detects a revoked session (401 shows the login form)
// before the socket is opened again.
async function reconnect() {
  clearTimeout(retryTimer);
  retryTimer = null;
  if (!loggedIn) return;
  await refresh().catch(() => {});
  connect();
}

function onStatus(message) {
  const key = statusKey(message.student_id, message.skill_id);
  if (!pendingWrites.has(key)) {
    if (message.status) store.statuses.set(key, message.status);
    else store.statuses.delete(key);
  }
  const view = currentRoute().view;
  if (view.onStatus) view.onStatus(message);
  else if (view.load) sync();
  else render();
}

// ---- refresh on launch and whenever the app comes back to the foreground ----

function wake() {
  if (!loggedIn || document.visibilityState !== 'visible') return;
  if (socket) sync();
  else reconnect(); // do not wait for the retry delay
}

document.addEventListener('visibilitychange', wake);
window.addEventListener('focus', wake);
window.addEventListener('pageshow', wake);
window.addEventListener('online', wake);

async function start() {
  loggedIn = true;
  try {
    await refresh(); // loads the state; shows the login form on 401
  } catch (error) {
    if (error.status === 401) return;
    main.replaceChildren(h('p', { class: 'hint' }, `${error.message} Nouvelle tentative automatique…`));
  }
  if (!loggedIn) return;
  document.body.classList.remove('anon');
  // The first load ran before the admin flag was known: load the view's own data now.
  if (store.me && currentRoute().view.load) sync();
  connect();
}

initPWA();
start();
