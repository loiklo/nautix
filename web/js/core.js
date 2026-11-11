// Shared state and helpers used by every view.

export const STATUS_LABELS = ['NA', 'ECA', 'A', 'M'];
export const STATUS_WEIGHTS = [0, 1, 2, 2];
const MAX_WEIGHT = 2;

// Application state, replaced as a whole by refresh().
export const store = {
  me: null,
  sections: [],  // {id, name}
  groups: [],    // {id, parent_id (section), name}, in display order
  skills: [],    // {id, parent_id (group), name}, in display order
  students: [],  // {id, parent_id (section), name}
  statuses: new Map(), // "studentId:skillId" -> status index (absent = NA)
  sectionId: null,     // section currently displayed
};

// Cells with a status write in progress; incoming echoes are ignored for them.
export const pendingWrites = new Map();

// Set by main.js: how to redraw the current view and load its own data.
export const hooks = { render() {}, load: async () => {} };

export const statusKey = (studentId, skillId) => `${studentId}:${skillId}`;

// ---- DOM ----

// h('div', {class: 'x', onclick: fn}, child, ...) builds a DOM element.
export function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  for (const [name, value] of Object.entries(attrs || {})) {
    if (value == null || value === false) continue;
    if (name.startsWith('on')) el.addEventListener(name.slice(2), value);
    else if (name === 'class') el.className = value;
    else el.setAttribute(name, value === true ? '' : value);
  }
  el.append(...children.flat(Infinity).filter((c) => c != null && c !== false));
  return el;
}

// ---- API ----

export async function api(method, path, body) {
  const options = { method, credentials: 'same-origin' };
  if (body !== undefined) {
    options.headers = { 'Content-Type': 'application/json' };
    options.body = JSON.stringify(body);
  }
  let response;
  try {
    response = await fetch(path, options);
  } catch {
    throw new Error('Serveur injoignable. Vérifiez la connexion.');
  }
  if (!response.ok) {
    const error = new Error((await response.text()).trim() || `Erreur ${response.status}`);
    error.status = response.status;
    if (response.status === 401 && path !== '/api/login') {
      window.dispatchEvent(new Event('nautix:unauthorized'));
    }
    throw error;
  }
  return response.json();
}

// ---- state loading ----

let inFlight = null;
let again = false;

// refresh reloads the state (and the current view's own data) then redraws.
// Calls made while a refresh is running are merged into one extra pass.
export function refresh() {
  if (inFlight) {
    again = true;
    return inFlight;
  }
  inFlight = (async () => {
    try {
      do {
        again = false;
        const [state] = await Promise.all([api('GET', '/api/state'), hooks.load()]);
        applyState(state);
        hooks.render();
      } while (again);
    } finally {
      inFlight = null;
    }
  })();
  return inFlight;
}

function applyState(state) {
  store.me = state.me;
  store.sections = state.sections;
  store.groups = state.groups;
  store.skills = state.skills;
  store.students = state.students;
  const statuses = new Map(state.statuses.map(([student, skill, status]) => [statusKey(student, skill), status]));
  // Keep the local value of cells still being saved: the server copy may predate them.
  for (const key of pendingWrites.keys()) statuses.set(key, store.statuses.get(key) || 0);
  store.statuses = statuses;

  const exists = (id) => store.sections.some((s) => s.id === id);
  if (!exists(store.sectionId)) {
    store.sectionId = exists(state.me.last_section_id) ? state.me.last_section_id : (store.sections[0]?.id ?? null);
  }
}

// act runs a modification, then reloads; errors are shown to the user.
export async function act(fn) {
  try {
    await fn();
  } catch (error) {
    alert(error.message);
    return false;
  }
  refresh().catch(() => {});
  return true;
}

// ---- section helpers ----

export function setSection(id) {
  store.sectionId = id;
  hooks.render();
  api('PUT', '/api/me/section', { section_id: id }).catch(() => {});
}

export function sectionSelect() {
  if (!store.sections.length) return h('span', { class: 'muted' }, 'Aucune section');
  return h('select', { class: 'section-select', 'aria-label': 'Section', onchange: (e) => setSection(Number(e.target.value)) },
    store.sections.map((s) => h('option', { value: s.id, selected: s.id === store.sectionId }, s.name)));
}

export const currentSection = () => store.sections.find((s) => s.id === store.sectionId) || null;

// Students of a section, in alphabetical order.
export function sectionStudents(sectionId) {
  return store.students
    .filter((s) => s.parent_id === sectionId)
    .sort((a, b) => a.name.localeCompare(b.name, 'fr', { sensitivity: 'base' }));
}

// Groups of a section with their skills, in manual order.
export function sectionGroups(sectionId) {
  return store.groups
    .filter((g) => g.parent_id === sectionId)
    .map((group) => ({ group, skills: store.skills.filter((k) => k.parent_id === group.id) }));
}

export function sectionSkillIds(sectionId) {
  return sectionGroups(sectionId).flatMap((g) => g.skills.map((k) => k.id));
}

// Progress of a student over the given skills, as a whole percentage.
export function progress(studentId, skillIds) {
  if (!skillIds.length) return 0;
  let sum = 0;
  for (const skillId of skillIds) {
    sum += STATUS_WEIGHTS[store.statuses.get(statusKey(studentId, skillId)) || 0];
  }
  return Math.round((100 * sum) / (MAX_WEIGHT * skillIds.length));
}

export function formatDate(iso) {
  return new Date(iso).toLocaleString('fr-FR', { dateStyle: 'short', timeStyle: 'medium' });
}
