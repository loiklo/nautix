// Journal view: the 100 most recent status changes.
import { api, formatDate, h } from './core.js';

let entries = [];

export default {
  async load() {
    entries = await api('GET', '/api/journal');
  },

  render(main) {
    main.append(h('div', { class: 'page' },
      entries.length
        ? h('table', { class: 'list' },
          h('thead', {}, h('tr', {}, h('th', {}, 'Date'), h('th', {}, 'Moniteur'), h('th', {}, 'Action'))),
          h('tbody', {}, entries.map((e) => h('tr', {},
            h('td', { class: 'shrink' }, formatDate(e.at)),
            h('td', { class: 'shrink' }, e.instructor),
            h('td', {}, e.action)))))
        : h('p', { class: 'hint' }, 'Le journal est vide.')));
  },
};
