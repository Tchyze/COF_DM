// Une fiche : lecture, édition, création.
import { $, $$, esc, api, toast, armer, dateHeure, lien } from "../ui.js";
import { schema, types, typeDe, relationDe, vignette, lienFiche, badgeType, valeurCourte, actif as fonctionActive } from "../schema.js";
import { rendu, inline, resolveur } from "../md.js";
import { selecteur, saisieDate, completerLiens, hauteurAuto, selecteurMonstre, editeurAdversaires, editeurChiffres, texteChiffres, listeEmplacements, editeurButin, texteButin } from "../composants.js";
import * as panneaux from "./panneaux.js";
import { panneauFeuille } from "./feuille.js";
import { organigramme } from "./organigramme.js";

let courant = "";
export const typeCourant = () => courant;

export async function mount(el, id, edition) {
  const f = await api("GET", "/api/entites/" + id);
  courant = f.type;
  document.title = f.nom + " — COF DM";
  return edition ? formulaire(el, f, false) : lecture(el, f);
}

/** Nouvelle fiche : choix du type si besoin, puis formulaire. Les paramètres préremplissent. */
export async function creer(el, type, params) {
  const nom = params.get("nom") || "";
  if (!typeDe(type)) {
    courant = "";
    el.innerHTML = `<div class="entete"><h1>${nom ? `Créer « ${esc(nom)} »` : "Nouvelle fiche"}</h1></div>
      <p class="hint" style="margin-bottom:12px">Quel genre de fiche ?</p>
      <div class="compteurs">${types().map(t => `<a href="#/nouveau/${t.cle}${nom ? "?nom=" + encodeURIComponent(nom) : ""}" data-type="${t.cle}">${vignette({ type: t.cle }, 36)}<span><b style="font:600 1rem var(--f-display)">${esc(t.nom)}</b></span></a>`).join("")}</div>`;
    return;
  }
  courant = type;
  document.title = "Nouvelle fiche — COF DM";
  const t = typeDe(type);
  const data = {};
  for (const [k, v] of params) {
    const ch = t.champs.find(c => c.cle === k);
    if (!ch) continue;
    data[k] = ch.genre === "ref" ? +v : ch.genre === "refs" ? v.split(",").map(Number) : v;
  }
  const refs = {};
  for (const ch of t.champs) {
    if ((ch.genre === "ref" || ch.genre === "refs") && data[ch.cle]) {
      for (const id of [].concat(data[ch.cle])) {
        const r = await api("GET", "/api/entites/" + id).catch(() => null);
        if (r) refs[id] = r;
      }
    }
  }
  return formulaire(el, { type, nom, data, tags: [], refs, nouveau: true }, true);
}

// ---------------------------------------------------------------- lecture

const PANNEAUX = {
  pnj: panneaux.combatPNJ, rencontre: panneaux.rencontre,
  seance: panneaux.combatsSeance, secret: panneaux.quiSait, aide: panneaux.envois, campagne: panneaux.potCommun,
};
export const COURTS = new Set(["texte", "choix", "nombre", "booleen", "ref", "refs", "date_jeu", "date"]);
export const RESUMES = ["resume", "impression", "but"];

function lecture(el, f) {
  const t = typeDe(f.type);
  const res = resolveur(f.mentions);
  const refs = f.refs || {};
  const resume = RESUMES.map(k => f.data[k]).find(Boolean);
  const courts = t.champs.filter(c => COURTS.has(c.genre) && !RESUMES.includes(c.cle) && f.data[c.cle] != null && f.data[c.cle] !== "");
  const groupes = {};
  for (const c of courts) (groupes[c.groupe || ""] ||= []).push(c);
  const longs = t.champs.filter(c => c.genre === "long" && f.data[c.cle]);
  const etapes = t.champs.find(c => c.genre === "etapes");
  const chiffres = t.champs.find(c => c.genre === "equipement" && f.data[c.cle]); // ceux d'un objet
  // sur un serveur, le MJ choisit les fiches que les joueurs voient (jamais une fiche « Secret »)
  const partageable = !!window.cofAcces?.serveur && !window.cofAcces.lecture_seule && f.type !== "secret" && f.type !== "aide";
  // la feuille de personnage (les joueurs tiennent la leur sur leur page)
  const feuille = f.type === "pj" && fonctionActive("feuilles");
  // panneau propre au type ; celui d'une séance montre ses combats
  const special = (f.type === "seance" && !fonctionActive("combat")) || (f.type === "campagne" && !fonctionActive("butin")) ? null : PANNEAUX[f.type];
  // le butin chiffré d'une rencontre, et son versement au pot commun
  const butin = t.champs.find(c => c.genre === "butin" && f.data[c.cle]);
  const versable = butin && !f.data.butin_verse && !window.cofAcces?.lecture_seule && f.data.campagne;

  el.innerHTML = `
    <div class="fiche-tete" data-type="${f.type}">
      ${vignette(f, 112)}
      <div class="infos">
        <div class="ligne">${badgeType(f.type)}${f.archive ? `<span class="badge archive">archivée</span>` : ""}${partageable && f.partage ? `<span class="badge tag" title="Les joueurs voient cette fiche, sans ses champs réservés au MJ">👁 visible des joueurs</span>` : ""}${f.tags.map(g => `<a class="badge tag" href="#/t/${f.type}?tag=${encodeURIComponent(g)}">#${esc(g)}</a>`).join("")}</div>
        <h1>${esc(f.nom)}</h1>
        ${resume ? `<div class="resume">${inline(resume, res)}</div>` : ""}
        <div class="actions">
          <a href="#/f/${f.id}/modifier"><button class="primary">✎ Modifier</button></a>
          ${fonctionActive("graphe") ? `<a href="#/graphe/${f.id}"><button>🕸 Graphe</button></a>` : ""}
          ${f.type === "seance" && fonctionActive("seances") ? `<a href="#/preparer/${f.id}"><button title="Rassembler les quêtes, secrets, PNJ et rencontres de la campagne">🧭 Préparer</button></a>` : ""}
          ${f.type === "seance" && !window.cofAcces?.lecture_seule && fonctionActive("seances") ? `<a href="#/apres/${f.id}"><button title="Reporter ce qui s'est passé et préparer la séance suivante">✅ Après la séance</button></a>` : ""}
          ${liensCreation(f)}
          <span style="flex:1"></span>
          ${partageable ? `<button id="partager" class="${f.partage ? "" : "ghost"}" title="Les joueurs ne voient jamais les champs réservés au MJ">${f.partage ? "Retirer aux joueurs" : "👁 Partager avec les joueurs"}</button>` : ""}
          <button id="archiver" class="${f.archive ? "" : "ghost"}">${f.archive ? "Restaurer" : "Archiver"}</button>
          ${f.archive ? `<button id="supprimer" class="danger">Supprimer définitivement</button>` : ""}
        </div>
      </div>
    </div>
    ${f.type === "faction" && fonctionActive("organigramme") ? `<section class="panneau" id="organigramme" style="margin-bottom:18px"></section>` : ""}
    <div class="fiche-corps">
      <div class="colonne">
        ${Object.entries(groupes).map(([g, cs]) => `<section class="panneau"${g === "MJ" ? ' style="border-style:dashed"' : ""}>
          <h3>${esc(g || "Informations")}</h3>
          <dl class="proprietes">${cs.map(c => `<div><dt>${esc(c.nom)}</dt><dd>${f.dates?.[c.cle] ? esc(f.dates[c.cle]) : valeurCourte(f.type, c.cle, f.data[c.cle], refs)}</dd></div>`).join("")}</dl>
        </section>`).join("")}
        ${feuille ? `<section class="panneau" id="p-feuille"></section>` : ""}
        ${special ? `<section class="panneau" id="p-special"></section>` : ""}
        ${etapes && f.data[etapes.cle]?.length ? `<section class="panneau"><h3>${esc(etapes.nom)} <span class="sp"></span><span class="hint" style="text-transform:none;letter-spacing:0">${f.data[etapes.cle].filter(e => e.fait).length} / ${f.data[etapes.cle].length}</span></h3>
          <ul class="etapes" id="etapes">${f.data[etapes.cle].map((e, i) => `<li><label><input type="checkbox" data-i="${i}" ${e.fait ? "checked" : ""}><span class="${e.fait ? "fait" : ""}">${inline(e.texte, res)}</span></label></li>`).join("")}</ul></section>` : ""}
        ${chiffres ? `<section class="panneau"><h3>${esc(chiffres.nom)}</h3><p>${f.data[chiffres.cle].emplacement ? `<span class="hint" id="chiffres-empl">${esc(f.data[chiffres.cle].emplacement)}</span> · ` : ""}<span class="fp-objet">${esc(texteChiffres(f.data[chiffres.cle]) || "aucun bonus")}</span></p>
          <p class="hint">Ils comptent quand un PJ le porte : le détenteur l'a dans son inventaire.</p></section>` : ""}
        ${butin ? `<section class="panneau"><h3>${esc(butin.nom)}${f.data.butin_verse ? ` <span class="badge">versé au pot</span>` : ""}</h3><p>${esc(texteButin(f.data[butin.cle], refs))}</p>
          ${versable ? `<div class="ligne"><button class="primary petit" id="verser">💰 Verser au pot commun</button><span class="hint">de ${f.refs?.[f.data.campagne] ? esc(f.refs[f.data.campagne].nom) : "la campagne"} ; la rencontre passe à « jouée »</span></div>`
            : !f.data.butin_verse && !f.data.campagne ? `<p class="hint">Rattachez la rencontre à une campagne pour verser ce butin à son pot commun.</p>` : ""}</section>` : ""}
        ${longs.map(c => `<section class="panneau section-md${c.secret ? " secret" : ""}"><h3>${esc(c.nom)}</h3><div class="md">${rendu(f.data[c.cle], res)}</div></section>`).join("")}
        ${!courts.length && !longs.length && !resume ? `<section class="panneau"><p class="vide">Fiche vide. <a href="#/f/${f.id}/modifier">La remplir</a>.</p></section>` : ""}
        ${sectionReferences(f)}
      </div>
      <aside class="colonne">
        ${["pnj", "pj", "faction"].includes(f.type) && fonctionActive("secrets") ? `<section class="panneau" id="p-secrets"></section>` : ""}
        <section class="panneau" id="relations"></section>
        <section class="panneau" id="images"></section>
        <section class="panneau meta-fiche">
          <span>Fiche n°${f.id} · créée le ${dateHeure(f.cree)}</span>
          <span>modifiée le ${dateHeure(f.modifie)}</span>
        </section>
      </aside>
    </div>`;

  relations(el, f);
  images(el, f);
  const empl = $("#chiffres-empl", el);
  if (empl) listeEmplacements().then(l => { empl.textContent = l.find(e => e.cle === empl.textContent)?.nom || empl.textContent; });
  const rafraichir = () => recharger(el, f.id);
  if (feuille) panneauFeuille($("#p-feuille", el), f, { peutEcrire: !window.cofAcces?.lecture_seule, apres: rafraichir });
  if (special) special($("#p-special", el), f, rafraichir);
  if ($("#p-secrets", el)) panneaux.secretsDe($("#p-secrets", el), f);
  const verser = $("#verser", el);
  if (verser) armer(verser, "Verser ?", async () => {
    try {
      await api("POST", `/api/entites/${f.id}/verser-butin`, {});
      toast("Butin versé au pot commun de la campagne.");
      window.dispatchEvent(new Event("cof-change"));
      recharger(el, f.id);
    } catch (err) { toast(err.message, true); }
  });
  const partager = $("#partager", el);
  if (partager) partager.onclick = async () => {
    try {
      await api("POST", `/api/entites/${f.id}/partage`, { partage: !f.partage });
      toast(f.partage ? "Les joueurs ne voient plus cette fiche." : "Les joueurs voient cette fiche, sans ses champs réservés au MJ.");
      recharger(el, f.id);
    } catch (err) { toast(err.message, true); }
  };
  if ($("#organigramme", el)) organigramme($("#organigramme", el), f, () => { window.dispatchEvent(new Event("cof-change")); recharger(el, f.id); });
  $(".fiche-tete .vignette", el).onclick = () => { if (f.medias.length) visionneuse(f, 0, () => recharger(el, f.id)); else $("#depot", el)?.click(); };

  const coches = $("#etapes", el);
  if (coches) coches.onchange = async e => {
    const i = +e.target.dataset.i;
    const liste = f.data[etapes.cle].map((x, j) => j === i ? { ...x, fait: e.target.checked } : x);
    try {
      const n = await api("PATCH", "/api/entites/" + f.id, { data: { [etapes.cle]: liste } });
      Object.assign(f, n);
      e.target.nextElementSibling.classList.toggle("fait", e.target.checked);
      window.dispatchEvent(new Event("cof-change"));
    } catch (err) { toast(err.message, true); e.target.checked = !e.target.checked; }
  };
  const arch = $("#archiver", el);
  const basculer = async () => {
    try {
      await api("POST", `/api/entites/${f.id}/archive`, { archive: !f.archive });
      toast(f.archive ? "Fiche restaurée." : "Fiche archivée : elle n'apparaît plus dans les listes.");
      window.dispatchEvent(new Event("cof-change"));
      recharger(el, f.id);
    } catch (err) { toast(err.message, true); }
  };
  if (f.archive) arch.onclick = basculer; else armer(arch, "Confirmer l'archivage", basculer);
  const sup = $("#supprimer", el);
  if (sup) armer(sup, "Confirmer : effacer pour toujours", async () => {
    try {
      await api("DELETE", `/api/entites/${f.id}?definitif=1`);
      toast("Fiche supprimée.");
      window.dispatchEvent(new Event("cof-change"));
      location.hash = "#/t/" + f.type;
    } catch (err) { toast(err.message, true); }
  });
  const raccourci = e => {
    if (e.target.closest("input,textarea,select")) return;
    if (e.key === "e" && !e.ctrlKey && !e.metaKey) location.hash = `#/f/${f.id}/modifier`;
  };
  document.addEventListener("keydown", raccourci);
  return () => document.removeEventListener("keydown", raccourci);
}

async function recharger(el, id) {
  const f = await api("GET", "/api/entites/" + id);
  const ancien = el;
  const nouveau = ancien.cloneNode(false);
  ancien.replaceWith(nouveau);
  lecture(nouveau, f);
}

/** Boutons de création de fiches rattachées (quête d'une campagne, PNJ d'un lieu…). */
function liensCreation(f) {
  const liens = [];
  for (const t of types()) {
    for (const c of t.champs) {
      if ((c.genre === "ref" || c.genre === "refs") && c.cibles.includes(f.type) && ["campagne", "campagnes", "lieu", "parent", "quete", "seance"].includes(c.cle)) {
        liens.push(`<a href="#/nouveau/${t.cle}?${c.cle}=${f.id}" title="${esc(t.nom)} : ${esc(c.nom)} ${esc(f.nom)}"><button class="ghost petit">＋ ${esc(t.nom)}</button></a>`);
        break;
      }
    }
  }
  return liens.slice(0, 6).join("");
}

/** Fiches qui pointent vers celle-ci, regroupées par type et par champ. */
function sectionReferences(f) {
  if (!f.references.length) return "";
  const groupes = new Map();
  for (const r of f.references) {
    const t = typeDe(r.source.type);
    const cle = r.genre === "mention" ? `Mentionné dans` : `${t.pluriel} — ${r.champ}`;
    if (!groupes.has(cle)) groupes.set(cle, []);
    if (!groupes.get(cle).some(x => x.id === r.source.id)) groupes.get(cle).push(r.source);
  }
  return `<section class="panneau"><h3>Liée à cette fiche</h3>
    ${[...groupes].map(([g, l]) => `<div class="rel-groupe"><small>${esc(g)} (${l.length})</small><div class="ligne" style="gap:4px 14px">${l.map(x => lienFiche(x)).join("")}</div></div>`).join("")}
  </section>`;
}

// ---------------------------------------------------------------- relations

function relations(el, f) {
  const box = $("#relations", el);
  const groupes = new Map();
  // sur une faction, membres et dirigeants sont dans l'organigramme
  const dansOrga = r => f.type === "faction" && r.sens === "entrante" && (r.type === "membre" || r.type === "dirige");
  const nbOrga = f.relations.filter(dansOrga).length;
  for (const r of f.relations) {
    if (dansOrga(r) || r.type === "sait" || (f.type === "aide" && r.type === "recu")) continue; // envois : leur panneau
    if (!groupes.has(r.libelle)) groupes.set(r.libelle, []);
    groupes.get(r.libelle).push(r);
  }
  box.innerHTML = `<h3>Relations <span class="sp"></span><button class="petit" id="rel-ajout">＋ Relier</button></h3>
    <form id="rel-form" class="formulaire" hidden style="gap:8px">
      <select id="rel-type" aria-label="Type de relation">
        ${schema.relations.filter(r => !r.reservee).map(r => `<option value="${r.cle}:a">${esc(f.nom)} ${esc(r.nom)}…</option>${r.inverse !== r.nom ? `<option value="${r.cle}:b">${esc(f.nom)} ${esc(r.inverse)}…</option>` : ""}`).join("")}
      </select>
      <div id="rel-cible"></div>
      <input id="rel-rang" placeholder="Rang dans la faction" hidden>
      <input id="rel-note" placeholder="Note (facultatif)">
      <label class="ligne" style="gap:8px;font-size:.85rem;color:var(--muted)"><input type="checkbox" id="rel-secret"> Relation secrète (connue du MJ seulement)</label>
      <div class="ligne"><button class="primary petit" type="submit">Relier</button><button class="ghost petit" type="button" id="rel-annuler">Annuler</button></div>
    </form>
    ${groupes.size ? [...groupes].map(([lib, l]) => `<div class="rel-groupe"><small>${esc(lib)}</small>${l.map(r => `<div class="rel">${lienFiche(r.autre)}${r.rang ? `<span class="note">${esc(r.rang)}</span>` : ""}${r.note && r.type !== "autre" && r.note !== r.rang ? `<span class="note">${esc(r.note)}</span>` : ""}${r.secrete ? `<span title="Secret (MJ)">🔒</span>` : ""}<button class="ghost petit" data-suppr="${r.id}" title="Retirer la relation" aria-label="Retirer">✕</button></div>`).join("")}</div>`).join("")
      : nbOrga ? "" : `<p class="vide">Aucune relation.${window.cofAcces?.lecture_seule ? "" : " Reliez cette fiche à ses alliés, ennemis, membres, lieux…"}</p>`}
    ${nbOrga ? `<p class="hint">${nbOrga} membre${nbOrga > 1 ? "s" : ""} et dirigeant${nbOrga > 1 ? "s" : ""} : voir l'organigramme.</p>` : ""}`;
  const form = $("#rel-form", box);
  let cible = null;
  $("#rel-ajout", box).onclick = () => {
    form.hidden = !form.hidden;
    if (!form.hidden && !cible) cible = selecteur($("#rel-cible", box), { cibles: [], multiple: false });
    if (!form.hidden) $("#rel-cible input", box).focus();
  };
  $("#rel-annuler", box).onclick = () => { form.hidden = true; };
  $("#rel-type", box).onchange = e => { $("#rel-rang", box).hidden = !e.target.value.startsWith("membre:"); };
  form.onsubmit = async e => {
    e.preventDefault();
    const autre = cible?.valeur();
    if (!autre) { toast("Choisissez la fiche à relier.", true); return; }
    const [type, sens] = $("#rel-type", box).value.split(":");
    const corps = sens === "a" ? { a: f.id, b: autre, type } : { a: autre, b: f.id, type };
    corps.note = $("#rel-note", box).value;
    corps.secrete = $("#rel-secret", box).checked;
    if (type === "membre") corps.rang = $("#rel-rang", box).value;
    try {
      await api("POST", "/api/relations", corps);
      window.dispatchEvent(new Event("cof-change"));
      recharger(el, f.id);
    } catch (err) { toast(err.message, true); }
  };
  box.onclick = async e => {
    const b = e.target.closest("button[data-suppr]");
    if (!b) return;
    try {
      await api("DELETE", "/api/relations/" + b.dataset.suppr);
      recharger(el, f.id);
    } catch (err) { toast(err.message, true); }
  };
}

// ---------------------------------------------------------------- images

function images(el, f) {
  const box = $("#images", el);
  // lecture seule (auditeur) : rien à déposer, le cadre n'apparaît qu'avec des images
  box.hidden = !!window.cofAcces?.lecture_seule && !f.medias.length;
  if (box.hidden) return;
  box.innerHTML = `<h3>Images</h3>
    ${f.medias.length ? `<div class="galerie">${f.medias.map((m, i) => `<figure class="${m.principal ? "principale" : ""}" data-i="${i}" title="${esc(m.legende || m.nom_original)}"><img src="${lien("/medias/" + esc(m.fichier))}" alt="${esc(m.legende)}" loading="lazy"></figure>`).join("")}</div>` : ""}
    <div class="depot" id="depot" tabindex="0" role="button">Déposer une image ici ou cliquer (PNG, JPEG, WebP)</div>
    <input type="file" id="fichier" accept="image/png,image/jpeg,image/webp,image/gif" multiple hidden>`;
  const depot = $("#depot", box), input = $("#fichier", box);
  const envoyer = async fichiers => {
    for (const fi of fichiers) {
      const fd = new FormData();
      fd.append("fichier", fi);
      try { await api("POST", `/api/entites/${f.id}/medias`, fd); }
      catch (err) { toast(`${fi.name} : ${err.message}`, true); }
    }
    window.dispatchEvent(new Event("cof-change"));
    recharger(el, f.id);
  };
  depot.onclick = () => input.click();
  depot.onkeydown = e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); input.click(); } };
  input.onchange = () => envoyer([...input.files]);
  depot.ondragover = e => { e.preventDefault(); depot.classList.add("survol"); };
  depot.ondragleave = () => depot.classList.remove("survol");
  depot.ondrop = e => { e.preventDefault(); depot.classList.remove("survol"); envoyer([...e.dataTransfer.files]); };
  box.onclick = e => { const fig = e.target.closest("figure[data-i]"); if (fig) visionneuse(f, +fig.dataset.i, () => recharger(el, f.id)); };
}

function visionneuse(f, i, apres) {
  const m = f.medias[i];
  const v = document.createElement("div");
  v.className = "visionneuse";
  v.innerHTML = `<img src="${lien("/medias/" + esc(m.fichier))}" alt="">
    <div class="ligne" style="width:min(640px,100%)"><input id="leg" value="${esc(m.legende)}" placeholder="Légende" style="flex:1"></div>
    <div class="ligne">
      ${f.medias.length > 1 ? `<button id="prec">←</button><button id="suiv">→</button>` : ""}
      ${m.principal ? "" : `<button id="principale">★ Image principale</button>`}
      <button id="suppr" class="danger">Supprimer</button><button id="fermer">Fermer</button>
    </div>`;
  document.body.append(v);
  let change = false;
  const fermer = async () => {
    const leg = $("#leg", v).value;
    if (leg !== m.legende) { await api("PATCH", "/api/medias/" + m.id, { legende: leg }).catch(() => {}); change = true; }
    v.remove();
    document.removeEventListener("keydown", clavier);
    if (change) apres();
  };
  const aller = d => { v.remove(); document.removeEventListener("keydown", clavier); visionneuse(f, (i + d + f.medias.length) % f.medias.length, apres); };
  const clavier = e => {
    if (e.target.id === "leg" && e.key !== "Escape") return;
    if (e.key === "Escape") fermer();
    if (e.key === "ArrowLeft") aller(-1);
    if (e.key === "ArrowRight") aller(1);
  };
  document.addEventListener("keydown", clavier);
  $("#fermer", v).onclick = fermer;
  v.onclick = e => { if (e.target === v) fermer(); };
  if ($("#prec", v)) { $("#prec", v).onclick = () => aller(-1); $("#suiv", v).onclick = () => aller(1); }
  const p = $("#principale", v);
  if (p) p.onclick = async () => { await api("PATCH", "/api/medias/" + m.id, { principal: true }); change = true; window.dispatchEvent(new Event("cof-change")); fermer(); };
  armer($("#suppr", v), "Confirmer", async () => { await api("DELETE", "/api/medias/" + m.id); change = true; window.dispatchEvent(new Event("cof-change")); fermer(); });
}

// ---------------------------------------------------------------- formulaire

function formulaire(el, f, nouveau) {
  const t = typeDe(f.type);
  const refs = f.refs || {};
  const groupes = [];
  for (const c of t.champs) {
    const g = c.groupe || "Informations";
    let grp = groupes.find(x => x.nom === g);
    if (!grp) groupes.push(grp = { nom: g, champs: [] });
    grp.champs.push(c);
  }
  el.innerHTML = `
    <div class="entete" data-type="${f.type}">${vignette(f, 42)}<h1>${nouveau ? `${t.nom === "PNJ" || t.nom === "PJ" ? "Nouveau" : "Nouvelle fiche :"} ${esc(t.nom)}` : `Modifier ${esc(f.nom)}`}</h1></div>
    <form class="formulaire" id="form" novalidate>
      <div class="erreur" id="err" hidden></div>
      <fieldset>
        <label class="champ pleine nom-fiche"><span>Nom</span><input id="nom" value="${esc(f.nom)}" required autocomplete="off"></label>
        <label class="champ pleine"><span>Tags <small>séparés par des virgules</small></span><input id="tags" value="${esc((f.tags || []).join(", "))}" autocomplete="off"></label>
      </fieldset>
      ${groupes.map(g => `<fieldset class="${g.nom === "MJ" ? "mj" : ""}"><legend>${g.nom === "MJ" ? "🔒 Réservé au MJ" : esc(g.nom)}</legend>
        ${g.champs.map(c => `<div class="${["long", "etapes", "refs", "adversaires", "equipement", "butin"].includes(c.genre) ? "pleine" : ""}" data-champ="${c.cle}"></div>`).join("")}
      </fieldset>`).join("")}
      <div class="barre-form">
        <span class="hint" style="margin-right:auto;align-self:center"><kbd>Ctrl</kbd>+<kbd>S</kbd> enregistrer · <kbd>Échap</kbd> annuler · <code>[[</code> pour lier une fiche</span>
        <button type="button" class="ghost" id="annuler">Annuler</button>
        <button type="submit" class="primary">${nouveau ? "Créer" : "Enregistrer"}</button>
      </div>
    </form>`;
  const lecteurs = {};
  let modifie = false;
  const marquer = () => { modifie = true; };
  for (const c of t.champs) {
    const zone = $(`[data-champ="${c.cle}"]`, el);
    const v = f.data[c.cle];
    const etiquette = `<span>${esc(c.nom)}${c.aide ? ` <small>${esc(c.aide)}</small>` : ""}</span>`;
    switch (c.genre) {
      case "texte": {
        zone.innerHTML = `<label class="champ">${etiquette}<input value="${esc(v ?? "")}" ${c.options ? `list="dl-${c.cle}"` : ""} autocomplete="off"></label>${c.options ? `<datalist id="dl-${c.cle}">${c.options.map(o => `<option value="${esc(o)}">`).join("")}</datalist>` : ""}`;
        const i = $("input", zone); lecteurs[c.cle] = () => i.value.trim() || null; break;
      }
      case "long": {
        zone.innerHTML = `<label class="champ">${etiquette}<textarea spellcheck="true">${esc(v ?? "")}</textarea></label>`;
        const ta = $("textarea", zone); hauteurAuto(ta); completerLiens(ta);
        lecteurs[c.cle] = () => ta.value.trim() ? ta.value : null; break;
      }
      case "choix": {
        zone.innerHTML = `<label class="champ">${etiquette}<select><option value=""></option>${c.options.map(o => `<option ${v === o ? "selected" : ""}>${esc(o)}</option>`).join("")}</select></label>`;
        const s = $("select", zone); lecteurs[c.cle] = () => s.value || null; break;
      }
      case "nombre": {
        zone.innerHTML = `<label class="champ">${etiquette}<input inputmode="decimal" value="${esc(v ?? "")}"></label>`;
        const i = $("input", zone); lecteurs[c.cle] = () => i.value.trim() === "" ? null : i.value.trim(); break;
      }
      case "booleen": {
        zone.innerHTML = `<label class="champ" style="flex-direction:row;align-items:center;gap:8px;padding-top:18px"><input type="checkbox" ${v ? "checked" : ""}>${etiquette}</label>`;
        const i = $("input", zone); lecteurs[c.cle] = () => i.checked || null; break;
      }
      case "date": {
        zone.innerHTML = `<label class="champ">${etiquette}<input type="date" value="${esc(v ?? "")}"></label>`;
        const i = $("input", zone); lecteurs[c.cle] = () => i.value || null; break;
      }
      case "ref": case "refs": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="w"></div></div>`;
        const s = selecteur($(".w", zone), { cibles: c.cibles, multiple: c.genre === "refs", valeur: v, refs, auChangement: marquer });
        lecteurs[c.cle] = () => { const x = s.valeur(); return (Array.isArray(x) ? x.length : x) ? x : null; }; break;
      }
      case "date_jeu": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="w"></div></div>`;
        const d = saisieDate($(".w", zone), v, marquer);
        lecteurs[c.cle] = d.valeur; break;
      }
      case "monstre": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="w"></div></div>`;
        const s = selecteurMonstre($(".w", zone), { valeur: v, auChangement: marquer });
        lecteurs[c.cle] = s.valeur; break;
      }
      case "adversaires": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="w"></div></div>`;
        const ed = editeurAdversaires($(".w", zone), v, marquer);
        lecteurs[c.cle] = ed.valeur; break;
      }
      case "equipement": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="w"></div></div>`;
        const ed = editeurChiffres($(".w", zone), v, marquer);
        lecteurs[c.cle] = ed.valeur; break;
      }
      case "butin": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="w"></div></div>`;
        const ed = editeurButin($(".w", zone), v, refs, marquer);
        lecteurs[c.cle] = ed.valeur; break;
      }
      case "etapes": {
        zone.innerHTML = `<div class="champ"><label class="champ">${etiquette}</label><div class="liste-etapes"></div><div><button type="button" class="petit">＋ Étape</button></div></div>`;
        const liste = $(".liste-etapes", zone);
        const ligne = (e = { texte: "", fait: false }) => {
          const d = document.createElement("div");
          d.className = "etape";
          d.innerHTML = `<input type="checkbox" ${e.fait ? "checked" : ""} aria-label="Fait"><input value="${esc(e.texte)}" placeholder="Étape…"><button type="button" class="ghost petit" aria-label="Retirer">✕</button>`;
          $("button", d).onclick = () => { d.remove(); marquer(); };
          liste.append(d);
          return d;
        };
        (v || []).forEach(e => ligne(e));
        $("button.petit", zone).onclick = () => $$("input", ligne())[1].focus();
        lecteurs[c.cle] = () => {
          const l = $$(".etape", liste).map(d => ({ fait: $$("input", d)[0].checked, texte: $$("input", d)[1].value.trim() })).filter(e => e.texte);
          return l.length ? l : null;
        };
        break;
      }
    }
  }
  const form = $("#form", el);
  form.addEventListener("input", marquer);
  form.addEventListener("change", marquer);
  $("#nom", el).focus();
  const annuler = () => {
    if (!nouveau) location.hash = `#/f/${f.id}`;
    else if (history.length > 1) history.back();
    else location.hash = "#/t/" + f.type;
  };
  $("#annuler", el).onclick = annuler;
  form.onsubmit = async e => {
    e.preventDefault();
    const err = $("#err", el);
    err.hidden = true;
    const data = {};
    for (const [k, lire] of Object.entries(lecteurs)) {
      const val = lire();
      if (val != null || !nouveau) data[k] = val;
    }
    const tags = $("#tags", el).value.split(",").map(x => x.trim()).filter(Boolean);
    try {
      const r = nouveau
        ? await api("POST", "/api/entites", { type: f.type, nom: $("#nom", el).value, data, tags })
        : await api("PATCH", "/api/entites/" + f.id, { nom: $("#nom", el).value, data, tags, modifie: f.modifie });
      modifie = false;
      toast(nouveau ? "Fiche créée." : "Fiche enregistrée.");
      window.dispatchEvent(new Event("cof-change"));
      location.hash = "#/f/" + r.id;
    } catch (ex) {
      err.textContent = ex.message;
      err.hidden = false;
      err.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  };
  const clavier = e => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") { e.preventDefault(); form.requestSubmit(); }
    if (e.key === "Escape" && !e.defaultPrevented && !$(".menu:not([hidden])", el)) annuler();
  };
  const quitter = e => { if (modifie) { e.preventDefault(); e.returnValue = ""; } };
  const garde = e => {
    if (modifie && !confirm("Abandonner les modifications non enregistrées ?")) {
      e.stopImmediatePropagation();
      history.pushState(null, "", e.oldURL);
    }
  };
  document.addEventListener("keydown", clavier);
  window.addEventListener("beforeunload", quitter);
  window.addEventListener("hashchange", garde, true);
  return () => {
    document.removeEventListener("keydown", clavier);
    window.removeEventListener("beforeunload", quitter);
    window.removeEventListener("hashchange", garde, true);
  };
}
