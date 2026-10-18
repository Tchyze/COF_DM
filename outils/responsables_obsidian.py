#!/usr/bin/env python3
"""Reporte dans COF DM le « responsable » des PNJ d'un vault Obsidian, sans rien recréer.

    COF_DM_JETON=<jeton> python3 -I outils/responsables_obsidian.py <vault> <http://127.0.0.1:8100> [--voir]

Le jeton d'API (inutile sur l'ordinateur du MJ) se passe par la variable d'environnement
COF_DM_JETON : en argument, il serait visible des autres utilisateurs de la machine.

Pour un monde déjà importé. Chaque note de PNJ (`type: pnj`) dont le frontmatter porte
`responsable: "[[X]]"` ou `responsable_secret: "[[X]]"` met à jour le champ du même nom de la fiche
qui porte son nom : c'est ce champ qui donne sa forme à l'organigramme des factions.

Rien n'est créé ni supprimé. Un champ déjà à jour n'est pas touché ; une propriété vide ou absente
ne vide pas la fiche. Avec --voir, le script montre ce qu'il ferait sans rien écrire.
"""
import json
import os
import re
import sys
import unicodedata
import urllib.error
import urllib.parse
import urllib.request


def lire_jeton(arg):
    """Le jeton d'API : variable COF_DM_JETON, ou (déconseillé) argument de la ligne de commande."""
    if arg:
        print("Attention : un jeton passé en argument est visible dans la liste des processus et"
              " l'historique du terminal ; préférez la variable COF_DM_JETON.", file=sys.stderr)
    return os.environ.get("COF_DM_JETON", "") or arg

ARGS = [a for a in sys.argv[1:] if a != "--voir"]
VOIR = "--voir" in sys.argv[1:]
if len(ARGS) < 2:
    raise SystemExit(__doc__)
VAULT, BASE = ARGS[0], ARGS[1].rstrip("/")
JETON = lire_jeton(ARGS[2] if len(ARGS) > 2 else "")
CHAMPS = ("responsable", "responsable_secret")


def api(methode, chemin, corps=None):
    req = urllib.request.Request(BASE + chemin, method=methode, data=json.dumps(corps).encode() if corps is not None else None,
                                 headers={"Content-Type": "application/json", "X-COF-DM": "1", "X-COF-Auteur": "vault"})
    if JETON:
        req.add_header("Authorization", "Bearer " + JETON)
    try:
        with urllib.request.urlopen(req) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{methode} {chemin} : {e.code} {e.read().decode(errors='replace')}")
    except urllib.error.URLError as e:
        raise SystemExit(f"COF DM ne répond pas à {BASE} ({e.reason}) : l'application est-elle ouverte ?")


def plier(s):
    """Comparaison sans accents, casse ni ponctuation."""
    s = unicodedata.normalize("NFD", s.replace("’", "'"))
    return re.sub(r"\s+", " ", "".join(c for c in s if unicodedata.category(c) != "Mn").lower()).strip()


def frontmatter(chemin):
    t = open(chemin, encoding="utf-8").read().replace("\r\n", "\n")
    m = re.match(r"---\n(.*?)\n---", t, re.S)
    fm = {}
    if m:
        for l in m.group(1).splitlines():
            if ":" in l and not l.startswith((" ", "-")):
                k, v = l.split(":", 1)
                fm[k.strip()] = v.strip().strip('"')
    return fm


def cible(v):
    """« [[dossier/X|texte]] » -> X ; rien si la valeur n'est pas un lien."""
    m = re.search(r"\[\[([^\]]+)\]\]", v or "")
    if not m:
        return ""
    return re.split(r"\\?\||#", m.group(1))[0].split("/")[-1].strip()


# fiches de COF DM, par nom
fiches = {}
for ty in ("pnj", "pj"):
    for f in api("GET", "/api/entites?type=" + ty):
        fiches.setdefault(plier(f["nom"]), f)

faits, pareils, avert = [], 0, []
for racine, _, fichiers in os.walk(VAULT):
    if os.path.basename(racine).startswith(".") or "Modèles" in racine:
        continue
    for fichier in sorted(fichiers):
        if not fichier.endswith(".md"):
            continue
        fm = frontmatter(os.path.join(racine, fichier))
        if fm.get("type") != "pnj" or not any(fm.get(c) for c in CHAMPS):
            continue
        nom = fichier[:-3]
        fiche = fiches.get(plier(nom))
        if not fiche or fiche["type"] != "pnj":
            avert.append(f"{nom} : aucune fiche de PNJ à ce nom dans COF DM")
            continue
        actuel = None
        patch = {}
        for champ in CHAMPS:
            brut = fm.get(champ, "")
            if not brut:
                continue
            chef = fiches.get(plier(cible(brut)))
            if not chef:
                avert.append(f"{nom} : {champ} « {brut} » ne mène à aucune fiche de PNJ ou de PJ (ignoré)")
                continue
            if chef["id"] == fiche["id"]:
                avert.append(f"{nom} : {champ} le désigne lui-même (ignoré)")
                continue
            if actuel is None:
                actuel = api("GET", f"/api/entites/{fiche['id']}")["data"]
            if actuel.get(champ) == chef["id"]:
                pareils += 1
                continue
            patch[champ] = chef["id"]
            faits.append(f"{nom} : {champ} -> {chef['nom']}")
        if patch and not VOIR:
            api("PATCH", f"/api/entites/{fiche['id']}", {"data": patch})

print(("À faire" if VOIR else "Mis à jour") + f" : {len(faits)} ; déjà à jour : {pareils}")
for l in faits:
    print(" -", l)
if avert:
    print(f"{len(avert)} avertissement(s)")
    for a in avert:
        print(" -", a)
