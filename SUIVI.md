# Suivi du chantier — COF DM

Fichier de passation : où en est le projet, comment on y travaille, les décisions prises et ce qu'il reste à faire. À lire avant de reprendre le travail, et à tenir à jour à la fin de chaque lot.

Dernière mise à jour : 29 octobre 2026 (version 1.2.4 ; `main` porte en plus les PR 24 à 26).

## Où on en est

- **Version stable : 1.2.4** (22 octobre 2026) : PR 17 (Paramètres en accordéon) et PR 19 à 22 (montée de niveau guidée).
- **Numéros d'échange** : `VersionModele` 7 sur `main` (5 dans la 1.2.4), `versionDonnees` 6, `VersionProtocole` 1. Un bureau et un serveur ne se synchronisent qu'au même modèle : ils se mettent à jour ensemble.
- **Branche en cours** : `butin-serveur` (PR 29, projet 14 lot 1 : serveur et données du butin et du pot commun ; modèle 8), à essayer par le MJ avant la fusion ; lots 2 (écrans du MJ) et 3 (page des joueurs) à venir, en PR empilées. La PR 28 (schéma des fiches ; préversion 1.2.5-test.31) est fusionnée le 31 octobre 2026 **sans essai, à la demande du MJ**. Les PR 24 (corrections rapides), 25 et 26 (inventaire, lots 1 et 2), essayées par le MJ en préversion 1.2.5-test.30, sont fusionnées dans `main` le 29 octobre 2026 ; la branche `preversion-inventaire` est supprimée. Pas encore de version stable 1.2.5 : seulement à la demande du MJ.
- **Chantier en cours** : projet 14 de `PROJETS.md`, le butin et les récompenses de fin de combat (s'appuie sur l'inventaire). Décisions prises le 31 octobre 2026 (« Butin et pot commun »).
- **Le serveur du MJ** tourne en Docker sur le poste WSL (`~/Outil/COF_DM`, `http://localhost:8100`, données dans `~/Outil/COF_DM/donnees`), sur `main` depuis le 29 octobre 2026, mis à jour à la main : `git pull && docker compose up -d --build`. Le bureau (exe Windows) s'y relie en mode serveur.
- **Fusion de tracker_COF** : terminée. Le dépôt tracker_COF n'évolue plus.

## Façon de travailler (consignes du MJ)

- **Une branche par lot**, PR vers `main`. **Ne pas fusionner avant que le MJ l'ait essayée.** Plusieurs lots d'un même chantier : PR empilées, chacune sur la précédente.
- **Faire essayer** : `gh workflow run release.yml --ref <branche> -f canal=test` ; la préversion `X.Y.(Z+1)-test.N` arrive dans l'application de qui suit le canal Test.
- **PR empilées** : avant de fusionner celle du dessous avec `--delete-branch`, reporter celle du dessus sur `main` par l'API REST (`gh api -X PATCH repos/Tchyze/COF_DM/pulls/<n> -f base=main` ; `gh pr edit --base` échoue). Sinon GitHub ferme la PR du dessus.
- **Version stable** : seulement à la demande du MJ, par un tag `vX.Y.Z` (trois nombres : `internal/majour` et le workflow *Release* ne lisent rien d'autre ; « 1.2.2.1 » a été écarté pour cette raison). En tête des notes de version, un résumé « Nouveautés » en français, ajouté à la main.
- **SUIVI.md** : commit direct sur `main`, sans PR. Le tenir à jour à la fin de chaque lot.
- **Écrans** : vérifiés dans Chrome (CDP), à 1300 et 390 px, sans erreur de console.
- **Questions de fond avant de coder** un gros chantier ; tout en **français** (code, commits, docs).
- **Aucune donnée des livres** dans le dépôt (code, interface, doc, tests) : voir `CLAUDE.md`.

## Dépôt et publication

- `Tchyze/COF_DM` est **public** depuis le 18 octobre 2026, reparti d'un commit unique. L'ancien dépôt, avec tout l'historique et ses PR 1 à 54, est **`Tchyze/COF_DM-historique`**, privé (remote `historique` du dépôt local). Les numéros de PR de ce fichier renvoient au dépôt public, sauf dans la section « Avant le dépôt public ».
- **Clé de publication** : `~/Outil/cle-publication-cof-dm.pem` (hors dépôt), aussi en secret `COF_DM_SIGNATURE` du dépôt. À sauvegarder ailleurs : sans elle, plus de mise à jour automatique (il faudrait changer la clé publique de `internal/majour`, et chacun réinstallerait à la main).
- **Versions publiées** : 1.2.0 (première publique) ; 1.2.2 (19 octobre 2026 : PR 3 à 6) ; 1.2.3 (21 octobre 2026 : PR 7 à 16, modèle 4) ; 1.2.4 (22 octobre 2026 : PR 17, 19 à 22, modèle 5).
- **Version affichée** : en petit à côté de « Thème » (une version d'essai en pastille ; « dev <commit> » pour une compilation de développement), lue dans `GET /api/sante` ; sur le bureau, elle mène à Paramètres → Mises à jour.

## Pour reprendre sur un autre poste

1. **Cloner le dépôt** `COF_DM`.
2. **Outils** : Go 1.26.9 au moins (fixé par `go.mod`) et un compilateur C pour SQLite (CGO). Commandes dans `CLAUDE.md`.
3. **Copier à la main les fichiers hors dépôt** (ci-dessous) : ils ne sont dans aucun dépôt, volontairement.
4. Vérifier que tout passe : `gofmt -l .`, `go vet -tags "sqlite_fts5 nogui" ./...`, `go test -tags "sqlite_fts5 nogui" ./...`.

### Fichiers hors dépôt, à emporter

Sur le poste WSL, dans `~/Outil/`, à côté du dépôt, avec le PDF du livre de base pour vérifier les tables. Sans le fichier de règles, le créateur de monstre, les repos et les listes de choix (peuples, profils, tailles, catégories) ne sont pas proposés.

| Fichier | Contenu |
|---|---|
| `Règles COF.json` | Le fichier de règles (`cof-dm-regles`) : listes, créateur de monstre, feuille de personnage et repos, difficulté des rencontres (`rencontre`), valeurs et états du combat (`combat`). À importer dans Paramètres → Bestiaire et règles. |
| `Bestiaire officiel COF.json`, `Profils et voies COF.json` | Le bestiaire officiel et les profils et voies, à importer dans chaque univers. Aussi dans le vault du MJ. |
| `exemples_monstres.json`, `exemples_personnages.json` | Fiches d'exemple de tracker_COF, retirées du dépôt. |
| `cle-publication-cof-dm.pem` | La clé de publication des mises à jour (voir plus haut). |

## Décisions prises (à respecter)

### Aucune donnée des livres dans le dépôt

Exigence ferme, pour que le logiciel se distribue sans problème. Ni liste, ni table chiffrée, ni nom de capacité ou de voie, ni texte, ni numéro de page — dans le code, l'interface, la documentation **et les tests** (qui utilisent des règles et des fiches inventées). Ces données viennent du **fichier de règles** et de la bibliothèque (profils, peuples, voies) importés ou saisis par le MJ et rangés dans l'univers. Le logiciel reste pensé pour COF : la structure des fiches et les formules restent du code. Le nom « COF DM » est gardé. Règle détaillée dans `CLAUDE.md`.

### Cartes : hors du projet (22 octobre 2026)

Les cartes (plans, marqueurs, carte montrée aux joueurs) sont retirées du projet : le MJ utilise un logiciel à part. Les images d'une fiche (portraits, illustrations) restent.

### Mode serveur et comptes

- Derrière un reverse proxy nginx qui porte le HTTPS ; seulement dans l'édition serveur (image Docker). Rien n'est accordé aux requêtes venues de la machine.
- Comptes (nom d'utilisateur, mot de passe), jetons d'API par compte. **Plusieurs univers ouverts à la fois** ; chacun choisit le sien, onglet par onglet.
- **Rôles** : administrateur pour tout le serveur ; MJ, auditeur et joueur **univers par univers**.
  - Auditeur : lit tout, secrets compris, ne modifie rien.
  - Joueur (21 octobre 2026) : n'a que sa page de joueur (plus de vue filtrée de l'application, ni par l'API) ; un compte à plusieurs rôles passe d'un univers à l'autre depuis sa page ou l'application. Lecture seule, sauf si le MJ de l'univers ouvre l'**édition par les joueurs** (Paramètres, ou options de l'écran du MJ ; réglage synchronisé) : toute la feuille de son PJ sauf les notes du MJ, et l'apparence, l'historique et les objectifs. Ajouter une voie de la bibliothèque reste au MJ.
- L'application de bureau reste sans compte.

### Bureau et serveur (18 octobre 2026)

- **Le bureau n'est qu'un éditeur** : le mode serveur n'existe que dans l'image Docker (étiquette de compilation `serveur`) ; le bureau refuse `-serveur`.
- **Deux modes de travail** du bureau : **local** (client seul) ou **serveur** (relié, connecté ou hors connexion). Une zone en bas à gauche le dit ; un clic ouvre le panneau (se connecter, changer de serveur, client seul, copier, envoyer, synchroniser).
- **Réseau local seulement en mode local** : tablette du MJ et page des joueurs à la maison. Relié, le bureau ne répond plus au réseau.
- **Authentification** : nom et mot de passe, échangés aussitôt contre un jeton propre au poste, retiré quand le poste quitte le serveur. Jamais de mot de passe gardé.
- **Quitter un serveur** : les copies restent, **en pause** ; « Détacher » en fait des univers locaux.

### Bureau et serveur de versions différentes (20 octobre 2026)

- Ce qui décide d'un échange : **protocole** et **modèle**, pas la version de l'application.
- **Serveur plus récent** : échange bloqué, la copie reste utilisable ; tout reprend après la mise à jour du bureau.
- **Bureau plus récent** : **réception seule** ; ses changements attendent la mise à jour du serveur.
- Le bureau propose de se mettre à jour ; l'administrateur voit la version de chaque poste relié (Paramètres → Version du serveur). Le serveur Docker annonce les versions publiées sans rien installer.
- Un univers ouvert par un modèle plus récent ne s'ouvre plus dans une version plus ancienne (même refus pour un export).
- Ce que l'autre côté refuse ou ne sait pas appliquer est gardé et repasse plus tard.

### Synchronisation

- Fusion **champ par champ** ; si le même champ a changé des deux côtés, la valeur du serveur reste partout et la locale attend dans « Conflits ».
- Archivage et suppression se propagent, sauf si la fiche a été modifiée de l'autre côté (gardée et signalée).
- En continu (toutes les 30 secondes), pour l'administrateur et le MJ. Tout voyage (fiches, relations, images, feuilles, bestiaire, règles, bibliothèque, calendrier, fonctionnalités, édition par les joueurs, journal, combats terminés), **sauf un combat en cours**.
- Chaque copie tire ses identifiants dans sa propre plage (`internal/store/identifiants.go`) ; la plage 8191 reçoit les profils, peuples et voies repris de l'ancien réglage `reference`.

### Fonctionnalités optionnelles (19 octobre 2026)

- Le cœur reste toujours là : PNJ, lieux, factions, objets, lore, événements, campagnes, quêtes, séances, PJ, bestiaire.
- **Coupés par défaut** : dés virtuels, aides de jeu, feuilles de personnage, page des joueurs. **Activés par défaut** : combat, rencontres, secrets, écran du MJ, préparation et après-séance, fiches de règles, graphe, chronologie, organigrammes, export Obsidian.
- Réglage **rangé dans l'univers** (synchronisé) ; une fonctionnalité coupée est **cachée et fermée** (API et MCP compris), **sans rien effacer** ; un univers d'avant garde ce dont il se sert.

### Voies, capacités, profils et peuples (20 octobre 2026)

- Le code ne garde qu'un **modèle vide** ; le MJ saisit (page « Voies et profils », menu Règles) ou importe tout le contenu, rangé **un par un** dans l'univers. Étiquette **officielle / maison** pour trier, rien de verrouillé.
- **Bonus permanents** d'une capacité (PV max, PM max, DEF, initiative, attaque, RD, caractéristique ; fixe, × rang, + niveau, + modificateur) ajoutés à la feuille quand le PJ a le rang, affichés **« A + b »** (b en vert, détail au survol). Les « divers » saisis ne sont jamais modifiés d'office ; un avertissement les signale.
- **En combat**, seuls les PJ ont des capacités : elles préremplissent l'action, le MJ valide.
- **Utilisations** : « x fois par » combat, jour, repos ou à la main ; « Nouveau jour » sur la feuille et l'écran du MJ ; un repos des règles peut valoir un nouveau jour.
- **Joueurs** : l'arbre de leurs voies en lecture (« Mes voies ») ; feuille déverrouillée, ils montent de rang ou écrivent une voie libre ; ajouter une voie de la bibliothèque reste au MJ.
- **Voie retirée** de la bibliothèque : les feuilles en gardent une copie (voie libre). Peuples : modificateurs affichés, jamais ajoutés d'office.
- **Claude** : lire, créer et modifier voies, profils et peuples ; aucun outil pour les retirer.

### Montée de niveau (22 octobre 2026)

- **Le MJ la lance** (depuis la feuille du PJ et l'onglet Joueurs de l'écran du MJ), **le joueur fait ses choix** (précisé le 22 octobre 2026) : le MJ accorde le niveau, et un **assistant s'ouvre seul** sur la page du joueur (et à chaque visite tant qu'il n'est pas pris ; « Plus tard » le referme pour la visite), même feuille verrouillée, une seule fois. Le MJ peut aussi faire la montée lui-même (joueur absent), avec le même assistant, ou retirer l'accord.
- **Assistant, fenêtre par fenêtre** : dés de vie → rangs de voie → PM du profil (s'il en gagne) → étapes en plus prévues par les règles à ce niveau → récapitulatif à valider. Le niveau reste lu sur la fiche ; le saisir à la main sur la fiche reste permis, sans historique.
- **Un niveau à la fois.** La seule baisse : « Annuler la dernière montée », qui défait exactement ce qu'elle avait fait (niveau, PV, rangs).
- **PV gagnés** : une formule du fichier de règles (dé de vie, mod. CON, valeur fixe…) ; **le dé de vie est lancé en vrai et saisi** (MJ comme joueur), borné par le dé de la feuille, le reste s'ajoute seul. **Rien n'est annoncé à la table** : le détail n'est que dans l'historique. Sans règles : PV saisis.
- **Règles officielles** (lues dans le livre de base le 22 octobre 2026, rangées dans `Règles COF.json`, hors dépôt ; copie d'avant : `Règles COF.avant-montee.json`) : dé de vie + mod. CON, 2 points (rangs 1-2 : 1 pt, 3-5 : 2 pts), pas de rang au-dessus du niveau, voie de prestige à 2 pts par rang, report des points, PM +1 ou +2 selon le profil, capacité épique (option) aux niveaux pairs dès 12. Écartés par le MJ : valeur moyenne du dé.
- **Rangs** : des points par niveau, le coût de chaque rang et le niveau minimal d'un rang, décrits par le fichier de règles, répartis dans les voies **déjà sur la feuille** (ajouter une voie de la bibliothèque se fait avant, en édition). Sans règles : rangs libres, sans contrôle. Les capacités débloquées sont rappelées.
- **Historique** rangé dans la feuille (synchronisé) : une ligne par niveau (date, PV gagnés et le jet, rangs pris).

### Inventaire (28 octobre 2026)

- **Une ligne par objet** sur la feuille : nom, quantité, porté ou non, emplacement, notes ; les objets courants restent des lignes libres. Le texte « Équipement », l'armure et le bouclier des anciennes feuilles sont **repris à l'ouverture** (une ligne par ligne de texte ; armure et bouclier portés, avec leur bonus de DEF).
- **Emplacements** : une liste générique dans le code (tête, cou, épaules, corps, mains, taille, pieds, deux anneaux, main principale, main secondaire, deux mains qui prend les deux), que le fichier de règles peut remplacer (`personnage.emplacements`). **Un seul objet porté par emplacement** (ou son nombre de places) : porter un second casque est refusé.
- **Chiffres sur l'objet**, saisis par le MJ : des bonus (mêmes valeurs que les capacités : DEF, Init, RD, attaques, caractéristiques, PV, PM) et une arme (type, DM, bonus, portée). Ils ne comptent **que portés** ; une arme portée ajoute sa ligne d'attaque.
- **Affichage** : une valeur changée par un objet a sa part **en bleu** (les capacités restent en vert), et **la formule au survol** : « DEF 12 = base 8 + 4 Casque du guerrier ».
- **Joueurs** (édition ouverte) : ils cochent « porté », comptent leurs objets (−/+), ajoutent ou retirent des objets courants ; les chiffres d'un objet et son emplacement restent au MJ.
- **Objets du monde** (lot 2) : une ligne peut désigner une fiche « Objet », dont elle prend le nom et les chiffres (champ « Chiffres » de la fiche : emplacement, bonus, arme). Le **Détenteur suit dans les deux sens** : donné à un PJ, l'objet entre dans son inventaire (et quitte celui d'un autre PJ) ; donné ailleurs ou ôté de l'inventaire, il n'y est plus / n'a plus de détenteur. Fiche effacée : la ligne reste, en objet courant. Lier ou retirer un objet du monde reste au MJ. Rien ne suit quand les feuilles de personnage sont coupées.

### Butin et pot commun (31 octobre 2026, projet 14, avant le code)

- **Butin de la rencontre en lignes chiffrées** : pièces (po / pa / pc) et objets (fiche « Objet » ou objet courant, quantité). Le texte libre « Butin » reste pour la description.
- **Pot commun par campagne** : il reçoit le butin d'un combat (versé par le MJ au bilan) et ce que le MJ y ajoute (récompense de quête, trésor trouvé hors combat). Les joueurs n'y déposent rien.
- **Partage par les joueurs, validé par le MJ** : depuis leur page, les joueurs proposent un partage (pièces, objets) ; le MJ valide ou refuse ; validé, les pièces vont dans la bourse et les objets dans l'inventaire. C'est compatible avec « les joueurs n'agissent jamais sur un combat » : ils ne touchent qu'au pot, après le combat.
- **Partage annulable par le MJ** : pièces et objets reviennent au pot, tant que le PJ les a encore.
- **Trace** : le champ « Butin & récompenses » de la séance se remplit, le journal note ce que chaque PJ reçoit, et la rencontre passe à « jouée », avec son butin marqué versé (il ne se verse pas deux fois).

### Schéma des fiches (31 octobre 2026)

- **Inventaire des champs** fait type par type ; corrigé en une PR (28) : cinq incohérences, puis les champs utiles en jeu.
- **Incohérences** : tactique d'une rencontre réservée au MJ (et notes du MJ) ; attitude du PNJ envers les PJ ; campagnes sur lieux, factions, objets, lore ; dirigeant d'un lieu = PNJ, PJ ou faction, d'une faction = PNJ ou PJ ; « Connu du MJ seulement » du lore **supprimé** (doublait le partage ; cochée sur une fiche partagée, la fiche n'est plus partagée à l'ouverture, `versionDonnees` 6).
- **Pas de champ « contrôlé par » sur le lieu** : le dirigeant peut être une faction. **Pas d'XP** sur la séance (COF n'en a pas). La récompense d'une quête est un texte long.
- **Laissé pour plus tard** : résumé des joueurs sur la séance (projet 30) ; résumé en une ligne du lore, de l'événement, de la campagne et de l'aide ; l'après-séance ne remplit pas encore les quêtes, lieux et PNJ de la séance.

### Aides de jeu (19 octobre 2026)

- Un **type de fiche** ; l'envoi est une relation `recu` d'un PJ vers l'aide, avec la séance. Le joueur voit **la version à jour** ; le MJ peut **retirer** un envoi.
- Indice d'un secret : l'envoi passe un secret **caché à soupçonné** ; la révélation reste un geste du MJ.
- Une aide n'apparaît **que dans les documents** du joueur, jamais comme fiche partagée. À la maison, table de confiance (qui choisit un PJ lit ses documents) ; sur un serveur, chaque compte ne voit que ceux de son PJ.

### Page des joueurs et combat

- Un joueur voit du combat **l'ordre d'initiative et les états**, sans aucun chiffre ; **les joueurs n'agissent jamais sur un combat**.
- À la maison, la page s'ouvre par un **interrupteur séparé**, éteint par défaut, réglé sur l'ordinateur du MJ ; chacun y choisit son PJ. Sur un serveur, par compte, restreinte au PJ du compte.
- Sans fichier de règles, **combat minimal** (initiative, PV, attaque contre la DEF, dégâts, états libres). PV, points de chance, PM et utilisations des PJ sont écrits en direct sur leur feuille. Un PNJ tombé n'est **jamais marqué mort d'office** : le bilan le propose. Un combat est rattaché d'office à la séance de la campagne.

### Serveur MCP et dés

- `cof-dm -mcp` (stdio), qui joint l'application lancée par son API avec un jeton. Écritures **directes et journalisées** (auteur `claude`) ; pas d'effacement définitif. Branché d'un clic par Paramètres → Claude (bureau). ChatGPT : écarté (19 octobre 2026).
- Les jets des joueurs et les jets publics du MJ se voient de **toute la table** ; le MJ a en plus des jets cachés.

### Paramètres (21 octobre 2026)

- Mêmes réglages, en quatre groupes : **Univers** (fonctionnalités, bestiaire et règles, calendrier), **Sauvegardes et exports**, **Joueurs et accès** (tablette et réseau, mon compte, comptes du serveur), **Application** (mises à jour, version du serveur, Claude, API).
- Titres seuls, description au survol ; un clic étend le bloc sur place, **un seul ouvert à la fois** ; le bloc ouvert se retient et a son lien direct `#/parametres/<bloc>`.

## Ce qui a été fait

### Dépôt public

| PR | Contenu |
|---|---|
| 1, 2 | Suivi de la publication ; version affichée à côté de « Thème ». |
| 3 | **Port du serveur MCP** : l'application publie le port qu'elle a réellement pris (`port.json`), que `-mcp` lit avant `config.json`. |
| 4 | **Aides de jeu** (projet 9, lot 1) : type `aide`, relation réservée `recu`, envoi depuis la fiche et l'écran du MJ, onglet « Documents » de la page des joueurs, indices dans « qui sait quoi », outils MCP, tests de non-fuite. |
| 5 | **Fonctionnalités optionnelles**, réglées par univers (Paramètres → Fonctionnalités) ; coupées : routes en `404`, types et écrans masqués, outils MCP retirés, rien d'effacé. |
| 6 | **Paramètres → Claude** : brancher COF DM dans Claude Desktop et Claude Code d'un clic. |
| 7 à 9 | **Versions** : numéros de protocole et de modèle ; échange selon les numéros (serveur plus récent : bloqué ; plus ancien : réception seule) ; deux pertes de données corrigées ; écrans (zone en bas à gauche, Paramètres → Version du serveur). |
| 10 à 15 | **Voies et capacités** (projet 10) : rangement un par un (modèle 2) ; grille des voies sur la feuille ; éditeur « Voies et profils » ; bonus « A + b » ; capacités en combat et utilisations, « Nouveau jour » (modèle 3) ; « Mes voies » sur la page des joueurs. |
| 16 | **Comptes joueurs** : un compte joueur n'a que sa page (récit du PJ, date en jeu, onglet Compte, choix d'univers) ; case « Édition par les joueurs » dans les options de l'écran du MJ, réglée par le MJ, synchronisée (modèle 4). |
| 17 | **Paramètres en accordéon** : quatre groupes, titres seuls, un bloc ouvert à la fois, lien direct par bloc. |
| 19 | **Montée de niveau, serveur** (projet 12, lot 1) : partie `personnage.montee` des règles ; `POST /api/feuilles/{id}/niveau` et `…/niveau/annuler` (MJ) ; niveau de la fiche et feuille en une écriture ; PV lancés au lanceur de la table ; historique `niveaux` et `points_capacite` dans la feuille ; outils MCP ; modèle 5. |
| 20 | **Montée de niveau, écrans** (lot 2) : dialogue « ⬆ Niveau » (feuille, onglet Joueurs de l'écran du MJ), historique et annulation sur la feuille, historique en lecture sur la page des joueurs. |
| 21 | **Montée de niveau accordée** (lot 3) : le MJ accorde le niveau (`…/niveau/accorder`, `niveau_accorde`), le joueur le prend depuis sa page (`POST /api/joueur/pj/{id}/niveau`) ; badge et retrait côté MJ ; outil MCP `accorder_niveau`. |
| 22 | **Montée de niveau en assistant** (lot 4) : fenêtres dés saisis → rangs → PM → étapes → récapitulatif ; ouverture automatique sur la page des joueurs ; règles `couts_origine`, `pm`, `etapes` ; plus de jet à la table. |
| 23 | `CLAUDE.md` réorganisé et resserré. |
| 24 | **Corrections rapides** : « ☀ Nouveau jour » et repos pendant un combat (les combats en cours reprennent les utilisations de la feuille) ; dés coupés depuis un autre appareil ; tiroir du menu en grand écran. |
| 25 | **Inventaire, lot 1** (projet 11) : inventaire sur la feuille, emplacements, bonus et armes des objets portés (en bleu, formule au survol), reprise de l'ancien équipement ; modèle 6, `versionDonnees` 5. |
| 26 | **Inventaire, lot 2** : lignes liées aux fiches « Objet » (nom et chiffres de la fiche), Détenteur suivi dans les deux sens, emplacements reconnus par leur nom, total affiché avec sa formule au survol. |
| 28 | **Schéma des fiches** (fusionnée sans essai) : 5 incohérences corrigées (tactique MJ, attitude du PNJ, campagnes, dirigeant, « Connu du MJ seulement » retiré) et champs utiles en jeu sur presque tous les types et le bestiaire ; modèle 7, `versionDonnees` 6. |
| 29 | **Butin, lot 1** (en essai) : butin chiffré des rencontres, pot commun par campagne (versement, ajouts, retraits, propositions validées par le MJ, partage direct, annulation), fonctionnalité `butin`, outils MCP ; modèle 8. |

### Avant le dépôt public (`COF_DM-historique`, PR 1 à 54)

| PR | Contenu |
|---|---|
| 1 à 5 | Fiches, liens, relations, recherche, graphe, chronologie, images, API, organigrammes, secrets et « qui sait quoi », lanceurs Windows et Linux. |
| 6 à 8, 30, 31, 33 | Audit de sécurité et corrections : réglages réservés à l'ordinateur du MJ, Go et SQLite à jour, limite d'essais, sessions révocables, restauration validée, actions GitHub épinglées, pilote SQLite défensif. |
| 9 à 13 | Mode serveur : comptes, rôles par univers, jetons, rôle joueur, plusieurs univers à la fois. |
| 14 à 22 | Fusion de tracker_COF : bestiaire, fichier de règles, feuille de personnage, rencontres et difficulté, moteur et écrans de combat, page des joueurs ; retrait du pont vers l'ancien tracker. |
| 23, 26 | Image Docker et cache de compilation. |
| 24, 25, 27, 28 | Retours d'usage : panneau des règles, bestiaire, graphe (liens élastiques). |
| 32 | Revue des écrans dans Chrome sans fenêtre. |
| 35, 36 | Dépôt d'un univers sur le serveur ; choix d'univers par onglet. |
| 37 à 41 | Synchronisation hors ligne : identifiants par plage, relevé champ par champ, protocole, bureau, écrans et conflits. |
| 42 | Export vers Obsidian. |
| 43 à 48 | P1 : serveur MCP, lanceur de dés, sorts et PM en combat, préparation de séance, après-séance, restes à vérifier. |
| 49 à 54 | Bureau et serveur : édition serveur, mode de travail du bureau, univers du serveur, port de la fenêtre, un jeton par poste, mises à jour signées du bureau. |

## Ce qu'il reste à faire

### Prochain chantier

**Projet 14, butin et récompenses de fin de combat** (`PROJETS.md`) : questions de fond posées le 31 octobre 2026, décisions dans « Butin et pot commun » plus haut. Il s'appuie sur l'inventaire (projet 11, fusionné le 29 octobre 2026 ; décisions dans « Inventaire » plus haut). Le fichier de règles du MJ (`Règles COF.json`, hors dépôt) porte sa partie `personnage.montee`, à réimporter dans chaque univers.

### Défauts connus

Aucun. Les trois notés jusqu'au 22 octobre 2026 sont corrigés (PR des corrections rapides) : « ☀ Nouveau jour » (ou un repos) pendant un combat n'est plus défait par l'action suivante ni par une annulation (les combats en cours adoptent les utilisations de la feuille : `recaler`) ; une page qui relit les dés s'arrête et retire le bouton quand ils sont coupés ailleurs ; le tiroir du menu ouvert ne casse plus la mise en page en grand écran (la classe `menu` de `#app` touchait tous les boutons par la règle `.menu button` : renommée `menu-ouvert`).

### À vérifier à la main (projet 6)

- **Exe Windows et AppImage** dans leur vraie fenêtre (WebView2, WebKitGTK) : seul Chrome a servi aux vérifications. L'exe tourne chez le MJ, relié à son serveur Docker.
- **Serveur derrière un vrai nginx** (HTTPS) : l'exemple du README n'a pas été essayé ; synchronisation sur une vraie coupure, et avec deux copies hors ligne en même temps.
- **Export vers Obsidian** ouvert dans le vrai vault (propriétés, tableaux, liens).
- **Serveur MCP** : essayé dans Claude Desktop sous Windows (19 octobre 2026) ; reste Claude Code.
- La reprise des anciens liens de tracker_COF sur un vrai univers ancien.

### Points en attente d'une décision

- **Objet donné à un PJ sans feuille** (lot 2 de l'inventaire) : la feuille est aujourd'hui créée d'office, vierge (PV 1), avec cette seule ligne (`mettreDansInventaire`). Le garder, ou attendre que la feuille existe ? Question posée au MJ le 29 octobre 2026 ; fusionné en l'état (création d'office) en attendant sa réponse.
- **Script d'import Obsidian** : en faire un vrai aller-retour (projet 31) ? Aujourd'hui, `vault_vers_cofdm.py` reste un import unique, taillé pour le vault d'origine.
