// Management view: sections, then the students and skills of the current section.
import { act, api, currentSection, h, sectionGroups, sectionSelect, sectionStudents, store } from './core.js';

// Small icon button.
export function iconButton(label, title, onclick, extraClass = '') {
  return h('button', { type: 'button', class: `icon ${extraClass}`, title, 'aria-label': title, onclick }, label);
}

// One-line form to add an item. The data-key lets the text and the focus
// survive a redraw triggered by another user's change.
export function addForm(key, placeholder, onAdd) {
  const input = h('input', { type: 'text', placeholder, 'data-key': key, maxlength: 200, autocomplete: 'off' });
  return h('form', {
    class: 'add',
    onsubmit: (event) => {
      event.preventDefault();
      const name = input.value.trim();
      if (!name) return;
      input.value = '';
      act(() => onAdd(name));
    },
  }, input, h('button', { type: 'submit' }, 'Ajouter'));
}

function rename(what, path, item) {
  const name = prompt(`Nouveau nom ${what} :`, item.name);
  if (name === null || !name.trim() || name.trim() === item.name) return;
  act(() => api('PUT', `${path}/${item.id}`, { name: name.trim() }));
}

function remove(message, path, item) {
  if (confirm(message)) act(() => api('DELETE', `${path}/${item.id}`));
}

const move = (path, item, direction) => act(() => api('POST', `${path}/${item.id}/move`, { direction }));

function row(label, buttons, extraClass = '') {
  return h('li', { class: extraClass }, h('span', { class: 'name' }, label), h('span', { class: 'actions' }, buttons));
}

function orderButtons(path, item, index, count) {
  return [
    h('button', { type: 'button', class: 'icon', title: 'Monter', disabled: index === 0, onclick: () => move(path, item, 'up') }, '↑'),
    h('button', { type: 'button', class: 'icon', title: 'Descendre', disabled: index === count - 1, onclick: () => move(path, item, 'down') }, '↓'),
  ];
}

export default {
  render(main) {
    const page = h('div', { class: 'page' });
    main.append(page);

    page.append(h('section', { class: 'block' },
      h('h2', {}, 'Sections'),
      h('ul', { class: 'items' }, store.sections.map((s) => row(s.name, [
        iconButton('✎', 'Renommer', () => rename('de la section', '/api/sections', s)),
        iconButton('✕', 'Supprimer', () => remove(
          `Supprimer la section « ${s.name} » ?\nSes élèves, ses compétences et tous les statuts associés seront définitivement supprimés.`,
          '/api/sections', s), 'danger'),
      ]))),
      addForm('add-section', 'Nouvelle section', (name) => api('POST', '/api/sections', { name }))));

    const section = currentSection();
    if (!section) return;

    page.append(h('div', { class: 'toolbar' }, h('span', {}, 'Section à gérer :'), sectionSelect()));

    page.append(h('section', { class: 'block' },
      h('h2', {}, `Élèves — ${section.name}`),
      h('ul', { class: 'items' }, sectionStudents(section.id).map((s) => row(s.name, [
        iconButton('✎', 'Renommer', () => rename('de l’élève', '/api/students', s)),
        iconButton('✕', 'Supprimer', () => remove(
          `Supprimer l’élève « ${s.name} » ?\nTous ses statuts seront définitivement supprimés.`, '/api/students', s), 'danger'),
      ]))),
      addForm('add-student', 'Nouvel élève', (name) => api('POST', '/api/students', { name, section_id: section.id }))));

    const groups = sectionGroups(section.id);
    page.append(h('section', { class: 'block' },
      h('h2', {}, `Compétences — ${section.name}`),
      groups.map(({ group, skills }, groupIndex) => h('ul', { class: 'items' },
        row(group.name, [
          orderButtons('/api/groups', group, groupIndex, groups.length),
          iconButton('✎', 'Renommer', () => rename('du groupe', '/api/groups', group)),
          iconButton('✕', 'Supprimer', () => remove(
            `Supprimer le groupe « ${group.name} » ?\nSes compétences et les statuts associés seront définitivement supprimés.`,
            '/api/groups', group), 'danger'),
        ], 'group'),
        skills.map((skill, skillIndex) => row(skill.name, [
          orderButtons('/api/skills', skill, skillIndex, skills.length),
          iconButton('✎', 'Renommer', () => rename('de la compétence', '/api/skills', skill)),
          iconButton('✕', 'Supprimer', () => remove(
            `Supprimer la compétence « ${skill.name} » ?\nLes statuts associés seront définitivement supprimés.`,
            '/api/skills', skill), 'danger'),
        ])),
        h('li', {}, addForm(`add-skill-${group.id}`, `Nouvelle compétence dans « ${group.name} »`,
          (name) => api('POST', '/api/skills', { name, group_id: group.id }))))),
      addForm('add-group', 'Nouveau groupe de compétences', (name) => api('POST', '/api/groups', { name, section_id: section.id }))));
  },
};
