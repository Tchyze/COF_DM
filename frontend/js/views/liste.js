// Liste des fiches d'un type, en cartes, avec filtres.
import { $, $$, esc, api, delai } from "../ui.js";
import { typeDe, vignette, valeurCourte, texteDate } from "../schema.js";

/** Valeur d'une carte, sans lien (la carte entière est déjà un lien). */
function valeurCarte(type, ch, v, refs) {
  if (v == null || v === "") return "";
  if (ch.genre === "ref") return refs[v] ? `${vignette(refs[v], 16)} ${esc(refs[v].nom)}` : "";
  if (ch.genre === "refs") return v.map(x => refs[x]?.nom).filter(Boolean).map(esc).join(", ");
  if (ch.genre === "date_jeu") return esc(texteDate(v));
  return valeurCourte(type, ch.cle, v);
}

const memoire = {}; // filtres gardés par type pendant la session

export async function mount(el, type, params) {
  const t = typeDe(type);
  const f = memoire[type] ||= { q: "", choix: "", archives: false, tag: params.get("tag") || "" };
  if (params.get("tag")) f.tag = params.get("tag");
  // filtre rapide sur le premier champ « choix » des cartes (statut, catégorie…)
  const champFiltre = t.carte.map(k => t.champs.find(c => c.cle === k)).find(c => c && c.genre === "choix");
  el.innerHTML = `
    <div class="entete" data-type="${type}">${vignette({ type }, 42)}<h1>${esc(t.pluriel)}</h1><span class="compte" id="compte"></span><span class="sp"></span>
      <a href="#/nouveau/${type}"><button class="primary">+ ${esc(t.nom === "PNJ" || t.nom === "PJ" ? "Nouveau " + t.nom : "Nouvelle fiche")}</button></a></div>
    <div class="filtres">
      <input type="search" id="q" placeholder="Filtrer…" value="${esc(f.q)}" autocomplete="off">
      ${champFiltre ? `<div class="puces" id="puces"><button data-v="" aria-pressed="${!f.choix}">Tout</button>${champFiltre.options.map(o => `<button data-v="${esc(o)}" aria-pressed="${f.choix === o}">${esc(o)}</button>`).join("")}</div>` : ""}
      <span id="tag-actif"></span>
      <label class="ligne" style="gap:6px;font-size:.85rem;color:var(--muted)"><input type="checkbox" id="arch" ${f.archives ? "checked" : ""}> archives</label>
    </div>
    <div class="cartes" id="cartes"></div>`;
  let refs = {};
  const charger = async () => {
    const p = new URLSearchParams({ type, limite: 1000 });
    if (f.q.trim()) p.set("q", f.q.trim());
    if (f.tag) p.set("tag", f.tag);
    if (f.archives) p.set("archives", "1");
    let l = await api("GET", "/api/entites?" + p);
    if (champFiltre && f.choix) l = l.filter(x => x.carte?.[champFiltre.cle] === f.choix);
    // noms des fiches citées par les cartes (lieu, campagne…)
    const ids = new Set();
    for (const x of l) for (const k of t.carte) {
      const ch = t.champs.find(c => c.cle === k);
      if (ch?.genre === "ref" && x.carte?.[k]) ids.add(x.carte[k]);
      if (ch?.genre === "refs") (x.carte?.[k] || []).forEach(i => ids.add(i));
    }
    const manquants = [...ids].filter(i => !refs[i]);
    if (manquants.length) {
      const toutes = await api("GET", "/api/entites?limite=5000&archives=1");
      refs = Object.fromEntries(toutes.map(x => [x.id, x]));
    }
    $("#compte", el).textContent = `${l.length} fiche${l.length > 1 ? "s" : ""}`;
    $("#tag-actif", el).innerHTML = f.tag ? `<span class="badge tag">#${esc(f.tag)} <button class="ghost petit" id="sans-tag" style="min-height:0;padding:0 2px">✕</button></span>` : "";
    const sansTag = $("#sans-tag", el);
    if (sansTag) sansTag.onclick = () => { f.tag = ""; charger(); };
    $("#cartes", el).innerHTML = l.length ? l.map(x => `
      <a class="carte${x.archive ? " archivee" : ""}" href="#/f/${x.id}" data-type="${x.type}">
        ${vignette(x, 54)}
        <span class="corps">
          <span class="nom">${esc(x.nom)}${x.partage && window.cofAcces?.serveur ? ` <span title="Visible des joueurs">👁</span>` : ""}</span>
          <span class="ligne" style="gap:5px">${t.carte.map(k => { const v = valeurCarte(type, t.champs.find(c => c.cle === k), x.carte?.[k], refs); return v ? `<span class="badge">${v}</span>` : ""; }).join("")}</span>
          ${x.extrait ? `<span class="ext">${esc(x.extrait.replace(/\[\[([^\]|]*?)(?:\\?\|([^\]]*))?\]\]/g, (_, c, tt) => tt || c)).replace(/«/g, "<mark>").replace(/»/g, "</mark>")}</span>` : ""}
          ${x.tags?.length ? `<span class="ligne" style="gap:4px">${x.tags.map(g => `<span class="badge tag">#${esc(g)}</span>`).join("")}</span>` : ""}
        </span>
      </a>`).join("")
      : `<p class="vide">${f.q || f.choix || f.tag ? "Aucune fiche ne correspond." : `Aucune fiche pour l'instant. <a href="#/nouveau/${type}">Créer la première</a>.`}</p>`;
  };
  $("#q", el).oninput = delai(e => { f.q = e.target.value; charger(); });
  $("#arch", el).onchange = e => { f.archives = e.target.checked; charger(); };
  const puces = $("#puces", el);
  if (puces) puces.onclick = e => {
    const b = e.target.closest("button[data-v]");
    if (!b) return;
    f.choix = b.dataset.v;
    $$("button", puces).forEach(x => x.setAttribute("aria-pressed", x === b));
    charger();
  };
  await charger();
}
