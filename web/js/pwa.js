// PWA support: service worker, install prompt, and the installation page.
import { h, hooks } from './core.js';

const isStandalone = () =>
  window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone === true;

const isIOS = () =>
  /iphone|ipad|ipod/i.test(navigator.userAgent) ||
  (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1); // iPadOS reports itself as a Mac

// Install prompt offered by Chrome, Edge and Android browsers.
let installPrompt = null;

export function initPWA() {
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('sw.js').catch(() => {});
  }
  window.addEventListener('beforeinstallprompt', (event) => {
    event.preventDefault();
    installPrompt = event;
    hooks.render();
  });
  window.addEventListener('appinstalled', () => {
    installPrompt = null;
    hooks.render();
  });
  // Installed app: no zoom, so double taps on the +/- buttons are not hijacked.
  if (isStandalone()) {
    document.documentElement.classList.add('standalone');
    document.getElementById('install-link').hidden = true; // already installed: no install icon
    document.querySelector('meta[name="viewport"]').setAttribute('content',
      'width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no');
  }
}

async function install() {
  if (!installPrompt) return;
  installPrompt.prompt();
  await installPrompt.userChoice;
  installPrompt = null;
  hooks.render();
}

export default {
  render(main) {
    const page = h('div', { class: 'page pwa' }, h('h2', {}, 'Installer Nautix comme application'));
    main.append(page);

    if (isStandalone()) {
      page.append(h('p', {}, '✔ Nautix est installée et fonctionne actuellement en mode application.'));
      return;
    }
    page.append(h('p', {},
      'Une fois installée, Nautix s’ouvre depuis son icône, en plein écran, comme une application classique.'));

    if (installPrompt) {
      page.append(h('p', {}, h('button', { type: 'button', class: 'primary', onclick: install }, 'Installer l’application')));
    } else if (isIOS()) {
      page.append(
        h('p', {}, 'Sur iPhone et iPad, l’installation se fait depuis Safari :'),
        h('ol', {},
          h('li', {}, 'Ouvrez cette page dans ', h('b', {}, 'Safari'), '.'),
          h('li', {}, 'Touchez le bouton ', h('b', {}, 'Partager'), ' (le carré avec une flèche vers le haut).'),
          h('li', {}, 'Choisissez ', h('b', {}, 'Sur l’écran d’accueil'), '.'),
          h('li', {}, 'Validez avec ', h('b', {}, 'Ajouter'), '.')));
    } else {
      page.append(
        h('p', {}, 'Le bouton d’installation n’est pas proposé par ce navigateur pour le moment. Installation manuelle :'),
        h('ul', {},
          h('li', {}, h('b', {}, 'Android (Chrome) : '), 'menu ⋮ puis « Installer l’application » ou « Ajouter à l’écran d’accueil ».'),
          h('li', {}, h('b', {}, 'Ordinateur (Chrome, Edge) : '), 'icône d’installation à droite de la barre d’adresse, ou menu puis « Installer Nautix ».'),
          h('li', {}, h('b', {}, 'Firefox sur Android : '), 'menu ⋮ puis « Ajouter à l’écran d’accueil ».')),
        h('p', { class: 'muted' }, 'L’application est peut-être déjà installée sur cet appareil.'));
    }
  },
};
