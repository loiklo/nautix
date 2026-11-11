// Main view: skills in rows, students in columns, one status pill per cell.
import {
  STATUS_LABELS, api, h, pendingWrites, progress, refresh, sectionGroups, sectionSelect,
  sectionSkillIds, sectionStudents, statusKey, store,
} from './core.js';

// Skills selected for focus mode: the other rows are blurred and inactive.
const focused = new Set();

// Status writes are sent one at a time so they reach the server in order.
let writeQueue = Promise.resolve();

function paintPill(pill, status) {
  pill.className = `pill s${status}`;
  pill.children[1].textContent = STATUS_LABELS[status];
  pill.children[0].disabled = status === 0;
  pill.children[2].disabled = status === STATUS_LABELS.length - 1;
}

// Redraws one cell and the progress of its student, if they are on screen.
function paintCell(studentId, skillId) {
  const table = document.querySelector('table.grid');
  if (!table) return;
  const cell = table.querySelector(`td[data-cell="${statusKey(studentId, skillId)}"]`);
  if (cell) paintPill(cell.firstChild, store.statuses.get(statusKey(studentId, skillId)) || 0);
  const percent = table.querySelector(`th[data-progress="${studentId}"]`);
  if (percent) percent.textContent = `${progress(studentId, sectionSkillIds(store.sectionId))} %`;
}

function changeStatus(studentId, skillId, delta) {
  const key = statusKey(studentId, skillId);
  const current = store.statuses.get(key) || 0;
  const next = Math.min(STATUS_LABELS.length - 1, Math.max(0, current + delta));
  if (next === current) return;

  // Optimistic update: the cell changes immediately, the save follows.
  store.statuses.set(key, next);
  paintCell(studentId, skillId);
  pendingWrites.set(key, (pendingWrites.get(key) || 0) + 1);
  writeQueue = writeQueue
    .then(() => api('PUT', '/api/statuses', { student_id: studentId, skill_id: skillId, status: next }))
    .then(() => release(key), (error) => {
      release(key);
      alert(`Modification non enregistrée : ${error.message}`);
      return refresh().catch(() => {});
    });
}

function release(key) {
  const count = pendingWrites.get(key) - 1;
  if (count > 0) pendingWrites.set(key, count);
  else pendingWrites.delete(key);
}

function applyFocus(table) {
  table.classList.toggle('has-focus', focused.size > 0);
  for (const row of table.querySelectorAll('tr[data-skill]')) {
    row.classList.toggle('selected', focused.has(Number(row.dataset.skill)));
  }
}

function onClick(event) {
  const table = event.currentTarget;
  const button = event.target.closest('button[data-delta]');
  if (button) {
    const [studentId, skillId] = button.closest('td').dataset.cell.split(':').map(Number);
    changeStatus(studentId, skillId, Number(button.dataset.delta));
    return;
  }
  const skillCell = event.target.closest('tr[data-skill] > th');
  if (skillCell) {
    const skillId = Number(skillCell.parentNode.dataset.skill);
    if (!focused.delete(skillId)) focused.add(skillId);
    applyFocus(table);
  }
}

function pill(status) {
  const el = h('span', {},
    h('button', { type: 'button', 'data-delta': '-1', 'aria-label': 'Diminuer' }, '−'),
    h('span', { class: 'label' }),
    h('button', { type: 'button', 'data-delta': '1', 'aria-label': 'Augmenter' }, '+'));
  paintPill(el, status);
  return el;
}

export default {
  render(main) {
    const students = sectionStudents(store.sectionId);
    const groups = sectionGroups(store.sectionId);
    const skillIds = groups.flatMap((g) => g.skills.map((k) => k.id));
    for (const id of [...focused]) if (!skillIds.includes(id)) focused.delete(id);

    const table = h('table', { class: 'grid', onclick: onClick },
      h('thead', {},
        h('tr', { class: 'names' },
          h('th', { class: 'corner' }, sectionSelect()),
          students.map((s) => h('th', { title: s.name }, h('div', {}, s.name)))),
        h('tr', { class: 'progress' },
          h('th', { class: 'corner' }, 'Progression'),
          students.map((s) => h('th', { 'data-progress': s.id }, `${progress(s.id, skillIds)} %`)))),
      h('tbody', {}, groups.map(({ group, skills }) => [
        h('tr', { class: 'group' },
          h('th', { colspan: students.length + 1 }, h('div', {}, group.name))),
        skills.map((skill) => h('tr', { 'data-skill': skill.id },
          h('th', {}, h('div', {}, skill.name)),
          students.map((s) => h('td', { 'data-cell': statusKey(s.id, skill.id) },
            pill(store.statuses.get(statusKey(s.id, skill.id)) || 0))))),
      ])));
    applyFocus(table);
    main.append(table);

    if (!store.sections.length) {
      main.append(h('p', { class: 'hint' }, 'Aucune section. Créez-en une dans l’onglet Gestion.'));
    } else if (!skillIds.length || !students.length) {
      main.append(h('p', { class: 'hint' }, 'Ajoutez des élèves et des compétences à cette section dans l’onglet Gestion.'));
    }
  },

  // A status changed elsewhere: update the cell in place.
  onStatus(message) {
    paintCell(message.student_id, message.skill_id);
  },
};
