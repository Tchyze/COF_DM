# COF DM

Gestionnaire de monde et de campagnes pour **Chroniques Oubliées Fantasy** : PNJ, lieux, factions, objets, lore, quêtes, rencontres, séances, PJ, secrets et règles, reliés entre eux, avec recherche, graphe, chronologie, « qui sait quoi » et une API complète pour qu'un programme (ou Claude) puisse tenir le monde à jour.

Application de bureau Windows / Linux, données locales, sans compte ni service payant. Le bestiaire, les feuilles de personnage, les voies et capacités, les rencontres et le combat sont dans l'application. Pour jouer à plusieurs, une **édition serveur** (image Docker) héberge les univers avec un compte par personne ; chaque joueur suit la partie sur son téléphone, et l'application de bureau du MJ s'y synchronise, même hors ligne.

Le monde est **unique et persistant** : il n'y a pas d'état par campagne. Un PNJ tué dans une campagne reste mort partout ; chaque événement a des conséquences sur la suite.

## Ce que fait l'application

### Fiches

Quatorze types de fiches, chacun avec ses champs propres, ses images et ses tags :

| Type | Principaux champs |
|---|---|
| 👤 PNJ | en une phrase, alias, statut (vivant, mort, disparu, inconnu), peuple et profil (listes du fichier de règles), allégeance apparente (une faction), où le trouver, campagnes, apparence, personnalité, rôle ; combat : un monstre du bestiaire qui lui sert de profil ; MJ : secrets, notes |
| 🏰 Lieu | impression, catégorie, fait partie de (hiérarchie), dirigeant, description, lieux notables, rumeurs ; MJ : secrets |
| ⚑ Faction | but, catégorie, siège, dirigeant, attitude envers les PJ, description, ressources ; MJ : secrets |
| 🗝 Objet | catégorie, rareté, prix (pc / pa / po), magique, détenteur, effets, histoire ; MJ : secrets / malédiction |
| 📜 Lore | catégorie, connu du MJ seulement, contenu ; MJ : vérité |
| ⏳ Événement | date en jeu, lieu, campagne, acteurs, ce qu'on en raconte, conséquences ; MJ : vérité |
| 📖 Campagne | statut, pitch, thèmes, début et date actuelle en jeu, où est le groupe, arc principal ; MJ : fils rouges |
| ✦ Quête | campagne, catégorie (arc principal, mission, secondaire), ordre, statut, commanditaire, lieu, niveau conseillé, accroche, objectif, **étapes à cocher**, indices, récompense, conséquences ; MJ : secrets |
| ⚔ Rencontre | campagne, quête, séance, lieu, créatures (monstres du bestiaire, nombre, camp), PNJ présents, mise en scène, tactique, butin ; difficulté calculée |
| 🎲 Séance | campagne, numéro, statut, date réelle, date en jeu, PJ présents, résumé ; ses combats et leur bilan ; MJ : préparation, notes |
| 🛡 PJ | joueur, campagne, statut, peuple, profil, niveau, apparence, historique, objectifs ; sa feuille de personnage ; MJ : ce que le MJ sait |
| 🔒 Secret | statut (caché, soupçonné, en partie révélé, révélé), importance, catégorie, campagne, fiches concernées, vérité, indices, conséquences, séance de la révélation |
| ✉ Aide de jeu | forme (lettre, image, indice…), campagne, texte remis aux joueurs, images ; MJ : secret dont c'est l'indice, séance prévue, notes |
| ⚖ Règle | officielle ou maison, domaine, livre et page, règle remplacée, texte |

Les champs se valident à l'enregistrement (valeurs permises, fiches référencées du bon type, dates conformes au calendrier). Les sections réservées au MJ sont encadrées.

### Liens entre fiches

- **Champs de référence** : « Où le trouver », « Siège », « Commanditaire »… pointent vers une autre fiche.
- **Mentions** : dans tout texte, `[[Nom]]` (ou `[[Nom|texte affiché]]`) crée un lien. Taper `[[` propose les fiches. Un aperçu s'affiche au survol. **Renommer une fiche met à jour toutes ses mentions.**
- **Relations** typées et lues dans les deux sens : allié, ennemi, rival, famille, parent / enfant, mentor / élève, membre de / a pour membre, dirige / dirigé par, travaille pour, informateur, dette, craint, cherche, habite, fréquente, possède, garde, cache, trahit… avec un rang, une note, et la possibilité de la marquer secrète (connue du MJ seulement).
- **Organigramme des factions** : le chef en tête, ses seconds côte à côte avec leur équipe, les membres sans équipe regroupés dans un cadre ; rang, sous-factions, membres morts grisés, appartenances secrètes en pointillés. Chacun se place sous le supérieur réglé dans la faction, sinon sous le **Responsable** de sa fiche de PNJ (un **Responsable secret**, réservé au MJ, place un infiltré sous son vrai maître). Un PNJ sans faction qui répond à un membre y figure comme rattaché. On ajoute un membre depuis la faction, on change son supérieur en glissant sa carte sur une autre.
- Chaque fiche montre **ce qui la cite** (« PNJ — Où le trouver », « Quêtes — Campagne », « Mentionné dans »…). Une campagne liste ainsi ses quêtes, séances et PNJ ; un lieu, ses sous-lieux et les PNJ présents.
- **Liens morts** : les `[[Nom]]` sans fiche, avec création en un clic.
- Supprimer une fiche la retire proprement des autres.

### Secrets : qui sait quoi

- Un **secret** est une fiche : la vérité, les indices semés, ce qui arrive s'il éclate, et les fiches qu'il concerne.
- **Qui le sait** : PJ, PNJ ou faction, avec la séance où il l'a appris et comment. « Révéler aux PJ… » l'inscrit pour plusieurs PJ d'un coup et met le statut à jour (en partie révélé, révélé quand tous les PJ de la campagne savent).
- Page **Qui sait quoi** : un tableau secrets × PJ ; un clic sur une case indique qu'un PJ sait (ou ne sait plus).
- Les fiches de PNJ, PJ et factions montrent les secrets qu'elles connaissent et ceux qui les concernent.

### Aides de jeu

- Une **aide de jeu** est une fiche : une lettre, une image, un indice, avec le texte remis aux joueurs et ses images. Réservés au MJ : le secret dont c'est l'indice, la séance pour laquelle elle est prévue, ses notes.
- **Envoyer** : depuis la fiche de l'aide (« ✉ Envoyer… », à un, plusieurs ou tous les PJ), ou depuis l'onglet **Aides** de l'écran du MJ (un PJ touché la reçoit ; « Tous » l'envoie aux PJ de la table ; la séance de l'écran est notée). Elle arrive en direct dans l'onglet **Documents** de la page des joueurs ; ✕ la leur retire.
- **Indice** : envoyer l'indice d'un secret encore caché le passe à « soupçonné », et « qui sait quoi » montre ✉ pour les PJ qui l'ont reçu. Le révéler reste un geste du MJ.
- Les joueurs voient toujours la version à jour (une coquille se corrige après l'envoi), et jamais rien d'autre que le nom, la forme, le texte et les images.

### Écran du MJ

La page pour jouer (**🧭 Écran du MJ**, `#/table`) : tout tient dans la fenêtre, sans défilement de page ; chaque volet défile seul s'il déborde.

- **Joueurs** : les PJ de la campagne (sauf morts et partis), avec leurs PV, DEF, initiative, points de chance, de récupération et de mana lus sur leur feuille de personnage. Pendant un combat, les PV et les points de chance bougent en direct.
- **Scène** : les PNJ, lieux, factions, objets et secrets liés à la quête (commanditaire, lieu, relations, rencontres, fiches qui la concernent, `[[liens]]` de ses textes), avec leur statut et un cadenas s'ils ont des secrets.
- **Quête** en cours : ses informations, ses étapes (à cocher sur place) et ses textes, secrets du MJ compris ; un onglet **Séance** montre la préparation de la prochaine séance. S'il y a plusieurs quêtes en cours, on choisit en haut ; l'appareil retient le choix.
- **Combat** : l'écran de combat est logé dans ce volet. Chaque rencontre de la quête a son bouton **Lancer** (touché deux fois) : ses créatures, ses PNJ présents et les PJ en jeu entrent en combat. « Combat improvisé » en lance un avec les seuls PJ. Le combat en cours s'affiche de lui-même ; `◀` le referme, `⇔` lui donne plus de place, `↗` l'ouvre en pleine page.
- Toucher une fiche l'ouvre dans un **tiroir** (profil de combat d'un PNJ, relations, secrets) sans quitter l'écran ; la recherche (`/`) fait de même.
- **Chronomètre de séance et rappels** (« torche : 120 min ») : désactivés par défaut, à cocher dans ⚙, par appareil.
- Sur un écran étroit (téléphone, tablette tenue debout), un volet à la fois, avec trois onglets en bas.

L'écran n'est pas destiné aux joueurs : il montre les secrets du MJ.

### Sur une tablette

Par défaut, COF DM ne répond qu'à l'ordinateur sur lequel il tourne. Pour l'ouvrir sur une tablette (ou tout appareil du même réseau), dans **Paramètres → Tablette et réseau**, sur l'ordinateur du MJ (en mode local seulement : une application reliée à un serveur ne s'ouvre pas au réseau, la tablette passe alors par le serveur) :

1. cochez **Autoriser les appareils du réseau local**, puis fermez et relancez COF DM ;
2. créez le **mot de passe MJ** (8 caractères au moins) ;
3. scannez le QR code avec la tablette (ou tapez l'adresse affichée) et saisissez le mot de passe.

La session dure 30 jours sur l'appareil. Le mot de passe est gardé haché (PBKDF2-SHA256) ; il ne se définit, ne se change et ne se retire que sur l'ordinateur du MJ, et le changer déconnecte les appareils. Huit essais ratés en cinq minutes bloquent une adresse quelques minutes. Sans mot de passe, aucun navigateur du réseau n'entre. Le jeton d'API et ces réglages ne sont pas accessibles depuis la tablette.

Au premier lancement après avoir coché la case, Windows demande d'autoriser COF DM sur le réseau : acceptez pour les réseaux privés. Sous Linux avec un pare-feu : `sudo ufw allow 8100/tcp`.

Pour jouer à distance, préférez un VPN (Tailscale, WireGuard…) à une redirection de port sur la box : l'application n'a pas de HTTPS.

### Page des joueurs

Chaque joueur suit la partie sur son téléphone, à l'adresse `http://<adresse>:8100/joueur.html` :

- **Ma fiche** : la feuille de son personnage, sans les notes du MJ, et son apparence, son historique et ses objectifs ; la date en jeu de sa campagne s'affiche en haut de la page. Pendant un combat, ses PV et ses points de chance y bougent en direct. Elle est en lecture seule tant que le MJ n'a pas déverrouillé les feuilles ; déverrouillée, le joueur la tient lui-même (PV, ressources, repos, attaques, voies, équipement ; ajouter une voie de la bibliothèque reste au MJ).
- **Mes voies** : chaque voie de son personnage, rang par rang (texte, effets, utilisations), les rangs atteints en surbrillance.
- **Combat** : le round, à qui c'est le tour, qui joue ensuite, l'ordre d'initiative et les états de chacun. Aucun PV, aucune DEF, aucune attaque, aucune statistique ; les adversaires hors de combat disparaissent de la liste. Un bandeau rappelle le tour sur sa fiche et annonce « C'est à vous ! ».
- **Documents** : les aides de jeu que le MJ lui a envoyées (lettres, images, indices), avec une pastille pour celles qu'il n'a pas encore ouvertes ; une nouvelle s'annonce aussitôt.
- **Dés** : des tests tout faits lus sur sa feuille (caractéristiques, attaques et leurs dégâts), avec dé bonus ou malus, une formule libre, et les jets de toute la table, en direct.
- **Hauts faits** : le récapitulatif de chaque combat terminé et les records de chaque PJ (combats, victoires, adversaires abattus, critiques, meilleur coup, titres).
- **Compte** (sur un serveur) : changer son mot de passe, se déconnecter.

Les joueurs regardent le combat, ils n'y agissent pas. Tout se met à jour en temps réel.

Le MJ déverrouille les feuilles dans **Paramètres** (Tablette et réseau à la maison, Comptes du serveur sur un serveur) ou, en pleine partie, dans les **options de l'écran du MJ** (⚙, case « Édition par les joueurs »).

**À la maison**, la page est libre, sans mot de passe, pour les appareils du réseau local : dans **Paramètres → Tablette et réseau**, sur l'ordinateur du MJ, cochez **Ouvrir la page des joueurs au réseau local** (éteinte par défaut ; l'application doit écouter sur le réseau, donc travailler en mode local) et faites scanner son QR code. Chacun y choisit son personnage, gardé sur son appareil. N'importe qui sur ce réseau peut donc lire les feuilles et les documents des PJ ; rien d'autre de l'univers n'est accessible sans le mot de passe MJ. L'apparence, l'historique et les objectifs s'y lisent ; seul le MJ les écrit.

**Sur un serveur**, on y entre par son compte, et c'est tout ce qu'un compte joueur a de l'application : il y est renvoyé dès sa connexion. Il n'y trouve que le PJ rattaché à son compte (fiche partagée), ses documents, les combats où il se bat et leurs hauts faits, et l'onglet **Compte** (mot de passe, déconnexion). Feuilles déverrouillées, il écrit aussi l'apparence, l'historique et les objectifs de son PJ. Un compte qui a un rôle dans plusieurs univers en change par la liste en haut de la page : là où il est joueur, sa page ; ailleurs, l'application.

### En serveur, derrière nginx

Pour héberger COF DM sur une machine distante, utilisez l'**image Docker** (ci-dessous, « Avec Docker ») : le mode serveur n'existe que dans cette édition (compilée avec l'étiquette `serveur`) ; l'application de bureau n'est qu'un éditeur, qui se relie à un serveur ou travaille seule. Le serveur tourne sans fenêtre, n'écoute que sur `127.0.0.1` et attend un reverse proxy qui porte le HTTPS. Dans ce mode, plus de mot de passe MJ ni de jeton unique : chacun entre avec **son compte** (nom d'utilisateur et mot de passe), et rien n'est accordé d'office aux requêtes venues de la machine.

Au premier lancement, le compte `admin` est créé et son mot de passe s'affiche une seule fois dans le terminal : notez-le, puis changez-le dans **Paramètres → Mon compte**. L'administrateur crée les autres comptes dans **Paramètres → Comptes du serveur** et leur donne un rôle dans l'univers ouvert ; chacun y gère aussi ses jetons d'API. Les comptes sont rangés dans `comptes.json` (mots de passe hachés).

| Rôle | Droits |
|---|---|
| Administrateur | Tout le serveur : comptes, univers, réglages. |
| MJ (par univers) | Lit et modifie l'univers. |
| Auditeur (par univers) | Lit tout, secrets du MJ compris, sans rien modifier. |
| Joueur (par univers) | N'a que sa page de joueur (voir « Page des joueurs »). |

Rien n'est partagé avec les joueurs par défaut : sur chaque fiche, le MJ clique **👁 Partager avec les joueurs** (et **Retirer aux joueurs**). Le PJ d'un joueur doit être partagé pour qu'il le voie ; les fiches partagées forment aussi l'export vers Obsidian « joueurs ». Les champs du groupe « MJ », les relations secrètes, le tableau « qui sait quoi », les fiches « Secret », le journal et l'écran du MJ n'y entrent jamais.

Les joueurs ne modifient rien, sauf si le MJ coche **Édition par les joueurs** (Paramètres → Comptes du serveur, ou les options de l'écran du MJ) et que l'administrateur rattache chaque compte à son PJ : depuis sa page, le joueur tient alors toute la feuille de ce PJ (sauf les notes du MJ) et son apparence, son historique et ses objectifs. Ajouter une voie de la bibliothèque reste au MJ.

Le serveur héberge **plusieurs univers à la fois** : tous les fichiers `.cofdm` de son dossier `univers/` sont ouverts, et chaque compte a son rôle dans chacun (MJ de l'un, joueur de l'autre). L'administrateur en crée un par le menu **Fichier** ; un compte qui a un rôle dans plusieurs univers passe de l'un à l'autre par la liste en haut du menu. Pour ajouter un univers préparé sur votre ordinateur, l'administrateur choisit **Fichier → Déposer un univers…** : le fichier est contrôlé (univers intact, sans objet étranger, qui n'est pas la copie d'un univers déjà présent) puis rangé dans `univers/`, sans relancer le serveur. Déposer le fichier dans `univers/` à la main et relancer le serveur reste possible.

Exemple de configuration nginx :

```nginx
server {
    listen 443 ssl http2;
    server_name jdr.example.org;
    ssl_certificate     /etc/letsencrypt/live/jdr.example.org/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/jdr.example.org/privkey.pem;

    client_max_body_size 1g;            # dépôt d'un univers, ou restauration d'un export, avec ses images
    add_header Strict-Transport-Security "max-age=31536000" always;

    location / {
        proxy_pass http://127.0.0.1:8100;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 600s;
    }

    # suivi d'un combat en direct (WebSocket)
    location ~ ^/api/combats/[0-9]+/direct$ {
        proxy_pass http://127.0.0.1:8100;
        proxy_http_version 1.1;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        "upgrade";
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
    }
}
```

Les deux en-têtes `X-Forwarded-*` comptent : le premier donne à la limite d'essais l'adresse réelle de chaque visiteur, le second fait poser le cookie de session en `Secure`. N'ouvrez pas le port 8100 au réseau : seul nginx doit le joindre.

### Avec Docker

Le dépôt fournit un `Dockerfile` et un `docker-compose.yml` pour le mode serveur, derrière le nginx de la machine (Linux) :

```bash
mkdir -p donnees
docker compose up -d --build
docker compose logs cof-dm     # premier lancement : le mot de passe du compte « admin »
```

- Le conteneur partage le réseau de la machine : COF DM n'écoute que sur `127.0.0.1:8100`, exactement comme sans Docker, et la configuration nginx ci-dessus s'applique telle quelle. Aucun port n'est publié. C'est aussi ce qui permet à l'application de se fier aux en-têtes `X-Forwarded-*` de nginx : ne remplacez pas ce partage par un port publié.
- Les données sont dans `./donnees` : `comptes.json`, `reglages.json`, `univers/` (un fichier `.cofdm` par univers) et `sauvegardes/`. Pour ajouter un univers préparé sur votre ordinateur : **Fichier → Déposer un univers…** (administrateur), ou son fichier dans `donnees/univers/` puis `docker compose restart`.
- Le conteneur tourne avec l'utilisateur `1000:1000`. Si `./donnees` appartient à un autre utilisateur, indiquez-le : `COF_UID=$(id -u) COF_GID=$(id -g) docker compose up -d`. `COF_PORT` change le port (8100 par défaut).
- Mise à jour : `git pull && docker compose up -d --build`. Le serveur ne s'installe rien, mais il cherche les versions publiées et le dit à l'administrateur (Paramètres → Version du serveur), avec la version des applications de bureau reliées. Sa version vient de git (`1.2.3`, ou `1.2.3+4.1a2b3c4` quelques commits après). L'univers est sauvegardé et fermé proprement à l'arrêt du conteneur. La première construction prend une à deux minutes, dont près d'une minute sans rien afficher à l'étape `go build` (compilation de SQLite) : elle n'est pas bloquée. Les suivantes réutilisent ce travail et prennent quelques secondes ; `--no-cache` n'est pas utile.

### Mode de travail : local ou serveur

En bas à gauche de la fenêtre, une zone dit comment travaille l'application :

- **Local** (client seul) : vos univers restent sur cet ordinateur. C'est le seul mode où l'application peut s'ouvrir au réseau local (tablette du MJ, page des joueurs à la maison).
- **Connecté** : l'application est reliée à un serveur COF DM (l'image Docker). Ses univers y sont copiés et se synchronisent tout seuls toutes les 30 secondes. Les joueurs et les autres MJ continuent pendant ce temps sur le serveur.
- **Hors connexion** : reliée, mais le serveur ne répond pas. Travaillez normalement : vos changements partiront à son retour.

Relié à un serveur, un bouton **Univers du serveur** apparaît sous le bouton Fichier : il ouvre la liste des univers du serveur où vous avez un rôle (l'univers ouvert, ceux déjà copiés sur l'ordinateur, ceux qui ne sont que sur le serveur). Un clic passe à l'un d'eux : sa copie s'ouvre, ou elle est d'abord tirée la première fois (il faut en être MJ). Hors connexion, la liste montre les copies de l'ordinateur, qui s'ouvrent normalement.

Un clic sur la zone ouvre le panneau : passer de **Client seul** à **Serveur** et inversement, **changer de serveur**, copier un univers du serveur, envoyer celui-ci, synchroniser tout de suite, voir les conflits. **Fichier → Serveur et synchronisation…** ouvre le même panneau.

- **Se connecter** : l'adresse du serveur (HTTPS ; le HTTP n'est admis que sur l'ordinateur ou le réseau local), votre nom d'utilisateur et votre mot de passe. Le mot de passe ne sert qu'à cette connexion et n'est jamais gardé : le serveur donne à l'ordinateur son propre jeton, visible dans « Mon compte » sur le serveur, et retiré quand l'ordinateur quitte ce serveur (sinon, retirez-le là). Ensuite, choisissez l'univers à copier (il faut en être MJ) ou envoyez l'univers ouvert.
- **Envoyer cet univers sur le serveur** : publie l'univers ouvert (compte administrateur) et le garde synchronisé. L'univers tel qu'il était reste dans ses sauvegardes.
- **Client seul** ou **Changer de serveur** : les copies du serveur quitté restent sur l'ordinateur, **en pause**. Vous pouvez y travailler ; leurs changements partiront quand vous vous reconnecterez à ce serveur. La page **Synchronisation** permet aussi de **détacher** une copie, qui devient un univers local indépendant.
- Tant que l'application est reliée, elle ne répond plus au réseau local, même si la case est cochée : la tablette et les joueurs passent par le serveur.
- Les changements se fusionnent **champ par champ**. Si le même champ a changé des deux côtés, la valeur du serveur reste en place partout, et la vôtre attend dans la liste **Conflits**, où vous choisissez. Une fiche supprimée d'un côté et modifiée de l'autre est gardée et signalée.
- Tout voyage (fiches, relations, images, feuilles, bestiaire, règles, calendrier, journal, combats terminés), sauf un combat en cours, qui se joue d'un seul côté.
- **Versions différentes** : l'application et le serveur n'ont pas à avoir la même version, seulement le même **modèle** de données (un numéro qui change quand un type de fiche, un champ ou un réglage apparaît). Serveur plus récent : la synchronisation s'arrête et la zone en bas à gauche propose de **mettre l'application à jour** ; vos changements attendent sur l'ordinateur. Serveur plus ancien : l'application **reçoit seulement** ; vos changements attendent sa mise à jour, que l'administrateur voit dans Paramètres → Version du serveur. Rien n'est perdu, et tout repart dès que les deux côtés sont au même modèle. Un univers ouvert par une version plus récente ne s'ouvre plus dans une plus ancienne.

### Fonctionnalités

**Paramètres → Fonctionnalités** : chaque univers active ce dont il se sert. PNJ, lieux, factions, objets, lore, événements, campagnes, quêtes, séances, PJ et bestiaire sont toujours là ; le reste s'active ou se coupe d'un clic :

- **Partie** : écran du MJ, préparation et après-séance, combat, rencontres, secrets (activés par défaut) ;
- **Joueurs** : feuilles de personnage, page des joueurs, dés virtuels, aides de jeu (coupés par défaut) ;
- **Outils** : fiches de règles, graphe, chronologie, organigrammes, export vers Obsidian (activés par défaut).

Couper une fonctionnalité la retire du menu, des fiches, de l'écran du MJ, de la page des joueurs, de l'API et des outils de Claude, sans rien effacer : ses fiches et ses données restent dans l'univers et reviennent quand on la réactive. Sur un serveur, le réglage vaut pour tous les comptes de l'univers (le MJ le change ; l'auditeur et les joueurs non). Un univers créé avant cette version garde ce dont il se sert déjà (ses feuilles de personnage et la page des joueurs, ses aides de jeu).

### Mises à jour

L'application de bureau se met à jour seule, sur votre clic : **Paramètres → Mises à jour**, ou le badge **⬆** en bas à gauche quand une version attend. Elle cherche au lancement puis toutes les six heures.

- **Canal Stable** : les versions publiées (`vX.Y.Z`), pour jouer.
- **Canal Test** : en plus, les versions d'essai d'une modification en cours (`vX.Y.Z-test.N`), publiées pour l'essayer avant de la fusionner. Repasser en Stable propose la prochaine version stable.
- **Installer et redémarrer** : l'univers est sauvegardé et fermé, la nouvelle version téléchargée puis **vérifiée** (la liste des empreintes est signée par la clé de publication de COF DM ; un fichier qui ne correspond pas est refusé et rien ne change), le programme remplacé, et l'application redémarre.
- Se met à jour seul : l'exe Windows et l'AppImage. Une version compilée à la main se met à jour à la main. Le serveur Docker, lui, se met à jour par `docker compose up -d --build`.

Pour publier une version d'essai d'une branche : onglet **Actions → Release → Run workflow**, la branche, canal **test**.

### Bestiaire et règles

Le **bestiaire** de l'univers (menu Combat) rassemble les monstres officiels et vos créations : filtres par source, catégorie, taille et NC, recherche dans les capacités, fiches officielles verrouillées qu'on duplique pour les adapter, et **créateur de monstre** (taille, archétype, rangs de boss, voies de créature).

**Le logiciel ne contient aucune donnée des livres** : ni liste de peuples ou de profils, ni table, ni texte. Vous importez vos propres fichiers, qui restent chez vous :

| Fichier | Où l'importer | Ce qu'il apporte |
|---|---|---|
| Fichier de règles (`cof-dm-regles`) | Paramètres → Bestiaire et règles | Les choix proposés dans les fiches (peuples, profils, tailles, catégories) et les tables du créateur de monstre. |
| Bestiaire (tableau de monstres) | Bestiaire → Importer des monstres | Les fiches officielles, verrouillées. |
| Profils et voies | Paramètres → Bestiaire et règles | Profils, peuples et voies, avec leurs capacités. |

Sans fichier de règles, tout fonctionne en saisie libre, et le créateur de monstre n'est pas proposé. Tout est rangé dans le fichier d'univers ; pour équiper un nouvel univers, reprenez la bibliothèque d'un autre depuis le même panneau. Le format du fichier de règles est décrit dans [docs/API.md](docs/API.md).

### Voies, capacités, profils et peuples

La page **Voies et profils** (menu, partie Règles) est la bibliothèque de l'univers : vous y saisissez ou importez vos voies (de 5 à 20 rangs selon vos règles), leurs capacités, vos profils et vos peuples, rangés un par un et étiquetés **officiel** ou **maison** pour les trier. Chaque capacité porte son texte et, si vous le décrivez :

- ses **bonus permanents** (PV max, PM max, DEF, initiative, attaque, RD, caractéristique ; une valeur fixe, éventuellement × rang atteint, + niveau, + modificateur), ajoutés d'office à la feuille quand le PJ a le rang ;
- son **action en combat** (attaque, sort, soin, état, coût en PM) ;
- ses **utilisations** (x fois par combat, par jour, par repos, ou à remettre à la main).

Renommer une voie est suivi par les feuilles et les profils ; retirer une voie en laisse une copie sur les feuilles qui s'en servent. Le dépôt ne contient aucune voie : elles viennent de vous.

### Feuille de personnage

La fiche d'un PJ porte sa **feuille de personnage** : caractéristiques et modificateurs, PV, DEF, initiative, points de chance, de récupération et de mana, attaques, voies et rangs, équipement, bourse, notes du MJ. Rien n'est saisi deux fois : le nom, le peuple, le profil et le niveau restent ceux de la fiche. Les modificateurs, la DEF, l'initiative et les bonus d'attaque sont calculés ; les PV et les ressources se règlent d'un clic, et les boutons de repos appliquent ce que décrit votre fichier de règles. « Appliquer le profil » ajoute les voies du profil à la feuille.

- **Voies en grille**, comme sur la feuille papier : une colonne par voie, une ligne par rang, les rangs acquis en surbrillance ; une bulle au survol ou au toucher donne le texte et les effets de la capacité.
- **« A + b »** : les bonus des capacités s'ajoutent aux valeurs saisies (A), affichés en vert (b), avec leur détail au survol. Les « divers » saisis à la main ne sont jamais modifiés d'office ; un avertissement les signale.
- **Utilisations** : ce qu'il reste de chaque capacité limitée ; ＋1 et ↺ sur la feuille, **☀ Nouveau jour** (feuille ou écran du MJ) pour les capacités « par jour », et les repos de vos règles.

Sur un serveur, un joueur voit la feuille de son PJ (sans les notes du MJ) sur sa page de joueur, et la tient lui-même quand le MJ a ouvert l'édition par les joueurs.

### Rencontres et profil de combat des PNJ

- **Rencontre** : ses créatures sont des monstres du bestiaire de l'univers (nombre, camp), auxquels s'ajoutent les PNJ présents qui ont un profil de combat. Sa fiche affiche chaque créature (NC, DEF, PV) et **la difficulté**, calculée avec le fichier de règles et les PJ de la campagne : niveau de la rencontre, verdict, alertes, détail du calcul, et l'effet d'une condition (terrain, obscurité…). Les alliés ne comptent pas. Sans fichier de règles, les créatures s'affichent sans verdict.
- **PNJ** : on lui choisit un monstre du bestiaire comme profil de combat ; le bloc (NC, DEF, PV, caractéristiques, attaques) s'affiche sur sa fiche et dans le tiroir de l'écran du MJ.
- Un monstre qui sert à une rencontre ou à un PNJ ne se supprime pas tant qu'il y est.

### Combat

**Combats** (menu) liste les combats de l'univers, en cours ou terminés, et en lance un : une rencontre préparée, ou un combat improvisé. On lance aussi une rencontre depuis sa fiche ou depuis l'écran du MJ.

L'écran de combat tient dans la fenêtre, sans défilement de page :

- à gauche, l'**ordre d'initiative**, une ligne par combattant (PV, DEF, états, case de PV rapide). S'il y a plus de combattants que de place, la liste garde ceux qui sont debout, à partir de celui qui joue, et **tout voir** ouvre la liste complète ;
- au centre, le **combattant actif** (ses actions, ses postures défensives) et la **résolution d'une attaque**. Le résultat prend la place du formulaire : on y applique les dégâts, puis « Nouvelle attaque » ;
- à droite, sur un grand écran, les dernières lignes du **journal**.

Tout le reste s'ouvre dans une fenêtre secondaire : la fiche d'un combattant (en touchant son nom), et par le **☰ Menu** : ajouter au combat (monstre du bestiaire, PJ, PNJ du monde, saisie libre), test de caractéristique, appliquer un état, journal, rappel des règles, options, terminer. Dans un volet étroit (téléphone, écran du MJ), l'ordre et l'action ont chacun leur onglet. Raccourcis : **N** fin du tour, **Z** annuler, **Échap** ferme une fenêtre.

**Dés réels, dés virtuels.** Par défaut, tous les dés sont lancés à la table : on saisit le dé d'attaque, puis, si l'attaque touche, le total des dés de dégâts ; l'application applique bonus, états, critique et RD. **Menu → Options** laisse l'application lancer les dés, pour les adversaires et les alliés seulement ou pour tous. Le réglage est propre à chaque appareil.

**Règles.** Le dé, le critique, l'échec automatique, le point de chance, le budget d'actions d'un tour et les états (avec leurs effets) viennent du fichier de règles de l'univers. Sans lui, le combat reste jouable en version minimale : initiative, PV, attaque contre la DEF, dégâts, états saisis librement et sans effet.

**PJ et PNJ.** Un PJ entre en combat avec sa feuille de personnage ; ses PV, ses points de chance et ses points de mana y sont reportés à chaque action. Un PNJ du monde entre avec son profil de combat et garde son nom.

**Sorts et capacités.** Un combattant qui a des points de mana les montre sur sa ligne ; un sort se lance comme une attaque, avec son **coût en PM**, payé qu'il touche ou non (refusé s'il n'en reste pas assez ; « Annuler » rend tout). Un PJ entre en combat avec les capacités actives de ses voies : une capacité à jet d'attaque prépare l'attaque (PM, bonus, dégâts), les autres ouvrent une fenêtre préremplie (PM, dégâts ou soin, état) que le MJ valide ; chaque utilisation est comptée, « par combat » dans le combat, « par jour » ou « par repos » sur la feuille.

**Fin de combat.** Quand un camp est à terre, l'écran propose de terminer : le récapitulatif (héros du combat, moments forts, bilan par PJ, courbe des PV du groupe) est gardé avec le combat et se copie en Markdown pour une note Obsidian. Le combat est rattaché à la séance de la campagne ; sa fiche liste ses combats, leur récapitulatif, et propose **Marquer mort** pour chaque PNJ du monde tombé. Rien n'est changé d'office sur la fiche d'un PNJ.

Plusieurs appareils suivent le même combat en direct. Sur un serveur, le MJ joue, un auditeur regarde ; les joueurs suivent l'ordre et les états sur leur page.

À la première ouverture d'un univers créé avec une version plus ancienne, les liens vers tracker_COF sont repris : les créatures des rencontres sont rattachées par leur nom aux monstres du bestiaire de l'univers ; celles qui ne s'y retrouvent pas et les profils de combat des PNJ sont retirés (notés au journal et, pour un PNJ, dans ses notes du MJ), à choisir de nouveau dans le bestiaire.

### Règles

Fiches de règles officielles (résumés) et maison, par domaine. **Paramètres → Sauvegardes, exports et import** ajoute des fiches depuis un fichier JSON sans rien remplacer, par exemple les aide-mémoire du vault.

### Outils

- **Préparer une séance** (bouton **🧭 Préparer** d'une séance, ou de l'onglet Séance de l'écran du MJ) : ce que sa campagne a en cours, rassemblé sur une page : la séance d'avant et son résumé, où est le groupe, les PJ, les fils rouges, les quêtes en cours et leurs étapes restantes, les secrets à semer (indices, PJ qui ne savent pas encore), les PNJ à avoir sous la main (ceux des quêtes et ceux du lieu où est le groupe), les rencontres prêtes. On coche ce qu'on garde, on ajoute ses notes, et le tout s'ajoute à la préparation de la séance, avec des liens vers les fiches. La préparation est réservée au MJ : elle nomme des secrets.
- **Après la séance** (bouton **✅ Après la séance**, ou « Après » dans l'onglet Séance de l'écran du MJ) : résumé, PJ présents, date en jeu et lieu du groupe (reportés sur la campagne), étapes faites et statut des quêtes, secrets appris et par qui, PNJ tombés dans les combats de la séance à noter morts, rencontres jouées ou évitées, et la séance suivante créée d'office. Tout s'enregistre d'un coup ; une valeur refusée n'en laisse passer aucune. On arrive ensuite sur la préparation de la séance suivante.
- **Dés** (bouton 🎲 en bas à droite, ou touche **D**, sur toutes les pages) : une formule (`1d20+3`, `2d6`, `2d20kh1` pour garder le meilleur de deux dés), des dés rapides, un libellé. Les jets du MJ et des joueurs se voient de toute la table, sur la page des joueurs ; un **jet caché** reste sur l'écran du MJ. Un jet de joueur arrivé panneau fermé s'annonce par un badge. Les 100 derniers jets sont gardés tant que l'application tourne, hors de l'univers.
- **Écran du MJ** : voir plus haut.
- **Tableau de bord** : le monde en chiffres, campagnes, quêtes en cours, dernières séances, dernières modifications.
- **Recherche** (`/` ou `Ctrl+K`) : sans accents, tolérante aux fautes de frappe et aux lettres inversées, sur tous les textes, `"phrase exacte"`, `#tag`.
- **Graphe** : les fiches et leurs liens autour d'une fiche (1 à 3 niveaux) ou du monde entier.
- **Chronologie** : toutes les dates en jeu, par année, filtrables par campagne.
- **Calendrier du monde** paramétrable : mois et durées, jours de la semaine, ère. Par défaut : douze mois de trente jours (noms républicains), une décade et cinq jours complémentaires.
- **Images** : plusieurs par fiche (portraits, illustrations), image principale, légendes, glisser-déposer.
- **Archivage** (masquer sans supprimer), **journal** de toutes les modifications avec leur auteur (MJ ou client de l'API).
- **Un univers = un fichier** `.cofdm` (fiches, images, journal) : menu **Fichier** pour créer un univers, en ouvrir un autre, retrouver les récents, enregistrer une copie.
- **Sauvegardes automatiques** de l'univers (à l'ouverture, toutes les demi-heures, à la fermeture), restauration en un clic ; **export JSON** complet, images comprises ; **export vers Obsidian** (voir plus bas).
- **Paramètres**, en quatre groupes (Univers ; Sauvegardes et exports ; Joueurs et accès ; Application) : seuls les titres s'affichent, avec leur description au survol, et un clic étend le réglage sur place.
- Thème sombre et thème clair ; interface utilisable sur tablette et sur téléphone.

### API

Tout ce que fait l'interface passe par une API REST documentée dans [docs/API.md](docs/API.md) : création et modification partielle des fiches, relations, images, recherche, graphe, chronologie, calendrier, export. `GET /api/schema` décrit tous les types et champs. Un client s'authentifie avec le jeton affiché dans **Paramètres → API** ; ses modifications sont signées dans le journal.

### Avec Claude (MCP)

`cof-dm -mcp` est un serveur [MCP](https://modelcontextprotocol.io) : Claude (Claude Desktop, Claude Code ou tout client MCP) lit et tient à jour l'univers ouvert dans l'application. Il passe par l'API, avec ses droits et sa validation ; ses écritures sont signées `claude` dans le journal.

- **Lire** : schéma, recherche, fiche complète (par identifiant ou par nom), fiches liées, chronologie, qui sait quoi, organigramme, partie en cours (écran du MJ), feuille de personnage, bestiaire, voies, profils et peuples, difficulté d'une rencontre, combats, jets de dés, préparation d'une séance, aides de jeu, journal, liens morts.
- **Écrire** : lancer des dés (cachés par défaut), clore une séance, envoyer une aide de jeu aux PJ, créer et modifier une fiche, l'archiver ou la restaurer, la partager avec les joueurs, créer, modifier et supprimer une relation, révéler un secret, modifier une feuille de personnage, passer au jour suivant (« Nouveau jour »), créer et modifier une voie, un profil ou un peuple. Rien ne s'efface définitivement par ce chemin.

**Le brancher** : **Paramètres → Claude**, dans l'application de bureau. Un bouton inscrit COF DM dans Claude Desktop (installation classique ou du Microsoft Store) et un autre dans Claude Code, avec le bon chemin du programme ; il ne reste qu'à relancer Claude. Le panneau dit où en est chaque client (non installé, pas branché, branché, programme déplacé : « Mettre à jour ») et donne aussi la commande à taper pour Claude Code. Seule l'entrée `cof-dm` du fichier de configuration est écrite ; le reste est gardé, et une copie du fichier d'avant (`….avant-cof-dm`) est faite la première fois.

L'application doit être lancée pendant que Claude s'en sert. Le serveur MCP trouve le port et le jeton dans le dossier de l'application : le port est celui que l'application a réellement pris en se lançant (`port.json`), même si le port voulu était occupé ; `-port` l'impose.

À la main, pour qui préfère : avec Claude Code,

```bash
claude mcp add --scope user cof-dm -- /chemin/vers/COF-DM-x.y.z-x86_64.AppImage -mcp
```

et avec Claude Desktop, dans `claude_desktop_config.json` :

```json
{"mcpServers": {"cof-dm": {"command": "C:\\Jeux\\COF-DM.exe", "args": ["-mcp"]}}}
```

Pour un serveur distant : `COF_DM_URL` (son adresse, en HTTPS), `COF_DM_JETON` (un jeton de votre compte, **Paramètres → Mon compte**) et, si le compte a plusieurs univers, `COF_DM_UNIVERS` (l'identifiant de l'univers), dans la partie `env` de la configuration du client MCP. Le jeton de l'ordinateur n'est jamais envoyé à une autre machine.

## Installation et lancement

Pas d'installation : les versions publiées (onglet *Releases*) fournissent un lanceur.

- **Windows** : `COF-DM.exe`, à lancer directement (WebView2, déjà présent sous Windows 10 et 11).
- **Linux** : `COF-DM-x.y.z-x86_64.AppImage` ; le rendre exécutable (`chmod +x`) puis le lancer. Il faut WebKitGTK 4.1 sur le système (`libwebkit2gtk-4.1-0` sur Debian / Ubuntu, `webkit2gtk4.1` sur Fedora).

Au lancement, l'application ouvre sa fenêtre ; ses données sont dans :

| Système | Dossier |
|---|---|
| Windows | `%APPDATA%\COF DM` |
| Linux | `~/.local/share/cof-dm` |

**Mode portable** : un dossier `COF DM - données` placé à côté du programme (ou de l'AppImage) est utilisé à la place, pour tout emporter sur une clé USB.

On y trouve `univers/` (les univers créés sans choisir d'emplacement), `sauvegardes/` (un dossier par univers), `reglages.json` (univers ouvert, mot de passe MJ haché, jeton d'API), `config.json`, `cof-dm.log` et, tant que l'application tourne, `port.json` (le port qu'elle a pris, lu par `-mcp`).

```
cof-dm -port 8100         port (par défaut 8100 ; le suivant libre s'il est pris)
cof-dm -data D:\JDR\monde dossier de l'application (réglages, univers par défaut, sauvegardes)
cof-dm -univers F.cofdm   ouvre ce fichier d'univers (créé s'il n'existe pas)
cof-dm -headless          sans fenêtre : l'interface s'ouvre dans le navigateur
cof-dm -reseau            écoute sur le réseau local, comme la case « Autoriser les appareils du réseau local » (mode local seulement)
cof-dm -mcp               serveur MCP pour Claude, sur l'entrée et la sortie standard (voir « Avec Claude »)
```

Le mode serveur (`-serveur`) n'existe que dans l'image Docker : l'application de bureau le refuse. Sans cette case ni `-reseau`, ou quand elle est reliée à un serveur, l'application ne répond qu'à l'ordinateur sur lequel elle tourne. Sur le réseau, l'API demande le jeton et l'interface le mot de passe MJ.

### Univers

Un univers est **un seul fichier** `.cofdm` : fiches, relations, images, calendrier et journal. Il peut être rangé n'importe où (Documents, clé USB, dossier synchronisé) et se copie tel quel. Le menu **Fichier**, en haut du menu latéral, permet de créer un univers, d'en ouvrir un autre, de rouvrir un univers récent et d'enregistrer une copie. L'application rouvre au lancement le dernier univers utilisé.

Une sauvegarde complète de l'univers est faite à son ouverture, toutes les demi-heures et à la fermeture de l'application, quand il a changé. Les cinq dernières sont gardées, puis une par jour pendant deux semaines ; celles que vous demandez (« Sauvegarder maintenant ») ne sont jamais effacées. **Paramètres → Sauvegardes, exports et import** les liste et permet de revenir à l'une d'elles.

**Depuis une version précédente** : au premier lancement, l'ancienne base et ses images sont reprises dans `univers/Mon univers.cofdm`. Rien n'est effacé : `cof-dm.db` est renommé `cof-dm.db.ancien` et le dossier `medias/` reste en place ; vous pouvez les supprimer une fois l'univers vérifié.

### Depuis un vault Obsidian

Deux scripts Python (bibliothèque standard, lecture seule du vault) parlent à l'API de COF DM, application ouverte :

```
python3 -I outils/vault_vers_cofdm.py <vault> http://127.0.0.1:8100           importe tout un vault dans un monde vide
python3 -I outils/responsables_obsidian.py <vault> http://127.0.0.1:8100      monde déjà importé : reporte le responsable des PNJ
```

Sur un autre appareil que l'ordinateur du MJ, ou sur un serveur, ils ont besoin d'un jeton d'API : passez-le par la variable d'environnement `COF_DM_JETON` (par exemple `read -s COF_DM_JETON && export COF_DM_JETON`), jamais en argument, où il serait visible dans la liste des processus et l'historique du terminal.

Le second lit les propriétés `responsable: "[[X]]"` et `responsable_secret: "[[X]]"` des notes de PNJ et met à jour les fiches du même nom, sans rien créer ni supprimer (`--voir` montre ce qu'il ferait).

### Vers un vault Obsidian

**Fichier → Exporter vers Obsidian…** (ou le bouton de Paramètres) télécharge un zip de notes Markdown, rangées sous un seul dossier au nom de l'univers, à décompresser dans le vault :

- une note par fiche, rangée comme le vault que lit `vault_vers_cofdm.py` : `01 - Monde` (lieux, factions, lore, événements et `Chronologie`), `02 - PNJ`, `03 - PJ`, `04 - Objets & économie`, `05 - Campagnes/<campagne>` (la campagne, ses fils rouges, ses quêtes, séances, rencontres et combats), `06 - Secrets`, `07 - Règles`, `Archives` ;
- les champs courts en propriétés (frontmatter : statut, lieu, campagne… les références en liens `[[Nom]]`), les textes en sections, les relations en tableau, les images dans `Images/` ;
- les récapitulatifs des combats terminés et le journal.

Une case choisit ce qui part : tout, secrets du MJ compris (par défaut), ou seulement ce que voient les joueurs (fiches partagées, sans leurs champs du MJ, sans combat ni journal ; un lien vers une fiche non partagée devient du texte).

Le script d'import relit cet export, avec ses limites : il ne connaît ni les objets, ni le lore, ni les règles, ni les rencontres, refait les événements à partir du tableau de la chronologie et recatégorise les lieux et les factions.

## Développement

Go 1.26 (1.26.9 au moins) et un compilateur C (SQLite). La recherche utilise le module FTS5 de SQLite : **toujours compiler avec `-tags sqlite_fts5`** (sinon la compilation s'arrête avec un message).

```bash
go test -tags "sqlite_fts5 nogui" ./...                 # tests
go run -tags "sqlite_fts5 nogui" . -data ./donnees      # lancer sans fenêtre
go build -tags sqlite_fts5 -o cof-dm .                  # application de bureau
go build -tags "sqlite_fts5 nogui serveur" -o cof-dm-serveur .   # édition serveur (celle de l'image Docker)
```

La fenêtre native utilise WebView2 sous Windows, GTK 3 et WebKitGTK 4.1 sous Linux.

| Dossier | Contenu |
|---|---|
| `internal/modele` | Types de fiches, champs, relations, calendrier, fonctionnalités, validation : la seule source du schéma |
| `internal/store` | Un univers = un fichier SQLite : fiches, relations, références, mentions, images, recherche FTS5, graphe, chronologie, journal, export, bibliothèque, synchronisation, versions |
| `internal/serveur` | API HTTP, contrôle d'accès, comptes et univers du serveur, menu Fichier, sauvegardes, secrets, aides de jeu, rencontres, combats, écran du MJ, page des joueurs |
| `internal/comptes` | Comptes de l'édition serveur : mots de passe hachés, rôles par univers, jetons |
| `internal/reglages` | Réglages de l'application sur cet ordinateur (univers ouvert, mot de passe MJ, jeton d'API, serveur relié) |
| `internal/synchro` | Synchronisation du bureau avec un serveur, numéros de version |
| `internal/fiches`, `internal/regles`, `internal/reference` | Monstres et feuilles de personnage, fichier de règles, profils, peuples et voies |
| `internal/combat` | Moteur de combat : tours, attaques, états, tests, capacités, récapitulatif |
| `internal/des` | Formules de dés hors combat |
| `internal/obsidian` | Export vers un vault Obsidian |
| `internal/majour` | Mises à jour signées du bureau |
| `internal/mcp` | Serveur MCP (`cof-dm -mcp`) : outils pour Claude, traduits en appels de l'API |
| `frontend` | Interface (HTML, CSS, JavaScript sans framework ni dépendance) : l'application (`index.html`) et la page des joueurs (`joueur.html`) |
| `docs/API.md` | Documentation de l'API |
| `outils/` | Scripts Python d'import depuis un vault Obsidian |

Le dépôt ne contient **aucune donnée des livres de règles** : ni liste de peuples ou de profils, ni table, ni voie, ni texte. Tout vient des fichiers importés ou saisis par le MJ, et l'application fonctionne en saisie libre sans eux.

Une version se publie en poussant un tag `vX.Y.Z` : le workflow *Release* compile l'exe Windows et l'AppImage, signe la liste des empreintes et les joint à la Release. Lancé à la main avec le canal `test`, il publie une version d'essai d'une branche.

## Feuille de route

Fait : fiches, liens, relations, recherche, graphe, chronologie, images, API ; organigrammes, secrets et « qui sait quoi » ; bestiaire, feuilles de personnage, rencontres et combat (repris de tracker_COF) ; préparation et après-séance ; mode serveur, page des joueurs et synchronisation ; serveur MCP pour Claude ; export vers Obsidian ; dés, aides de jeu, fonctionnalités optionnelles ; voies et capacités utilisables ; mises à jour signées.

La suite, avec ses priorités, est dans [PROJETS.md](PROJETS.md) : d'abord la montée de niveau guidée, puis l'inventaire et le butin, l'afficheur de table, les notes rapides de séance, l'impression.

## Licence

[GNU GPL v3](LICENSE).
