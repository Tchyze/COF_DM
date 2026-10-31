# API de COF DM

Tout ce que fait l'interface passe par cette API : un programme (ou Claude) peut créer, lire, modifier et relier les fiches exactement comme le MJ.

- Adresse : `http://127.0.0.1:8100` par défaut (le port est dans les paramètres de l'application).
- Format : JSON en UTF-8. Les erreurs renvoient `{"detail": "message lisible"}` avec le code HTTP adapté (`401`, `403`, `404`, `409`, `415`, `422`). Une erreur imprévue répond `500` sans son détail (chemins de fichiers, requêtes), qui reste dans le journal de l'application (`cof-dm.log`).
- Le schéma complet (types de fiches, champs, valeurs permises, types de relations, calendrier) est donné par `GET /api/schema` : c'est la référence à lire en premier.

## Accès

| Client | Comment |
|---|---|
| Interface, sur l'ordinateur du MJ | Rien à faire. Les écritures portent l'en-tête `X-COF-DM: 1`. |
| Programme, Claude | En-tête `Authorization: Bearer <jeton>`. Le jeton s'affiche dans **Paramètres → API** et peut y être renouvelé. |
| Interface, sur un autre appareil du réseau (tablette) | Session ouverte avec le mot de passe MJ (cookie `cof_dm_mj`, 30 jours). Les écritures portent l'en-tête `X-COF-DM: 1` ; elles sont inscrites au journal sous l'auteur `mj (réseau)`. |

Facultatif : `X-COF-Auteur: claude` inscrit ce nom dans le journal des modifications (sinon `api`).

**Serveur MCP.** `cof-dm -mcp` expose cette API à Claude par le protocole MCP (JSON-RPC sur l'entrée et la sortie standard ; voir le README, « Avec Claude »). Il n'a pas de route à lui : chaque outil est un ou plusieurs appels ci-dessous, à l'adresse de l'application de cet ordinateur (port lu dans `port.json` du dossier de l'application, publié à chaque lancement, sinon dans `config.json`), sinon `COF_DM_URL`, avec le jeton (lu dans `reglages.json` du dossier de l'application pour l'application de cet ordinateur, sinon `COF_DM_JETON`), `X-COF-Auteur: claude` et, si `COF_DM_UNIVERS` est donné, `X-COF-Univers`. Il n'efface rien définitivement (`DELETE /api/entites/{id}?definitif=1` n'est pas offert).

Par défaut l'application n'écoute que sur l'ordinateur du MJ. En mode local (application qui n'est reliée à aucun serveur, voir « Synchronisation »), avec la case « Autoriser les appareils du réseau local » (Paramètres) ou l'option `-reseau`, elle écoute sur le réseau local : l'API y demande le jeton, l'interface le mot de passe MJ. Un navigateur du réseau qui n'est pas connecté ne reçoit que les fichiers de l'interface, `GET /api/acces` et `POST /api/acces/connexion`, et seulement s'il demande la page par une adresse IP ou un nom du réseau local (sans point, ou en `.local`).

| Requête | Effet |
|---|---|
| `GET /api/acces` | `{"mj", "local", "mdp_defini"}` : la requête est-elle celle du MJ, vient-elle de son ordinateur, un mot de passe MJ existe-t-il. Pour le MJ, en plus : `"reseau"` (l'application écoute sur le réseau), `"reseau_voulu"` (réglage enregistré, appliqué au prochain lancement), `"reseau_force"` (`-reseau`), `"reglable"`, `"adresses"` (celles de l'application sur le réseau local), `"relie"` (reliée à un serveur : le réseau local reste fermé). |
| `POST /api/acces/connexion` `{"mdp": "…"}` | Ouvre la session. `401` : mot de passe incorrect ; `409` : aucun mot de passe défini ; `429` : huit essais ratés en cinq minutes depuis cette adresse. |
| `POST /api/acces/deconnexion` | Ferme la session de l'appareil : son cookie n'est plus accepté, même recopié. |
| `POST /api/acces/mdp` `{"nouveau": "…"}` | Définit ou change le mot de passe MJ (8 caractères au moins) ; `""` le retire. Les appareils connectés sont déconnectés. |
| `POST /api/acces/reseau` `{"reseau": true}` | Enregistre l'ouverture au réseau local, appliquée au prochain lancement. `409` pour l'ouvrir quand l'application est reliée à un serveur. |
| `POST /api/acces/joueurs` `{"active": true}` | Ouvre (ou ferme) la page des joueurs au réseau local, sans mot de passe ; effet immédiat. Sur l'ordinateur du MJ seulement ; `409` pour l'ouvrir quand l'application est reliée à un serveur. `GET /api/acces` donne au MJ `"page_joueurs"` et `"edition_joueurs"`. |
| `PUT /api/joueurs/edition` `{"active": true}` | Déverrouille (ou verrouille) les feuilles de personnage pour les joueurs. Réglage de l'univers (synchronisé) ; le MJ le règle, sur son ordinateur ou depuis la tablette (écran du MJ). |
| `GET /api/acces/qr?url=http://…` | QR code (PNG) d'une adresse. |

**Application reliée à un serveur.** Tant que l'application de bureau est en mode serveur, elle ne répond plus qu'à son ordinateur, même si elle écoute sur le réseau local : toute requête venue d'ailleurs (tablette, page des joueurs, jeton d'API) reçoit `403`. Elle ne s'ouvre pas au réseau à son lancement. La tablette et les joueurs passent alors par le serveur.

Depuis une session ouverte par mot de passe, `POST /api/acces/mdp`, `POST /api/acces/reseau` et `POST /api/parametres/jeton` répondent `403`, et `GET /api/parametres` ne donne pas le jeton : ces réglages se font sur l'ordinateur du MJ. Il en va de même avec le jeton d'API depuis le réseau.

## Mode serveur

L'**édition serveur** (image Docker, compilée avec l'étiquette `serveur`) tourne sans fenêtre sur une machine distante, derrière un reverse proxy qui porte le HTTPS (voir le README). L'accès décrit plus haut ne s'applique plus : il n'y a ni mot de passe MJ, ni jeton unique, et **rien n'est accordé aux requêtes venues de la machine elle-même** (celles du proxy en viennent toutes). Chacun entre avec son compte, ou avec un jeton d'API de son compte (`Authorization: Bearer cofdm_…`).

| Rôle | Droits |
|---|---|
| Administrateur (tout le serveur) | Tout : comptes, univers (`/api/univers`), et MJ de chaque univers. |
| `mj` (par univers) | Lit et modifie l'univers. |
| `auditeur` (par univers) | Lit tout l'univers, secrets du MJ et journal compris ; toute écriture répond `403`. |
| `joueur` (par univers) | N'a que sa page de joueur (`/joueur.html`). Voir « Comptes joueurs ». |
| Aucun rôle dans l'univers ouvert | Son compte et ses jetons seulement ; l'univers répond `403`. |

**Plusieurs univers.** Le serveur ouvre tous les univers de son dossier (`univers/`) et chaque compte a son rôle dans chacun. Une requête vise un univers par l'en-tête `X-COF-Univers: <identifiant>` ; pour une lecture (`GET`), aussi par le paramètre `?univers=<identifiant>`, qui sert aux images, au suivi en direct et aux téléchargements d'un onglet (il est ignoré en écriture et ne donne aucun droit) ; sinon par le choix du navigateur (`POST /api/mes-univers/courant`) ; sinon c'est le premier univers où le compte a un rôle. Un jeton d'API dont le compte a accès à plusieurs univers **doit** envoyer l'en-tête (`400` sans lui). Partout dans cette page, « l'univers ouvert » désigne alors l'univers visé par la requête.

| Requête | Effet |
|---|---|
| `GET /api/mes-univers` | `{"univers": [{"id", "nom", "role"}], "courant", "version", "protocole", "modele"}` : les univers où le compte a un rôle, et la version et les numéros du serveur (voir « Synchronisation », Versions). |
| `POST /api/mes-univers/courant` `{"id"}` | Choisit l'univers de ce navigateur (cookie `cof_dm_univers`), celui que prend un nouvel onglet. L'interface garde ensuite l'univers de chaque onglet et l'envoie dans l'en-tête : deux onglets peuvent tenir deux univers. `404` : univers inconnu ou fermé à ce compte. |

Pour l'administrateur, les routes `/api/univers` changent de sens : `nouveau` ne prend qu'un `nom` (créé dans le dossier du serveur, ouvert à côté des autres), `ouvrir` ne prend qu'un fichier de ce dossier et y fait passer le navigateur, `copie` et `restaurer` portent sur l'univers visé. La copie d'un univers porte son identifiant : elle ne s'ouvre pas à côté de l'original.

Un jeton d'API a les droits de son compte, sauf ce qui garde le compte : le mot de passe, les jetons et la gestion des comptes demandent une session ouverte par mot de passe (`403` avec un jeton). Les écritures faites depuis une session portent l'en-tête `X-COF-DM: 1`. Le journal porte le nom du compte (`X-COF-Auteur` est ignoré).

| Requête | Effet |
|---|---|
| `GET /api/acces` | `{"serveur": true, "connecte", "mj", "lecture_seule", "joueur", "compte"}` : `mj` dit si le compte entre dans l'univers ouvert, `joueur` s'il n'y a que sa page de joueur (l'application le renvoie vers `/joueur.html`). |
| `POST /api/acces/connexion` `{"nom", "mdp"}` | Ouvre la session (cookie `cof_dm_mj`, 30 jours, `Secure` en HTTPS). `401` : nom ou mot de passe incorrect ; `429` : huit essais ratés en cinq minutes depuis cette adresse, ou vingt sur ce nom. |
| `POST /api/acces/deconnexion` | Ferme la session. |
| `GET /api/compte` | Son compte : `{"nom", "admin", "inactif", "role"` (dans l'univers ouvert)`, "roles"` (par identifiant d'univers)`, "jetons": [{"id", "nom", "cree"}], "cree"}`. |
| `POST /api/compte/mdp` `{"ancien", "nouveau"}` | Change son mot de passe (10 caractères au moins). Les autres sessions du compte sont fermées. |
| `POST /api/compte/jetons` `{"nom": "script"}` | Crée un jeton d'API : `{"jeton": "cofdm_…", "id", "nom"}`. Il ne s'affiche que cette fois (20 par compte au plus). |
| `DELETE /api/compte/jetons/{id}` | Retire un jeton. |
| `DELETE /api/compte/jeton` | Le jeton d'API qui fait la requête se retire lui-même (un poste qui quitte le serveur) ; les autres jetons du compte restent. `409` sans jeton. Comme toutes les routes `/api/compte` et `/api/acces`, elle ne vise aucun univers : un jeton qui a accès à plusieurs univers n'a pas à envoyer `X-COF-Univers`. |
| `GET /api/comptes` | Administrateur : tous les comptes. |
| `POST /api/comptes` `{"nom", "mdp", "admin"?, "role"?}` | Administrateur : crée un compte. Nom : 3 à 32 caractères parmi `a-z`, `0-9`, `-`, `_`. `role` (`mj`, `auditeur` ou `joueur`) vaut pour l'univers ouvert. |
| `PATCH /api/comptes/{nom}` `{"mdp"?, "admin"?, "inactif"?, "role"?, "pj"?}` | Administrateur : modifie un compte ; `"role": ""` retire l'accès à l'univers ouvert. Changer le mot de passe ou désactiver le compte ferme ses sessions. `409` s'il ne restait plus d'administrateur actif. |
| `DELETE /api/comptes/{nom}` | Administrateur : supprime un compte. |

### Comptes joueurs

Le MJ choisit les fiches que les joueurs voient : `POST /api/entites/{id}/partage` `{"partage": true}` (`false` pour la retirer). Chaque fiche porte `"partage"` ; une fiche « Secret » ou une aide de jeu ne se partage pas (`422` : une aide s'envoie, voir « Aides de jeu »), une fiche archivée n'est plus montrée. Le partage décide de ce que contient l'export vers Obsidian « joueurs » (`GET /api/export/obsidian?secrets=0`), et qu'un PJ s'ouvre à son joueur.

Un compte `joueur` n'a que sa page de joueur : il n'atteint que les routes ci-dessous ; tout le reste répond `403` (fiches, listes, schéma, calendrier, journal, export, graphe, chronologie, organigramme, écran du MJ, bestiaire, combats, secrets, écritures). Un jeton d'API de ce compte a les mêmes droits.

| Requête | Ce qu'un joueur reçoit |
|---|---|
| `GET /api/joueur/…`, `PATCH /api/joueur/pj/{id}` | La page des joueurs, restreinte à son PJ (voir « Page des joueurs »), documents compris. |
| `GET /api/entites/{id}/feuille`, `PUT` | La feuille de son propre PJ seulement (voir « Feuille de personnage »). |
| `GET /api/reference?pj=<id>` | La part de la bibliothèque qui concerne son PJ : les voies de sa feuille (par `ref` ou par nom), son profil et son peuple (par nom). Le PJ d'un autre : `404` ; sans `pj` : une bibliothèque vide. |
| `GET /api/sante` | Version (et numéros : voir « Synchronisation », Versions). |
| `GET /api/acces`, `/api/compte…`, `/api/mes-univers…` | Son compte (mot de passe, jetons) et ses univers : un compte qui a un rôle dans plusieurs univers passe de l'un à l'autre depuis sa page. |

**Édition par les joueurs.** Fermée par défaut. Le MJ de l'univers (ou l'administrateur) l'ouvre avec `PUT /api/joueurs/edition` `{"active": true}` ; l'administrateur rattache chaque compte à son PJ avec `PATCH /api/comptes/{nom}` `{"pj": <id>}` (`0` : aucun). Le réglage se synchronise : ouvert sur un bureau relié, il vaut sur le serveur après l'échange, et les pages des joueurs en sont prévenues. Si la fiche du PJ est partagée, son joueur tient alors toute sa feuille, sauf les notes du MJ (« Feuille de personnage »), et `PATCH /api/joueur/pj/{id}` `{"data": {…}, "modifie"?}` écrit les seuls champs de sa fiche que le schéma marque `"joueur": true`. Tout autre champ, le nom, les tags répondent `403` ou `422` ; le PJ d'un autre, une autre fiche, `404`. `GET /api/acces` donne `"edition_joueurs"`.

Sans objet en mode serveur : `POST /api/acces/mdp`, `POST /api/acces/reseau` et `POST /api/parametres/jeton` (`404`). `GET /api/parametres` ne donne les chemins du serveur qu'à l'administrateur.

## Fiches

Types : `pnj`, `lieu`, `faction`, `objet`, `lore`, `evenement`, `campagne`, `quete`, `rencontre`, `seance`, `pj`, `secret`, `regle`.

Le monde est unique : il n'y a pas d'état par campagne (un PNJ mort l'est partout).

Une fiche :

```json
{
  "id": 12, "type": "pnj", "nom": "Gautier Roussel",
  "data": { "statut": "vivant", "lieu": 4, "role": "Capitaine de [[Mont-Veille]]." },
  "tags": ["milice"], "archive": false, "image": "a1b2….png",
  "cree": 1791400000.12, "modifie": 1791400100.34
}
```

`data` ne contient que les champs du type (voir `/api/schema`), selon leur genre :

| Genre | Valeur |
|---|---|
| `texte` | chaîne d'une ligne |
| `long` | Markdown ; `[[Nom]]` (ou `[[Nom\|texte affiché]]`) crée un lien vers la fiche de ce nom |
| `choix` | une des `options` (la casse est corrigée) |
| `nombre` | nombre (`"1,5"` est accepté) |
| `booleen` | `true` / `false` |
| `ref` | identifiant d'une fiche d'un des types `cibles` |
| `refs` | liste d'identifiants |
| `date_jeu` | `{"an": 1216, "mois": 1, "jour": 1}`, contrôlée par le calendrier du monde |
| `date` | `"AAAA-MM-JJ"` |
| `etapes` | `[{"texte": "…", "fait": false}]` (une simple chaîne est acceptée) |
| `monstre` | identifiant d'un monstre du bestiaire de l'univers (`profil_combat` d'un PNJ) |
| `adversaires` | `[{"monstre": 3, "nom": "Brigand", "qte": 4, "camp": "ennemi"}]` : monstres du bestiaire de l'univers (`monstre` : leur identifiant ; `nom` : gardé pour l'affichage) ; `qte` de 1 à 99, `camp` `ennemi` ou `allie` |

Les champs `peuple` et `profil` des PNJ et des PJ prennent les noms officiels (liste dans `/api/schema`) ; `allegeance` d'un PNJ est une faction.

Toute valeur refusée est expliquée dans l'erreur `422` (champ inconnu avec la liste des champs permis, option invalide avec les options, fiche référencée inexistante ou du mauvais type…). Rien n'est enregistré si un seul champ est refusé.

| Requête | Effet |
|---|---|
| `GET /api/entites?type=pnj&q=…&tag=…&campagne=3&archives=1&limite=50` | Liste (résumés). `q` : recherche sans accents, tolérante aux fautes, `"phrase exacte"`, `#tag`. |
| `POST /api/entites` `{"type","nom","data","tags"}` | Crée une fiche. Réponse `201` : la fiche complète. |
| `GET /api/entites/{id}` | Fiche complète : `relations`, `references` (qui pointe vers elle, par un champ ou une mention), `mentions` (les `[[Nom]]` de ses textes, résolus), `refs` (noms des fiches citées par ses champs), `medias`, `dates` (dates en jeu en toutes lettres). |
| `PATCH /api/entites/{id}` `{"nom"?, "data"?, "tags"?, "modifie"?}` | Modification **partielle** : `data` est fusionné, une clé à `null` efface le champ. `modifie` (facultatif) : la valeur lue ; si la fiche a changé entre-temps, réponse `409`. Renommer une fiche réécrit les `[[Ancien nom]]` des autres fiches. |
| `PUT /api/entites/{id}` | Comme `PATCH`, mais `data` remplace tous les champs. |
| `DELETE /api/entites/{id}` | Archive la fiche (masquée, récupérable). |
| `DELETE /api/entites/{id}?definitif=1` | Efface la fiche, ses relations, ses images, et la retire des champs des autres fiches. |
| `POST /api/entites/{id}/archive` `{"archive": false}` | Archive ou restaure. |

## Relations

Liens libres entre deux fiches, lus dans les deux sens (`membre` : « A membre de B » / « B a pour membre A »). Les types sont dans `/api/schema` ; `autre` prend son libellé dans `note`. Un type peut borner les fiches qu'il relie (`de`, `vers` : types de A et de B, `422` sinon) ; un type `reservee` est posé par l'application et refusé ici (`recu` : l'envoi d'une aide de jeu à un PJ, voir « Aides de jeu »), y compris en changeant le type d'une relation.

| Requête | Effet |
|---|---|
| `POST /api/relations` `{"a": 12, "b": 7, "type": "membre", "rang": "lieutenant", "superieur": 9, "secrete": false, "contexte": 31, "note": "…"}` | Crée la relation de A vers B. Seuls `a`, `b` et `type` sont obligatoires. `contexte` : une fiche qui situe la relation (pour `sait` : la séance où il l'a appris). |
| `PATCH /api/relations/{id}` `{"type"?, "note"?, "rang"?, "superieur"?, "secrete"?, "contexte"?}` | Modifie les champs donnés. `"superieur": 0` ou `"contexte": 0` les retire. |
| `DELETE /api/relations/{id}` | Supprime. Les subordonnés d'un membre retiré passent sous son propre supérieur. |
| `GET /api/entites/{id}/organigramme` | Arbre d'une faction : `[{"fiche", "rang", "relation", "superieur", "dirigeant", "rattache", "repond", "secret", "statut", "enfants": [...]}]`. |

**Organigramme.** Une faction est dirigée par la fiche de son champ `dirigeant` et par les relations `dirige` qui pointent vers elle. Ses membres sont les relations `membre` (A membre de la faction B), avec un `rang` et un `superieur` : l'identifiant d'un autre membre ou dirigeant de la même faction (une boucle est refusée). Une faction peut être membre d'une autre (sous-faction, allié intégré). `secrete: true` marque une appartenance ou une relation connue du MJ seulement.

Dans l'arbre, chacun se place sous celui à qui il répond : le `superieur` de son appartenance, sinon le `responsable` de sa fiche de PNJ, sinon son `responsable_secret` (le poste est alors `secret`), sinon le premier dirigeant. Un lien qui fermerait une boucle est ignoré. `superieur` ne reprend que le réglage de l'appartenance. Y figurent aussi, avec `"rattache": true` et sans `relation`, les PNJ qui n'appartiennent à aucune faction et dont le `responsable` est dans l'arbre, et ceux dont le `responsable_secret` y est. `repond` donne le responsable d'un poste quand celui-ci est hors de l'arbre.

## Secrets

Un secret est une fiche de type `secret`. « X sait le secret S » est la relation `sait` de X (PJ, PNJ, faction) vers S, avec `contexte` = la séance et `note` = comment.

| Requête | Effet |
|---|---|
| `GET /api/secrets?campagne=3` | Tableau « qui sait quoi » : `{"secrets", "pj", "pnj", "connaissances": [{"relation", "secret", "qui", "contexte", "note"}], "indices": [{"relation", "secret", "qui", "aide", "seance"}]}`. `pnj` : PNJ et factions qui savent. `indices` : les aides de jeu (non archivées) envoyées à un PJ (`qui`) et qui sont l'indice du secret ; recevoir un indice n'est pas savoir. |
| `POST /api/secrets/{id}/reveler` `{"qui": [4, 5], "seance": 31, "note": "…"}` | Ces fiches apprennent le secret. Le statut passe à `révélé` quand tous les PJ de la campagne le savent, `en partie révélé` sinon ; `revelation` reçoit la séance. |

Pour « ne sait plus », supprimer la relation (`DELETE /api/relations/{id}`).

## Aides de jeu

Une aide de jeu (lettre, image, indice) est une fiche de type `aide` : `forme`, `campagne`, `contenu` (le texte remis aux joueurs), ses images, et réservés au MJ `indice_de` (le secret dont c'est l'indice), `prevue` (la séance pour laquelle elle est prévue) et `notes`. L'envoyer à un PJ pose la relation réservée `recu` du PJ vers l'aide, avec la séance en `contexte` ; la retirer, c'est supprimer cette relation (`DELETE /api/relations/{id}`), et les pages des joueurs relisent leurs documents. Une aide n'est jamais une fiche partagée : les joueurs ne la voient que dans leurs documents.

| Requête | Effet |
|---|---|
| `GET /api/aides?campagne=3` | Les aides non archivées (de la campagne et sans campagne) : `[{"fiche", "indice_de"?, "prevue"?, "envois": [{"relation", "pj", "seance"?, "date"}]}]`, par nom. |
| `POST /api/aides/{id}/envoyer` `{"qui": [4, 5], "seance": 31}` ou `{"tous": true}` | Envoie l'aide à ces PJ, ou à tous ceux de sa campagne (tous sans campagne) qui ne sont ni morts ni partis. Un PJ qui l'a déjà reçue est passé. Réponse : `{"envoyes": [ids], "statut_secret"}`. Si l'aide est l'indice d'un secret `caché` (ou sans statut), il passe à `soupçonné` (`statut_secret`) ; la révélation reste `POST /api/secrets/{id}/reveler`. Les pages des joueurs des destinataires sont prévenues aussitôt. `422` : pas une aide, aide archivée, destinataire qui n'est pas un PJ, séance inconnue. |

## Bestiaire et règles

Le bestiaire (monstres officiels et maison), le fichier de règles et les profils, peuples et voies sont rangés dans l'univers. **Le logiciel ne contient aucune donnée des livres** : ni liste, ni table, ni nom, ni texte. Tout s'importe depuis les fichiers du MJ. En mode serveur, le MJ les modifie, l'auditeur les lit, le joueur n'y a pas accès.

| Requête | Effet |
|---|---|
| `GET /api/bestiaire` | Tous les monstres : la fiche, avec `"id"` et `"modifie"`. |
| `POST /api/bestiaire` | Crée un monstre. Champs : `nom`, `source` (`maison` par défaut, ou `officiel`), `nc`, `categorie`, `taille`, `tags`, `carac` (`{"FOR", "DEX", "CON", "INT", "SAG", "CHA"}`, modificateurs), `superieures`, `defense`, `pv`, `init`, `rd`, `crit`, `attaques` (`[{"nom", "type": "C"｜"D"｜"M", "bonus", "dm", "notes"}]`), `capacites`, `description`, `comportement`, `habitat`, `butin`, `langues`, `notes`, et pour une fiche officielle `ref`, `livre`, `page`. `422` si la fiche est invalide. |
| `GET /api/bestiaire/{id}`, `PUT`, `DELETE` | Lit, remplace, supprime. `PUT` répond `409` sur une fiche officielle (`source: officiel` avec `ref`) : elle est verrouillée. |
| `POST /api/bestiaire/{id}/dupliquer` | Crée une copie maison modifiable. |
| `POST /api/bestiaire/import` | Un tableau de monstres, ou `{"monstres": [...]}`. Une fiche dont la `ref` existe déjà est mise à jour, les autres ajoutées. Réponse `{"crees", "mis_a_jour", "rejets": [{"ligne", "nom", "motif"}]}`. |
| `GET /api/bestiaire/tables` | `{"creature", "listes"}` : les tables du créateur de monstre, telles que le fichier de règles les donne. `404` si l'univers n'a pas de règles pour le créateur. |
| `POST /api/bestiaire/calculer` | Conçoit une créature selon ces tables (`404` sans elles) : `{"taille", "archetype", "categorie", "type_boss", "rangs_boss", "voies": {"<clé>": rang}, "sup_magie", "INT", "SAG", "CHA", "attaque_multiple"}` → `{"nc", "carac", "superieures", "defense", "pv", "init", "rd", "att", "dm", "conception", "capacites", "detail", "alertes"}`. Rien n'est enregistré. |
| `GET /api/regles` | Le fichier de règles de l'univers (vide s'il n'y en a pas). |
| `POST /api/regles/import` | Importe un fichier de règles (voir ci-dessous). Chaque partie qu'il apporte remplace celle de l'univers ; ce qu'il ne dit pas reste. Réponse `{"titre", "createur", "rencontre", "listes": {<liste>: <nombre>}}`. |
| `DELETE /api/regles` | Retire le fichier de règles : les fiches gardent leurs valeurs. |
| `GET /api/reference` | `{"profils", "peuples", "voies"}` de l'univers, triés par nom ; chaque élément porte son `id` (son document dans l'univers). |
| `GET /api/reference/export` | Les mêmes, en un fichier à importer dans un autre univers (`"format": "cof-dm-reference"`, sans les `id`). |
| `POST /api/reference/import` | Fusionne un fichier de profils, peuples et voies (même `ref`, ou même nom : remplacé). `?etiquette=officielle` ou `maison` donne cette étiquette à tout le fichier ; sans elle, chaque élément garde la sienne. Réponse `{"bilan", "alertes", "total"}`. |
| `DELETE /api/reference` | Retire tous les profils, peuples et voies (les feuilles ne changent pas). |
| `POST /api/reference/{genre}` | Crée un élément (`genre` : `voies`, `profils` ou `peuples` ; autre : `404`). Corps : l'élément, au format ci-dessous. Même identité (`ref`, ou nom sans `ref`) qu'un autre de sa collection : `422`. Réponse `201 {"element"}`, avec son `id`. |
| `GET /api/reference/{genre}/{id}` | `{"element", "utilisee_par"}`. `utilisee_par` : les PJ dont la feuille a cette voie (`{"id", "nom", "rang", "def_divers", "init_divers", "att_divers"}`), ou dont la fiche a ce profil ou ce peuple (`{"id", "nom"}`). Ni le joueur ni la page des joueurs n'y ont accès. |
| `PUT /api/reference/{genre}/{id}` | Remplace l'élément en entier. Une voie renommée l'est aussi sur les feuilles qui la désignent et dans les voies des profils et des peuples. Réponse `{"element", "avertissements"}` : pour une voie, les PJ qui ont atteint le rang d'une capacité à bonus de DEF, d'initiative ou d'attaque alors que le « divers » de ce champ n'est pas nul (`{"id", "nom", "capacite", "champ"}`) ; rien n'est changé sur leur feuille. |
| `DELETE /api/reference/{genre}/{id}` | Retire l'élément. Les feuilles gardent leur copie des noms : la voie y devient une voie libre. |
| `GET /api/bibliotheque` | Le bestiaire, le fichier de règles et les profils et voies de l'univers en un fichier (`"format": "cof-dm-bibliotheque"`). |
| `POST /api/bibliotheque` | Ajoute une bibliothèque à l'univers : fiches officielles mises à jour par leur `ref`, monstre maison du même nom non doublé. |
| `POST /api/univers/bibliotheque` `{"source": "<fichier d'univers>"}` | La même reprise, directement depuis un autre univers. Comme les autres routes `/api/univers` : ordinateur du MJ, ou administrateur d'un serveur (source parmi les univers du serveur). |

**Profils, peuples et voies** (`"format": "cof-dm-reference"`, version 2). Rangés dans l'univers un par un (collections de documents `profils`, `peuples`, `voies`). L'ancien format (`"tracker-cof-reference"`, version 1, ou un simple tableau de voies) se lit encore à l'import. Tous les champs sont facultatifs, sauf `nom` :

```json
{"format": "cof-dm-reference", "version": 2,
 "profils": [{"ref": "…", "nom": "…", "etiquette": "officielle", "famille": "…", "dv": "d8", "carac_magie": "SAG",
              "armes": "…", "armures": "…", "equipement": "…", "description": "…", "voies": ["…"], "livre": "…", "page": 0}],
 "peuples": [{"ref": "…", "nom": "…", "etiquette": "maison", "caracs": "…", "modificateurs": {"DEX": 2, "CON": -2},
              "description": "…", "voies": ["…"], "livre": "…", "page": 0}],
 "voies": [{"ref": "…", "nom": "…", "etiquette": "officielle", "type": "profil", "profils": ["…"], "prerequis": "…",
            "description": "…", "livre": "…", "page": 0,
            "rangs": [{"nom": "…", "texte": "…", "limitee": false, "sort": false,
                       "bonus": [{"cible": "def", "valeur": 1, "par_rang": false, "niveau": false, "carac": ""}],
                       "action": {"genre": "sort", "cout_pm": 0, "attaque": "M", "bonus": 0, "degats": "1d6", "soin": "", "etat": ""},
                       "utilisations": {"fois": 1, "par": "combat"}}]}]}
```

- `etiquette` : `officielle` ou `maison`, une simple marque pour trier et filtrer, qui ne verrouille rien. Absente : officielle si l'élément a une `ref`, maison sinon.
- `modificateurs` d'un peuple : pour l'affichage et la création d'un PJ, jamais ajoutés d'office à une feuille (ses valeurs les comprennent déjà). Absents, ils sont lus dans le texte `caracs` (« +2 DEX, −2 CON »).
- Une voie a de 5 à 20 rangs (complétée jusqu'à 5).
- `bonus` d'une capacité : ajouté à la feuille quand le PJ a atteint son rang. `cible` : `pv`, `pm`, `def`, `init`, `att` (toutes les attaques), `att_c`, `att_d`, `att_m`, `rd`, ou une caractéristique (`FOR`…`CHA`, sa valeur). Il vaut `valeur`, multipliée par le rang atteint dans la voie si `par_rang`, plus le niveau si `niveau`, plus le modificateur de `carac`.
- `action` : ce que la capacité préremplit en combat. `genre` : `sort`, `attaque`, `soin`, `etat` ou `autre` ; `attaque` : `C`, `D` ou `M` (vide : pas de jet) ; `etat` : la clé d'un état des règles de combat.
- `utilisations` : `fois` (1 à 99) par `combat`, `jour`, `repos:<clé>` (un repos des règles de la feuille) ou `main` (remise à zéro à la main).

**Fichier de règles** (`"format": "cof-dm-regles"`). Toutes ses parties sont facultatives :

```json
{"format": "cof-dm-regles", "version": 1, "titre": "…",
 "listes": {"peuples": [], "profils": [], "peuples_pnj": [], "profils_pnj": [], "tailles": [], "categories": []},
 "creature": {
   "archetypes": [{"cle": "…", "description": "…"}],
   "table": {"<taille>": {"<archétype>": {"nc": 0, "FOR": 0, "DEX": 0, "CON": 0, "dexSup": false, "forSup": false, "def": 0, "pv": 1, "init": 0, "att": 0, "dm": "1d6"}}},
   "boss": [{"cle": "…", "libelle": "…", "att": 0, "dm": 0, "dmDes": 0, "def": 0, "pv": 0, "description": "…"}],
   "voies": [{"cle": "…", "nom": "…", "page": 0, "sup": ["CON"], "choix": false, "rangs": ["…"], "rd": [0], "init": [0], "remarque": "…"}],
   "mots_cles": {"INT": [{"nom": "…", "valeur": 0}]},
   "armes": [{"cle": "…", "libelle": "…", "de": "1d8"}], "tailles_sans_arme": [],
   "nc_par_rang_boss": 1, "nc_par_rang_voie": 1, "nc_par_rang_voie_faible": 0.5, "nc_faible": 0.5, "nc_attaque_multiple": 1,
   "rangs_max_voie": 3, "voies_conseillees": 3,
   "textes": {"intro": "…", "alerte_voies_nc": "…"}},
 "personnage": {"niveau_max": 0, "valeur_min": 0, "valeur_max": 0, "des_de_vie": [], "de_de_vie": "", "defense_base": 0, "pc_max": 0, "pr_max": 0, "rang_max": 0, "origines_voie": [], "emplacements": []},
 "rencontre": {"points": [{"nc": 0, "points": 0}], "points_au_dela": 0, "groupe_reference": 0,
   "conditions": [{"cle": "…", "libelle": "…", "niveau": 0, "detail": "…"}],
   "verdicts": [{"cle": "…", "texte": "…", "jusqu_a": 0, "inclus": false, "ton": "ok"}],
   "alertes": [{"quand": "nc", "seuil": 0, "texte": "…"}]},
 "combat": {"de": 20, "echec_naturel": 0, "critique_dm": 0, "chance": 0, "budget_actions": false,
   "etats": [{"nom": "…", "description": "…", "att": 0, "att_distance": 0, "def": 0, "init": 0, "de": 0,
              "bloque": false, "critique_automatique": false, "une_action": false, "fin": "tour", "action": "A"}]}}
```

Une taille dont le seul archétype s'appelle `unique` n'en propose pas le choix. Les `listes` deviennent les choix proposés par les champs des fiches qui portent `"liste"` dans le schéma (peuple et profil des PJ et des PNJ) ; ces champs restent du texte libre, avec ou sans fichier.

La partie `rencontre` donne la difficulté des rencontres. `points` : ce que vaut une créature selon son NC (la ligne la plus proche ; au-delà de la dernière, `points_au_dela` par niveau) ; la même table donne le niveau d'une rencontre, celui dont les points sont les plus proches du total. `groupe_reference` : taille du groupe pour laquelle la table est écrite (les points sont multipliés par cette taille et divisés par le nombre de PJ ; `0` : pas d'ajustement). `conditions` : ce qu'une condition ajoute au niveau. `verdicts`, du plus facile au plus dur : le premier dont l'écart (niveau de la rencontre − niveau moyen des PJ) est sous `jusqu_a` l'emporte (`inclus` : la borne compte) ; un verdict sans `jusqu_a` vaut pour tout écart et ferme la liste ; `ton` : `ok`, `warn` ou `danger`. `alertes` : texte affiché quand le plus fort NC (`"quand": "nc"`) ou l'écart entre le plus fort et le plus faible (`"ecart_nc"`) atteint `seuil`. Une partie d'une autre forme est refusée à l'import ; rangée dans un univers avant ce format, elle est ignorée.

La partie `combat` donne les valeurs du combat et les états des combattants ; sans elle, le combat reste jouable (initiative, PV, attaque contre la DEF, dégâts, états libres sans effet). `de` : dé des attaques et des tests (20 à défaut). `echec_naturel` : résultat du dé qui fait toujours échouer (`0` : aucun). `critique_dm` : par combien un critique multiplie les dégâts (`0` : pas de critique) ; un critique est une réussite automatique et n'existe qu'avec le dé normal. `chance` : ce qu'un point de chance ajoute à un jet déjà lancé (`0` : pas de point de chance). `budget_actions` : un tour se limite à une action limitée, ou un mouvement et une attaque, ou deux mouvements. `etats` : `att` et `att_distance` (qui remplace `att` pour une attaque à distance), `def`, `init` s'ajoutent ; `de` remplace le dé normal ; `bloque` interdit toute action ; `critique_automatique` : toute attaque contre lui est un critique ; `une_action` : une seule action par tour ; `fin` : `round` (retiré à la fin du premier round) ou `tour` (au début du prochain tour du combattant) ; `action` (`A` ou `L`) : l'état se prend à son propre tour et coûte cette action (posture défensive).

## Rencontres

Les créatures d'une rencontre (`adversaires`) et le profil de combat d'un PNJ (`profil_combat`) désignent des monstres du bestiaire de l'univers. Un monstre qui sert à une fiche ne se supprime pas (`409`, avec le nom des fiches).

| Requête | Effet |
|---|---|
| `GET /api/entites/{id}/difficulte` | Les créatures d'une rencontre et sa difficulté : `{"creatures": [{"monstre", "nom", "qte", "camp", "nc", "defense", "pv", "pnj"?, "introuvable"?}], "pnj_sans_profil": [fiche], "nb_pj", "niveau_pj", "conditions": [{"cle", "libelle", "niveau", "detail"}], "evaluation"}`. `creatures` : celles de la fiche, puis ses PNJ présents qui ont un profil de combat (`pnj` : leur fiche) ; `introuvable` : monstre retiré du bestiaire. Le groupe : les PJ de la campagne de la rencontre (sinon de sa quête) qui ne sont ni `mort`, ni `parti`, ni `absent`, avec le niveau de leur fiche (1 à défaut). `evaluation` : `{"points", "points_ajustes", "niveau", "ecart", "cle", "verdict", "ton", "detail", "alertes"}`, `null` sans partie `rencontre` dans les règles ; `cle` : celle du verdict, `vide` sans créature ennemie, `inconnu` sans PJ. Les alliés ne comptent pas. Paramètres : `?condition=<clé>`, et `?nb_pj=` / `?niveau_pj=` pour évaluer face à un autre groupe. `404` si la fiche n'est pas une rencontre. Réservée au MJ et à l'auditeur. |

L'export de l'univers (`GET /api/export`) porte la version de son modèle de fiches (`"donnees"`) : un export plus ancien est remis en forme à la restauration, comme un univers à son ouverture (les créatures des rencontres sont rattachées au bestiaire par leur nom, les liens vers l'ancien tracker qui ne s'y retrouvent pas sont retirés et notés au journal ; les champs `fiche_tracker` d'un PJ et `combats` d'une séance, qui n'existent plus, sont retirés). Il contient le bestiaire et les profils, peuples et voies (`"documents"`) et le fichier de règles (`"regles"`) ; les restaurer les remplace. Un export d'avant le modèle 2 porte les profils et voies dans `"reference"` : ils sont rangés un par un à la restauration. Un export plus ancien, qui ne les contient pas, laisse ceux de l'univers en place.

## Page des joueurs

Ce que la page des joueurs (`/joueur.html`) lit : la feuille de son personnage, le combat en cours réduit à ce qu'un joueur en voit, ses documents (aides de jeu reçues), les hauts faits. Rien n'y permet d'agir sur un combat.

Deux façons d'y entrer :

- **à la maison** (application de bureau) : quand le MJ a ouvert la page (`POST /api/acces/joueurs`) et que l'application écoute sur le réseau, les routes ci-dessous répondent aux appareils du réseau local **sans mot de passe** ; toute autre route répond `401`. Un appareil y lit la feuille de n'importe quel PJ ;
- **sur un serveur** : par son compte. Un compte `joueur` n'y voit que le PJ rattaché à son compte (fiche partagée), les combats où ce PJ se bat et leurs hauts faits. Le MJ et l'auditeur voient tout.

| Requête | Effet |
|---|---|
| `GET /api/joueur/etat` | `{"serveur", "pj", "edition", "combat", "univers", "de", "fonctionnalites"}` : `serveur` dit si l'on entre par un compte, `pj` est alors le PJ du compte ; `edition` : les feuilles sont déverrouillées ; `combat` : voir ci-dessous, ou `null` ; `de` : le dé des tests (partie `combat` des règles, 20 à défaut). |
| `GET /api/joueur/pj` | Les PJ parmi lesquels choisir le sien, ni morts, ni partis, ni archivés : `[{"id", "nom", "joueur", "peuple", "profil", "niveau", "statut", "feuille", "date", "recit", "modifie"}]`. `date` : la date en jeu de sa campagne, en toutes lettres (rien d'autre de la campagne) ; `recit` : les champs de sa fiche que le schéma marque `"joueur": true`, `[{"cle", "nom", "texte"}]`. |
| `PATCH /api/joueur/pj/{id}` `{"data": {…}, "modifie"?}` | Sur un serveur, un compte `joueur`, édition ouverte : écrit le récit de son PJ (voir « Comptes joueurs ») ; réponse : le PJ comme dans la liste. Absente de la page libre à la maison (`401`). |
| `GET /api/joueur/hauts-faits` | `{"combats": [{"id", "recap"}], "records": [{"pj", "nom", "combats", "victoires", "infliges", "abattus", "critiques", "touches", "tentatives", "tombe", "meilleur_coup", "meilleur_txt", "meilleur_ou", "titres"}]}` : les combats terminés, du plus récent au plus ancien, et les records de chaque PJ. |
| `GET /api/joueur/des?depuis=`, `POST /api/joueur/des` | Les jets de dés de la table, et un jet du joueur pour son PJ (voir « Lanceur de dés »). |
| `GET /api/joueur/documents?pj=4` | Les aides de jeu reçues par le PJ, de la plus récente à la plus ancienne : `[{"id", "nom", "forme", "image", "recu"}]` (`image` : l'image principale). Sur un serveur, un compte `joueur` lit celles de son PJ, quel que soit `pj` ; à la maison et pour le MJ, `pj` est obligatoire (`404` sinon). |
| `GET /api/joueur/documents/{id}?pj=4` | Une aide reçue par ce PJ : `{"id", "nom", "forme", "image", "recu", "contenu", "images": [{"fichier", "legende"}]}`. Rien d'autre de la fiche : ni le secret, ni la séance prévue, ni les notes ; les liens `[[Nom]]` du texte sont remplacés par leur texte (`![[…]]` retiré). Aide non reçue, archivée ou inexistante : `404`. |
| `GET /api/joueur/documents/{id}/images/{fichier}?pj=4` | Une image de cette aide, si le PJ l'a reçue ; aucune autre image ne passe par là. |
| `GET /api/joueur/direct` | WebSocket, qui ne fait que recevoir : `{"type": "etat", "edition", "combat"}` à la connexion, puis `{"type": "combat", "combat"}` à chaque action, `{"type": "fiche", "id"}` quand une feuille change, `{"type": "edition", "active"}`, `{"type": "hauts_faits"}` quand un combat se termine, `{"type": "jet", "jet"}` à chaque jet public, `{"type": "document", "id", "nom", "pj": [ids]}` quand une aide arrive (un compte `joueur` n'est prévenu que pour son PJ), `{"type": "documents"}` quand un envoi est retiré. `{"type": "ping"}` → `{"type": "pong"}`. Même contrôle d'origine que le suivi d'un combat. |
| `GET /api/entites/{id}/feuille`, `PUT …` | À la maison, sur la page libre : la feuille d'un PJ sans les notes du MJ ; l'écriture demande les feuilles déverrouillées (`403` sinon) et ne touche pas aux notes du MJ. Sur un serveur, un joueur passe par les mêmes routes pour son seul PJ (« Comptes joueurs »). |
| `GET /api/reference?pj=<id>` | Les voies de la feuille de ce PJ, son profil et son peuple ; rien d'autre de la bibliothèque. Sans `pj` : une bibliothèque vide. |

Le combat vu par les joueurs : `{"nom", "round", "ordre": [{"nom", "camp", "pj"?, "a_terre"?, "courant", "suivant", "etats": ["…"]}]}`. `pj` : la fiche du PJ, pour que son joueur reconnaisse son tour ; `a_terre` : PJ inconscient. Les adversaires et les alliés hors de combat n'y figurent pas, et aucun chiffre (PV, DEF, initiative, attaques) n'est transmis. C'est le combat en cours le plus récemment joué.

## Lanceur de dés

Des jets hors combat, vus de toute la table : les jets des joueurs et les jets publics du MJ s'affichent sur la page des joueurs, en direct ; le MJ a aussi des **jets cachés**, que lui seul voit. Les jets ne sont pas rangés dans l'univers : l'application garde les 100 derniers tant qu'elle tourne (ils ne se synchronisent pas, et repartent de zéro au redémarrage).

Formule : des termes ajoutés ou retranchés, chacun un nombre ou un groupe de dés `NdF` (`1d20+3`, `2d6-1`, `1d8+1d6`) ; `kh` garde les meilleurs dés d'un groupe, `kl` les pires (`2d20kh1` : le meilleur de deux d20 ; `4d6kh3`). Au plus 100 dés, 1000 faces, 12 termes et 60 caractères. Une formule illisible répond `422`.

Un jet : `{"id", "quand", "qui", "pj"?, "mj"?, "cache"?, "libelle"?, "formule", "total", "detail", "nat"?, "faces"?}`. `qui` : « MJ » ou le nom du PJ ; `detail` : chaque dé lancé, les dés écartés entre parenthèses (`2d20kh1[(4),17] + 5`) ; `nat` et `faces` : le dé qui compte quand le premier groupe n'en garde qu'un (pour repérer un résultat naturel).

| Requête | Effet |
|---|---|
| `GET /api/des?depuis=12` | `{"jets", "dernier"}` : les jets après `depuis` (tous sans lui), jets cachés compris. MJ et auditeur. |
| `POST /api/des` `{"formule", "libelle"?, "cache"?}` | Un jet du MJ (`201`, le jet). `cache` : vu du MJ seulement. |
| `GET /api/joueur/des?depuis=` | Comme `GET /api/des`, sans les jets cachés. Ouverte à la page des joueurs. |
| `POST /api/joueur/des` `{"formule", "libelle"?, "pj"}` | Un jet d'un joueur, jamais caché. À la maison, `pj` : le PJ choisi sur la page ; sur un serveur, c'est toujours le PJ du compte (`422` s'il n'en a pas). Seule écriture de la page libre avec la feuille : elle ne touche pas à l'univers. |

## Combats

Les combats vivent dans l'univers, en cours ou terminés. Le serveur est la seule source de vérité : une action est appliquée par le moteur avec les règles de l'univers, enregistrée, puis diffusée aux écrans qui suivent le combat. Les PV, les points de chance et les points de mana des PJ sont reportés sur leur feuille de personnage à chaque action, annulation comprise. Réservés au MJ (l'auditeur lit et suit, sans agir).

| Requête | Effet |
|---|---|
| `GET /api/combats` | Les combats, ceux en cours d'abord : `[{"id", "nom", "round", "termine", "updated", "pj", "ennemis", "total", "issue"?, "rencontre"?, "campagne"?, "seance"?}]`. `?seance=12` : ceux d'une séance ; `?en_cours=1` : ceux qui ne sont pas terminés. |
| `POST /api/combats` `{"nom"?, "rencontre"?, "campagne"?, "pj"?: [ids]}` | Lance un combat. Avec `rencontre` : ses créatures, ses PNJ présents qui ont un profil de combat (ils gardent leur nom et le lien vers leur fiche) et les PJ en jeu de sa campagne, ni absents, ni morts, ni partis. `pj` remplace ce choix ; `campagne` seule prend ses PJ. Sans rien : un combat vide. Le combat est rattaché à la séance de la campagne que montre l'écran du MJ. Réponse `201` `{"combat": {…}, "absents": ["…"]}` ; `absents` : ce qui n'a pas pu entrer (PJ sans feuille de personnage, PNJ sans profil, monstre retiré du bestiaire). |
| `GET /api/combats/{id}` | `{"id", "etat", "annulable"}` : l'état complet (`nom`, `round`, `tour`, `ordre`, `cbts`, `log`, `resultat`, `test`, `termine`, `fin`, `recap`, `courbe`, `rencontre_id`, `campagne`, `seance`) et le nombre d'actions annulables. Un combattant : `{"id", "nom", "camp", "ref": {"kind", "id"}, "init", "defense", "pvMax", "pv", "rd", "crit", "pc", "pm"?, "pmMax"?, "carac", "attaques", "capacites", "voies"?, "utilise"?, "etats", "act", "cible"}` ; `ref.kind` : `monstre` (bestiaire), `pnj` ou `pj` (fiche). `pm` / `pmMax` : points de mana (ceux de la feuille d'un PJ ; absents : le combattant n'en a pas). `voies` : les capacités acquises dans les voies d'un PJ (jusqu'à son rang) ; `utilise` : les capacités à usage limité déjà utilisées dans ce combat. `actives` (PJ) : ses capacités de la bibliothèque qui ont une action en combat ou des utilisations comptées, `{"cle", "nom", "voie", "rang", "texte", "limitee", "action", "fois", "par", "deja"}` (`deja` : ce qui en a servi avant le combat, lu sur la feuille) ; `usages` : `{"<clé>": n}`, leurs utilisations dans ce combat. Celles qui ne sont pas « par combat » sont reportées sur la feuille (`utilisations`), comme les PV, annulation comprise. |
| `PATCH /api/combats/{id}` `{"nom"?, "seance"?}` | Renomme le combat ou le rattache à une autre séance (`0` : aucune). |
| `DELETE /api/combats/{id}` | Supprime le combat. |
| `POST /api/combats/{id}/actions` `{"type": "…", …}` | Applique une action et renvoie le nouvel état, comme `GET`. Une action que les règles refusent répond `422` avec la raison (`"detail"`). |
| `GET /api/combats/{id}/direct` | WebSocket : `{"type": "etat", "id", "etat", "annulable"}` à la connexion puis à chaque changement, `{"type": "supprime"}` si le combat est supprimé. La connexion ne fait que recevoir (`{"type": "ping"}` → `{"type": "pong"}`) ; elle n'est ouverte qu'à une page servie par l'application (`Origin`). |

Actions (`type`) : `suivant`, `precedent`, `trier`, `deplacer` (`id`, `sens` : `haut`/`bas`), `ajouter` (`kind` : `monstre`, `pnj` ou `pj` avec `ref_id` — la fiche est lue par le serveur — ou saisie libre avec `fiche` ; `qte`, `camp`), `retirer` (`id`), `modifier` (`id`, `champs` : `nom`, `camp`, `init`, `defense`, `pv`, `pvMax`, `rd`, `crit`, `pc`, `pm`, `pmMax`, `capacites`, `attaques`), `pv` (`id`, `sens` : `degats`/`soin`, `valeur`), `pm` (`id`, `sens` : `depense`/`gain`, `valeur` ; `422` au-delà des PM restants, la récupération s'arrête au maximum), `usage` (`id`, `cle` : nom de la capacité, 80 caractères au plus ; `valeur` : `true` la marque utilisée dans ce combat, `422` si elle l'est déjà ; `false` la rend), `attaque` (`att`, `cible`, `attaque` : rang de l'attaque, `act` : `A`, `L`, `LS` ou `G`, `mode`, `nat`, `divers`, `dm`, `cout_pm` : points de mana d'un sort, dépensés au lancer qu'il touche ou non, `422` si l'attaquant n'en a pas assez ; `capacite` : la clé d'une capacité active de l'attaquant, qui compte une utilisation, `422` s'il n'en reste plus), `capacite` (`id` : le PJ, `capacite` : la clé, `cible`, `cout_pm`, `dm` et `soin` : formules de dés ou nombres, `etat`, `duree` : une capacité sans jet d'attaque, telle que le MJ l'a validée ; PM dépensés, utilisation comptée, dégâts ou soins lancés sur la cible, état posé ; `422` sans PM ou utilisation restante, ou sur une formule refusée, et rien n'est alors dépensé), `chance` (`cle` : `test` pour un test), `appliquer_dm` (`valeur` ; `cle` : `des` pour un total lancé à la table), `ignorer_dm`, `test` (`id`, `carac`, `difficulte` ou `opposant` et `carac_opp`, `nat`, `nat_opp`, `divers`, `etat_echec`, `duree`), `fermer_test`, `etat` (`id`, `etat`, `duree`), `retirer_etat`, `defense` (`etat`), `action_utilisee` (`cle`, `valeur`), `vider_journal`, `terminer`, `continuer`, `rouvrir`, `annuler`.

Dés : `mode` absent, l'application lance tout ; `manuel`, le dé d'attaque est saisi (`nat`) et l'application lance les dégâts ; `reel`, le dé puis le total des dégâts sont saisis (`appliquer_dm` avec `"cle": "des"`), l'application applique le critique et la RD. Un état que les règles ne décrivent pas se pose quand même (40 caractères au plus) : il n'a aucun effet.

## Feuille de personnage

Chaque PJ a une feuille de personnage (caractéristiques, ressources, attaques, voies), qui complète sa fiche : le nom, le joueur, le peuple, le profil, le **niveau** et les textes restent sur la fiche et ne se saisissent pas deux fois. Ses bornes et ses valeurs de départ viennent de la partie `"personnage"` du fichier de règles ; sans elle, la feuille n'impose rien.

| Requête | Effet |
|---|---|
| `GET /api/entites/{id}/feuille` | La feuille du PJ, avec `"entite"`, `"niveau"`, `"modifie"`, `"existe"` (`false` : feuille vierge, pas encore enregistrée) et `"emplacements"` (ceux de l'inventaire, voir plus bas). `422` si la fiche n'est pas un PJ. |
| `PUT /api/entites/{id}/feuille` | Enregistre la feuille. `"modifie"` (facultatif) : la date connue du client ; `409` si la feuille a changé depuis. `422` hors des bornes des règles. |

Champs saisis : `sexe`, `age`, `taille_physique`, `poids`, `divinite`, `langues`, `valeurs` (`{"FOR", "DEX", "CON", "INT", "SAG", "CHA"}`), `dv`, `pv`, `pv_actuel`, `rd`, `crit`, `def_divers`, `init_divers`, `att_divers` (`{"C", "D", "M"}`), `carac_magie`, `attaques` (`[{"nom", "type", "mod", "dm", "portee", "notes"}]`), `pc`, `pc_max`, `pr`, `pr_max`, `pm`, `pm_max`, `voies` (`[{"nom", "origine", "rang", "capacites", "ref"}]`), `capacites`, `inventaire` (voir plus bas), `bourse` (`{"po", "pa", "pc"}`), `notes` (notes du MJ).

Champs calculés à chaque enregistrement et à chaque lecture : `carac` (modificateurs), `init` (valeur de DEX + divers), `defense` (base des règles + mod. DEX + divers ; l'armure et le bouclier sont des objets portés), `bonus` de chaque attaque (niveau + modificateur de FOR, de DEX ou de la caractéristique de magie + divers + `mod`). Les ressources sont bornées par leur maximum.

**Bonus des capacités.** Une voie de la feuille liée à la bibliothèque (même `ref`, sinon même nom) apporte les `bonus` des capacités jusqu'au rang atteint ; une voie libre n'en apporte aucun. Les valeurs calculées (`carac`, `init`, `defense`, `bonus` des attaques) les comprennent ; les valeurs saisies (`valeurs`, `pv`, `pm_max`, `rd`) ne changent pas. Les PV et les PM actuels sont bornés par le maximum saisi plus les bonus. La réponse porte alors `"effets"` : `{"base", "bonus", "detail"}`, où `base` donne chaque valeur sans les capacités et `bonus` ce qu'elles ajoutent (clés `FOR`…`CHA`, `def`, `init`, `pv`, `pm`, `rd`, `att_C`, `att_D`, `att_M` ; seulement les non nulles), et `detail` chaque bonus (`{"cible", "valeur", "capacite", "voie", "rang"}`, ou `{"cible", "valeur", "objet"}` pour celui d'un objet porté). `effets` n'est jamais rangé dans l'univers : il se recalcule à chaque lecture, et ce qui est envoyé dans un `PUT` est ignoré. Un PJ qui entre en combat y arrive avec ces totaux.

**Inventaire.** `inventaire` : `[{"id", "nom", "quantite", "porte", "emplacement", "bonus", "arme", "notes"}]`. `id` est propre à la feuille : le garder pour un objet existant, `0` ou absent pour un nouveau (le serveur le donne). `quantite` : 1 à 9999. `emplacement` : la clé d'un des `"emplacements"` de la réponse (`[{"cle", "nom", "nombre", "occupe"}]` : ceux de `personnage.emplacements` du fichier de règles, sinon une liste générique : tête, cou, épaules, corps, mains, taille, pieds, deux anneaux, main principale, main secondaire, deux mains). Un objet `porte` a un emplacement ; un emplacement ne prend que `nombre` objets portés (1 par défaut), et un objet porté à un emplacement qui en `occupe` d'autres les prend tous (deux mains). Sinon `422` (« Tête : « Heaume » ne se porte pas avec « Casque » »). Un emplacement que les règles ne connaissent plus reste accepté, avec une place. `bonus` : `[{"cible", "valeur"}]`, mêmes cibles que les bonus des capacités, de −20 à +20 ; `arme` : `{"type": "C"|"D"|"M", "dm", "mod", "portee"}`. Seul un objet porté compte : ses bonus s'ajoutent comme ceux des capacités (avec `objet` dans `effets.detail`), et son arme ajoute une ligne à `attaques`, avec `"objet": <id>` ; ces lignes se recalculent et ne se rangent pas (celles reçues dans un `PUT` sont ignorées).

Quand le joueur écrit sa feuille, un objet qui existait garde ses `bonus`, son `arme` et son `emplacement` ; un objet qu'il ajoute n'a ni bonus ni arme. Il change les noms, les quantités, les notes et ce qu'il porte.

**Objets du monde.** Une ligne peut désigner une fiche « Objet » (`"fiche": <id>`, une fois par inventaire ; une autre fiche : `422`). Elle en prend le nom et les chiffres, rangés dans le champ `equipement` de la fiche (`{"emplacement", "bonus": [{"cible", "valeur"}], "arme": {"type", "dm", "mod", "portee"}}`). L'emplacement d'une fiche se donne par sa clé ou par son nom, sans tenir compte des majuscules ni des accents (« Tête », « deux mains »), et se range par sa clé ; un emplacement inconnu : `422`, avec la liste. Il en va de même dans l'inventaire : « tête » et `tete` sont le même emplacement ; ce qu'envoie le client pour ces valeurs est ignoré. Le **détenteur** de la fiche suit dans les deux sens : la lier dans l'inventaire d'un PJ en fait le détenteur (et la retire de l'inventaire d'un autre PJ) ; l'en ôter efface le détenteur ; changer le détenteur de la fiche (`PATCH /api/entites/{id}`) la retire de l'inventaire de l'ancien PJ et la met dans celui du nouveau ; changer son nom ou ses chiffres met la ligne à jour (un objet porté dont l'emplacement change est ôté). Une fiche effacée reste dans l'inventaire, en objet courant. Le joueur ne lie ni ne retire un objet du monde (`403`). Ces règles ne jouent que si les feuilles de personnage sont activées. `GET /api/emplacements` : les emplacements de l'univers (ceux du fichier de règles, sinon la liste générique), pour saisir les chiffres d'un objet.

Une feuille d'avant l'inventaire (champs `equipement`, `armure`, `bouclier`) est reprise à l'ouverture de l'univers : une ligne par ligne du texte (« 3 torches » : trois torches), l'armure et le bouclier en objets portés (`corps`, `main_secondaire`) avec leur bonus de DEF.

**Utilisations des capacités.** `utilisations` (champ saisi) : `{"<clé>": {"par", "n"}}`, ce qui a servi des capacités comptées « par jour », « par repos » (`repos:<clé>`) ou « à la main » depuis leur remise à zéro ; la clé d'une capacité est le nom de sa voie sans accents ni majuscules, `#`, son rang (`voie du gue#2`). Celles « par combat » se comptent dans le combat seulement. La réponse porte aussi `"comptees"` : les capacités atteintes qui ont des utilisations comptées (`{"cle", "nom", "voie", "rang", "fois", "par", "deja"}`). Un repos des règles remet à zéro celles « par » ce repos, et celles « par jour » s'il a `"nouveau_jour": true`.

| `POST /api/feuilles/nouveau-jour` `{"pj": [ids]}` | Un nouveau jour : les capacités « par jour » de ces PJ (`pj` absent : tous) redeviennent utilisables. Réponse `{"feuilles"}` : le nombre de feuilles changées. Suit la fonctionnalité « feuilles ». |

**Montée de niveau.** Le MJ la lance : il la fait lui-même, ou l'**accorde** et le joueur la prend depuis sa page en faisant ses choix (PV, rangs). Le niveau reste sur la fiche du PJ ; la feuille garde l'historique. Les routes `/api/feuilles/…` sont refusées à l'auditeur et aux joueurs.

| Requête | Effet |
|---|---|
| `POST /api/feuilles/{id}/niveau` `{"des": [ ], "pv", "rangs": [{"voie", "rang"}], "etapes": {"<clé>": "texte"}, "modifie"}` | Le PJ passe au niveau suivant (un seul à la fois), sur sa fiche et sa feuille en une écriture, notée au journal (« montée de niveau »). **PV gagnés** : si les règles en donnent la formule (`personnage.montee.pv`), `des` porte le résultat de chaque dé de vie lancé en vrai (entre 1 et les faces du dé de vie de la feuille), et l'application ajoute le reste de la formule ; `pv` est alors ignoré. Sans formule, `pv` (0 à 999) est requis. Rien n'est annoncé au lanceur de dés de la table. **PM** : les PM max (et actuels) gagnent ce que les règles donnent au profil du PJ (`montee.pm`). **Rangs** : les rangs pris dans les voies **déjà sur la feuille** (`voie` : son indice dans `voies`, `rang` : le rang atteint), au prix que fixent les règles. **Étapes** : ce que le joueur note aux étapes que les règles prévoient à ce niveau (`montee.etapes`, requis, 200 caractères au plus), ajouté en une ligne aux `capacites` de la feuille. Réponse : la feuille, comme `GET …/feuille`, avec `"montee"` (la ligne ajoutée à l'historique). `422` : feuille absente, dé de vie manquant ou hors du dé, niveau maximum atteint, rang déjà pris, au-delà de `rang_max` ou du niveau qu'il demande, points insuffisants, étape non remplie ; `409` : feuille changée depuis `modifie`. |
| `POST /api/feuilles/{id}/niveau/accorder` `{"accorde", "modifie"}` | `"accorde": true` accorde au PJ le niveau suivant (`niveau_accorde` de la feuille), que son joueur prendra lui-même ; `false` retire l'accord. Le joueur en est prévenu en direct. Réponse : la feuille. `422` si la feuille n'existe pas ou au-delà de `niveau_max`. |
| `POST /api/joueur/pj/{id}/niveau` `{"des", "pv", "rangs", "etapes", "modifie"}` | Le joueur prend le niveau que le MJ a accordé à son PJ, comme `POST /api/feuilles/{id}/niveau`, même feuilles verrouillées. Sur un serveur : le PJ de son compte (`404` pour un autre) ; à la maison : la page des joueurs ouverte. `403` sans accord en cours (ou s'il ne vise plus le niveau suivant). La réponse est sans les notes du MJ. L'accord tombe une fois le niveau pris, ou si le MJ fait la montée lui-même. |
| `POST /api/feuilles/{id}/niveau/annuler` `{"modifie"}` | Défait la dernière montée : niveau de la fiche, PV max, rangs et points gardés reviennent à ce qu'ils étaient, la ligne quitte l'historique (journal : « montée de niveau annulée »). Réponse : la feuille, avec `"annulee"` (la ligne retirée). `422` si l'historique est vide, ou si le niveau de la fiche, les PV max ou les voies ont changé depuis la montée (à corriger à la main). |

Champs de la feuille ajoutés par les montées : `niveaux`, l'historique (`[{"niveau" : le niveau atteint, "quand", "pv" : les PV max gagnés, "jet" : le détail des PV (dés saisis et reste de la formule), vide si les PV ont été saisis, "pm" : les PM max gagnés, "rangs": [{"voie", "ref", "de", "a", "capacites" : les capacités débloquées}], "points" : les points de capacité dépensés, "reste" : ceux gardés avant, "etapes": [{"cle", "libelle", "texte"}]}]`), `points_capacite`, les points gardés pour le niveau suivant, et `niveau_accorde`, le niveau accordé par le MJ et pas encore pris (absent : aucun). Le MJ peut les réécrire par `PUT …/feuille` ; un joueur jamais (ce qu'il envoie est remplacé par ce qui est rangé). La réponse de `GET …/feuille` porte aussi `"montee_pm"` : les PM max que ce PJ gagne par niveau, selon son profil (absent : aucun). L'annulation retire aussi les PM gagnés et les lignes que les étapes ont ajoutées aux capacités.

Partie `"personnage"` du fichier de règles, toutes clés facultatives : `niveau_max`, `valeur_min`, `valeur_max`, `des_de_vie`, `de_de_vie`, `caracs_magie`, `carac_magie`, `defense_base`, `pc_max`, `pr_max`, `rang_max`, `origines_voie`, `emplacements` (ceux de l'inventaire, `[{"cle", "nom", "nombre", "occupe": ["<clé>"]}]`, trente au plus ; absents : la liste générique, voir « Inventaire »), `montee` et `repos` : les boutons de repos de la feuille, `[{"cle", "libelle", "aide", "cout_pr", "gain_pr", "pv": ["dv", "con", "niveau"], "pv_max", "pm_max", "nouveau_jour"}]` (`pv` : les PV rendus, somme d'un dé de vie lancé, du modificateur de CON et du niveau). La réponse de `GET …/feuille` joint ces règles dans `"regles"`. `niveau_max` borne aussi le champ niveau de la fiche du PJ (`422` au-delà). `montee` : ce que gagne un PJ qui passe un niveau, `{"pv": ["dv", "con", "niveau"], "pv_fixe", "pv_min", "points", "couts": [ ], "couts_origine": {"<origine>": [ ]}, "niveau_min": [ ], "report", "pm": [{"profils": [ ], "gain"}], "etapes": [{"cle", "libelle", "aide", "niveaux": [ ]}], "aide"}` — `pv` et `pv_fixe` : la formule des PV gagnés (un dé de vie, lancé en vrai et saisi, par `dv` ; le modificateur de CON ; le nouveau niveau ; une valeur fixe), au moins `pv_min` ; sans formule, les PV se saisissent. `points` : les points de capacité gagnés par niveau ; `couts` : le coût de chaque rang de voie (le dernier vaut pour les suivants ; vide : 1) ; `couts_origine` : de même pour les voies d'une origine (`origine` de la voie sur la feuille) ; `niveau_min` : le niveau qu'il faut pour prendre chaque rang (0 : aucun) ; `report` : les points non dépensés se gardent. Sans `points`, les rangs se prennent librement. `pm` : les PM max gagnés par niveau par un PJ de ces profils (profil de sa fiche, sans tenir compte des majuscules). `etapes` : des écrans de plus, aux niveaux donnés, où le joueur note un choix.

En mode serveur : le MJ écrit, l'auditeur lit. Un joueur ne lit que la feuille de son propre PJ (rattaché à son compte, fiche partagée), sans `notes` ; il l'écrit quand l'édition par les joueurs est ouverte, et les notes du MJ restent en place. Il monte de rang dans ses voies et écrit des voies libres ; une voie de la bibliothèque qui n'était pas sur la feuille (même tapée à la main) est refusée (`403`) : seul le MJ l'ajoute. Même règle sur la page des joueurs à la maison. L'export de l'univers contient les feuilles (`"feuilles"`).

## Écran du MJ

Ce que l'écran du MJ (`#/table`) affiche. Ces réponses contiennent les champs réservés au MJ.

| Requête | Effet |
|---|---|
| `GET /api/table?quete=12` | La partie en cours : `{"quetes", "quete", "campagne", "seance", "pj", "scene", "rencontres"}`. `quetes` : les quêtes `en cours`, la dernière modifiée d'abord. `quete` : celle demandée, sinon la première de `quetes` (fiche complète, `null` s'il n'y en a pas ; `404` si l'identifiant n'est pas une quête). `campagne` : celle de la quête, sinon la campagne `en cours`, sinon la seule qui existe. `seance` : la prochaine séance `prévue` de la campagne, sinon la dernière. `pj` : `[{"fiche", "statut", "peuple", "vie"?}]`, les PJ de la campagne sauf `mort` et `parti`, les `absent` en dernier ; `vie` : `{"pv", "pv_max", "defense", "init", "pc", "pc_max", "pr", "pr_max", "pm", "pm_max"}`, lue sur leur feuille de personnage (absente sans feuille). `scene` : `[{"fiche", "resume", "statut", "raison", "secret"}]`, les PNJ, lieux, factions, objets, lore, événements et secrets liés à la quête, chacun une fois ; `raison` dit pourquoi (nom du champ, libellé de la relation, `rencontre « … »`, `cité`), `secret` qu'un champ réservé au MJ est rempli. `rencontres` : `[{"fiche", "resume", "lieu", "creatures"}]`, celles de la quête (`creatures` : « 4 × Brigand, 1 × Chef »). |
| `GET /api/entites/{id}/preparation` | La préparation d'une séance (`404` si la fiche n'en est pas une), d'après sa campagne : `{"seance", "campagne", "precedente": {"fiche", "resume"}, "lieu_groupe", "date", "fils_rouges", "pj", "quetes": [{"fiche", "statut", "categorie", "objectif", "etapes_faites", "etapes_restantes", "indices", "lieu", "commanditaire"}], "quetes_a_venir", "secrets": [{"fiche", "resume", "statut", "importance", "indices", "ignorent"}], "pnj": [{"fiche", "resume", "statut", "raison", "secret"}], "rencontres": [{"fiche", "resume", "lieu", "creatures", "quete"}]}`. `precedente` : la séance jouée de plus grand numéro inférieur au sien (sans numéro : la dernière jouée modifiée). `quetes` : celles `en cours`, l'arc principal d'abord. `secrets` : ceux de la campagne qui ne sont pas `révélé`, du plus important au moins important ; `ignorent` : les PJ en jeu (ni absents, ni morts, ni partis) sans relation `sait`. `pnj` : ceux de la scène des quêtes en cours (comme l'écran du MJ), puis ceux dont « Où le trouver » est le lieu du groupe ou l'un de ses sous-lieux directs ; jamais les morts. `rencontres` : celles de la campagne ou de ses quêtes en cours qui ne sont ni `jouée` ni `évitée`. Rien n'est écrit : l'écran compose le texte et l'enregistre dans le champ `preparation` de la séance (réservé au MJ). Une séance sans campagne : tout est vide. |
| `POST /api/entites/{id}/apres-seance` | Clôt une séance en une fois : `{"resume"?, "presents"?: [ids], "date_jeu"?, "lieu_groupe"?, "quetes"?: [{"id", "etapes"?: [bool], "statut"?}], "secrets"?: [{"id", "qui": [ids], "note"?}], "morts"?: [ids], "rencontres"?: [{"id", "statut"?}], "suivante"?}`. La séance passe `jouée` (avec résumé, présents, date en jeu) ; la date en jeu et `lieu_groupe` vont aussi sur la campagne ; `etapes` donne le « fait » de chaque étape dans l'ordre de la fiche (`422` si leur nombre a changé) ; les secrets sont révélés comme par `reveler`, avec la séance ; les PNJ de `morts` passent `mort` ; une rencontre passe `jouée` (ou le `statut` donné) et se rattache à la séance ; `suivante` crée la séance suivante (`prévue`, numéro + 1, même campagne, même date et mêmes présents). Tout est contrôlé avant la première écriture : une valeur refusée (`422`) n'en laisse passer aucune. Réponse `{"seance", "modifiees", "revelations", "suivante"}`. |
| `GET /api/table/etat?campagne=3` | Ce qui bouge pendant la partie, à relire en boucle : `{"pj": {"<id de fiche>": {…vie…}}, "combat": {…} \| null}`. `combat` : le résumé (comme dans `GET /api/combats`) du combat en cours le plus récent, celui de la campagne d'abord. |

La page n'affiche aucun cadre. Sa politique de sécurité n'admet de WebSocket que vers l'application elle-même (`connect-src 'self' ws://<hôte> wss://<hôte>`), pour le suivi d'un combat.

## Import

| Requête | Effet |
|---|---|
| `POST /api/import/fiches` `{"fiches": [{"type", "nom", "data", "tags"}]}` (ou la liste seule) | Crée les fiches ; celles dont le type et le nom existent déjà sont ignorées. Réponse `{"crees", "ignores", "erreurs"}`. |

## Images

| Requête | Effet |
|---|---|
| `POST /api/entites/{id}/medias` | Ajoute une image (PNG, JPEG, WebP, GIF ; 20 Mo au plus) : formulaire multipart avec le champ `fichier`, ou contenu brut de l'image avec `?nom=portrait.png`. La première image devient l'image principale. |
| `PATCH /api/medias/{id}` `{"legende": "…", "principal": true}` | Légende, image principale. |
| `DELETE /api/medias/{id}` | Supprime. |
| `GET /medias/{fichier}` | L'image (rangée dans le fichier de l'univers). |

## Claude (application de bureau)

Inscrire le serveur MCP de cette application (`<programme> -mcp`) dans Claude Desktop ou Claude Code de cet ordinateur. Sur l'ordinateur du MJ seulement (`403` depuis le réseau, même avec le jeton) ; ces routes n'existent pas sur un serveur.

| Requête | Effet |
|---|---|
| `GET /api/claude` | `{"programme", "args", "cibles": [{"client": "desktop" \| "code", "nom", "chemin", "present", "branche", "a_jour", "erreur"?}], "commande_code"}`. `present` : le client semble installé ; `a_jour` : l'entrée lance ce programme avec ces arguments ; `commande_code` : la commande équivalente pour Claude Code. |
| `POST /api/claude` `{"client": "desktop", "actif": true}` | Écrit (ou met à jour) l'entrée `cof-dm` de `mcpServers` dans chaque configuration présente de ce client ; `false` la retire. Le reste du fichier est gardé ; la première fois, une copie `….avant-cof-dm` est faite. Client absent : `404` ; fichier qui n'est pas du JSON : `422`, laissé tel quel. Journalisé. |

## Fonctionnalités

Certaines parties de l'application sont optionnelles, réglées **univers par univers** (le réglage est rangé dans l'univers et se synchronise) : `ecran_mj`, `seances` (préparation et après-séance), `combat`, `rencontres`, `secrets`, `feuilles`, `page_joueurs`, `des`, `aides`, `regles`, `graphe`, `chronologie`, `organigramme`, `obsidian`. Coupées par défaut dans un univers neuf : `des`, `aides`, `feuilles`, `page_joueurs`. Un univers d'avant cette version garde ce dont il se sert (des feuilles de personnage : `feuilles` et `page_joueurs` ; des aides de jeu : `aides`).

| Requête | Effet |
|---|---|
| `GET /api/fonctionnalites` | `[{"cle", "nom", "description", "groupe", "defaut", "types"?, "active"}]`. |
| `PUT /api/fonctionnalites` `{"des": true, "combat": false}` | Active ou coupe ; les autres gardent leur état. Clé inconnue : `422`. Journalisé. Réservé à qui écrit (MJ ; ni auditeur, ni joueur). Couper `page_joueurs` déconnecte les appareils qui la suivaient. |

Une fonctionnalité coupée :

- ses routes répondent `404` `{"detail": "Fonctionnalité désactivée dans cet univers : …", "fonctionnalite": "<clé>"}` : `/api/des` et `/api/joueur/des` (`des`), `/api/aides` et `/api/joueur/documents` (`aides`), `/api/entites/{id}/feuille` (`feuilles`), `/api/combats` et `/api/joueur/hauts-faits` (`combat`), `/api/entites/{id}/difficulte` (`rencontres`), `/api/secrets` (`secrets`), `/api/table` (`ecran_mj`), `/api/entites/{id}/preparation` et `/apres-seance` (`seances`), `/api/graphe`, `/api/chronologie`, `/api/entites/{id}/organigramme`, `/api/export/obsidian`. Toute route `/api/joueur/…` et `POST /api/acces/joueurs` demandent `page_joueurs` ; à la maison, la page libre se ferme (`401`) ;
- ses types de fiches (`rencontre`, `secret`, `aide`, `regle`) quittent le schéma, et avec eux les champs qui ne désignent qu'eux ; ils sortent de `GET /api/entites` et ne se créent plus (`422`, import compris). Une fiche existante reste lisible par `GET /api/entites/{id}` ;
- la préparation d'une séance ne remonte plus ses secrets (`secrets`) ni ses rencontres (`rencontres`) ;
- rien n'est effacé : ses données restent dans l'univers, voyagent avec la synchronisation et l'export, et reviennent à la réactivation.

Le serveur MCP ne propose pas les outils d'une fonctionnalité coupée (il lit `GET /api/fonctionnalites` à chaque `tools/list`).

## Autres

| Requête | Effet |
|---|---|
| `GET /api/schema` | Types, champs, relations, calendrier, genres de champ, et `fonctionnalites` (`{"des": false, …}`, voir « Fonctionnalités »). Les types et les champs d'une fonctionnalité coupée n'y figurent pas. |
| `GET /api/stats` | Comptes par type, liens morts, journal récent, campagnes, séances, quêtes en cours. |
| `GET /api/resoudre?nom=Mont-Veille` | La fiche qui porte ce nom et sa phrase de présentation. |
| `GET /api/liens-morts` | Les `[[Nom]]` qui ne mènent à aucune fiche, et où ils sont. |
| `GET /api/graphe?id=12&profondeur=2` | Fiches liées (relations, champs, mentions) : `{"noeuds", "aretes"}`. Sans `id` : tout le monde. |
| `GET /api/chronologie?campagne=3` | Toutes les dates en jeu, dans l'ordre (avec une campagne : la campagne et l'histoire du monde). |
| `GET /api/calendrier`, `PUT /api/calendrier` | Calendrier du monde : `{"ere", "mois": [{"nom","jours"}], "semaine": [...]}`. |
| `GET /api/journal?limite=100` | Dernières modifications (avec leur auteur). |
| `GET /api/export` | Toutes les données de l'univers en JSON, images comprises (champ `contenu` de chaque média, en base64). |
| `GET /api/export/obsidian` | L'univers en notes Markdown pour un vault Obsidian, dans un zip (`cof-dm-obsidian-<date>.zip`) : une note par fiche (propriétés, sections, relations, images), la chronologie, les récapitulatifs des combats terminés et le journal. `?secrets=0` : seulement ce que voient les joueurs (fiches partagées, sans champ ni relation réservés au MJ, sans note de relation, sans combat ni journal) ; le zip s'appelle alors `…-joueurs.zip`. Fermé au rôle joueur. |
| `POST /api/restaurer` | Remplace tout par un export (sauvegarde faite avant). L'export est d'abord validé comme une saisie (noms, champs et références des fiches, relations, images, feuilles, bestiaire, règles) : au moindre refus, rien n'est modifié (422, avec la fiche ou la relation en cause). Un export d'un modèle plus ancien est remis en forme ; un export d'une version plus récente de COF DM est refusé. Un export sans le contenu des images garde celles de l'univers qui portent le même nom de fichier. |
| `POST /api/sauvegarde` | Copie l'univers entier dans un fichier de sauvegarde : `{"fichier": "…/sauvegardes/<id>/<date>_manuelle.cofdm"}`. |
| `GET /api/parametres` | `{"version", "adresse", "jeton", "univers": {"nom", "id", "chemin"}, "donnees", "sauvegardes"}`. Depuis une session ouverte par mot de passe : ni jeton, ni chemins. |

## Univers

Un univers est un fichier `.cofdm` qui contient tout : fiches, relations, images, calendrier et journal. L'application de bureau en ouvre un à la fois, et toutes les autres routes portent sur l'univers ouvert (un serveur les tient tous ouverts : voir « Plusieurs univers »). Le mot de passe MJ, le jeton d'API et le serveur relié tiennent à l'ordinateur (`reglages.json` du dossier de l'application), pas à l'univers.

Ces routes touchent aux fichiers de l'ordinateur du MJ : elles ne répondent **qu'à lui** (`403` depuis le réseau, même avec le jeton ou une session).

| Requête | Effet |
|---|---|
| `GET /api/univers` | `{"courant": {"nom", "chemin", "id", "taille"}, "recents": [...], "autres": [...]` (univers du dossier par défaut), `"dossier"` (dossier par défaut), `"parcourir"` (une boîte de dialogue du système existe), `"avertissement"`, `"dossier_sauvegardes"`, `"sauvegardes": [{"fichier", "quand", "motif", "taille"}]}`. |
| `POST /api/univers/ouvrir` `{"chemin": "…"}` | Ouvre un fichier d'univers existant. `422` : fichier absent, qui n'est pas un univers, abîmé (contrôle d'intégrité de SQLite) ou qui contient un objet qu'un univers n'a jamais (déclencheur, vue, table virtuelle autre que l'index de recherche) ; l'univers courant reste alors ouvert et le fichier n'est pas modifié. |
| `POST /api/univers/nouveau` `{"nom": "…"}` ou `{"chemin": "…"}` | Crée un univers vide (dans le dossier par défaut, ou au chemin donné) et l'ouvre. `409` : le fichier existe déjà. |
| `POST /api/univers/deposer?nom=…` | Serveur seulement (administrateur) : ajoute un univers existant, dont le fichier `.cofdm` est le corps brut de la requête (1 Go au plus). Le fichier est contrôlé avant d'entrer dans le dossier des univers ; l'administrateur passe ensuite dans cet univers (réponse comme `GET /api/univers`). `409` : nom déjà pris, ou copie d'un univers déjà sur le serveur ; `413` : trop lourd ; `422` : pas un univers, abîmé, ou qui contient un objet étranger (voir `ouvrir`), ou application de bureau (ouvrir le fichier directement). |
| `GET /api/univers/version` | Serveur seulement (administrateur) : `{"version", "protocole", "modele", "canal", "verifie", "erreur", "disponible", "commande", "univers", "copies"}`. Le serveur cherche les versions stables publiées au lancement puis toutes les six heures : `disponible` (`{"version", "notes", "page", "date"}`) quand une version plus récente est publiée, et `commande`, celle qui le met à jour (il n'installe rien). `copies` : les postes reliés à l'univers visé, `{"numero", "nom", "compte", "vu", "version", "ecart"}`, `ecart` valant `serveur_recent` (poste à mettre à jour : il ne se synchronise plus), `bureau_recent` (le poste attend la mise à jour du serveur) ou vide. |
| `POST /api/univers/copie` `{"chemin": "…", "remplacer": false}` | Enregistre une copie complète de l'univers ouvert, qui reste ouvert. `409` : le fichier existe et `remplacer` n'est pas `true`. |
| `POST /api/univers/parcourir` `{"enregistrer": false, "nom": "…"}` | Ouvre la boîte « Ouvrir » ou « Enregistrer » du système : `{"chemin": "…"}` (`""` si le MJ annule). `501` : pas de boîte de dialogue sur ce système. |
| `POST /api/univers/restaurer` `{"fichier": "<date>_<motif>.cofdm"}` | Remet l'univers ouvert dans l'état d'une de ses sauvegardes ; l'état actuel est sauvegardé avant (`motif` `avant-restauration`). |

Sauvegardes automatiques : à l'ouverture d'un univers, toutes les demi-heures et à la fermeture, s'il a des fiches et a changé depuis la dernière. Les cinq dernières sont gardées, puis la dernière de chaque jour pendant quatorze jours ; les sauvegardes `manuelle` et `avant-restauration` ne sont jamais effacées.

## Synchronisation

Une application de bureau garde une **copie** d'un univers du serveur et la tient à jour, y compris après un travail hors ligne. Ces routes n'existent qu'en mode serveur, pour les comptes qui écrivent dans l'univers (MJ, administrateur) ; l'auditeur et le joueur en sont exclus.

L'univers y est vu champ par champ : `{"objet", "cle", "champ", "valeur", "seq"}`. `objet` vaut `entite` (fiche), `relation`, `media` (image), `document` (bestiaire, profil, peuple, voie, combat terminé), `feuille`, `meta` (`calendrier`, `regles`, `fonctionnalites`, `edition_joueurs` ; `reference` : l'ancien réglage des profils et voies, envoyé par un serveur d'avant le modèle 2, que la copie range aussitôt un par un) ou `journal`. `champ` vaut `nom`, `type`, `tags`, `archive`, `partage`, `cree`, `data.<clé>` pour une fiche ou une feuille, le nom de la colonne pour une relation ou une image, `data` pour un document, `valeur` pour un réglage, `ligne` pour le journal. Le champ `*` dit qu'un objet existe ; `null`, qu'il est supprimé. Un combat en cours ne se synchronise pas. Chaque copie a un numéro et tire les identifiants de ce qu'elle crée dans sa propre plage (2^40 par copie) : une fiche créée hors ligne garde son identifiant partout. La dernière plage (numéro 8191) n'est donnée à aucune copie : elle reçoit les profils, peuples et voies repris de l'ancien réglage, sous un identifiant tiré de leur `ref` ou de leur nom, le même sur toutes les copies.

| Route | Rôle |
|---|---|
| `POST /api/sync/copie` `{"nom": "Portable de Marion"}` | Inscrit une copie et renvoie le fichier `.cofdm` qu'elle gardera, avec les en-têtes `X-COF-Replique` (son numéro) et `X-COF-Univers` (l'univers). |
| `POST /api/sync` `{"replique", "curseur", "changements"}` | Un échange : la copie envoie les champs qu'elle a changés depuis son dernier envoi, et `curseur`, le dernier numéro d'ordre du serveur qu'elle a reçu. Réponse : `{"curseur", "changements", "conflits", "images_attendues"}`. `409` : copie inconnue de ce serveur. |
| `PUT /api/sync/images/{fichier}` | Le contenu (corps brut) d'une image que le serveur attend. `415` si le contenu ne correspond pas à son format ; `404` pour une image inconnue. |
| `GET /api/sync/copies` | Les copies inscrites : `{"copies": [{"numero", "nom", "compte", "cree", "vu", "version", "protocole", "modele"}]}`. `version`, `protocole`, `modele` : ceux de l'application qui tenait la copie à son dernier passage (`version` vide : application d'avant la 1.2.3). |

**Versions.** `GET /api/sante` et `GET /api/mes-univers` donnent, avec `version`, deux numéros distincts de la version de l'application : `protocole` (la forme de l'échange) et `modele` (ce que contient un univers et que l'échange transporte : types, champs, relations, fonctionnalités, réglages synchronisés…). Deux applications aux mêmes numéros se synchronisent sans rien perdre, quelles que soient leurs versions. Un serveur qui ne les publie pas (le bureau lit alors sa version dans `GET /api/sante`) a le protocole 1 et le modèle 1 depuis la 1.2.2, 0 avant. Le bureau annonce les siens dans chaque requête (`X-COF-Version`, `X-COF-Protocole`, `X-COF-Modele`). Dans un échange, le serveur répond `426` à un bureau plus ancien que lui (rien n'est reçu ni appliqué : ses changements attendent sur l'ordinateur) et `409` à un bureau plus récent qui lui envoie des changements. Le bureau compare les numéros avant chaque échange : serveur plus récent, il n'échange pas (état `version`) ; serveur plus ancien, il ne fait que recevoir, et ses changements partent quand le serveur est à jour. Il ne tire pas de copie d'un serveur plus récent (`409` : mettre l'application à jour) et n'envoie un univers qu'à un serveur au même modèle (`409`). Un univers (ou un export) ouvert par une version plus récente ne s'ouvre pas dans une plus ancienne (`422`), qui n'y touche pas.

**Côté application de bureau** (ordinateur du MJ seulement, jamais sur un serveur). L'application travaille en **mode local** (client seul) ou en **mode serveur** (reliée à un serveur). La connexion se fait avec le nom d'utilisateur et le mot de passe du compte, échangés aussitôt contre un jeton d'API propre au poste (nommé « Synchronisation — <nom du poste> » dans « Mon compte ») : le mot de passe n'est jamais gardé. Un poste n'a qu'un jeton : à la connexion, ceux qu'il aurait laissés à ce nom (serveur injoignable quand il l'a quitté) sont retirés. La connexion (adresse, compte, jeton) et les liens des copies sont rangés dans les réglages de l'ordinateur, pas dans l'univers. En mode serveur, une copie de ce serveur échange en arrière-plan toutes les 30 secondes ; sinon, l'application vérifie que le serveur répond. Une copie d'un autre serveur, ou ouverte en mode local, est **en pause** : ses changements attendent qu'on se reconnecte à son serveur.

| Route | Rôle |
|---|---|
| `GET /api/univers/serveur` | Le mode et l'univers ouvert : `{"mode", "serveur", "reseau", "relie", "actif", "pause", "copie", "url", "compte", "nom_copie", "synchro": {"etat", "derniere", "erreur", "bilan"}, "conflits"}`. `mode` : `local` ou `serveur`. `serveur` (mode serveur) : `{"url", "compte", "etat", "verifie", "erreur"}`, `etat` valant `connecte`, `hors_ligne`, `refuse` (jeton retiré sur le serveur : se reconnecter) ou `inconnu`. `relie` : l'univers ouvert est une copie reliée ; `actif` : elle se synchronise ; `pause` : elle attend son serveur. `synchro.etat` : `a_jour`, `hors_ligne`, `version` (le serveur est plus récent : mettre l'application à jour), `erreur` ou `jamais`. `synchro.ecart` : `serveur_recent` ou `bureau_recent` (réception seule), avec `version_serveur` et `message` ; `synchro.bilan` : `envoyes`, `recus`, `conflits`, `images`, `en_attente` (objets changés ici qui attendent le serveur), `recus_en_attente` (objets reçus que cette application ne sait pas encore appliquer). |
| `POST /api/univers/serveur/connexion` `{"url", "nom", "mdp"}` | Passe en mode serveur (HTTPS, sauf sur l'ordinateur ou le réseau local) et renvoie l'état, avec `univers` : ceux du compte (`ici` : déjà sur cet ordinateur). Une connexion d'avant est remplacée et son jeton rendu à son serveur (`avertissement` s'il ne répond pas) ; les copies d'un autre serveur passent en pause. Les appareils qui suivaient la page des joueurs sont coupés. |
| `POST /api/univers/serveur/local` | Repasse en mode local : le jeton du poste est rendu au serveur (`avertissement` s'il ne répond pas : à retirer dans « Mon compte »), les copies restent, en pause. |
| `GET /api/univers/serveur/univers` | Les univers du serveur où le compte a un rôle, par nom : `{"url", "univers": [{"id", "nom", "role", "ici", "ouvert"}]}` (`ici` : le fichier de sa copie sur cet ordinateur ; `ouvert` : c'est l'univers ouvert). Serveur injoignable : `"hors_ligne": true` et les seules copies de ce serveur rangées ici. `409` en mode local. |
| `POST /api/univers/serveur/ouvrir` `{"univers", "copie"?}` | Passe à un univers du serveur : sa copie sur cet ordinateur s'ouvre (même hors connexion) ; sans copie, elle est d'abord tirée, comme `relier`. Réponse : l'état de `GET /api/univers`. `409` en mode local. |
| `POST /api/univers/serveur/relier` `{"univers", "copie"?}` | Tire une copie d'un univers du serveur (rôle MJ), l'ouvre et la relie. `409` en mode local, ou si une copie est déjà sur cet ordinateur (une copie dont le fichier a disparu peut être tirée de nouveau). |
| `POST /api/univers/serveur/envoyer` `{"nom", "copie"?}` | Publie l'univers ouvert sur le serveur (compte administrateur), puis le remplace ici par sa copie reliée ; l'original est gardé dans ses sauvegardes (`avant-envoi`). `409` en mode local. |
| `POST /api/univers/serveur/synchroniser` | Un échange tout de suite ; la réponse est l'état (`hors_ligne` si le serveur ne répond pas). `409` si la copie est en pause. |
| `POST /api/univers/serveur/delier` | Détache l'univers ouvert : il devient un univers local indépendant (la copie reste, avec ses données). |
| `GET /api/univers/mise-a-jour` | Mises à jour de l'application : `{"version", "canal", "automatique", "verifie", "erreur", "installation", "disponible": {"version", "preversion", "notes", "page", "date", "fichier", "taille"}, "installable"}`. `canal` : `stable` ou `test`. `automatique` : ce programme se remplace seul (exe Windows, AppImage, version publiée). L'application cherche cinq secondes après son lancement, puis toutes les six heures. |
| `POST /api/univers/mise-a-jour/chercher` | Cherche tout de suite ; réponse : l'état. |
| `PUT /api/univers/mise-a-jour/canal` `{"canal": "stable" \| "test"}` | Change de canal (réglage de l'ordinateur) et cherche aussitôt. |
| `POST /api/univers/mise-a-jour/installer` | Télécharge la version trouvée, vérifie la signature de `SHA256SUMS` et l'empreinte du programme, le remplace (l'ancien reste en `.ancien` jusqu'au lancement suivant), puis relance l'application après la réponse `{"installee", "relance"}`. `409` sans version trouvée, `422` si la vérification échoue (rien n'est remplacé). |
| `GET /api/conflits`, `POST /api/conflits/{id}` `{"garder": "local" \| "serveur"}` | La liste « Conflits » de la copie, et le choix du MJ : garder la valeur locale la renvoie au serveur. |

Dans un échange, un champ que le serveur a aussi changé depuis le curseur garde **la valeur du serveur** : la réponse le signale (`conflits`, genre `champ`, avec `local` et `serveur`) et la copie le range dans sa liste « Conflits ». Une suppression ne passe pas si l'objet a été modifié de l'autre côté : il est gardé et signalé (genre `supprime_ici` : la réponse rend l'objet entier ; genre `supprime_la` : la copie le renverra entier). Un champ refusé par la validation est signalé (genre `refus`, avec `raison`), et la réponse porte la valeur du serveur ; un champ refusé que le serveur n'a pas reste sur la copie (rien n'est renvoyé : il peut venir d'un modèle plus récent), et un objet refusé que le serveur n'a pas du tout n'est signalé qu'une fois, par son champ `*`. Dans « Conflits », garder la valeur locale d'un refus la renvoie (tout l'objet, pour le champ `*`) ; garder celle du serveur la retire de la copie. Ce que la copie reçoit et ne sait pas appliquer attend sur elle et repasse à chaque échange (un changement plus récent du même champ le remplace). Les images voyagent à part : la copie envoie le contenu de celles que le serveur attend, et lit celles qui lui manquent par `GET /medias/{fichier}`.

## Exemple

```bash
J=…le jeton…
curl -s -X POST http://127.0.0.1:8100/api/entites \
  -H "Authorization: Bearer $J" -H "X-COF-Auteur: claude" -H "Content-Type: application/json" \
  -d '{"type":"pnj","nom":"Hilde Ferrat","data":{"statut":"vivant","profil":"aucun","resume":"Forgeronne de [[Mont-Veille]]."},"tags":["sans-gages"]}'
```
