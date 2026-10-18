# Suivi du chantier — COF DM

Fichier de passation : ce qui a été fait, les décisions prises et ce qu'il reste à faire. À lire avant de reprendre le travail sur un autre poste, et à tenir à jour à la fin de chaque lot.

Dernière mise à jour : 16 octobre 2026, avec les points 11 et 12 de l'audit (PR 30 et 31) la revue des écrans des PR 5 à 16 (PR 32) les points mineurs de l'audit (PR 33) le dépôt d'un univers sur le serveur (PR 35) le choix d'univers par onglet (PR 36) la synchronisation hors ligne (PR 37 à 41), l'export vers Obsidian (PR 42) le serveur MCP (PR 43), le lanceur de dés (PR 44), les sorts et PM en combat (PR 45), la préparation de séance (PR 46) et l'après-séance (PR 47), puis le bureau en simple éditeur avec son mode de travail (local ou serveur, 18 octobre 2026) : le P1 de `PROJETS.md` est fait, hors restes à vérifier. La fusion de tracker_COF est terminée ; `main` est la seule branche, dans les deux dépôts.

## Dépôt public (18 octobre 2026)

`Tchyze/COF_DM` est **public** depuis le 18 octobre 2026, reparti d'un commit unique (le code de l'époque, nettoyé). L'ancien dépôt, avec tout l'historique, les PR 1 à 54 citées plus bas et les versions v1.0.0 et v1.1.0, est devenu **`Tchyze/COF_DM-historique`**, privé : les numéros de PR de ce fichier renvoient à lui. Le dépôt local a les deux remotes (`origin`, `historique`). Première version publique : **v1.2.0**.

- **Clé de publication** : `~/Outil/cle-publication-cof-dm.pem` (hors dépôt), aussi en secret `COF_DM_SIGNATURE` du dépôt. À sauvegarder ailleurs : sans elle, plus de mise à jour automatique (il faudrait changer la clé publique de `internal/majour`, et chacun réinstallerait à la main).
- **Faire essayer une PR** : `gh workflow run release.yml --ref <branche> -f canal=test`, puis l'application (canal Test) la propose.

## Pour reprendre sur un autre poste

Le MJ utilise l'application en mode serveur avec `docker compose` sur le poste WSL (`~/Outil/COF_DM`, `http://localhost:8100`), données dans `~/Outil/COF_DM/donnees`. Mise à jour : `git pull && docker compose up -d --build`.

1. **Cloner le dépôt** `COF_DM`. Le dépôt `tracker_COF`, entièrement absorbé, n'évolue plus : il n'est utile que pour consulter son histoire.
2. **Outils** : Go 1.26.9 au moins (fixé par `go.mod`) et un compilateur C pour SQLite (CGO). Commandes dans `CLAUDE.md`.
3. **Copier à la main les fichiers hors dépôt** (voir ci-dessous) : ils ne sont dans aucun dépôt, volontairement.
4. Vérifier que tout passe : `gofmt -l .`, `go vet -tags "sqlite_fts5 nogui" ./...`, `go test -tags "sqlite_fts5 nogui" ./...`.

### Fichiers hors dépôt, à emporter

Ils étaient dans `~/Outils/` sur le premier poste ; le second poste (WSL) a les siens dans `~/Outil/`, à côté des deux dépôts, avec le PDF du livre de base pour vérifier les tables. Le logiciel ne contient aucune donnée des livres : sans le premier fichier, le créateur de monstre, les repos et les listes de choix (peuples, profils, tailles, catégories) ne sont plus proposés.

| Fichier | Contenu |
|---|---|
| `Règles COF.json` | Le fichier de règles (`cof-dm-regles`) : listes, créateur de monstre, feuille de personnage et repos, difficulté des rencontres (`rencontre`), valeurs et états du combat (`combat`). À importer dans Paramètres → Bestiaire et règles. La version complète est sur le second poste (régénérée le 12 octobre 2026 et comparée au livre et à tracker_COF) : celle du premier poste n'a ni la partie `rencontre` au format du lot 3, ni la partie `combat`. |
| `exemples_monstres.json`, `exemples_personnages.json` | Fiches d'exemple de tracker_COF, retirées du dépôt. |
| `Bestiaire officiel COF.json`, `Profils et voies COF.json` | Le bestiaire officiel et les profils et voies, à importer dans chaque univers. |

Les fichiers du bestiaire officiel et des profils et voies restent, eux, dans le vault du MJ.

## Décisions prises (à respecter)

### Façon de travailler

- **Lots courts**, validés un par un : une branche par lot, PR vers `main`, fusion après passage de la CI, branche supprimée ensuite. Il ne doit rester que `main`.
- Poser les questions de fond **avant** de coder un gros chantier.
- Tout en **français** (code, commits, docs).

### Aucune donnée des livres dans le dépôt

Exigence ferme, pour que le logiciel se distribue sans problème. Ni liste, ni table chiffrée, ni nom de capacité ou de voie, ni texte, ni numéro de page — dans le code, l'interface, la documentation **et les tests** (qui utilisent des règles et des fiches inventées). Ces données viennent du **fichier de règles** importé par le MJ et rangé dans l'univers. Le logiciel reste pensé pour COF : la structure des fiches et les formules restent du code. Le nom « COF DM » est gardé. Règle détaillée dans `CLAUDE.md`.

### Mode serveur

- Derrière un reverse proxy nginx qui porte le HTTPS ; option `-serveur`. Rien n'est accordé aux requêtes venues de la machine.
- Comptes (nom d'utilisateur, mot de passe), jetons d'API par compte.
- **Rôles** : administrateur pour tout le serveur ; MJ, auditeur et joueur **univers par univers**.
  - Auditeur : lit tout, secrets compris, ne modifie rien.
  - Joueur : ne voit que les fiches que le MJ partage, sans leurs champs MJ ; jamais les fiches « Secret » ; lecture seule, sauf si l'administrateur ouvre l'édition par les joueurs (trois champs texte de son PJ, et toute sa feuille de personnage sauf les notes du MJ).
- **Plusieurs univers ouverts à la fois** ; chacun choisit le sien dans son navigateur.
- L'application de bureau reste sans compte.

### Bureau et serveur (décisions du 18 octobre 2026)

- **Le bureau n'est qu'un éditeur** : le mode serveur n'existe que dans l'image Docker (édition serveur, étiquette de compilation `serveur`). L'application de bureau refuse `-serveur`.
- **Deux modes de travail** du bureau : **local** (client seul) ou **serveur** (relié à un serveur, connecté ou hors connexion). Une zone en bas à gauche de la fenêtre le dit ; un clic ouvre le panneau (se connecter, changer de serveur, repasser en client seul, copier, envoyer, synchroniser).
- **Réseau local seulement en mode local** : la tablette du MJ et la page des joueurs à la maison ne s'ouvrent que si l'application n'est reliée à aucun serveur. Reliée, elle ne répond plus au réseau, même si la case est cochée.
- **Authentification** : nom d'utilisateur et mot de passe, échangés aussitôt contre un jeton propre au poste (choisi comme le plus sûr : le mot de passe n'est jamais gardé ni recopié, le jeton se retire seul quand le poste quitte le serveur, et l'administrateur le voit et peut le retirer). Pas de saisie d'une clé d'API à la main.
- **Quitter un serveur** (client seul, ou changement de serveur) : les copies restent sur l'ordinateur, **en pause** ; elles reprennent si l'on se reconnecte au même serveur. « Détacher » (page Synchronisation) en fait des univers locaux indépendants.

### Fusion de tracker_COF dans COF DM

- **Fusion complète** : bestiaire, feuilles de personnage, rencontres et combats dans le fichier d'univers, mêmes comptes et rôles. Le dépôt tracker_COF n'évolue plus (dernier ajout : les dés réels et l'écran de combat sans défilement, PR 2 de tracker_COF, à reprendre au lot 4).
- **Page des joueurs** (lot 5) : un joueur voit du combat **l'ordre d'initiative et les états**, sans aucun chiffre. À la maison, la page s'ouvre par un **interrupteur séparé**, éteint par défaut, réglé sur l'ordinateur du MJ ; **chacun y choisit son PJ** librement ; elle ne montre que **la feuille, le combat et les hauts faits**, pas les fiches du monde.
- **Combat** (lot 4) : sans fichier de règles, **combat minimal** (initiative, PV, attaque contre la DEF, dégâts, états libres sans effet). Les **PV et les points de chance** des PJ sont écrits en direct sur leur feuille. Un PNJ du monde tombé à 0 PV n'est **jamais marqué mort d'office** : le bilan le propose. Un combat est **rattaché d'office à la séance** de la campagne que montre l'écran du MJ, et se détache ou se déplace ensuite.
- Bestiaire et règles **dans chaque univers**, avec reprise d'un univers à l'autre.
- **Une seule fiche par PJ** : la fiche porte la feuille de personnage. Un PNJ porte son profil de combat ou pointe vers un monstre du bestiaire.
- Page joueurs : **libre à la maison** (réseau local, QR code), **par compte sur un serveur**.
- **Rien à reprendre** de l'ancien tracker : pas d'outil de migration.
- L'intégration passe **avant** la synchronisation.

### Priorité P1 de `PROJETS.md` (décisions du 16 octobre 2026)

- `PROJETS.md` liste tous les chantiers possibles, classés P1 à P4. Le P1 est lancé : serveur MCP, lanceur de dés général, sorts et PM en combat, préparation de séance, routine d'après-séance, restes à vérifier.
- **Façon de travailler pour le P1** : enchaîner les lots ; chaque PR est fusionnée dès que la CI passe, sans attendre de relecture. S'arrêter seulement sur une question de fond ou un échec.
- **Serveur MCP** : une option du même binaire, `cof-dm -mcp` (stdio), qui joint l'application lancée par son API avec un jeton ; rien de plus à publier. Ses écritures sont **directes et journalisées** (auteur `claude`), comme tout client de l'API ; pas d'effacement définitif.
- **Lanceur de dés** : les jets des joueurs et les jets publics du MJ se voient de **toute la table** ; le MJ a en plus des jets cachés.

### Synchronisation (faite, PR 37 à 41)

- Pendant qu'un MJ est hors ligne, les joueurs et d'autres MJ modifient le même univers sur le serveur ; plusieurs MJ peuvent être hors ligne en même temps.
- Conflit : **fusion champ par champ**, question seulement si le même champ a changé des deux côtés.
- Archivage et suppression : **se propagent, sauf si la fiche a été modifiée de l'autre côté** (alors gardée et signalée).
- **En continu** : la copie locale est tenue à jour en arrière-plan. Les pages qui se mettent à jour seules ne sont pas demandées.
- Réservée à l'**administrateur et au MJ**.
- Bureau : « Se connecter à un serveur » = **copie locale synchronisée**.
- **Périmètre** (15 octobre 2026) : tout se synchronise (fiches, relations, images, feuilles, bestiaire, règles, profils et voies, calendrier, combats terminés), **sauf un combat en cours**, qui se joue d'un seul côté et part une fois terminé.
- **Conflit en attente** : le reste se synchronise ; le champ garde **la valeur du serveur** partout, la valeur locale attend dans une liste « Conflits » où le MJ choisit plus tard.
- **Lien dans les deux sens** : « Se connecter à un serveur » télécharge un univers du serveur en copie locale ; « Envoyer sur le serveur » publie un univers local (dépôt), puis le garde synchronisé.
- **Journal fusionné** : les écritures faites hors ligne apparaissent dans le journal du serveur avec leur auteur et leur heure, et inversement.
- **Identifiants** (décision technique) : chaque copie a un numéro et tire ce qu'elle crée dans sa plage d'identifiants (`internal/store/identifiants.go`) ; rien n'est à traduire d'un côté à l'autre. Plage 0 : le serveur et tout univers jamais copié.

**Lots** : 1. identifiants par plage ; 2. état de synchronisation, champ par champ (relevé, application) ; 3. protocole côté serveur (inscription d'une copie, échange, fusion, conflits, suppressions, journal) ; 4. bureau (se connecter, envoyer, synchronisation en arrière-plan, panneau « Conflits »).

## Ce qui a été fait

### Audit de sécurité et corrections

| PR | Contenu |
|---|---|
| 6 | Réglages réservés à l'ordinateur du MJ, même avec le jeton ; tracker sans redirection ni fuite de mot de passe. |
| 7 | Go 1.26.9 au minimum, go-sqlite3 à jour (25 failles de la bibliothèque standard éliminées). |
| 8 | Limite d'essais, sessions révocables, en-têtes contre le clickjacking, droits des fichiers, délais HTTP. |

### Mode serveur

| PR | Contenu |
|---|---|
| 9 | Comptes, rôles par univers, jetons d'API par compte, premier compte `admin`. |
| 10 | Interface des comptes (mon compte, comptes du serveur, lecture seule). |
| 11 | Rôle joueur et fiches partagées par le MJ (liste blanche de routes). |
| 12 | Édition par les joueurs de leur fiche de PJ. |
| 13 | Plusieurs univers ouverts à la fois. |

### Fusion de tracker_COF

| PR | Lot | Contenu |
|---|---|---|
| 14 | 1 | Bestiaire dans l'univers, créateur de monstre, fichier de règles, profils et voies, bibliothèque d'un univers à l'autre. Retrait de toutes les données des livres du code. |
| 15 | 2a | Feuille de personnage côté serveur : modèle, valeurs calculées, bornes venues des règles, rôles, export. |
| 16 | 2b | Écran de la feuille dans la fiche du PJ, repos décrits par les règles, application d'un profil, niveau maximum. |
| 18 | 3 | Rencontres et PNJ sur le bestiaire de l'univers : créatures et profil de combat (`adversaires`, `profil_combat`), difficulté calculée avec la partie `rencontre` des règles (`GET /api/entites/{id}/difficulte`), reprise des anciens liens à l'ouverture (`versionDonnees` 3). Retirés : envoi des rencontres et des PNJ au tracker, évaluation par le tracker, lancement du combat, champ `tracker_rencontre`. |
| 19 | 4a | Moteur de combat (`internal/combat`) et son API : combats rangés dans l'univers, lancement d'une rencontre avec les PJ de la campagne, actions par `POST`, suivi en direct par WebSocket, rôles, PV et points de chance reportés sur les feuilles, rattachement à la séance. Partie `combat` du fichier de règles (dé, critique, point de chance, budget d'actions, états) ; combat minimal sans elle. Aucun écran. |
| 20 | 4b | Écrans du combat : liste des combats et écran de jeu repris de tracker_COF (dés réels, fenêtres secondaires), logé aussi dans l'écran du MJ à la place du cadre du tracker ; « Lancer » depuis une rencontre et depuis l'écran du MJ ; PV des PJ lus sur les feuilles (`GET /api/table/etat`) ; combats d'une séance, leur récapitulatif et « Marquer mort » pour les PNJ tombés. Retirés : `/api/table/tracker`, le cadre du tracker et sa politique `frame-src`. |
| 21 | 6 | Retrait du pont vers tracker_COF : `internal/tracker`, les routes `/api/tracker`, les réglages `tracker_url` / `tracker_mdp`, la section de Paramètres, `tracker.js`, les champs `fiche_tracker` et `combats`, les genres `tracker` / `trackers` (`versionDonnees` 4, retirés sans bruit des fiches existantes). |
| 22 | 5 | Page des joueurs (`/joueur.html`) : sa feuille (modifiable quand le MJ déverrouille), le combat réduit à l'ordre et aux états, les hauts faits, en temps réel. Libre sur le réseau local à la maison quand le MJ l'ouvre (liste blanche de routes à part) ; par compte sur un serveur, restreinte au PJ du joueur. |

### Déploiement

| PR | Contenu |
|---|---|
| 23 | `Dockerfile` et `docker-compose.yml` pour le mode serveur derrière le nginx de la machine (réseau de l'hôte, écoute sur `127.0.0.1` seulement, données dans `./donnees`). Essayé en local : image construite, compte `admin` créé, port fermé au réseau. Non essayé derrière un vrai nginx. |
| 26 | Cache de compilation gardé entre deux constructions Docker : 77 s la première fois (SQLite), 3,5 s ensuite. `--no-cache` inutile. |

### Retours d'usage (premiers essais du MJ, octobre 2026)

| PR | Contenu |
|---|---|
| 24 | Paramètres : le panneau « Bestiaire et règles » détaille les cinq parties du fichier de règles importé et dit qu'il ne crée aucune fiche (la liste « Règles » vient de « Importer des fiches »). |
| 25 | Bestiaire : lignes de la liste qui se chevauchaient (éléments flexibles comprimés). Graphe : cadrage, légende qui filtre les types, fiches sans lien cachées par défaut. |
| 27 | Graphe : un trait par paire de fiches, fiches écartées avec leur nom, noms qui ne se chevauchent plus (taille constante à l'écran), mentions cachées par défaut, légende sous le graphe. |
| 28 | Graphe : liens élastiques (simulation continue, comme Obsidian) ; une fiche tirée entraîne ses voisines, tout se repose seul. |

### Audit, suite

| PR | Contenu |
|---|---|
| 30 | Point 11 : la restauration d'un export valide tout comme une saisie (noms, champs et références des fiches, relations avec supérieur et contexte, noms et rattachement des images, feuilles des PJ) avant d'écrire ; au moindre refus, rien ne change. Un export d'une version plus récente est refusé. Essayé sur une copie de l'univers du MJ : export et restauration à l'identique. |
| 31 | Point 12 : actions GitHub épinglées par SHA (versions sous Node 24), `appimagetool` 1.9.1 et le runtime AppImage en version fixe vérifiés par leur empreinte, `SHA256SUMS` joint à chaque Release, droits des workflows réduits. *Release* lancé à la main sur la branche : exe et AppImage construits ; l'étape des empreintes, qui ne tourne que sur un tag, n'a été essayée qu'en local. Pour mettre à jour une action ou un outil : relever son nouveau SHA ou sa nouvelle empreinte (commentaire en tête de `release.yml`). L'image Docker (bases `golang` et `debian` prises par tag) reste hors de cette chaîne. |

| 33 | Points mineurs : une erreur 500 ne renvoie plus son détail (il reste dans `cof-dm.log`) ; un univers s'ouvre avec un pilote SQLite défensif (`trusted_schema=OFF`, `cell_size_check`), un contrôle d'intégrité, et le refus des déclencheurs, vues et tables virtuelles inconnues ; les scripts Python lisent le jeton dans `COF_DM_JETON` (l'argument reste accepté, avec un avertissement). L'univers du MJ (copie) passe les contrôles. |

### Synchronisation

| PR | Contenu |
|---|---|
| 37 | Lot 1 : identifiants par plage. Chaque copie a un numéro (`meta` `replique`, 0 par défaut) et tire les identifiants de ce qu'elle crée (fiches, relations, images, documents, journal) dans sa plage de 2^40 ; la plage 0 n'est pas déplacée par les identifiants d'une copie. Univers existants inchangés. |
| 38 | Lot 2 : état de synchronisation (`internal/store/sync.go`). L'univers vu champ par champ (fiches, relations, images, documents, feuilles, calendrier, règles, profils et voies) ; `Relever` compare l'univers à la table `sync_etat` et numérote ce qui a changé (rien à noter au moment d'écrire, aucune écriture oubliée) ; `ChangementsDepuis` ; `Appliquer` écrit des changements reçus, validés comme une saisie (champ refusé rendu à part). Combat en cours ni relevé ni pris pour supprimé. Sur une copie de l'univers du MJ : 4 638 champs relevés en 60 ms (20 ms ensuite), et un univers vide qui reçoit tout en ressort identique, sans refus. |
| 39 | Lot 3 : protocole (`internal/store/protocole.go`, `internal/serveur/sync.go`). Côté serveur, `Recevoir` traite un échange dans une seule transaction (conflit de champ : le serveur garde le sien ; suppression contre modification : objet gardé et signalé, renvoyé entier dans le bon sens ; refus de validation signalé). Côté copie, `Preparer`, `Integrer` (y compris ce qui change pendant l'échange), la liste « Conflits » et `ResoudreConflit`. Le journal voyage comme un objet. Routes `POST /api/sync/copie` (inscription et fichier de la copie), `POST /api/sync`, `PUT /api/sync/images/{fichier}`, `GET /api/sync/copies`, en mode serveur, pour le MJ et l'administrateur. |
| 40 | Lot 4a : application de bureau, sans écran. Client `internal/synchro` (connexion par compte qui ne garde qu'un jeton, copie, dépôt, échange, images). Routes du menu Fichier : connexion, relier (copie d'un univers du serveur), envoyer (publier l'univers ouvert puis le remplacer par sa copie reliée, original en sauvegarde), synchroniser, délier ; liste « Conflits ». Échange en arrière-plan toutes les 30 secondes ; hors ligne, l'état le dit. Lien rangé dans `reglages.json`. |
| 41 | Lot 4b : écrans. Fichier → « Se connecter à un serveur… » (connexion, choix de l'univers, nom de la copie), « Envoyer sur le serveur… », « Synchronisation… » ; état sous le bouton Fichier (à jour, hors ligne, conflits) ; page « Synchronisation » avec la liste « Conflits » (champ en clair, les deux valeurs, le choix). Premier échange dès le lancement. Vu dans Chrome avec un serveur et un bureau de test : connexion, copie, conflit puis « Garder la mienne », hors ligne, écran de 390 px. |

### Confort

| PR | Contenu |
|---|---|
| 35 | Fichier → **Déposer un univers…** (administrateur d'un serveur) : le fichier `.cofdm` est envoyé, contrôlé à part (univers reconnu, intact, sans objet étranger, pas la copie d'un univers présent), puis rangé dans `univers/` et ouvert, sans relancer. `POST /api/univers/deposer`. Vu dans Chrome sur l'instance de test. L'exemple nginx passe à `client_max_body_size 1g`. |
| 36 | Choix d'univers **par onglet** : chaque onglet garde le sien (`sessionStorage`) et l'envoie dans `X-COF-Univers`, ou `?univers=` pour les images, WebSocket et téléchargements ; un nouvel onglet prend le dernier choisi. Vu dans Chrome : deux onglets du même navigateur sur deux univers, image et export du bon univers. |

### Revue des écrans (PR 5 à 16)

| PR | Contenu |
|---|---|
| 32 | Écrans des PR 5 à 16 vus dans Chrome sans fenêtre, sur une instance de test en mode serveur (deux univers, comptes administrateur, MJ, auditrice et joueuse, règles, profils et bestiaire importés), sans erreur dans la console : connexion (réussie et ratée), Mon compte et jeton d'API, comptes du serveur (rôles, PJ du joueur, édition par les joueurs), choix d'univers, lecture seule de l'auditrice (fiche, secrets, refus de modifier), vue de la joueuse (fiches partagées, fiche cachée introuvable, sa fiche et sa feuille modifiées), feuille du MJ avec profil appliqué, bestiaire, créateur de monstre, vue de la joueuse et page des joueurs à 390 px sans débordement. Corrigé : une feuille vierge partait de la valeur minimale des règles (−5 partout, DEF 5) ; elle part de la valeur neutre (modificateur nul) dans les bornes. En lecture seule, plus d'invitation à relier la fiche ni de cadre « Images » vide. |

### Feuille de route

| PR | Contenu |
|---|---|
| 42 | **Export vers Obsidian** (décisions du 16 octobre 2026 : zip téléchargé depuis l'application, secrets au choix, arborescence du vault que lit `vault_vers_cofdm.py`, avec les récapitulatifs des combats et le journal ; ni feuilles ni bestiaire). Fichier → « Exporter vers Obsidian… » et bouton de Paramètres ; `GET /api/export/obsidian` (`?secrets=0` : ce que voient les joueurs, par `mondeJoueur`). Paquet `internal/obsidian` : une note par fiche (propriétés, sections, relations en tableau, images), chronologie, fils rouges à part, combats terminés, journal, archives à part ; noms de fichiers nettoyés et homonymes départagés, liens `[[Nom]]` réécrits. Sur une copie de l'univers du MJ : 214 fichiers, tous les liens mènent à une note. Réimporté par `vault_vers_cofdm.py` dans un univers vide : PNJ, lieux, factions, quêtes, campagne, événements et 107 relations retrouvés ; perdus ou approximatifs : objets, lore, règles, rencontres (inconnus du script), texte des événements (refait depuis la chronologie), catégories des lieux et factions et missions (refaites par mots-clés). Corrigé au passage : le script d'import plantait depuis la PR 14 (peuples et profils sans liste dans le schéma), il les prend tels quels. Dialogue vu dans Chrome, à 1300 et 390 px, sans erreur. |

### Priorité P1

| PR | Contenu |
|---|---|
| 43 | **Serveur MCP** (projet 1) : `cof-dm -mcp`, paquet `internal/mcp` (JSON-RPC sur stdio, sans dépendance, versions 2024-11-05 à 2025-11-25 du protocole). 25 outils traduits en appels de l'API : 17 en lecture (schéma, statistiques, recherche, fiche par identifiant ou par nom, graphe, chronologie, qui sait quoi, organigramme, partie en cours, feuille, bestiaire résumé et filtré, monstre, difficulté d'une rencontre, combats, combat, journal, liens morts), 8 en écriture (créer, modifier, archiver ou restaurer, partager une fiche ; créer, modifier, supprimer une relation ; révéler un secret ; modifier une feuille, champ par champ, avec contrôle de modification concurrente). Pas d'effacement définitif. Sur le bureau, sans réglage : port lu dans `config.json`, jeton dans `reglages.json` (seulement vers cet ordinateur) ; sinon `COF_DM_URL`, `COF_DM_JETON`, `COF_DM_UNIVERS`. Une réponse trop longue est coupée avec une invitation à filtrer. Essayé avec le binaire contre une application sans fenêtre : création d'une fiche, journal signé `claude`. Non essayé : Claude Desktop et Claude Code réels, l'exe Windows (sous-système graphique) lancé par un client MCP. |
| 44 | **Lanceur de dés** (projet 4) : paquet `internal/des` (formules `NdF`, `kh` / `kl` pour garder les meilleurs ou les pires, bornes : 100 dés, 1000 faces), jets de la table en mémoire (100 derniers, hors de l'univers, ni synchronisés ni journalisés), `GET` / `POST /api/des` (MJ, jets cachés) et `/api/joueur/des` (page des joueurs : à la maison pour le PJ choisi, sur un serveur pour le PJ du compte), diffusion des jets publics par le WebSocket des joueurs. Écrans : bouton 🎲 et touche D sur toutes les pages du MJ (formule, dés rapides, jet caché, historique relu toutes les 3 s, badge et annonce des jets des joueurs ; l'auditeur lit sans lancer) ; onglet « Dés » de la page des joueurs (tests de caractéristique et d'attaque lus sur la feuille, dégâts, dé bonus ou malus, formule libre, jets de la table). Deux outils MCP de plus : `lancer_des` (caché par défaut) et `jets`. Tests de non-fuite des jets cachés (maison, serveur, direct). Vu dans Chrome : panneau du MJ à 1300 px (tableau de bord et écran du MJ), onglet des joueurs à 390 px en sombre et en clair, jet du joueur reçu par le MJ, sans erreur de console. |
| 45 | **Sorts et PM en combat** (projet 5) : un combattant porte ses points de mana (`pm`, `pmMax`, lus sur la feuille d'un PJ) et les capacités acquises dans ses voies (`voies`, jusqu'au rang). Actions `pm` (dépenser, récupérer, dans les bornes), `usage` (capacité à usage limité marquée utilisée dans ce combat, ou rendue) et `cout_pm` d'une attaque (sort payé au lancer, qu'il touche ou non ; refusé sans assez de PM). Les PM d'un PJ sont reportés sur sa feuille comme les PV et les points de chance, annulation comprise. Écran : PM sur la ligne, la carte du combattant actif et sa fiche (− PM / + PM), « Coût PM » dans le formulaire d'attaque pour qui a des PM, capacités à toucher (barrées une fois utilisées) et saisie libre, PM et PM max dans « Modifier », rappel des règles. Tests : moteur (sorts, bornes, modification, usages), actions aléatoires avec PM, parcours serveur avec report et annulation. Vu dans Chrome à 1300 et 390 px : sort à 4 PM, sort trop cher refusé, capacité utilisée, PM récupérés, formulaire en dés réels, sans erreur. |
| 46 | **Préparation de séance** (projet 2) : `GET /api/entites/{id}/preparation` rassemble, d'après la campagne de la séance, la séance précédente, où est le groupe, les PJ, les fils rouges, les quêtes en cours (étapes restantes), les secrets non révélés (indices, PJ qui les ignorent), les PNJ probables (scène des quêtes, lieu du groupe et ses sous-lieux, sans les morts) et les rencontres prêtes. Page `#/preparer/<séance>` (bouton « 🧭 Préparer » de la séance et de l'onglet Séance de l'écran du MJ) : on coche, on ajoute ses notes, le tout s'ajoute en Markdown avec des `[[liens]]` au champ « Préparation », qui passe **réservé au MJ** (il nomme des secrets ; aucune donnée à reprendre). Outil MCP `preparer_seance`. Tests : contenu complet sur une campagne inventée, séance sans campagne, non-fuite (route et champ) pour une joueuse. Vu dans Chrome à 1300 et 390 px, écriture dans la séance, sans erreur. |
| 47 | **Après la séance** (projet 3) : `POST /api/entites/{id}/apres-seance` clôt une séance en une fois (résumé, présents, date en jeu et lieu du groupe reportés sur la campagne, étapes et statut des quêtes, secrets appris avec la séance, PNJ morts, rencontres jouées ou évitées et rattachées, séance suivante créée). Tout est contrôlé avant la première écriture (`Store.Verifier`, validation sans écriture) : une valeur refusée n'en laisse passer aucune. La révélation d'un secret est mise en commun (`revelerSecret`). Page `#/apres/<séance>` (bouton « ✅ Après la séance », et « Après » dans l'onglet Séance de l'écran du MJ) ; elle propose les PNJ du monde à terre dans les combats de la séance, et mène à la préparation de la suivante. Outil MCP `clore_seance`. Tests : refus sans aucune écriture, étapes changées, secret appris par un lieu, parcours complet, nom de la séance suivante. Vu dans Chrome à 1300 et 390 px, sans erreur. |
| 48 | **Restes à vérifier** (projet 6, en partie) : Paramètres tient dans 390 px (grille des panneaux bornée à la largeur, chemins longs coupés, tableau des sauvegardes qui défile dans son cadre) ; vus dans Chrome : deux appareils sur le même combat, thème clair de l'écran de combat. Restent à faire à la main : voir « À vérifier à la main ». |

### Bureau et serveur

| PR | Contenu |
|---|---|
| 49 | **Édition serveur** : `edition_serveur.go` / `edition_bureau.go` (étiquette `serveur`), `Dockerfile` et CI compilent l'édition serveur ; le bureau refuse `-serveur` avec un message. **Mode de travail** (`internal/serveur/distant.go`) : une seule connexion par application (réglage `serveur` : adresse, compte, jeton), à la place d'un jeton par univers ; reprise des anciens liens au lancement. Routes `POST /api/univers/serveur/local` et `GET /api/univers/serveur/univers` ; la connexion ne dure plus 15 minutes, elle fait passer en mode serveur. Le serveur répond-il ? vérifié toutes les 30 secondes même sans copie ouverte (`connecte`, `hors_ligne`, `refuse`). Jeton rendu par le poste : `DELETE /api/compte/jeton` (le jeton se retire lui-même). Réseau fermé quand l'application est reliée (`Options.Relie`) : requêtes du réseau refusées, ouverture du réseau et de la page des joueurs refusées, pas d'écoute réseau au lancement. **Écran** : zone en bas à gauche (`frontend/js/connexion.js`), pied du menu collant, panneau du mode, dialogues de connexion, de copie et d'envoi ; menu Fichier réduit à « Serveur et synchronisation… » ; page Synchronisation avec l'état « en pause » ; note dans Paramètres. Tests : `TestBureauModes`, `TestBureauRepriseLiens`, `TestRendreJeton`. Vu dans Chrome avec un serveur et un bureau de test : local, connexion, copie, hors connexion puis retour (fiche écrite hors connexion arrivée), client seul (jeton retiré du serveur), 390 px, thème clair, sans erreur de console. Corrigé au passage : les éléments du menu latéral se tassaient dans le tiroir étroit. |

| 51 | **Univers du serveur** : relié à un serveur, un bouton sous Fichier ouvre la fenêtre de ses univers (ouvert, copié ici, seulement sur le serveur ; filtre au-delà de six). Un clic passe à l'univers : sa copie s'ouvre, ou elle est tirée d'abord (`POST /api/univers/serveur/ouvrir`). Hors connexion, les copies de ce serveur rangées ici restent listées et s'ouvrent. Les liens retiennent le nom de l'univers et le fichier de la copie (noté aussi à chaque ouverture d'une copie). Test `TestBureauUniversDuServeur`. Vu dans Chrome : trois univers, copie puis ouverture, retour sur une copie existante, hors connexion à 390 px, sans erreur. |
| 54 | **Mises à jour du bureau** : paquet `internal/majour` (versions publiées sur GitHub, canaux stable et test, comparaison des versions et préversions, téléchargement, vérification de la signature Ed25519 de `SHA256SUMS` et de l'empreinte, remplacement du programme avec l'ancien en `.ancien`, relance). Paramètres → Mises à jour (canal, recherche, notes, « Installer et redémarrer ») et badge ⬆ du pied du menu ; recherche au lancement puis toutes les six heures ; rien ne s'installe sans clic. Workflow *Release* : canal `test` au lancement manuel (préversion `X.Y.(Z+1)-test.N` de la branche), `SHA256SUMS.sig` signé avec le secret `COF_DM_SIGNATURE`. Clé privée de publication gardée hors dépôt par le MJ (`~/Outil/cle-publication-cof-dm.pem`, à sauvegarder : sans elle, plus de mise à jour automatique possible). Tests : versions, recherche, installation, signature d'une autre clé et fichier altéré refusés, routes. Ne marche pour de vrai qu'une fois le dépôt public (l'API de GitHub ne montre pas les versions d'un dépôt privé). |
| 52 | **Un jeton par poste** : un compte qui avait accès à plusieurs univers ne rendait jamais son jeton (le serveur exigeait `X-COF-Univers` même pour `DELETE /api/compte/jeton`) : les routes du compte ne visent plus d'univers. À la connexion, le poste retire d'abord les jetons laissés à son nom ; un jeton déjà inconnu du serveur compte comme rendu. Test `TestBureauJetonUnique` (compte sur deux univers). Constaté par le MJ : dix jetons « Synchronisation — <poste> » après dix connexions. |
| 50 | **Port de la fenêtre** : ouverte au réseau, l'application exige que `127.0.0.1` soit libre sur son port. Sous Windows, le relais de WSL y tenait le serveur Docker, et la fenêtre de l'exe l'ouvrait à la place de l'application. |

## Ce qu'il reste à faire

### Prochain chantier

Le P1 de `PROJETS.md` est fait (PR 43 à 47), sauf le projet 6 (restes à vérifier, ci-dessous). Ensuite : le P2, à commencer par les aides de jeu poussées aux joueurs (projet 9). Les cartes (projets 7 et 8) sont reportées bien plus tard (décision du 18 octobre 2026 : le MJ utilise un logiciel à part). Les restes à vérifier (6) au fil de l'eau.

### Audit

Tous les points sont traités (PR 6 à 8, 30, 31 et 33).

### Confort

Rien en attente (PR 35 et 36).

## Points en attente d'une décision

- **Passage en public** (décidé le 18 octobre 2026) : dépôt neuf, en un commit, à partir du code nettoyé ; l'actuel est renommé « COF_DM-historique » et reste privé (les anciens commits contiennent les listes des livres, et GitHub garde ceux des PR). tracker_COF reste privé.

- **Script d'import et export Obsidian** : faire de `vault_vers_cofdm.py` un vrai aller-retour (lire les notes d'événements, d'objets, de lore, de règles et de rencontres, les propriétés `catégorie` exactes) ? Aujourd'hui il reste un script d'import unique, taillé pour le vault d'origine.

## À vérifier à la main

- **Serveur MCP** : le brancher dans Claude Desktop (Windows, `COF-DM.exe -mcp`) et dans Claude Code, et vérifier que l'exe, compilé en sous-système graphique, parle bien sur les tubes que lui donne le client.

- **Export vers Obsidian** : ouvrir le zip dans le vrai vault (rendu des propriétés et des tableaux, liens).

- Les écrans des PR 5 à 16 ont été vus dans Chrome (PR 32). Restent à voir : le menu Fichier et Paramètres de l'application de bureau (sans compte), et l'ouverture au réseau avec le mot de passe MJ.
- Le second poste peut piloter Chrome sans fenêtre : les écrans des lots 3 et 4b y ont été vus, sans erreur dans la console (rencontre et difficulté, profil de combat d'un PNJ, sélecteur du bestiaire ; liste des combats, écran de combat en large et en étroit, attaque critique aux dés réels, état, posture défensive, fin de combat et récapitulatif, combat logé dans l'écran du MJ, bilan d'une séance et « Marquer mort »). La page des joueurs (lot 5) y a été vue par l'adresse réseau, comme depuis un téléphone, pendant que le MJ agissait (page fermée, choix du PJ, feuille verrouillée puis déverrouillée, combat en direct, « C'est à vous ! », hauts faits, réglage dans Paramètres). Vus depuis (PR 48) : deux appareils sur le même combat (le tour et le round suivent en direct) et le thème clair de l'écran de combat. Non essayés : WebView2 et WebKitGTK (seul Chrome a servi), et la reprise des anciens liens sur un vrai univers.
- **Mode de travail du bureau** : essayé avec un serveur et un bureau de test sur le même poste, dans Chrome. Pas encore essayé dans la vraie fenêtre (WebView2, WebKitGTK), ni avec le vrai serveur derrière nginx. Défaut connu, déjà présent avant : en agrandissant la fenêtre avec le tiroir du menu ouvert (écran étroit), la mise en page reste celle du tiroir jusqu'au prochain clic sur ☰.
- **Synchronisation** : essayée par les tests et avec un serveur et un bureau de test sur le même poste. Pas encore essayée avec le vrai serveur derrière nginx (HTTPS), ni sur un portable qui perd vraiment la connexion, ni avec deux copies hors ligne en même temps.
- **L'exemple de configuration nginx** du README n'a pas été essayé avec un vrai nginx.
- L'exe Windows et l'AppImage se compilent avec Go 1.26 (workflow *Release* lancé sans tag), mais n'ont pas été lancés.
