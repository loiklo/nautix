// Administration view: instructors, open sessions and bulk deletions.
import { act, api, formatDate, h } from './core.js';
import { iconButton } from './manage.js';

let instructors = [];
let sessions = [];

function createInstructor(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const name = form.elements.name.value.trim();
  const password = form.elements.password.value;
  const isAdmin = form.elements.admin.checked;
  if (!name || !password) return;
  act(() => api('POST', '/api/admin/instructors', { name, password, is_admin: isAdmin })).then((done) => {
    if (!done) return;
    for (const key of ['new-instructor-name', 'new-instructor-password']) {
      const input = document.querySelector(`[data-key="${key}"]`);
      if (input) input.value = '';
    }
    const box = document.querySelector('[data-key="new-instructor-admin"]');
    if (box) box.checked = false;
  });
}

function instructorRow(i) {
  // The server decides; the page only hides actions it would refuse anyway.
  const isRoot = i.protected;
  const canSetPassword = i.can_password;
  const update = (body) => act(() => api('PUT', `/api/admin/instructors/${i.id}`, body));
  return h('tr', {},
    h('td', {}, i.name),
    h('td', { class: 'shrink' }, h('label', {},
      h('input', {
        type: 'checkbox', checked: i.is_admin, disabled: isRoot,
        onchange: (e) => update({ is_admin: e.target.checked }),
      }), ' admin')),
    h('td', { class: 'shrink actions' },
      !isRoot && iconButton('✎', 'Renommer', () => {
        const name = prompt('Nouveau nom du moniteur :', i.name);
        if (name !== null && name.trim() && name.trim() !== i.name) update({ name: name.trim() });
      }),
      canSetPassword && iconButton('🔑', 'Changer le mot de passe', () => {
        const password = prompt(`Nouveau mot de passe pour ${i.name} :\n(ses autres sessions seront fermées)`);
        if (password) update({ password });
      }),
      !isRoot && iconButton('✕', 'Supprimer', () => {
        if (confirm(`Supprimer le moniteur « ${i.name} » ?`)) act(() => api('DELETE', `/api/admin/instructors/${i.id}`));
      }, 'danger')));
}

export default {
  async load() {
    [instructors, sessions] = await Promise.all([
      api('GET', '/api/admin/instructors'),
      api('GET', '/api/admin/sessions'),
    ]);
  },

  render(main) {
    main.append(h('div', { class: 'page' },
      h('section', { class: 'block' },
        h('h2', {}, 'Moniteurs'),
        h('table', { class: 'list' }, h('tbody', {}, instructors.map(instructorRow))),
        h('form', { class: 'add', onsubmit: createInstructor },
          h('input', { type: 'text', name: 'name', placeholder: 'Nom', 'data-key': 'new-instructor-name', maxlength: 100, autocomplete: 'off' }),
          h('input', { type: 'text', name: 'password', placeholder: 'Mot de passe', 'data-key': 'new-instructor-password', maxlength: 72, autocomplete: 'off' }),
          h('label', {}, h('input', { type: 'checkbox', name: 'admin', 'data-key': 'new-instructor-admin' }), ' admin'),
          h('button', { type: 'submit' }, 'Ajouter'))),

      h('section', { class: 'block' },
        h('h2', {}, 'Sessions ouvertes'),
        h('table', { class: 'list' },
          h('thead', {}, h('tr', {}, h('th', {}, 'Moniteur'), h('th', {}, 'Dernière utilisation'), h('th', {}))),
          h('tbody', {}, sessions.map((s) => h('tr', {},
            h('td', {}, s.instructor, s.current && h('span', { class: 'muted' }, ' (cette session)')),
            h('td', {}, formatDate(s.last_used_at)),
            h('td', { class: 'shrink actions' }, iconButton('✕', 'Supprimer la session', () => {
              const warning = s.current ? '\nC’est votre session : vous serez déconnecté.' : '';
              if (confirm(`Supprimer cette session de ${s.instructor} ?${warning}`)) {
                act(() => api('DELETE', `/api/admin/sessions/${s.id}`));
              }
            }, 'danger'))))))),

      h('section', { class: 'block' },
        h('h2', {}, 'Remise à zéro'),
        h('div', { class: 'toolbar' },
          h('button', {
            type: 'button', class: 'danger',
            onclick: () => {
              if (confirm('Supprimer TOUS les élèves de TOUTES les sections ?\nLeurs statuts seront définitivement perdus. Les sections et les compétences sont conservées.')) {
                act(() => api('DELETE', '/api/admin/students'));
              }
            },
          }, 'Supprimer tous les élèves'),
          h('button', {
            type: 'button', class: 'danger',
            onclick: () => {
              if (confirm('Effacer toutes les actions du journal ?')) act(() => api('DELETE', '/api/admin/journal'));
            },
          }, 'Effacer le journal')))));
  },
};
