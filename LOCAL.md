# Installation du MJ — hors dépôt

Ce fichier n'est **pas suivi par git** (`.gitignore`) : il décrit l'installation et les fichiers du MJ, qui n'ont rien à faire dans le dépôt public. À lire avec `SUIVI.md` avant de reprendre ; à tenir à jour ici, jamais dans un fichier suivi.

## Serveur du MJ

- Docker sur le poste WSL : dépôt `~/Outil/COF_DM`, `http://localhost:8100`, données dans `~/Outil/COF_DM/donnees`.
- Sur `main` (1.2.5) depuis le 31 octobre 2026, construit à la main : `git pull && docker compose up -d --build`. Peut passer sur l'image publiée `tchyze/cof-dm:X.Y.Z` (README, « Avec Docker »).
- Le bureau (exe Windows) s'y relie en mode serveur.
- Univers d'avant le modèle 8 : sauvegarde automatique `donnees/sauvegardes/0900d828a03a8aab/2026-10-31_11-57-10_auto.cofdm` (modèle 6).

## Dépôts

- L'ancien dépôt, avec tout l'historique et ses PR 1 à 54 (section « Avant le dépôt public » de `SUIVI.md`), est `Tchyze/COF_DM-historique`, privé : remote `historique` du dépôt local.

## Fichiers hors dépôt, à emporter

Sur le poste WSL, dans `~/Outil/`, à côté du dépôt, avec le PDF du livre de base pour vérifier les tables. Sans le fichier de règles, le créateur de monstre, les repos et les listes de choix (peuples, profils, tailles, catégories) ne sont pas proposés.

| Fichier | Contenu |
|---|---|
| `Règles COF.json` | Le fichier de règles (`cof-dm-regles`) : listes, créateur de monstre, feuille de personnage et repos, montée de niveau (`personnage.montee`), difficulté des rencontres (`rencontre`), valeurs et états du combat (`combat`). À importer dans Paramètres → Bestiaire et règles, dans chaque univers. Copie d'avant la montée de niveau : `Règles COF.avant-montee.json`. |
| `Bestiaire officiel COF.json`, `Profils et voies COF.json` | Le bestiaire officiel et les profils et voies, à importer dans chaque univers. Aussi dans le vault du MJ. |
| `exemples_monstres.json`, `exemples_personnages.json` | Fiches d'exemple de tracker_COF, retirées du dépôt. |
| `cle-publication-cof-dm.pem` | La clé de publication des mises à jour (secret `COF_DM_SIGNATURE`). À sauvegarder ailleurs : sans elle, plus de mise à jour automatique. |

## Règles officielles de la montée de niveau

Lues dans le livre de base le 22 octobre 2026, rangées dans `Règles COF.json` : dé de vie + mod. CON, 2 points (rangs 1-2 : 1 pt, 3-5 : 2 pts), pas de rang au-dessus du niveau, voie de prestige à 2 pts par rang, report des points, PM +1 ou +2 selon le profil, capacité épique (option) aux niveaux pairs dès 12. Écartée par le MJ : la valeur moyenne du dé.

## Essais faits chez le MJ

- L'exe Windows tourne chez le MJ, relié à son serveur Docker.
- `vault_vers_cofdm.py` a été taillé pour le vault d'origine du MJ.
