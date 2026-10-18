# Projets — COF DM

Liste de tous les chantiers possibles pour faire de COF DM le meilleur outil de MJ pour Chroniques Oubliées Fantasy, classés par priorité. Établie le 16 octobre 2026 à partir de l'analyse du dépôt (`main`, PR 42).

Rappel des règles qui s'appliquent à chaque projet (voir `CLAUDE.md`) : aucune donnée des livres dans le dépôt (les tables viennent du fichier de règles ou du MJ), un univers = un fichier, rien de secret vers les joueurs (liste blanche + test de non-fuite), tout nouvel objet pris en compte par la synchronisation (`aplatir` / `appliquer`).

## Légende

- **Priorité** : P1 = à faire maintenant, P2 = ensuite, P3 = quand le reste tourne, P4 = un jour, si le besoin se confirme.
- **Effort** : S = un lot (une PR), M = deux ou trois lots, L = quatre lots ou plus.
- **Valeur** : ce que le MJ et les joueurs y gagnent à la table (★ à ★★★).

## Vue d'ensemble

| # | Projet | Priorité | Effort | Valeur | Dépend de |
|---|---|---|---|---|---|
| 1 | ~~Serveur MCP pour Claude~~ (fait, PR 43) | P1 | S | ★★★ | — |
| 2 | ~~Préparation de séance générée~~ (fait, PR 46) | P1 | M | ★★★ | — |
| 3 | ~~Routine d'après-séance~~ (fait, PR 47) | P1 | M | ★★★ | 2 (conseillé) |
| 4 | ~~Lanceur de dés général (MJ et joueurs)~~ (fait, PR 44) | P1 | S | ★★★ | — |
| 5 | ~~Sorts et points de mana en combat~~ (fait, PR 45) | P1 | M | ★★★ | — |
| 6 | Restes à vérifier (SUIVI.md) | P1 | S | ★★ | — |
| 7 | ~~Cartes avec marqueurs liés aux fiches~~ (reporté, voir en fin de liste) | — | L | ★★★ | — |
| 8 | ~~Cartes montrées aux joueurs, brouillard de guerre~~ (reporté, voir en fin de liste) | — | M | ★★★ | 7 |
| 9 | Aides de jeu poussées aux joueurs | P2 | M | ★★★ | — |
| 10 | Capacités des voies utilisables | P2 | L | ★★★ | 5 |
| 11 | Inventaire structuré relié aux objets | P2 | M | ★★ | — |
| 12 | Montée de niveau guidée | P2 | M | ★★ | 10 (conseillé) |
| 13 | Impression et export PDF | P2 | M | ★★ | — |
| 14 | Butin et récompenses de fin de combat | P2 | S | ★★ | 11 |
| 15 | Afficheur de table (télé) | P2 | S | ★★ | — |
| 16 | Générateurs et tables aléatoires du MJ | P2 | M | ★★ | — |
| 17 | PNJ express en jeu | P2 | S | ★★ | 16 (conseillé) |
| 18 | Notes rapides de séance | P2 | S | ★★ | — |
| 19 | Groupes de sbires en combat | P3 | M | ★★ | — |
| 20 | Effets de zone (plusieurs cibles) | P3 | S | ★★ | — |
| 21 | Résistances et vulnérabilités par type de dégâts | P3 | S | ★ | — |
| 22 | Initiative variable (option des règles) | P3 | S | ★ | — |
| 23 | Mort, blessures graves et séquelles | P3 | S | ★★ | — |
| 24 | Horloges et comptes à rebours de faction | P3 | M | ★★ | — |
| 25 | Avancer le temps (+1 jour) et événements prévus | P3 | S | ★★ | 24 (conseillé) |
| 26 | Réputation des factions par PJ | P3 | S | ★★ | — |
| 27 | Assistant de création de PJ | P3 | M | ★★ | 12 |
| 28 | Annuler / rétablir depuis le journal | P3 | M | ★★ | — |
| 29 | Notifications et page des joueurs installable (PWA) | P3 | S | ★★ | — |
| 30 | Journal de campagne côté joueurs | P3 | M | ★★ | 3 |
| 31 | Aller-retour Obsidian complet | P3 | M | ★ | — |
| 32 | Tests automatisés de l'interface (CDP en CI) | P3 | M | ★★ | — |
| 33 | Nettoyage avant publication du dépôt | P3 | S | ★★ | — |
| 34 | Joueurs actifs en combat (option) | P4 | M | ★★ | décision de fond |
| 35 | Compagnons, montures, familiers | P4 | M | ★ | 11 |
| 36 | Commerce et boutiques | P4 | M | ★ | 11 |
| 37 | Monnaie et dépenses (conversion, journal) | P4 | S | ★ | 11 |
| 38 | Champs personnalisés et modèles de fiche | P4 | L | ★★ | — |
| 39 | Fil de campagne (frise séances et quêtes) | P4 | S | ★ | — |
| 40 | Défis de compétences et poursuites | P4 | M | ★ | — |
| 41 | Ambiance sonore par scène | P4 | M | ★ | — |
| 42 | Accessibilité (clavier, contrastes, lecteurs d'écran) | P4 | M | ★ | — |
| 43 | Version macOS, installeur Windows signé | P4 | M | ★ | — |
| 44 | Paquets communautaires (bestiaires, aventures maison) | P4 | M | ★ | 38 (conseillé) |

## Ordre conseillé

1. **Projet 6** (restes à vérifier) en tâche de fond, avant toute version publiée.
2. **Projet 1** (MCP) : le plus court, il ne fait qu'exposer l'API existante, et il aide à remplir le monde pour tout le reste.
3. **Projets 4 et 5** (dés, sorts et PM) : ce qui manque à la première séance.
4. **Projets 2 et 3** (préparation et après-séance) : le cycle d'une campagne.
5. **Projet 9** (aides de jeu) : le grand chantier visible des joueurs. Les cartes (7 et 8) sont reportées.
6. Le reste de P2, puis P3 par valeur, puis P4 à la demande.

---

## P1 — À faire maintenant

### 1. Serveur MCP pour Claude — fait (PR 43)
Point 7 de la feuille de route. Un serveur MCP (stdio, et HTTP pour le mode serveur) au-dessus de l'API REST : chercher, lire, créer et modifier des fiches, relations, secrets, rencontres ; lire le schéma (`GET /api/schema`) pour connaître les champs. Authentification par jeton d'API, écritures signées au journal comme tout client.
- Lot 1 : outils en lecture (recherche, fiche, graphe, chronologie, schéma).
- Lot 2 : outils d'écriture, avec un mode « proposer » qui montre le changement avant de l'appliquer.
- À décider : binaire à part ou sous-commande `cof-dm -mcp`.

### 2. Préparation de séance générée — fait (PR 46)
Point 5 de la feuille de route. Depuis une campagne, une page de préparation : quêtes en cours et étapes restantes, secrets « caché » ou « soupçonné » avec leurs indices non semés, PNJ probables (lieu du groupe, lieux de la quête, relations), rencontres non jouées et leur difficulté, fils rouges. Tout se coche et se copie dans le champ « Préparation » de la prochaine séance.

### 3. Routine d'après-séance — fait (PR 47)
Un assistant en fin de séance : secrets révélés (et à qui), PNJ morts ou rencontrés, étapes de quête faites, date en jeu atteinte, lieu du groupe, butin distribué, combats rattachés. Il remplit la séance (résumé, statut) et met les fiches à jour en une fois, journalisé.

### 4. Lanceur de dés général — fait (PR 44)
Le moteur de dés (`internal/combat/des.go`) ne sert qu'au combat.
- Formules libres (`2d6+3`, garder le meilleur de deux d20…), jets cachés du MJ, historique de la séance.
- Côté joueur, sur sa page : un lanceur avec ses tests tout faits lus sur sa feuille (caractéristiques, attaques), visibles du MJ.
- Les règles du dé (critique, échec) viennent du fichier de règles, comme en combat.

### 5. Sorts et points de mana en combat — fait (PR 45)
La feuille porte des PM, le combattant non (`Combattant` n'a ni `Pm` ni `PmMax`).
- Reporter les PM des PJ en combat, comme les PV et les points de chance (écrits sur la feuille).
- Action « lancer un sort » : attaque magique ou effet, coût en PM, état appliqué.
- Capacités à usage limité (« une fois par combat ») suivies pendant le combat.

### 6. Restes à vérifier (SUIVI.md) — en partie fait (PR 48)
- ~~Paramètres à 390 px : la page déborde (489 px).~~ (corrigé, PR 48)
- WebView2 et WebKitGTK jamais lancés ; exe et AppImage jamais démarrés.
- nginx réel, HTTPS, synchronisation sur une vraie coupure, deux copies hors ligne en même temps.
- ~~Deux appareils sur le même combat, thème clair de l'écran de combat.~~ (vus, PR 48)
- Export Obsidian ouvert dans le vrai vault.

## P2 — Ensuite

### 9. Aides de jeu poussées aux joueurs
Lettre, image, indice : le MJ l'envoie en direct à un, plusieurs ou tous les joueurs ; elle reste dans un onglet « Documents » de leur page. Relié au secret dont c'est un indice (« qui sait quoi » mis à jour).

### 10. Capacités des voies utilisables
Les capacités sont aujourd'hui du texte. Les décrire dans le fichier de profils et voies (type, coût, utilisations, effet sur les valeurs, état) pour qu'elles s'appliquent sur la feuille et proposent une action en combat. Sans fichier, elles restent du texte.

### 11. Inventaire structuré
`Equipement` est un texte libre. Une liste d'objets (quantité, porté ou non), pouvant pointer vers une fiche « Objet » : le champ « Détenteur » de l'objet se tient alors seul, et une arme ou une armure portée compte dans les attaques et la DEF.

### 12. Montée de niveau guidée
Passer un niveau depuis la feuille : PV gagnés (dé de vie), rang de voie à choisir, capacités débloquées rappelées, historique des niveaux. Le niveau reste lu sur la fiche (pas de doublon).

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
Bouton « +1 jour / +1 semaine » sur la campagne : date en jeu, horloges, événements prévus à cette date proposés au MJ.

### 26. Réputation des factions par PJ
Attitude de chaque faction envers chaque PJ, avec l'historique (séance, raison), en plus de l'attitude envers le groupe.

### 27. Assistant de création de PJ
Peuple, profil, caractéristiques, voies de départ, équipement, tirés du fichier de règles ; sans lui, saisie libre.

### 28. Annuler / rétablir
Revenir sur une modification depuis le journal (qui garde déjà l'avant/après) ; Ctrl+Z dans la fiche ouverte.

### 29. Notifications et PWA joueurs
Page des joueurs installable, notifications « C'est à vous ! » et « nouvelle aide de jeu » quand le téléphone est en veille.

### 30. Journal de campagne côté joueurs
Résumé public de chaque séance (distinct du résumé du MJ), lu et complété par les joueurs (champ `Joueur: true`).

### 31. Aller-retour Obsidian complet
`vault_vers_cofdm.py` relit tout l'export : objets, lore, règles, rencontres, événements, catégories exactes.

### 32. Tests de l'interface
Quelques parcours rejoués dans Chrome sans fenêtre (CDP) en CI : connexion, fiche, combat, page des joueurs, 390 px, sans erreur de console.

### 33. Nettoyage avant publication
Décidé le 18 octobre 2026 : **dépôt neuf** et public (l'actuel, renommé, reste privé avec son historique et ses PR) ; `outils/regles_obsidian.py` supprimé ; les formules de la feuille restent dans le code (structure du jeu).

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
Exporter et importer des bestiaires, tables et aventures maison (sans contenu des livres) en un fichier.

## Reporté (bien plus tard)

Décision du 18 octobre 2026 : les cartes sortent des priorités. Le MJ utilise pour l'instant un logiciel à part ; elles seront reprises bien plus tard, questions de fond comprises, avant tout code.

### 7. Cartes avec marqueurs
Point 4 de la feuille de route. Une image de carte rangée dans l'univers, zoom et déplacement, marqueurs reliés aux fiches (lieu, PNJ, rencontre, secret) qui ouvrent le tiroir. Cartes imbriquées : un marqueur de ville ouvre son plan. Marqueur « le groupe » relié au champ « où est le groupe » de la campagne. Marqueurs secrets (MJ seulement). À prendre en compte par la synchronisation.

### 8. Cartes montrées aux joueurs, brouillard de guerre
Le MJ dévoile des zones (pinceau ou polygones) ; la page des joueurs affiche la carte dévoilée en direct (WebSocket). Les marqueurs secrets et les zones cachées ne partent jamais (liste blanche, test de non-fuite).
