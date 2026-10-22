# Projets — COF DM

Les chantiers possibles pour faire de COF DM le meilleur outil de MJ pour Chroniques Oubliées Fantasy, classés par priorité. Liste établie le 16 octobre 2026, remise à jour le 22 octobre 2026 (version 1.2.3, `main` en 1.2.4 d'essai).

Rappel des règles qui s'appliquent à chaque projet (voir `CLAUDE.md`) : aucune donnée des livres dans le dépôt (les tables viennent du fichier de règles ou du MJ), un univers = un fichier, rien de secret vers les joueurs (liste blanche + test de non-fuite), tout nouvel objet pris en compte par la synchronisation (`aplatir` / `appliquer`) et par `VersionModele`.

Les numéros des projets ne changent pas (`SUIVI.md` et les PR y renvoient) ; les numéros 7 et 8, retirés, ne sont pas réattribués.

## Légende

- **Priorité** : P1 = à faire maintenant, P2 = ensuite, P3 = quand le reste tourne, P4 = un jour, si le besoin se confirme.
- **Effort** : S = un lot (une PR), M = deux ou trois lots, L = quatre lots ou plus.
- **Valeur** : ce que le MJ et les joueurs y gagnent à la table (★ à ★★★).

## Où on en est

- **Fait** : P1 entier (projets 1 à 5), projet 10 (voies et capacités utilisables), projet 12 (montée de niveau guidée, version 1.2.4), lot 1 du projet 9 (aides de jeu), projet 33 (dépôt public). Le détail, PR par PR, est dans `SUIVI.md`.
- **Prochain chantier** : projet 11, inventaire structuré (puis 14, le butin).
- **Au fil de l'eau** : projet 6, les essais qui ne se font qu'à la main.

## Vue d'ensemble

| # | Projet | Priorité | Effort | Valeur | Dépend de | État |
|---|---|---|---|---|---|---|
| 1 | Serveur MCP pour Claude | P1 | S | ★★★ | — | fait |
| 2 | Préparation de séance générée | P1 | M | ★★★ | — | fait |
| 3 | Routine d'après-séance | P1 | M | ★★★ | 2 | fait |
| 4 | Lanceur de dés général (MJ et joueurs) | P1 | S | ★★★ | — | fait |
| 5 | Sorts et points de mana en combat | P1 | M | ★★★ | — | fait |
| 6 | Restes à vérifier à la main | P1 | S | ★★ | — | en cours |
| 9 | Aides de jeu poussées aux joueurs | P2 | M | ★★★ | — | lot 1 fait |
| 10 | Voies et capacités utilisables | P2 | L | ★★★ | 5 | fait |
| 11 | Inventaire structuré relié aux objets | P2 | M | ★★ | — | **prochain** |
| 12 | Montée de niveau guidée | P2 | M | ★★ | 10 | fait |
| 13 | Impression et export PDF | P2 | M | ★★ | — | |
| 14 | Butin et récompenses de fin de combat | P2 | S | ★★ | 11 | |
| 15 | Afficheur de table (télé) | P2 | S | ★★ | — | |
| 16 | Générateurs et tables aléatoires du MJ | P2 | M | ★★ | — | |
| 17 | PNJ express en jeu | P2 | S | ★★ | 16 (conseillé) | |
| 18 | Notes rapides de séance | P2 | S | ★★ | — | |
| 19 | Groupes de sbires en combat | P3 | M | ★★ | — | |
| 20 | Effets de zone (plusieurs cibles) | P3 | S | ★★ | — | |
| 21 | Résistances et vulnérabilités par type de dégâts | P3 | S | ★ | — | |
| 22 | Initiative variable (option des règles) | P3 | S | ★ | — | |
| 23 | Mort, blessures graves et séquelles | P3 | S | ★★ | — | |
| 24 | Horloges et comptes à rebours de faction | P3 | M | ★★ | — | |
| 25 | Avancer le temps (+1 jour) et événements prévus | P3 | S | ★★ | 24 (conseillé) | |
| 26 | Réputation des factions par PJ | P3 | S | ★★ | — | |
| 27 | Assistant de création de PJ | P3 | M | ★★ | 12 | |
| 28 | Annuler / rétablir depuis le journal | P3 | M | ★★ | — | |
| 29 | Notifications et page des joueurs installable (PWA) | P3 | S | ★★ | — | |
| 30 | Journal de campagne côté joueurs | P3 | M | ★★ | 3 | |
| 31 | Aller-retour Obsidian complet | P3 | M | ★ | — | |
| 32 | Tests automatisés de l'interface (CDP en CI) | P3 | M | ★★ | — | |
| 33 | Nettoyage avant publication du dépôt | P3 | S | ★★ | — | fait |
| 34 | Joueurs actifs en combat (option) | P4 | M | ★★ | décision de fond | |
| 35 | Compagnons, montures, familiers | P4 | M | ★ | 11 | |
| 36 | Commerce et boutiques | P4 | M | ★ | 11 | |
| 37 | Monnaie et dépenses (conversion, journal) | P4 | S | ★ | 11 | |
| 38 | Champs personnalisés et modèles de fiche | P4 | L | ★★ | — | |
| 39 | Fil de campagne (frise séances et quêtes) | P4 | S | ★ | — | |
| 40 | Défis de compétences et poursuites | P4 | M | ★ | — | |
| 41 | Ambiance sonore par scène | P4 | M | ★ | — | |
| 42 | Accessibilité (clavier, contrastes, lecteurs d'écran) | P4 | M | ★ | — | |
| 43 | Version macOS, installeur Windows signé | P4 | M | ★ | — | |
| 44 | Paquets communautaires (bestiaires, aventures maison) | P4 | M | ★ | 38 (conseillé) | |

## Ordre conseillé

1. **Projets 11 puis 14** (inventaire, puis butin) : l'arme et l'armure portées comptent dans la feuille ; le butin se répartit à la fin du combat.
2. Les petits lots visibles à la table, au choix : **15** (afficheur), **18** (notes rapides), **9** (suite des aides de jeu).
3. **13** (impression), puis **16** et **17** (tables aléatoires, PNJ express).
4. Le P3 par valeur, puis le P4 à la demande.
5. **Projet 6** au fil de l'eau, avant chaque version stable.

---

## P1 — Fait

### 1. Serveur MCP pour Claude — fait
`cof-dm -mcp` : les outils de Claude, traduits en appels de l'API, avec ses droits et son journal ; branché d'un clic par Paramètres → Claude.

### 2. Préparation de séance générée — fait
Page « Préparer » d'une séance : quêtes en cours, secrets à semer, PNJ probables, rencontres prêtes, versés dans la préparation.

### 3. Routine d'après-séance — fait
« Après la séance » : résumé, présents, date et lieu du groupe, quêtes, secrets appris, PNJ morts, rencontres, séance suivante, en une fois.

### 4. Lanceur de dés général — fait
Formules libres, jets cachés du MJ, tests tout faits sur la page des joueurs, jets de toute la table en direct.

### 5. Sorts et points de mana en combat — fait
PM des PJ reportés sur la feuille, coût d'un sort, capacités à usage limité suivies pendant le combat.

### 6. Restes à vérifier à la main — en cours
Ce qui ne s'essaie pas dans Chrome sur le poste de développement :
- l'exe Windows (WebView2) et l'AppImage (WebKitGTK) dans leur vraie fenêtre ;
- le serveur derrière un vrai nginx en HTTPS ; la synchronisation sur une vraie coupure, et avec deux copies hors ligne en même temps ;
- l'export Obsidian ouvert dans le vrai vault ;
- Claude Code branché par Paramètres → Claude (Claude Desktop est essayé).

## P2 — Ensuite

### 9. Aides de jeu poussées aux joueurs — lot 1 fait
Lettre, image, indice : envoyés en direct depuis la fiche ou l'écran du MJ, rangés dans l'onglet « Documents » de la page des joueurs, reliés au secret dont c'est l'indice.
- Suite possible : cocher les aides dans la préparation de séance ; les aides reçues dans l'export Obsidian « joueurs » ; envoi rapide à la volée (image collée) ; impression (projet 13).

### 10. Voies et capacités utilisables — fait
Voies, capacités, profils et peuples saisis ou importés par le MJ, un par un ; bonus des capacités sur la feuille (« A + b ») ; capacités en combat et leurs utilisations (par combat, jour, repos) ; arbre des voies sur la page des joueurs. Aucune donnée des livres : le code n'a que le modèle.

### 11. Inventaire structuré
`Équipement` est un texte libre. Une liste d'objets (quantité, porté ou non), pouvant pointer vers une fiche « Objet » : le champ « Détenteur » de l'objet se tient alors seul, et une arme ou une armure portée compte dans les attaques et la DEF.

### 12. Montée de niveau guidée — fait
Le MJ lance la montée (feuille du PJ, onglet Joueurs de l'écran du MJ) : il l'accorde et un assistant s'ouvre sur la page du joueur, ou il la fait lui-même. Fenêtres : dés de vie saisis, rangs dans le tableau des voies (coût et niveau minimal des règles), PM du profil, étapes en plus, récapitulatif. Historique des niveaux sur la feuille, annulation de la dernière montée. Tout vient de la partie `personnage.montee` du fichier de règles ; sans elle, saisie libre.

### 13. Impression et export PDF
Mise en page imprimable (feuille de style `@media print`, sans dépendance) : feuille de personnage, fiche, préparation de séance, rencontre, bloc d'un monstre, aide de jeu.

### 14. Butin et récompenses de fin de combat
Butin noté dans la rencontre, proposé au bilan du combat et réparti entre les PJ (bourse, objets de l'inventaire).

### 15. Afficheur de table
Une vue plein écran pour une télé : ordre d'initiative, round, états, santé des adversaires en mots (« indemne », « blessé », « en sang ») plutôt qu'en chiffres. Construite sur la page des joueurs (même filtre `reduire`).

### 16. Générateurs et tables aléatoires
Une fiche « Table aléatoire » que le MJ remplit (noms, rumeurs, rencontres par zone, trésors), avec pondération et tables imbriquées. Tirage depuis l'écran du MJ. Aucune table des livres dans le dépôt : elles viennent du MJ ou du fichier de règles.

### 17. PNJ express en jeu
Créer un PNJ en quelques secondes pendant la séance (nom tiré, trait, lieu courant, campagne en cours), marqué « à compléter » pour l'après-séance.

### 18. Notes rapides de séance
Un bloc-notes horodaté dans l'écran du MJ, avec `[[liens]]`, versé dans le résumé de la séance par la routine d'après-séance.

## P3 — Quand le reste tourne

### 19. Groupes de sbires
Plusieurs créatures identiques sur une ligne (PV de chacune), attaque de groupe, retrait de ceux qui tombent.

### 20. Effets de zone
Une attaque ou un sort sur plusieurs cibles : un jet, dégâts et tests de chaque cible.

### 21. Résistances et vulnérabilités
Par type de dégâts, au-delà de la seule RD ; types décrits dans le fichier de règles.

### 22. Initiative variable
Option du fichier de règles : initiative au dé en plus de la valeur fixe, relancée ou non à chaque round.

### 23. Mort, blessures graves et séquelles
Seuil et règles décrits dans le fichier de règles ; séquelles notées sur la feuille ; « Marquer mort » proposé pour un PJ comme pour un PNJ.

### 24. Horloges de faction
Des compteurs (« le rituel dans 6 jours », « 3 segments sur 8 ») attachés à une faction, une quête ou un secret, qui avancent avec la date en jeu et alertent sur l'écran du MJ.

### 25. Avancer le temps
Bouton « +1 jour / +1 semaine » sur la campagne : date en jeu, horloges, événements prévus à cette date proposés au MJ ; il pourrait déclencher le « Nouveau jour » des feuilles.

### 26. Réputation des factions par PJ
Attitude de chaque faction envers chaque PJ, avec l'historique (séance, raison), en plus de l'attitude envers le groupe.

### 27. Assistant de création de PJ
Peuple, profil, caractéristiques, voies de départ, équipement, tirés du fichier de règles et des profils saisis par le MJ ; sans eux, saisie libre.

### 28. Annuler / rétablir
Revenir sur une modification depuis le journal (qui garde déjà l'avant/après) ; Ctrl+Z dans la fiche ouverte.

### 29. Notifications et PWA joueurs
Page des joueurs installable, notifications « C'est à vous ! » et « nouvelle aide de jeu » quand le téléphone est en veille.

### 30. Journal de campagne côté joueurs
Résumé public de chaque séance (distinct du résumé du MJ), lu et complété par les joueurs sur leur page (champ `Joueur: true`).

### 31. Aller-retour Obsidian complet
`vault_vers_cofdm.py` relit tout l'export : objets, lore, règles, rencontres, événements, catégories exactes.

### 32. Tests de l'interface
Quelques parcours rejoués dans Chrome sans fenêtre (CDP) en CI : connexion, fiche, Paramètres, combat, page des joueurs, 390 px, sans erreur de console.

### 33. Nettoyage avant publication — fait
Dépôt neuf et public depuis le 18 octobre 2026 ; l'ancien, avec son historique, est `Tchyze/COF_DM-historique` (privé).

## P4 — Un jour, si le besoin se confirme

### 34. Joueurs actifs en combat
Le joueur dont c'est le tour déclare son jet sur son téléphone, le MJ valide. **Contraire à une décision prise** (« les joueurs n'agissent jamais sur un combat ») : à rediscuter avant tout code.

### 35. Compagnons, montures, familiers
Mini-feuille rattachée au PJ, qui entre en combat avec lui.

### 36. Commerce et boutiques
Inventaire de marchand sur un lieu ou un PNJ, prix et stock, achat qui débite la bourse.

### 37. Monnaie et dépenses
Conversion pc / pa / po, journal des gains et dépenses par PJ et pour le groupe.

### 38. Champs personnalisés et modèles
Champs ajoutés par le MJ à un type de fiche (dans l'univers, publiés par `/api/schema`), modèles de fiche pré-remplis. Gros chantier : touche validation, synchronisation, export et filtres des joueurs.

### 39. Fil de campagne
Frise des séances, quêtes et arcs, à côté de la chronologie en jeu.

### 40. Défis de compétences et poursuites
Suivi de réussites et d'échecs sur plusieurs tests, hors combat.

### 41. Ambiance sonore
Sons et musiques rangés dans l'univers, liés à un lieu ou une rencontre, lancés depuis l'écran du MJ.

### 42. Accessibilité
Navigation au clavier, contrastes, rôles ARIA des dialogues et de l'écran de combat.

### 43. macOS et installeur Windows signé
Pour diffuser au-delà de la table du MJ.

### 44. Paquets communautaires
Exporter et importer des bestiaires, tables, voies et aventures maison (sans contenu des livres) en un fichier.
