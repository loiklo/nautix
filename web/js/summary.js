// Summary view: students of the current section ranked by progress.
import { h, progress, sectionSelect, sectionSkillIds, sectionStudents, store } from './core.js';

export default {
  render(main) {
    const skillIds = sectionSkillIds(store.sectionId);
    // sectionStudents is alphabetical and the sort is stable, so ties stay alphabetical.
    const rows = sectionStudents(store.sectionId)
      .map((student) => ({ student, percent: progress(student.id, skillIds) }))
      .sort((a, b) => b.percent - a.percent);

    main.append(h('div', { class: 'page' },
      h('div', { class: 'toolbar' }, sectionSelect()),
      rows.length
        ? h('table', { class: 'list summary' },
          h('thead', {}, h('tr', {}, h('th', {}, 'Élève'), h('th', {}, 'Progression'))),
          h('tbody', {}, rows.map(({ student, percent }) => h('tr', {},
            h('td', { class: 'shrink' }, student.name),
            h('td', {}, h('div', { class: 'bar' },
              h('div', { class: 'fill', style: `width:${percent}%` }),
              h('span', {}, `${percent} %`)))))))
        : h('p', { class: 'hint' }, 'Aucun élève dans cette section.')));
  },
};
