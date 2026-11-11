# Nautix

Suivi des compétences des élèves d'un club de plongée, à l'usage des moniteurs.
Application web installable (PWA) : backend Go, base MariaDB, frontend en JavaScript pur, synchronisation en temps réel par WebSocket.

## Démarrage avec Docker

```sh
cp .env.example .env      # puis choisir DB_PASSWORD
docker compose up -d --build
```

L'application écoute en **HTTP simple** sur le port 8080 (variable `HTTP_PORT`).
Au premier démarrage sur une base vide, un compte administrateur est créé :

- nom : `Alex`
- mot de passe : `changeme` — **à changer immédiatement** dans l'onglet Admin.

## Reverse proxy

Le TLS est assuré par le reverse proxy placé devant l'application. Il doit :

- transmettre les connexions **WebSocket** (chemin `/ws`) ;
- conserver l'en-tête **`Host`** d'origine (le serveur refuse un WebSocket dont l'origine ne correspond pas à `Host`) ;
- idéalement renseigner `X-Forwarded-For` (utilisé pour limiter les tentatives de connexion).

Exemple nginx :

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 1h;
}
```

Caddy (`reverse_proxy 127.0.0.1:8080`) et Traefik gèrent tout cela par défaut.

L'installation en PWA et le service worker exigent que les utilisateurs accèdent au site en HTTPS (ou via `localhost`).

## Configuration

Variables d'environnement du binaire :

| Variable | Défaut | Rôle |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | Adresse d'écoute HTTP |
| `DB_HOST` / `DB_PORT` | `127.0.0.1` / `3306` | Serveur MariaDB |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `nautix` | Accès à la base |
| `COOKIE_SECURE` | `true` | Cookie de session marqué `Secure`. Mettre `false` uniquement pour un test en HTTP hors `localhost` |

Le schéma de la base est créé automatiquement au démarrage.

## Sans Docker

```sh
go build -o nautix .
DB_HOST=127.0.0.1 DB_USER=nautix DB_PASSWORD=... DB_NAME=nautix ./nautix
```

Le binaire contient tout le frontend ; il suffit d'une base MariaDB vide et d'un utilisateur ayant tous les droits dessus.

## Organisation du code

```
main.go            démarrage, configuration, service des fichiers statiques
internal/db        connexion, schéma (schema.sql), compte administrateur initial
internal/auth      connexion, sessions en base, contrôle d'accès
internal/api       API JSON (état, gestion, statuts, journal, administration)
internal/hub       diffusion WebSocket
web/               frontend embarqué dans le binaire (HTML, CSS, modules JS, PWA)
tools/genicons     génération des icônes : go run ./tools/genicons
```

## Règles métier

- Statuts : NA, ECA, A, M. Poids : NA = 0, ECA = 1, A = 2, M = 2. Progression = somme des poids / (2 × nombre de compétences de la section).
- Chaque section possède ses propres élèves, groupes et compétences. Un élève ne change pas de section ; deux élèves peuvent porter le même nom s'ils sont dans des sections différentes.
- Le journal enregistre les changements de statut ; la page affiche les 100 derniers.
- Au plus 5 sessions par moniteur : la plus ancienne est fermée à la connexion suivante.
- Compte `Alex` : lui seul peut changer son mot de passe ; il ne peut être ni renommé, ni supprimé, ni privé de ses droits d'administrateur.
