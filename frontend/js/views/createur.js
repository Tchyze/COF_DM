// Créateur de monstre. Tout ce qu'il affiche et calcule vient du fichier de règles de l'univers
// (GET /api/bestiaire/tables) : tailles, archétypes, rangs de boss, voies de créature, textes.
// Le calcul est fait par le serveur (POST /api/bestiaire/calculer).
import { $, $$, esc, api, toast } from "../ui.js";
import { signe, blocStats } from "./bestiaire.js";

export async function mount(el) {
  if (window.cofAcces?.lecture_seule) { location.replace("#/bestiaire"); return; }
  let T;
  try { T = await api("GET", "/api/bestiaire/tables"); }
  catch (e) {
    el.innerHTML = `<div class="entete"><h1>Créer un monstre selon les règles</h1><span class="sp"></span><a href="#/bestiaire"><button>◀ Bestiaire</button></a></div>
      <div class="panneau"><p class="alerte">${esc(e.message)}</p></div>`;
    return;
  }
  const C = T.creature, texte = C.textes || {};
  const tailles = Object.keys(C.table).sort((a, b) => (T.listes.tailles || []).indexOf(a) - (T.listes.tailles || []).indexOf(b));
  const archetypes = C.archetypes || [], boss = C.boss || [], armes = C.armes || [], voies = C.voies || [];
  const motsCles = C.mots_cles || {};
  const rangsMax = Math.max(1, C.rangs_max_voie || 1);
  const d = { nom: "", taille: tailles.find(t => !C.table[t].unique) || tailles[0], archetype: archetypes[0]?.cle || "", categorie: (T.listes.categories || [])[0] || "",
    type_boss: boss[0]?.cle || "", rangs_boss: 0, voies: {}, sup_magie: "INT", INT: 0, SAG: 0, CHA: 0, attaque_multiple: false, arme: armes[0]?.cle || "", nomAtk: "", typeAtk: "C" };
  let res = null;
  const opts = (liste, val) => liste.map(([v, l]) => `<option value="${esc(v)}"${String(v) === String(val) ? " selected" : ""}>${esc(l)}</option>`).join("");
  const titre = (n, cle, defaut) => `<h3>${n}. ${esc(texte[cle] || defaut)}</h3>`;
  const caracLibre = k => motsCles[k]?.length
    ? `<select id="${k}">${opts(motsCles[k].map(m => [m.valeur, `${m.nom} (${signe(m.valeur)})`]), 0)}</select>`
    : `<input type="number" id="${k}" value="0">`;

  el.innerHTML = `
    <div class="entete"><h1>Créer un monstre selon les règles</h1><span class="sp"></span><a href="#/bestiaire"><button>◀ Bestiaire</button></a></div>
    <div class="bst">
      <section class="panneau colonne" style="gap:12px">
        ${texte.intro ? `<p class="hint">${esc(texte.intro)}</p>` : ""}
        <h3>1. Concept</h3>
        <div class="bst-grille">
          <label class="champ" style="grid-column:span 2"><span>Nom</span><input id="nom"></label>
          <label class="champ"><span>Catégorie</span><input id="categorie" list="c-categories" value="${esc(d.categorie)}"><datalist id="c-categories">${(T.listes.categories || []).map(c => `<option value="${esc(c)}">`).join("")}</datalist></label>
        </div>
        ${titre(2, "titre_archetype", "Taille et archétype")}
        <div class="bst-grille">
          <label class="champ"><span>Taille</span><select id="taille">${opts(tailles.map(t => [t, t]), d.taille)}</select></label>
          <label class="champ" style="grid-column:span 2"><span>Archétype</span><select id="archetype">${opts(archetypes.map(a => [a.cle, a.cle]), d.archetype)}</select></label>
        </div>
        <p class="hint" id="arch-desc"></p>
        ${titre(3, "titre_mots_cles", "INT, SAG et CHA")}
        <div class="bst-grille">${["INT", "SAG", "CHA"].map(k => `<label class="champ" style="grid-column:span 2"><span>${k}</span>${caracLibre(k)}</label>`).join("")}</div>
        ${boss.length ? `${titre(4, "titre_boss", "Rangs de boss")}
        <div class="bst-grille">
          <label class="champ"><span>Rangs</span><input type="number" id="rangs_boss" min="0" max="10" value="0"></label>
          <label class="champ" style="grid-column:span 2"><span>Type</span><select id="type_boss">${opts(boss.map(b => [b.cle, b.libelle || b.cle]), d.type_boss)}</select></label>
        </div>
        <p class="hint" id="boss-desc"></p>` : ""}
        ${voies.length ? `${titre(5, "titre_voies", "Voies de créature")}
        <div class="colonne" style="gap:8px">${voies.map(v => `
          <div class="bst-voie"><div><b>${esc(v.nom)}</b> <span class="hint">${v.page ? `p. ${esc(v.page)}` : ""}${v.sup?.length ? " · " + esc(v.sup.join(", ")) + " sup." : v.choix ? " · INT, SAG ou CHA sup." : ""}</span>
            <div class="hint">${(v.rangs || []).map((r, i) => `${i + 1}. ${esc(r)}`).join(" · ")}</div></div>
            <div class="ligne" data-voie="${esc(v.cle)}" style="gap:2px">${Array.from({ length: Math.min(rangsMax, (v.rangs || []).length || rangsMax) + 1 }, (_, r) => `<button type="button" class="petit" data-r="${r}" aria-pressed="${r === 0}">${r}</button>`).join("")}</div>
          </div>`).join("")}</div>
        <label class="champ" id="sup-magie-zone" hidden><span>Caractéristique supérieure au choix</span><select id="sup_magie">${opts([["INT", "INT"], ["SAG", "SAG"], ["CHA", "CHA"]], "INT")}</select></label>` : ""}
        ${C.nc_attaque_multiple ? `<label class="t-coche"><input type="checkbox" id="attaque_multiple"><span>${esc(texte.attaque_multiple || "Attaque multiple")}</span></label>` : ""}
        <h3>6. Attaque</h3>
        <div class="bst-grille">
          <label class="champ" style="grid-column:span 2"><span>Nom de l'attaque</span><input id="nomAtk"></label>
          <label class="champ"><span>Type</span><select id="typeAtk">${opts([["C", "Contact"], ["D", "Distance"], ["M", "Magique"]], "C")}</select></label>
          ${armes.length ? `<label class="champ" style="grid-column:1/-1"><span>Dégâts</span><select id="arme">${opts(armes.map(a => [a.cle, a.libelle || a.cle]), d.arme)}</select></label>` : ""}
        </div>
        ${texte.aide_armes ? `<p class="hint">${esc(texte.aide_armes)}</p>` : ""}
      </section>
      <section class="panneau" id="c-res"><p class="vide">Calcul…</p></section>
    </div>`;

  const val = (id, defaut = "") => $("#" + id, el)?.value ?? defaut;
  const lireForm = () => {
    d.nom = val("nom").trim(); d.categorie = val("categorie").trim();
    d.taille = val("taille"); d.archetype = val("archetype");
    for (const k of ["INT", "SAG", "CHA"]) d[k] = +val(k) || 0;
    d.rangs_boss = Math.max(0, +val("rangs_boss") || 0); d.type_boss = val("type_boss");
    d.sup_magie = val("sup_magie", "INT"); d.attaque_multiple = !!$("#attaque_multiple", el)?.checked;
    d.nomAtk = val("nomAtk").trim(); d.typeAtk = val("typeAtk", "C"); d.arme = val("arme");
  };
  // le dé de l'arme choisie remplace celui de l'archétype, sauf pour les tailles qui gardent le leur
  const dmFinal = dm => {
    const de = armes.find(a => a.cle === d.arme)?.de;
    if (!de || (C.tailles_sans_arme || []).includes(d.taille)) return dm;
    const m = dm.match(/^\d+d\d+(.*)$/);
    return de + (m ? m[1] : "");
  };

  async function calculer() {
    lireForm();
    const unique = !!C.table[d.taille]?.unique;
    $("#archetype", el).disabled = unique;
    $("#arch-desc", el).textContent = unique ? (texte.archetype_unique || "") : (archetypes.find(a => a.cle === d.archetype)?.description || "");
    const bd = $("#boss-desc", el);
    if (bd) bd.textContent = d.rangs_boss ? (boss.find(b => b.cle === d.type_boss)?.description || "") : (texte.sans_rang_boss || "");
    const zoneSup = $("#sup-magie-zone", el);
    if (zoneSup) zoneSup.hidden = !voies.some(v => v.choix && d.voies[v.cle] > 0);
    const zone = $("#c-res", el);
    try { res = await api("POST", "/api/bestiaire/calculer", d); } catch (e) { zone.innerHTML = `<p class="alerte">${esc(e.message)}</p>`; return; }
    const attaque = [{ nom: d.nomAtk || "Attaque", bonus: res.att, dm: dmFinal(res.dm) }];
    zone.innerHTML = `
      <h2 style="margin-top:0">${esc(d.nom || "Nouvelle créature")}</h2>
      <p class="hint">[${esc(res.conception)}]</p>
      ${blocStats({ ...res, categorie: d.categorie, taille: d.taille }, attaque)}
      ${res.capacites ? `<div class="bst-texte">${esc(res.capacites)}</div>` : ""}
      ${res.alertes.length ? `<div class="alerte">${res.alertes.map(esc).join("<br>")}</div>` : ""}
      <details><summary>Détail du calcul</summary><ul>${res.detail.map(x => `<li>${esc(x)}</li>`).join("")}</ul></details>
      ${texte.aide_capacites ? `<p class="hint">${esc(texte.aide_capacites)}</p>` : ""}
      <div><button class="primary" id="c-enregistrer">Enregistrer dans le bestiaire</button></div>`;
    $("#c-enregistrer", zone).onclick = enregistrer;
  }

  async function enregistrer() {
    if (!d.nom) { toast("Donnez un nom à la créature.", true); $("#nom", el).focus(); return; }
    const fiche = {
      nom: d.nom, source: "maison", nc: res.nc, categorie: d.categorie, taille: d.taille, tags: ["créé selon les règles"],
      carac: res.carac, superieures: res.superieures || [], defense: res.defense, pv: res.pv, init: res.init, rd: res.rd, crit: 20,
      attaques: [{ nom: d.nomAtk || "Attaque", type: d.typeAtk, bonus: res.att, dm: dmFinal(res.dm) }],
      capacites: res.capacites, description: "", notes: `Conception : ${res.conception}`,
    };
    try {
      const m = await api("POST", "/api/bestiaire", fiche);
      toast("Créature ajoutée au bestiaire.");
      location.hash = `#/bestiaire/${m.id}`;
    } catch (e) { toast(e.message, true); }
  }

  el.addEventListener("input", e => { if (e.target.closest("input,select")) calculer(); });
  el.addEventListener("click", e => {
    const b = e.target.closest("[data-voie] button");
    if (!b) return;
    const seg = b.parentElement, r = +b.dataset.r;
    $$("button", seg).forEach(x => x.setAttribute("aria-pressed", x === b));
    if (r) d.voies[seg.dataset.voie] = r; else delete d.voies[seg.dataset.voie];
    calculer();
  });
  calculer();
}
