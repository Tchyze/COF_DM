#!/usr/bin/env python3
"""Remplit COF DM à partir d'un vault Obsidian de campagne (dossiers 01 Monde, 02 PNJ, 04 Objets,
05 Campagnes), par l'API de COF DM.

    COF_DM_JETON=<jeton> python3 -I outils/vault_vers_cofdm.py <vault> <http://127.0.0.1:8100>

Le jeton d'API (inutile sur l'ordinateur du MJ) se passe par la variable d'environnement
COF_DM_JETON : en argument, il serait visible des autres utilisateurs de la machine (liste des
processus) et resterait dans l'historique du terminal.

Le script ne crée rien dans le vault ; il lit les notes (frontmatter + sections du modèle) et crée les
fiches, les liens (champs, relations, appartenances aux factions) et les événements de la chronologie.
Les textes sont repris tels quels : seuls les [[liens]] sont réécrits vers le nom des fiches créées.
Une cible de lien sans fiche (monstre du bestiaire, par exemple) devient du texte simple.
"""
import json
import os
import re
import sys
import unicodedata
import urllib.error
import urllib.request


def lire_jeton(arg):
    """Le jeton d'API : variable COF_DM_JETON, ou (déconseillé) argument de la ligne de commande."""
    if arg:
        print("Attention : un jeton passé en argument est visible dans la liste des processus et"
              " l'historique du terminal ; préférez la variable COF_DM_JETON.", file=sys.stderr)
    return os.environ.get("COF_DM_JETON", "") or arg

VAULT, BASE = sys.argv[1], sys.argv[2].rstrip("/")
JETON = lire_jeton(sys.argv[3] if len(sys.argv) > 3 else "")
AVERT = []


def api(methode, chemin, corps=None):
    h = {"Content-Type": "application/json", "X-COF-DM": "1", "X-COF-Auteur": "import-vault"}
    if JETON:
        h["Authorization"] = "Bearer " + JETON
    r = urllib.request.Request(BASE + chemin, method=methode, headers=h,
                               data=None if corps is None else json.dumps(corps).encode())
    try:
        with urllib.request.urlopen(r) as x:
            return json.loads(x.read() or "null")
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{methode} {chemin} -> {e.code} {e.read().decode()}")


def plier(s):
    s = unicodedata.normalize("NFD", s.lower())
    return "".join(c for c in s if unicodedata.category(c) != "Mn").strip()


# ---------------------------------------------------------------- lecture des notes
class Note:
    pass


def lire(chemin):
    t = open(chemin, encoding="utf-8").read().replace("\r\n", "\n")
    n = Note()
    n.chemin = chemin
    n.stem = os.path.basename(chemin)[:-3]
    n.fm = {}
    m = re.match(r"---\n(.*?)\n---\n?", t, re.S)
    if m:
        for l in m.group(1).splitlines():
            if ":" in l and not l.startswith((" ", "-")):
                k, v = l.split(":", 1)
                n.fm[k.strip()] = v.strip().strip('"')
        t = t[m.end():]
    h1 = re.search(r"^# (.+)$", t, re.M)
    n.titre = h1.group(1).strip() if h1 else n.stem
    reste = t[h1.end():] if h1 else t
    parts = re.split(r"^##+ ", reste, flags=re.M)
    avant = parts[0].strip()
    n.citation = ""
    n.intro = []
    for l in avant.splitlines():
        if l.startswith(">") and not n.citation:
            n.citation = l.lstrip("> ").strip()
        elif l.strip():
            n.intro.append(l)
    n.sections = []
    # on garde le niveau du titre pour reconstruire les sous-sections
    for m2 in re.finditer(r"^(##+) ([^\n]*)\n?(.*?)(?=^##+ |\Z)", reste, re.M | re.S):
        n.sections.append((m2.group(2).strip(), m2.group(3), len(m2.group(1))))
    return n


NOTES = []
for racine in ("01 - Monde", "02 - PNJ", "04 - Objets & économie", "05 - Campagnes"):
    for r, _, fs in os.walk(os.path.join(VAULT, racine)):
        for f in sorted(fs):
            if f.endswith(".md"):
                NOTES.append(lire(os.path.join(r, f)))

TYPE_FM = {"pnj": "pnj", "lieu": "lieu", "faction": "faction", "quête": "quete", "campagne": "campagne",
           "aide de jeu": "objet", "calendrier": "lore", "économie": "lore"}
fiches = []  # {type, nom, data, tags, note, refs:{champ: (stem|[stems])}}
REG = {}  # stem normalisé -> fiche
FILS = None
CHRONO = None
for n in NOTES:
    ty = TYPE_FM.get(n.fm.get("type", ""))
    if n.stem == "Chronologie":
        CHRONO = n
        continue
    if n.stem == "Fils rouges":
        FILS = n
        continue
    if not ty:
        continue
    nom = n.titre if n.stem == "_Campagne" else n.stem
    f = {"type": ty, "nom": nom, "data": {}, "tags": [], "note": n, "refs": {}, "id": None}
    fiches.append(f)
    REG[plier(n.stem)] = f
    REG[plier(nom)] = f
noms = {}
for f in fiches:
    k = plier(f["nom"])
    if k in noms:
        raise SystemExit("Nom en double : " + f["nom"])
    noms[k] = f

# ---------------------------------------------------------------- textes
RE_LIEN = re.compile(r"(!?)\[\[([^\]]*?)\]\]")


def reecrire(t, perdus=None):
    def un(m):
        inner = m.group(2)
        sep = "\\|" if "\\|" in inner else "|"
        cible, _, alias = inner.partition(sep)
        cible = cible.split("#")[0].strip()
        base = cible.split("/")[-1].strip()
        if not base:
            return ""
        f = REG.get(plier(base))
        if f:
            return f"[[{f['nom']}{sep + alias if alias else ''}]]"
        AVERT.append("lien sans fiche : " + base)
        return alias or base
    return RE_LIEN.sub(un, t)


VIDE = re.compile(r"^\s*(-|\d+\.|- \[ \]|\|\s*\|\s*\||- NC : · DEF : · PV : · Init :|- Attaque :)?\s*$")


def propre(t):
    out = []
    for l in t.splitlines():
        if VIDE.match(l) or re.match(r"^\s*\|(\s*\|)+\s*$", l):
            continue
        if re.match(r"^\s*-\s*$", l):
            continue
        out.append(l.rstrip())
    s = "\n".join(out).strip()
    s = reecrire(s)
    s = re.sub(r"\n{3,}", "\n\n", s)
    # tableaux dont toutes les lignes de données sont vides
    return s


def liens_de(t):
    """Fiches citées par [[...]] dans t, dans l'ordre."""
    res = []
    for m in RE_LIEN.finditer(t):
        base = re.split(r"\\\||\|", m.group(2))[0].split("#")[0].split("/")[-1].strip()
        f = REG.get(plier(base)) if base else None
        if f and f not in res:
            res.append(f)
    return res


def un_lien(v):
    """Valeur de frontmatter « [[X]] » -> fiche (ou None)."""
    ls = liens_de(v or "")
    return ls[0] if ls else None


def tous_liens(v):
    return liens_de(v or "")


def fusion(titre_niv, items):
    """[(titre, texte)] -> markdown avec sous-titres."""
    out = []
    for t, x in items:
        if x:
            out.append(f"{titre_niv} {t}\n{x}" if t else x)
    return "\n\n".join(out)


def ligne(s):
    s = reecrire(s)
    s = re.sub(r"[*_`]", "", s)
    return re.sub(r"\s+", " ", s).strip()


# ---------------------------------------------------------------- schéma
SCHEMA = api("GET", "/api/schema")
OPT = {}
for t in SCHEMA["types"]:
    for c in t["champs"]:
        if c.get("options"):
            OPT[(t["cle"], c["cle"])] = c["options"]


def option(ty, champ, v):
    if (ty, champ) not in OPT:  # liste venue du fichier de règles (peuples, profils) : saisie libre
        return v.strip() or None
    for o in OPT[(ty, champ)]:
        if plier(o) == plier(v):
            return o
    return None


# ---------------------------------------------------------------- conversion par type
def secret_titre(h):
    k = plier(h)
    return "secret" in k or "(mj)" in k or "ce qui se passe reellement" in k or k.startswith("notes mj")


CAT_LIEU = [("royaume", "royaume"), ("forestiere", "forêt"), ("region", "région"), ("capitale", "ville"),
            ("quartier", "quartier"), ("abbaye", "temple"), ("tour", "bâtiment"), ("phare", "bâtiment"),
            ("riviere", "cours d'eau"), ("route", "route"), ("sentier", "route"), ("repaire", "donjon"),
            ("donjon", "donjon"), ("grotte", "site"), ("mine", "site"), ("carriere", "site"),
            ("ferme", "ferme"), ("vignoble", "ferme"), ("village", "village"), ("garnison", "fort"),
            ("fort", "fort"), ("camp", "site"), ("auberge", "auberge"), ("cote", "côte"),
            ("collines", "montagne"), ("champs", "site"), ("lac", "site")]
CAT_FAC = [("eglise", "ordre religieux"), ("brigands", "hors-la-loi & rebelles"), ("tribu", "tribu & peuple"),
           ("noblesse", "pouvoir & institution"), ("milice", "pouvoir & institution"),
           ("espions", "pouvoir & institution"), ("garde", "pouvoir & institution"),
           ("mercenaires", "guilde & compagnie"), ("guilde", "guilde & compagnie")]


def categorie(table, brut):
    k = plier(brut)
    for mot, cible in table:
        if mot in k:
            return cible
    return "autre"


def tag(s):
    return re.sub(r"[,\n]", " ", s).strip().lower()


def conv_lieu(f, n):
    d = f["data"]
    d["impression"] = ligne(n.citation)
    brut = n.fm.get("catégorie", "")
    d["categorie"] = categorie(CAT_LIEU, brut)
    if brut:
        f["tags"].append(tag(brut))
    f["refs"]["parent"] = un_lien(n.fm.get("région"))
    f["refs"]["dirigeant"] = un_lien(n.fm.get("dirigeant"))
    desc, notables, rum, sec = [], [], [], []
    extra = []
    for h, x, niv in n.sections:
        k = plier(h)
        x = propre(x)
        if not x or k == "carte":
            continue
        if k == "description":
            desc.append(("", x))
        elif k == "lieux notables":
            notables.append(("", x))
        elif k == "pnj presents":
            notables.append(("PNJ présents", x))
        elif k.startswith("rumeurs"):
            rum.append(("", x))
        elif secret_titre(h):
            sec.append(("" if len(sec) == 0 else h, x))
        else:
            extra.append((h, x))
    d["description"] = fusion("###", desc + extra)
    d["lieux_notables"] = fusion("###", notables)
    d["rumeurs"] = fusion("###", rum)
    d["secrets"] = fusion("###", sec)


def conv_faction(f, n):
    d = f["data"]
    d["but"] = ligne(n.citation)
    brut = n.fm.get("catégorie", "")
    d["categorie"] = categorie(CAT_FAC, brut)
    if brut:
        f["tags"].append(tag(brut))
    d["attitude"] = option("faction", "attitude", n.fm.get("attitude_envers_pj", "")) or "neutre"
    f["refs"]["siege"] = un_lien(n.fm.get("siège"))
    f["refs"]["dirigeant"] = un_lien(n.fm.get("dirigeant"))
    desc, res, sec = [], [], []
    f["membres"], f["rel_fac"] = "", ""
    if n.fm.get("statut"):
        desc.append(("", f"**Statut : {n.fm['statut']}.**"))
    for h, x, niv in n.sections:
        k = plier(h)
        if k.startswith("membres"):
            f["membres"] = x
            continue
        if k.startswith("allies"):
            f["rel_fac"] = x
            continue
        x = propre(x)
        if not x:
            continue
        if k == "description":
            desc.append(("", x))
        elif k.startswith("ressources"):
            res.append(("", x))
        elif secret_titre(h):
            sec.append(("", x))
        else:
            desc.append((h, x))
    d["description"] = fusion("###", desc)
    d["ressources"] = fusion("###", res)
    d["secrets"] = fusion("###", sec)


def conv_pnj(f, n):
    d = f["data"]
    d["resume"] = ligne(n.citation)
    d["statut"] = option("pnj", "statut", n.fm.get("statut", "vivant")) or "vivant"
    notes = []
    race = n.fm.get("race", "")
    o = option("pnj", "peuple", race) or option("pnj", "peuple", re.sub(r"e$", "", race))
    if not o:
        o = "Autre"
    d["peuple"] = o
    if plier(race) not in (plier(o), plier(o) + "e"):
        notes.append(("Peuple (vault)", race))
    brut = reecrire(n.fm.get("profil", ""))
    premier = re.split(r"[(+]", brut)[0].strip()
    p = option("pnj", "profil", premier)
    d["profil"] = p or "Aucun"
    if brut and not p and not plier(brut).startswith("aucun"):
        notes.append(("Profil de combat (vault)", brut))
    elif brut and plier(brut) != plier(p or ""):
        notes.append(("Profil (vault)", brut))
    facs = tous_liens(n.fm.get("factions"))
    f["factions"] = [x for x in facs if x["type"] == "faction"]
    if f["factions"]:
        f["refs"]["allegeance"] = f["factions"][0]
    f["refs"]["lieu"] = un_lien(n.fm.get("lieu"))
    f["refs"]["campagnes"] = [x for x in tous_liens(n.fm.get("campagnes")) if x["type"] == "campagne"]
    # à qui il répond : donne sa forme à l'organigramme des factions
    for champ, titre in (("responsable", "Responsable (vault)"), ("responsable_secret", "Responsable secret (vault)")):
        brut = n.fm.get(champ, "")
        if un_lien(brut):
            f["refs"][champ] = un_lien(brut)
        elif brut.strip():  # un responsable sans fiche reste en note du MJ
            notes.append((titre, ligne(brut)))
    f["relations"] = ""
    sec = []
    for h, x, niv in n.sections:
        k = plier(h)
        if k == "relations":
            f["relations"] = x
            continue
        xx = propre(x)
        if not xx:
            continue
        if k == "apparence":
            d["apparence"] = xx
        elif k.startswith("personnalite"):
            d["personnalite"] = xx
        elif k.startswith("role"):
            d["role"] = xx
        elif secret_titre(h):
            sec.append(("" if not sec else h, xx))
        else:
            notes.append((h, xx))
    d["secrets"] = fusion("###", sec)
    d["notes"] = fusion("###", [(h, x) for h, x in notes if x])


def conv_quete(f, n):
    d = f["data"]
    chem = n.chemin
    d["categorie"] = ("arc principal" if "Arc principal" in chem else
                      "mission" if "Missions de la Compagnie" in chem else "secondaire")
    m = re.match(r"(\d+) - ", n.stem)
    if m and d["categorie"] == "arc principal":
        d["ordre"] = int(m.group(1))
    d["statut"] = option("quete", "statut", n.fm.get("statut", "")) or "à venir"
    f["refs"]["campagne"] = un_lien(n.fm.get("campagne"))
    f["refs"]["commanditaire"] = un_lien(n.fm.get("commanditaire"))
    f["refs"]["lieu"] = un_lien(n.fm.get("lieu"))
    if n.fm.get("niveau_conseillé"):
        d["niveau"] = n.fm["niveau_conseillé"]
    if n.fm.get("récompense"):
        d["recompense"] = ligne(n.fm["récompense"])
    acc, obj, ind, cons, sec, extra, etapes = [], [], [], [], [], [], []
    intro = propre("\n".join(n.intro))
    for h, x, niv in n.sections:
        k = plier(h)
        if k == "etapes":
            for l in x.splitlines():
                m2 = re.match(r"\s*- \[( |x|X)\] (.*)", l)
                if m2 and m2.group(2).strip():
                    etapes.append({"texte": reecrire(m2.group(2).strip()), "fait": m2.group(1) != " "})
            continue
        if k == "suivi":
            if re.search(r"\|\s*\w", re.sub(r"\|\s*-+.*", "", "\n".join(x.splitlines()[2:]))):
                extra.append((h, propre(x)))
            continue
        xx = propre(x)
        if not xx:
            continue
        if k == "accroche":
            acc.append(("", xx))
        elif k == "objectif":
            obj.append(("", xx))
        elif k.startswith("indices") or k.startswith("ce que la quete revele"):
            ind.append((h, xx))
        elif k.startswith("enjeux"):
            cons.append(("", xx))
        elif secret_titre(h):
            sec.append(("", xx))
        elif k.startswith("recompense") and "recompense" not in d:
            d["recompense"] = xx
        else:
            extra.append((h, xx))
    d["accroche"] = fusion("###", ([("", intro)] if intro else []) + acc + extra)
    d["objectif"] = fusion("###", obj)
    d["indices"] = fusion("###", ind) if len(ind) > 1 else (ind[0][1] if ind else "")
    d["consequences"] = fusion("###", cons)
    d["secrets"] = fusion("###", sec)
    if etapes:
        d["etapes"] = etapes


def conv_campagne(f, n):
    d = f["data"]
    d["statut"] = {"en revision": "en préparation"}.get(plier(n.fm.get("statut", "")), None) or \
        option("campagne", "statut", n.fm.get("statut", "")) or "en préparation"
    arc, pitch, themes, autres = "", "", "", []
    for h, x, niv in n.sections:
        k = plier(h)
        xx = propre(x)
        if not xx:
            continue
        if k == "pitch":
            pitch = xx
        elif k.startswith("themes"):
            themes = " · ".join(re.sub(r"^\s*-\s*", "", l).strip() for l in xx.splitlines() if l.strip())
        elif k == "arc principal":
            arc = xx
        else:
            autres.append((h, xx))
    d["pitch"] = pitch
    d["themes"] = ligne(themes)
    d["arc"] = fusion("###", [("", arc)] + autres)
    d["date_debut"] = {"an": 1215, "mois": 12, "jour": 1}
    if FILS:
        corps = propre("\n".join(f"{'#' * (niv + 1)} {h}\n{x}" for h, x, niv in FILS.sections))
        d["fils_rouges"] = corps


def conv_objet(f, n):
    d = f["data"]
    d["categorie"] = "document"
    intro = propre("\n".join(n.intro))
    d["resume"] = ligne(re.sub(r"^\*|\*$", "", n.intro[0].strip())) if n.intro else ""
    corps = [(h if niv > 2 or True else "", propre(x)) for h, x, niv in n.sections if not secret_titre(h)]
    sec = [("", propre(x)) for h, x, niv in n.sections if secret_titre(h) and propre(x)]
    d["histoire"] = fusion("###", [("", intro)] + [(h, x) for h, x in corps if x])
    d["secrets"] = fusion("###", sec)
    f["refs"]["detenteur"] = None
    if "lettre" in plier(n.stem):
        f["refs"]["detenteur"] = REG.get(plier("Aurèle Morvan"))
    f["tags"].append("aide de jeu")


def conv_lore(f, n):
    d = f["data"]
    d["categorie"] = "calendrier" if n.fm.get("type") == "calendrier" else "économie"
    d["contenu"] = fusion("##", [(h, propre(x)) for h, x, niv in n.sections if propre(x)])


CONV = {"lieu": conv_lieu, "faction": conv_faction, "pnj": conv_pnj, "quete": conv_quete,
        "campagne": conv_campagne, "objet": conv_objet, "lore": conv_lore}
for f in fiches:
    CONV[f["type"]](f, f["note"])
    f["data"] = {k: v for k, v in f["data"].items() if v not in ("", None, [])}

# ---------------------------------------------------------------- calendrier du monde
cal = api("GET", "/api/calendrier")
cal["ere"] = "de la Couronne"
if cal["mois"] and cal["mois"][-1]["nom"].lower().startswith("jours compl"):
    cal["mois"][-1]["nom"] = "Fête des moissons"
api("PUT", "/api/calendrier", cal)
MOIS = {plier(m["nom"]).split()[0]: i + 1 for i, m in enumerate(cal["mois"])}
MOIS["fete"] = len(cal["mois"])

# ---------------------------------------------------------------- création des fiches (sans références)
for f in fiches:
    r = api("POST", "/api/entites", {"type": f["type"], "nom": f["nom"], "data": f["data"], "tags": f["tags"]})
    f["id"] = r["id"]
print(f"{len(fiches)} fiches créées")

# ---------------------------------------------------------------- références
CIBLES = {}
for t in SCHEMA["types"]:
    for c in t["champs"]:
        if c["genre"] in ("ref", "refs"):
            CIBLES[(t["cle"], c["cle"])] = c["cibles"]
for f in fiches:
    patch = {}
    for champ, v in f["refs"].items():
        ok = CIBLES.get((f["type"], champ), [])
        if isinstance(v, list):
            ids = [x["id"] for x in v if x and x["type"] in ok]
            if ids:
                patch[champ] = ids
        elif v:
            if v["type"] in ok:
                patch[champ] = v["id"]
            else:
                AVERT.append(f"{f['nom']} : {champ} -> {v['nom']} ({v['type']}) refusé")
    if patch:
        api("PATCH", "/api/entites/" + str(f["id"]), {"data": patch})

# ---------------------------------------------------------------- relations
VUES = set()


def relier(a, b, typ, note="", rang="", secrete=False):
    if a is b:
        return
    cle = (min(a["id"], b["id"]), max(a["id"], b["id"]), typ) if typ not in ("membre", "dirige") else (a["id"], b["id"], typ)
    if cle in VUES:
        return
    VUES.add(cle)
    corps = {"a": a["id"], "b": b["id"], "type": typ}
    if note:
        corps["note"] = note
    if rang:
        corps["rang"] = rang
    if secrete:
        corps["secrete"] = True
    try:
        api("POST", "/api/relations", corps)
    except RuntimeError as e:
        AVERT.append(f"relation {a['nom']} / {b['nom']} : {e}")


def type_rel(texte, cible):
    """Type de relation d'après le début du texte (« ennemis », « ami d'enfance »…) ; sinon « connaît »."""
    k = plier(re.split(r"[,;(:—]", RE_LIEN.sub("", texte))[0])
    if re.search(r"\bpas\b|\bne\b|\bn'", k):
        return "connait" if cible["type"] == "pnj" else "autre"
    for motif, typ in (
            (r"^(ennemi|hostile)", "ennemi"), (r"^rival", "rival"), (r"^alli", "allie"),
            (r"^(conjoint|epou|mari\b|fiance)", "conjoint"), (r"^(amoureu|amant)", "amour"),
            (r"^(frere|soeur|fille|fils|pere|mere|parrain|filleul|cousin|oncle|tante|neveu|niece|famille)", "famille"),
            (r"^(ami|amie)\b", "ami")):
        if re.search(motif, k):
            return typ
    return "connait" if cible["type"] == "pnj" else "autre"


def cellules(l):
    l = l.strip()
    if not l.startswith("|"):
        return None
    c, cur, i = [], "", 1
    while i < len(l):
        if l[i] == "\\" and i + 1 < len(l) and l[i + 1] == "|":
            cur += "\\|"
            i += 2
            continue
        if l[i] == "|":
            c.append(cur.strip())
            cur = ""
        else:
            cur += l[i]
        i += 1
    if cur.strip():
        c.append(cur.strip())
    return c


def lignes_table(txt):
    for l in txt.splitlines():
        c = cellules(l)
        if c and len(c) >= 2 and not re.match(r"^:?-+:?$", c[0]) and plier(c[0]) not in ("avec", "faction"):
            yield c


# infiltration : Jehan Corbel dans les Sans-Gages (précisé par la fiche de la faction)
jehan, sg = REG.get(plier("Jehan Corbel")), REG.get(plier("Les Sans-Gages"))
if jehan and sg and "Jehan" in sg["data"].get("secrets", ""):
    relier(jehan, sg, "membre", rang="infiltré dans la Compagnie", secrete=True)

for f in sorted(fiches, key=lambda x: x["type"] != "faction"):
    if f["type"] == "pnj":
        for c in lignes_table(f["relations"]):
            cibles = liens_de(c[0])
            if cibles:
                cible = cibles[0]
                if cible["type"] == "faction" and (cible in f["factions"] or (f["id"], cible["id"], "membre") in VUES):
                    continue  # déjà dit par l'appartenance à la faction
                texte = reecrire(c[1]) if c[1] else ""
                relier(f, cible, type_rel(texte, cible), ligne(texte))
        for fa in f["factions"]:
            if fa["refs"].get("dirigeant") is not f:
                relier(f, fa, "membre")
    if f["type"] == "faction":
        dirig = f["refs"].get("dirigeant")
        for l in f["membres"].splitlines():
            m = re.match(r"\s*-\s*(.*)", l)
            if not m:
                continue
            cibles = liens_de(m.group(1))
            if not cibles:
                continue
            cible = cibles[0]
            reste = RE_LIEN.sub("", m.group(1), count=1)
            rang = ""
            mm = re.search(r"\(([^)]*)\)", reste)
            if mm:
                rang = mm.group(1)
            else:
                rang = re.sub(r"^[\s:—–-]+", "", reste).strip()
            rang = re.sub(r"\s+", " ", re.sub(r"[*_`\[\]|#]", "", RE_LIEN.sub(lambda m: re.split(r"\\\||\|", m.group(2))[-1].split("/")[-1], rang))).strip()[:80]
            if cible["type"] not in ("pnj", "faction"):
                continue
            if dirig is cible:
                continue
            relier(cible, f, "membre", rang=rang)
        for c in lignes_table(f["rel_fac"]):
            cibles = liens_de(c[0])
            if cibles and cibles[0]["type"] == "faction":
                texte = ligne(c[1])
                t = type_rel(texte, cibles[0])
                relier(f, cibles[0], t if t != "connait" else "autre", texte)
            elif cibles:
                relier(f, cibles[0], "autre", ligne(c[1]))
# ---------------------------------------------------------------- chronologie
evts = []
if CHRONO:
    for c in lignes_table("\n".join(x for h, x, niv in [("", CHRONO.intro and "\n".join(CHRONO.intro) or "", 2)] + CHRONO.sections)):
        if len(c) < 3 or not re.search(r"\d{4}", c[0]):
            continue
        evts.append(c)
campagne = next((f for f in fiches if f["type"] == "campagne"), None)


def nom_evt(txt):
    t = RE_LIEN.sub(lambda m: re.split(r"\\\||\|", m.group(2))[-1].split("/")[-1], txt)
    t = re.sub(r"[*_`\[\]|#]", "", t)
    t = re.sub(r"\s+", " ", t).strip()
    t = re.split(r"[;:]|, |\. | \(| — ", t)[0]
    t = t.strip()
    if len(t) > 70:
        t = t[:70].rsplit(" ", 1)[0]
    return t[:1].upper() + t[1:]


# titres tirés de la première phrase ; ceux qui seraient incomplets sont précisés ici
RENOMS_EVT = {"Aurèle Morvan": "Aurèle Morvan est recueilli par les Delorme",
              "Sélène Vauquelin": "Sélène Vauquelin disparaît pendant un an",
              "Après une attaque de gnolls": "Les Moissac se réfugient à Mont-Veille"}
noms_evt = set(noms)
for c in evts:
    date_txt, texte = c[0], c[2]
    source = c[3] if len(c) > 3 else ""
    an = int(re.search(r"\d{4}", date_txt).group(0))
    k = plier(date_txt)
    mois, jour, approx = 1, 1, True
    for nm, i in MOIS.items():
        if nm in k and nm != "fete":
            mois, approx = i, False
            md = re.search(r"(\d+)(?:er)?\s+" + nm, k)
            jour = int(md.group(1)) if md else 1
            break
    if k.startswith("fin "):
        mois, jour, approx = MOIS["fete"], 1, True
    cites = liens_de(texte + " " + source)
    nom = nom_evt(texte)
    nom = RENOMS_EVT.get(nom, nom)
    if plier(nom) in noms_evt:
        nom = f"{nom} ({an})"
    noms_evt.add(plier(nom))
    data = {"date_jeu": {"an": an, "mois": mois, "jour": jour},
            "verite": (f"*Date indiquée : {date_txt}.*\n\n" if approx or not re.fullmatch(r"\d+(er)? \w+ \d{4}", date_txt.strip()) else "") + propre(texte)}
    acteurs = [x["id"] for x in cites if x["type"] in ("pnj", "faction")]
    if acteurs:
        data["acteurs"] = acteurs
    lieu = next((x for x in cites if x["type"] == "lieu"), None)
    if lieu:
        data["lieu"] = lieu["id"]
    if campagne and an >= 1215:
        data["campagne"] = campagne["id"]
    api("POST", "/api/entites", {"type": "evenement", "nom": nom, "data": data, "tags": ["chronologie"]})
print(f"{len(evts)} événements créés")

# ---------------------------------------------------------------- secrets de campagne (repris des notes)
SECRETS = [
    ("La reine Cassandre a été tuée par Ophélie de Varenne", "capital", "passé", ["Ophélie de Varenne", "Aldéric"],
     "Ophélie a tué la reine pendant l'émeute. Personne ne le sait, pas même Aurèle Morvan. Fil pour une campagne ultérieure.", "Le noble derrière le refus de paiement"),
    ("Ophélie de Varenne est un changeforme", "capital", "identité", ["Ophélie de Varenne", "La Chambre des Pairs"],
     "Ophélie de Varenne est une créature déguisée (changeforme) ; c'est elle qui a poussé la cour à ne pas payer la Guilde des Bâtisseurs.", "Le noble derrière le refus de paiement"),
    ("Ophélie de Varenne sert un dragon ancien", "capital", "complot", ["Ophélie de Varenne"],
     "Ophélie sert un dragon ancien, boss d'une campagne bien plus lointaine. Rien n'en transparaît dans Les Moissons d'Ambreval.", "Le maître d'Ophélie"),
    ("Maëlle Morvan est vivante", "important", "identité", ["Maëlle Morvan", "Anselme Delorme"],
     "Anselme Delorme, parrain de Maëlle, la croit morte. S'il apprend qu'elle est vivante, il fera tout pour la protéger, y compris contre les PJ ou la couronne.", "Anselme Delorme, parrain de Maëlle"),
    ("Jehan Corbel détourne armes et rations pour les Sans-Gages", "important", "trahison", ["Jehan Corbel", "La Compagnie de Mont-Veille", "Les Sans-Gages"],
     "Jehan Corbel, milicien de la Compagnie, est infiltré pour les Sans-Gages et détourne armes et rations. Gautier Roussel ne le soupçonne pas.", None),
    ("Gautier Roussel est la première cible de l'Équerre", "important", "complot", ["Gautier Roussel", "L'Équerre", "Les Sans-Gages"],
     "Gautier Roussel est en tête de la liste de cibles de l'Équerre, l'assassin de la Tour du Guetteur, à « exécuter le jour du départ » du navire. Il l'ignore.", None),
]
for nom, imp, cat, concerne, verite, _ in SECRETS:
    ids = [REG[plier(x)]["id"] for x in concerne if plier(x) in REG]
    data = {"statut": "caché", "importance": imp, "categorie": cat, "concerne": ids, "verite": reecrire(verite)}
    if campagne:
        data["campagne"] = campagne["id"]
    api("POST", "/api/entites", {"type": "secret", "nom": nom, "data": data})
print(f"{len(SECRETS)} secrets créés")

vus = sorted(set(AVERT))
print(f"{len(vus)} avertissements")
for a in vus:
    print(" -", a)
