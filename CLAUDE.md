# CLAUDE.md — COF DM

Gestionnaire de monde et de campagnes pour Chroniques Oubliées Fantasy. Application de bureau Windows / Linux (fenêtre native autour d'une interface web embarquée), un fichier SQLite `.cofdm` par univers. Une édition serveur (image Docker) héberge plusieurs univers avec des comptes ; le bureau s'y synchronise. Le projet frère tracker_COF (bestiaire, feuilles, rencontres, combat) y est entièrement fusionné.

- **Tout en français** : code, identifiants, commentaires, commits, docs.
- **Avant de reprendre** : lire `SUIVI.md` (état, consignes du MJ, décisions) et `PROJETS.md` (chantiers et priorités). Pour un gros chantier, poser les questions de fond au MJ avant de coder.

## Stack et commandes

- Go ≥ 1.26.9 (`go.mod`), **CGO obligatoire** (`mattn/go-sqlite3`).
- Tag **`sqlite_fts5` toujours** (sans lui, `internal/store/fts5_requis.go` fait échouer la compilation). Tag `nogui` : sans fenêtre (tests, CI, headless). Tag `serveur` : édition serveur.
- Fenêtre : `webview/webview_go` (WebView2 / GTK 3 + WebKitGTK 4.1).
- Frontend : HTML / CSS / JS **sans framework, sans dépendance, sans build**, embarqué par `//go:embed frontend`. Jamais de npm ni de bundler.
- `outils/` : Python 3, bibliothèque standard seulement (import d'un vault Obsidian).

```bash
gofmt -l .                                                      # doit ne rien afficher (CI)
go vet -tags "sqlite_fts5 nogui" ./...
go test -tags "sqlite_fts5 nogui" ./...
go run -tags "sqlite_fts5 nogui" . -data ./donnees              # sans fenêtre, interface dans le navigateur
go build -tags sqlite_fts5 -o cof-dm .                          # bureau
go build -tags "sqlite_fts5 nogui serveur" -o cof-dm-serveur .  # édition serveur (Docker)
```

Options : `-port` (8100), `-data`, `-univers <fichier.cofdm>`, `-headless`, `-reseau`, `-serveur` (édition serveur seulement), `-mcp` (serveur MCP sur stdio).

## Où se trouve quoi

| Chemin | Rôle |
|---|---|
| `internal/modele` | **Seule source du schéma** (types, champs, relations, calendrier, validation), publié par `GET /api/schema`. |
| `internal/store` | Un univers ouvert : fiches, relations, mentions `[[Nom]]`, images, FTS5, journal, export, réparation (`reparer.go`), synchro (`sync.go`), versions. |
| `internal/serveur` | API REST (routes `METHODE /chemin`), accès (`acces.go`), comptes, page des joueurs, combats, dés, aides, multi-univers (`multi.go`), bureau relié (`distant.go`). `univers.go` : `Application`. |
| `internal/comptes`, `internal/reglages` | Comptes du mode serveur (`comptes.json`) ; réglages de l'ordinateur (`reglages.json`). |
| `internal/regles`, `internal/reference`, `internal/fiches` | Fichier de règles importé ; profils, peuples, voies ; validation des monstres et feuilles. |
| `internal/combat`, `internal/des` | Moteur de combat (lit la partie `combat` des règles) ; formules de dés. |
| `internal/synchro`, `internal/majour` | Client de synchro du bureau ; mises à jour signées (Ed25519 sur `SHA256SUMS`, clé privée = secret `COF_DM_SIGNATURE`, jamais dans le dépôt). |
| `internal/mcp`, `internal/obsidian` | Serveur MCP ; export vers Obsidian. |
| `frontend/` | `index.html` + `js/` (MJ et auditeur, routes `#/…`, vues dans `js/views/`) ; `joueur.html` + `js/joueur.js` (joueurs) ; `style.css`, `combat.css`. |
| `docs/API.md` | Documentation de l'API. |

## Invariants à ne jamais casser

### Données et univers

- **Aucune donnée des livres dans le dépôt** (code, interface, docs **et tests**) : ni liste (peuples, profils, tailles, catégories, états…), ni table chiffrée, ni nom de voie ou de capacité, ni texte, ni numéro de page. Elles viennent du fichier de règles importé par le MJ (`internal/regles`, `GET /api/regles`) ; le code ne garde que structure et formules, et tout marche en saisie libre sans ce fichier. Un champ à choix venus des livres porte `Liste` (pas `Options`). Tests : règles et fiches inventées. Avant toute constante, liste ou message : vient-il d'un livre ? Si oui, fichier de règles.
- **Schéma** : tout champ ou type part de `internal/modele` ; le frontend se construit dessus, jamais de champ codé en dur.
- **Un univers = un fichier** : tout ce qui est au monde (images comprises) va dans le `.cofdm` ; ce qui tient à l'ordinateur (mot de passe MJ, jeton, serveur relié) va dans `internal/reglages`. Journal SQLite en mode DELETE, pas de WAL.
- **Monde unique et persistant** : pas d'état par campagne.
- **Intégrité des liens** : renommer met à jour les `[[Nom]]`, supprimer retire la fiche des autres. Toute fonction touchant noms ou références le préserve, avec un test.
- **Journal** : toute écriture passe par le store, avec son auteur.
- **Données tracker_COF** : `documents` par collection (`internal/store/documents.go`), suivent les rôles. La feuille d'un PJ (`feuilles`) lit son niveau sur la fiche : ne jamais dupliquer un champ de la fiche dans la feuille. Rencontres et profils de combat désignent des monstres du bestiaire de l'univers ; difficulté dans `internal/regles/rencontres.go`.
- **Champ qui change de sens ou disparaît** : augmenter `versionDonnees` (`internal/store/reparer.go`).
- **Fonctionnalités optionnelles** (`fonctionnalites.go` dans `modele`, `store`, `serveur`) : le cœur (PNJ, lieux, factions, objets, lore, événements, campagnes, quêtes, séances, PJ, bestiaire) n'en dépend jamais. Une nouvelle se déclare (clé, défaut, types) et se branche partout : routes dans `fonctionsDeRoute` (coupée : 404), types dans `Types`, outils MCP par `Fonction`, écrans par `actif()` (`schema.js`) et `fonction()` (page des joueurs). Couper ne supprime rien. Tests `serveur` / `mcp` : tout activé (`main_test.go`) ; coupures : `TestFonctionnalites*`.

### Accès et sécurité

- **Écoute sur `127.0.0.1` par défaut.** L'ouverture au réseau reste un choix du MJ (Paramètres ou `-reseau`) : jeton pour l'API, mot de passe MJ pour l'interface. Ces réglages ne se font que sur l'ordinateur du MJ (`surPlace`).
- **Toute nouvelle route est protégée par défaut** : n'ajouter à `ouvert()` que ce que la page de connexion exige. Jamais d'écriture derrière un `GET`.
- **Toute modification de l'API** se reporte dans `docs/API.md` dans le même commit.
- **`/api/univers` et `/api/claude`** ne répondent qu'à l'ordinateur du MJ. `/api/claude` (bureau seulement) n'écrit que l'entrée `cof-dm` de la config Claude, le reste du fichier gardé.
- **Secrets MJ** : champs `Secret` et relations réservées ne fuitent jamais vers les joueurs (page, comptes joueurs, export Obsidian « joueurs »).
- **Bureau / serveur** : le bureau n'est qu'un éditeur, le mode serveur n'existe que dans l'édition serveur (`edition_serveur.go`). Relié à un serveur, le bureau ne répond plus au réseau local (`Options.Relie`, `msgReseauRelie`, `BureauRelie`). Connexion par nom et mot de passe échangés contre un jeton propre au poste, rendu au départ (`DELETE /api/compte/jeton`) ; jamais de mot de passe gardé.
- **Mode serveur** : derrière un reverse proxy tout vient de `127.0.0.1` → **ne jamais se fier à `local()`**. Accès par `acces.entrer`, droits par `servirComptes` (admin : serveur ; `mj` : écrire ; `auditeur` : lire, toute route non-`GET` lui est refusée).
- **Multi-univers** : univers visé = en-tête `X-COF-Univers`, sinon cookie, sinon premier univers du compte (un jeton multi-univers doit envoyer l'en-tête, sauf `proprePerso`). Dans `/api/univers`, jamais `a.st` : `a.vise(r)`. Rôle lu dans l'univers visé : `c.Role(s.store.ID())`.
- **MCP** (`internal/mcp`) : chaque outil passe par l'API REST avec un jeton (jamais le store) ; description en français, annotations lecture / destructif, un test ; aucun effacement définitif ; stdout réservé au protocole, traces sur stderr.

### Joueurs

- **Page des joueurs** (`pagejoueurs.go`) : seule partie sans mot de passe à la maison, quand le MJ l'a ouverte. **Liste blanche** `s.libre` (`pageJoueursOuverte()`, `routeLibre()`) : seulement des lectures faites pour les joueurs (jamais notes MJ, fiche du monde ni chiffre d'un combat en cours : `reduire`), avec test de non-fuite (`TestPageJoueursLibre`). Seules écritures : feuille déverrouillée, niveau accordé par le MJ (`TestMonteeAccordee*`), jet de dés (un jet caché du MJ ne part jamais : `TestDes*`), proposition de partage du pot commun, que seul le MJ valide (`TestButinJoueurs`). Sur un serveur, restreintes au PJ du compte (`pjDuCompte`).
- **Comptes `joueur`** (`joueur.go`) : uniquement `joueur.html`, **liste blanche** `routesJoueur` + `routesPageJoueurs`, tout le reste en 403, avec test (`TestServeurJoueur`). Écrit seulement la feuille de son PJ et les champs `Joueur: true` quand le MJ a ouvert l'édition (`edition_joueurs`), et les propositions de partage du pot pour son PJ (`TestButinCompteJoueur`) ; ne jamais marquer `Joueur` un champ `Secret`, une référence ou ce qui pèse sur le jeu (statut, niveau). Filtre `mondeJoueur` : un nouveau genre de champ reste caché tant qu'il n'est pas dans `genresJoueur` ; il sert aussi à l'export Obsidian « joueurs » (`TestExportObsidian`).
- **Aides de jeu** (`aides.go`) : relation réservée `recu` (jamais posée par `POST /api/relations`), jamais `partagee`. Un champ ajouté au type `aide` n'arrive aux joueurs que si `documentJoueur` le prend explicitement.
- **Les joueurs n'agissent jamais sur un combat.**

### Combat

- Combats dans `documents` (collection `combats`). Toute action passe par `POST /api/combats/{id}/actions` ; le WebSocket `…/direct` ne fait que recevoir (vérifie `Origin`), jamais appliquer.
- La fiche d'un combattant est lue par le serveur (`ficheCombat`), jamais fournie par l'écran. PV et chance des PJ reportés sur la feuille hors journal (qui note début et fin).
- Écran (`views/combat.js`, `combat.css`) : styles sous `.combat`, adaptation par requêtes de conteneur sur `.combat-hote` (jamais la taille de la fenêtre), aucune valeur du jeu écrite : tout vient de `GET /api/regles`, et il reste utilisable sans.

### Synchronisation et versions

- Tout nouvel objet ou champ rangé dans l'univers passe par `aplatir` et `appliquer` (avec un test), sinon il ne voyage pas ; ce qui tient à un seul côté (réglage local, combat en cours) n'y entre pas. Identifiants par `prochainID`, jamais l'auto-incrément.
- **`VersionModele`** augmente à chaque changement de ce que l'échange transporte (type, champ, options, relation, fonctionnalité, réglage synchronisé, collection, forme d'une feuille ou des règles, hausse de `versionDonnees`) — `TestVersionModele` le vérifie. **`VersionProtocole`** quand la forme de l'échange change. Ces numéros, pas la version de l'application, décident de l'échange.

## Avant de rendre un lot

1. `gofmt -l .`, `go vet` et `go test` passent avec les tags ci-dessus.
2. Écrans touchés vérifiés dans Chrome (CDP) à 1300 et 390 px, sans erreur de console.
3. `docs/API.md` et `SUIVI.md` à jour.
4. Rien de local committé : ni `.db`, `.cofdm`, `.log`, ni médias réels, ni `donnees/`.

## Git et publication

- Une branche et une PR vers `main` par lot (workflow *Tests* sur chaque PR). **Ne pas fusionner avant l'essai du MJ.**
- PR empilées : avant de fusionner celle du dessous (`--delete-branch`), reporter celle du dessus sur `main` : `gh api -X PATCH repos/Tchyze/COF_DM/pulls/<n> -f base=main` (`gh pr edit --base` échoue).
- Faire essayer : `gh workflow run release.yml --ref <branche> -f canal=test`.
- Version stable : tag `vX.Y.Z` (trois nombres, rien d'autre), **jamais sans demande explicite** ; notes de version commençant par « Nouveautés » en français.
- Image Docker : le job `docker` de *Release* la construit et l'essaie à chaque passage, et ne l'envoie sur Docker Hub (`X.Y.Z` et `latest`) que pour un tag stable. Secrets `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` ; image `<compte>/cof-dm`, ou variable `DOCKERHUB_IMAGE`.
- `SUIVI.md` seul : commit direct sur `main`, sans PR.
