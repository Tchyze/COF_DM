// Bestiaire de l'univers (repris de tracker_COF) : filtres, fiches officielles verrouillées, éditeur
// des monstres maison, import de fichiers.
import { $, $$, esc, api, toast, armer } from "../ui.js";

export const CARAC = ["FOR", "DEX", "CON", "INT", "SAG", "CHA"];
const TYPE = { C: "Contact", D: "Distance", M: "Magique" };
const FILTRES_VIDES = { q: "", source: "", categorie: "", taille: "", ncMin: "", ncMax: "", tri: "nc" };
const PREFS = "cof-dm-filtres-bestiaire";

export const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
export const fmtNC = n => (n === 0.5 ? "½" : String(n ?? 0).replace(".", ","));
const verrouillee = m => m.source === "officiel" && !!m.ref;
const sup = (m, c) => (m.superieures || []).includes(c) ? "*" : "";
const opts = (liste, val) => liste.map(x => `<option${x === val ? " selected" : ""}>${esc(x)}</option>`).join("");

function vide() {
  return { nom: "", source: "maison", nc: 1, categorie: "", taille: "", tags: [], superieures: [],
    defense: 12, pv: 8, init: 10, rd: 0, crit: 20, carac: Object.fromEntries(CARAC.map(c => [c, 0])),
    attaques: [{ nom: "Attaque", type: "C", bonus: 2, dm: "1d6" }], capacites: "", description: "", notes: "" };
}

/** Bloc de statistiques d'un monstre (aussi utilisé par le créateur). */
export function blocStats(m, attaques = m.attaques || []) {
  return `<div class="bst-stats">
    <div class="bst-nc">${["NC " + fmtNC(m.nc), m.categorie, m.taille && "taille " + m.taille].filter(Boolean).map(esc).join(" · ")}</div>
    <div class="bst-car">${CARAC.map(c => `<span><b>${c}</b> ${signe(m.carac?.[c] ?? 0)}${sup(m, c)}</span>`).join("")}</div>
    <div class="bst-car"><span><b>DEF</b> ${esc(m.defense)}</span><span><b>PV</b> ${esc(m.pv)}${m.rd ? ` (RD ${esc(m.rd)})` : ""}</span><span><b>Init</b> ${esc(m.init)}</span></div>
    ${attaques.map(a => `<div><b>${esc(a.nom)}</b> ${signe(+a.bonus || 0)} DM ${esc(a.dm || "—")}${a.notes ? ` <span class="hint">${esc(a.notes)}</span>` : ""}</div>`).join("")}
    ${m.superieures?.length ? `<p class="hint">* Carac. supérieure : deux dés, on garde le meilleur.</p>` : ""}
  </div>`;
}

export async function mount(el, arg) {
  const lecture = !!window.cofAcces?.lecture_seule; // auditeur : consulter sans modifier
  let filtres = { ...FILTRES_VIDES }, sel = null, data = [];
  try { filtres = { ...FILTRES_VIDES, ...JSON.parse(localStorage.getItem(PREFS) || "{}") }; } catch { /* stockage indisponible */ }
  // tailles et catégories : celles du fichier de règles de l'univers, complétées par celles des fiches
  let listes = {};
  try { listes = (await api("GET", "/api/regles")).listes || {}; } catch { /* sans règles : saisie libre */ }
  let TAILLES = listes.tailles || [], CATEGORIES = listes.categories || [];
  const proposer = (id, liste) => `<datalist id="${id}">${liste.map(x => `<option value="${esc(x)}">`).join("")}</datalist>`;
  const garder = () => { try { localStorage.setItem(PREFS, JSON.stringify(filtres)); } catch { /* stockage indisponible */ } };

  el.innerHTML = `
    <div class="entete"><h1>Bestiaire</h1><span class="sp"></span>
      ${lecture ? "" : `<a href="#/createur" id="b-createur" hidden><button>Créer selon les règles</button></a><button class="primary" id="b-nouveau">＋ Nouveau monstre</button>`}</div>
    <div class="bst">
      <section class="panneau colonne" style="gap:8px">
        <input type="search" id="f-q" placeholder="Nom, capacité, description, tag…" aria-label="Rechercher">
        <div class="ligne">
          <select id="f-source" aria-label="Source"><option value="">Toutes sources</option><option value="officiel">Officiels</option><option value="maison">Maison</option></select>
          <select id="f-categorie" aria-label="Catégorie"></select>
          <select id="f-taille" aria-label="Taille"></select>
        </div>
        <div class="ligne">
          <label class="hint">NC de <input type="number" id="f-ncMin" min="0" step="0.5" style="width:70px"></label>
          <label class="hint">à <input type="number" id="f-ncMax" min="0" step="0.5" style="width:70px"></label>
          <select id="f-tri" aria-label="Trier"><option value="nc">Tri par NC</option><option value="nom">Tri par nom</option><option value="page">Tri par page</option></select>
          <button class="ghost petit" id="f-raz">Effacer les filtres</button>
        </div>
        <p class="hint" id="b-compte"></p>
        <div class="bst-liste" id="b-liste"></div>
        ${lecture ? "" : `<details><summary>Importer des monstres</summary>
          <div class="colonne" style="gap:8px;margin-top:8px">
            <p class="hint">Fichier JSON : un tableau de fiches, ou un objet <code>{"monstres": [...]}</code>. Une fiche dont la référence (<code>ref</code>) existe déjà est mise à jour ; les autres sont ajoutées. Les monstres maison ne sont jamais modifiés.</p>
            <input type="file" id="b-fichier" accept=".json,application/json">
            <div><button id="b-importer">Importer</button></div>
            <p class="hint" id="b-import-msg" role="status"></p>
          </div></details>`}
      </section>
      <section class="panneau" id="b-fiche"><p class="vide">Choisissez un monstre dans la liste${lecture ? "" : " ou créez-en un"}.</p></section>
    </div>`;
  const fiche = $("#b-fiche", el);
  // le créateur de monstre n'existe que si le fichier de règles en donne les tables
  if (!lecture) api("GET", "/api/bestiaire/tables").then(() => { $("#b-createur", el).hidden = false; }, () => {});
  const aller = id => history.replaceState(null, "", id ? `#/bestiaire/${id}` : "#/bestiaire");

  for (const k of Object.keys(FILTRES_VIDES)) {
    const i = $("#f-" + k, el);
    if (i.tagName !== "SELECT" || i.options.length) i.value = filtres[k];
    i.addEventListener("input", () => { filtres[k] = i.value; garder(); rendreListe(); });
  }
  $("#f-raz", el).onclick = () => { filtres = { ...FILTRES_VIDES }; for (const k of Object.keys(filtres)) $("#f-" + k, el).value = filtres[k]; garder(); rendreListe(); };

  // les filtres proposent les valeurs des règles puis celles que portent les fiches
  function rendreFiltres() {
    const avec = (liste, cle) => [...new Set([...liste, ...data.map(m => m[cle]).filter(Boolean)])];
    TAILLES = avec(listes.tailles || [], "taille"); CATEGORIES = avec(listes.categories || [], "categorie");
    for (const [id, tous, liste, k] of [["f-categorie", "Toutes catégories", CATEGORIES, "categorie"], ["f-taille", "Toutes tailles", TAILLES, "taille"]]) {
      if (!liste.includes(filtres[k])) filtres[k] = "";
      $("#" + id, el).innerHTML = `<option value="">${tous}</option>${opts(liste, filtres[k])}`;
    }
  }

  async function charger() {
    try { data = await api("GET", "/api/bestiaire"); } catch (e) { toast(e.message, true); data = []; }
    rendreFiltres();
    rendreListe();
  }

  function filtrees() {
    const f = filtres, mots = f.q.trim().toLowerCase().split(/\s+/).filter(Boolean);
    let l = data.filter(m => (!f.source || m.source === f.source) && (!f.categorie || m.categorie === f.categorie) && (!f.taille || m.taille === f.taille) &&
      (f.ncMin === "" || m.nc >= +f.ncMin) && (f.ncMax === "" || m.nc <= +f.ncMax));
    if (mots.length) l = l.filter(m => {
      const t = [m.nom, m.categorie, m.taille, m.capacites, m.description, m.notes, ...(m.tags || []), ...(m.attaques || []).map(a => a.nom)].join(" ").toLowerCase();
      return mots.every(w => t.includes(w));
    });
    const cmp = { nc: (a, b) => a.nc - b.nc || a.nom.localeCompare(b.nom, "fr"), nom: (a, b) => a.nom.localeCompare(b.nom, "fr", { numeric: true }),
      page: (a, b) => (a.livre || "~").localeCompare(b.livre || "~") || (a.page || 0) - (b.page || 0) }[f.tri] || (() => 0);
    return l.sort(cmp);
  }

  function rendreListe() {
    const l = filtrees(), n = data.length;
    $("#b-compte", el).textContent = `${l.length} monstre${l.length > 1 ? "s" : ""}${l.length !== n ? ` sur ${n}` : ""}`;
    $("#b-liste", el).innerHTML = l.length ? l.slice(0, 400).map(m => `
      <button type="button" class="${m.id === sel ? "actif" : ""}" data-id="${m.id}">
        <span class="t"><b>${esc(m.nom)}</b> <span class="badge">${esc(m.source)}</span>${verrouillee(m) ? ` <span title="Fiche officielle verrouillée">🔒</span>` : ""}<span class="nc">NC ${fmtNC(m.nc)}</span></span>
        <span class="hint">${[m.taille, m.categorie].filter(Boolean).map(x => esc(x) + " · ").join("")}DEF ${esc(m.defense)} · PV ${esc(m.pv)}${m.rd ? " · RD " + esc(m.rd) : ""} · Init ${esc(m.init)}${m.page ? ` · p. ${esc(m.page)}` : ""}</span>
      </button>`).join("")
      : `<p class="vide">${n ? "Aucun monstre ne correspond aux filtres." : "Le bestiaire est vide. Importez un fichier de monstres ou créez-en un."}</p>`;
  }

  // ---------------------------------------------------------------- lecture (fiche officielle, ou auditeur)
  function afficher(m) {
    sel = m.id; rendreListe();
    fiche.innerHTML = `
      <div class="ligne"><h2 style="flex:1;margin:0">${esc(m.nom)}</h2><span class="badge">${esc(m.source)}</span></div>
      ${verrouillee(m) ? `<p class="hint">${esc(m.livre || "")}${m.page ? `, page ${esc(m.page)}` : ""} · fiche verrouillée. Dupliquez-la pour en faire une version maison modifiable.</p>` : ""}
      ${blocStats(m)}
      ${m.capacites ? `<h3>Capacités</h3><div class="bst-texte">${esc(m.capacites)}</div>` : ""}
      ${m.description ? `<h3>Description</h3><div class="bst-texte">${esc(m.description)}</div>` : ""}
      ${m.notes ? `<p class="hint bst-texte">${esc(m.notes)}</p>` : ""}
      ${lecture ? "" : `<div class="ligne" style="margin-top:12px"><button class="primary" id="b-dup">Dupliquer pour personnaliser</button><span style="flex:1"></span><button class="danger" id="b-suppr">Supprimer</button></div>`}`;
    if (lecture) return;
    $("#b-dup", fiche).onclick = async () => {
      try { const c = await api("POST", `/api/bestiaire/${m.id}/dupliquer`); toast("Copie maison créée : vous pouvez la modifier."); await charger(); ouvrir(c); aller(c.id); }
      catch (e) { toast(e.message, true); }
    };
    armer($("#b-suppr", fiche), "Confirmer la suppression", () => supprimer(m));
  }

  async function supprimer(m) {
    try { await api("DELETE", `/api/bestiaire/${m.id}`); toast("Monstre supprimé."); sel = null; fiche.innerHTML = `<p class="vide">Monstre supprimé.</p>`; aller(); await charger(); }
    catch (e) { toast(e.message, true); }
  }

  // ---------------------------------------------------------------- éditeur (monstres maison)
  function ouvrir(f, estNouveau = false) {
    if (lecture || (!estNouveau && verrouillee(f))) return afficher(f);
    sel = f.id ?? null; rendreListe();
    const c = f.carac || {};
    const nombre = (nom, libelle, valeur, plus = "") => `<label class="champ"><span>${libelle}</span><input name="${nom}" type="number" value="${esc(valeur)}" ${plus}></label>`;
    fiche.innerHTML = `
      <form id="b-form" class="colonne" style="gap:12px" novalidate>
        <h2 style="margin:0">${estNouveau ? "Nouveau monstre" : esc(f.nom)}</h2>
        <div class="bst-grille">
          <label class="champ" style="grid-column:span 2"><span>Nom</span><input name="nom" required value="${esc(f.nom)}"></label>
          ${nombre("nc", "NC", f.nc, 'step="0.5" min="0"')}
          <label class="champ"><span>Catégorie</span><input name="categorie" list="b-categories" value="${esc(f.categorie)}">${proposer("b-categories", CATEGORIES)}</label>
          <label class="champ"><span>Taille</span><input name="taille" list="b-tailles" value="${esc(f.taille)}">${proposer("b-tailles", TAILLES)}</label>
          <label class="champ" style="grid-column:1/-1"><span>Tags <small>séparés par des virgules</small></span><input name="tags" value="${esc((f.tags || []).join(", "))}"></label>
        </div>
        <h3>Combat</h3>
        <div class="bst-grille">
          ${nombre("defense", "DEF", f.defense)}${nombre("pv", "PV", f.pv, 'min="1"')}${nombre("init", "Init", f.init)}${nombre("rd", "RD", f.rd, 'min="0"')}${nombre("crit", "Critique sur", f.crit, 'min="2" max="20"')}
        </div>
        <div class="bst-grille">${CARAC.map(k => `<label class="champ"><span>${k} <small><input type="checkbox" name="s_${k}"${sup(f, k) ? " checked" : ""}> sup.</small></span><input name="c_${k}" type="number" value="${esc(c[k] ?? 0)}"></label>`).join("")}</div>
        <p class="hint">Modificateurs (−2, +1, +3…). « sup. » : Carac. supérieure, deux dés et on garde le meilleur.</p>
        <h3>Attaques</h3>
        <div class="colonne" style="gap:6px" id="b-atks"></div>
        <div><button type="button" class="petit" id="b-atk">＋ Attaque</button></div>
        <label class="champ"><span>Capacités <small>affichées pendant le combat</small></span><textarea name="capacites" rows="5">${esc(f.capacites)}</textarea></label>
        <label class="champ"><span>Description</span><textarea name="description" rows="3">${esc(f.description)}</textarea></label>
        <label class="champ"><span>Notes du MJ</span><textarea name="notes" rows="2">${esc(f.notes)}</textarea></label>
        <div class="ligne"><button type="submit" class="primary">Enregistrer</button>${estNouveau ? "" : `<button type="button" id="b-dup">Dupliquer</button><span style="flex:1"></span><button type="button" class="danger" id="b-suppr">Supprimer</button>`}</div>
      </form>`;
    const atks = $("#b-atks", fiche);
    const ligneAtk = a => `<div class="bst-atk">
        <input data-k="nom" value="${esc(a.nom)}" placeholder="Nom de l'attaque" aria-label="Nom de l'attaque">
        <select data-k="type" aria-label="Type">${Object.entries(TYPE).map(([k, v]) => `<option value="${k}"${a.type === k ? " selected" : ""}>${v}</option>`).join("")}</select>
        <input data-k="bonus" type="number" value="${esc(a.bonus)}" aria-label="Bonus d'attaque">
        <input data-k="dm" value="${esc(a.dm)}" placeholder="1d8+2" aria-label="Dégâts">
        <button type="button" class="ghost petit" data-retirer aria-label="Retirer cette attaque">✕</button></div>`;
    atks.innerHTML = (f.attaques || []).map(ligneAtk).join("");
    $("#b-atk", fiche).onclick = () => { atks.insertAdjacentHTML("beforeend", ligneAtk({ nom: "", type: "C", bonus: 0, dm: "" })); atks.lastElementChild.querySelector("input").focus(); };
    atks.onclick = e => { if (e.target.closest("[data-retirer]")) e.target.closest(".bst-atk").remove(); };
    const form = $("#b-form", fiche);
    const lire = () => {
      const fd = new FormData(form), g = k => fd.get(k);
      return {
        ...f, nom: (g("nom") || "").trim(), source: "maison", ref: undefined, nc: Math.max(0, +g("nc") || 0), categorie: (g("categorie") || "").trim(), taille: (g("taille") || "").trim(),
        tags: g("tags").split(",").map(s => s.trim()).filter(Boolean),
        defense: +g("defense") || 0, pv: Math.max(1, +g("pv") || 1), init: +g("init") || 0, rd: Math.max(0, +g("rd") || 0), crit: Math.min(20, Math.max(2, +g("crit") || 20)),
        carac: Object.fromEntries(CARAC.map(k => [k, +g("c_" + k) || 0])), superieures: CARAC.filter(k => fd.get("s_" + k)),
        attaques: $$(".bst-atk", atks).map(r => ({ nom: $("[data-k=nom]", r).value.trim() || "Attaque", type: $("[data-k=type]", r).value,
          bonus: +$("[data-k=bonus]", r).value || 0, dm: $("[data-k=dm]", r).value.trim() })),
        capacites: g("capacites") || "", description: g("description") || "", notes: g("notes") || "",
      };
    };
    const DM_OK = /^\s*([+-]?\s*(\d*d\d+|\d+)\s*)+$/i;
    form.onsubmit = async e => {
      e.preventDefault();
      const d = lire();
      if (!d.nom) { toast("Le nom est obligatoire.", true); form.nom.focus(); return; }
      const faux = d.attaques.find(a => a.dm && !DM_OK.test(a.dm));
      if (faux) { toast(`Dégâts illisibles pour « ${faux.nom} » : utilisez une formule comme 1d8+2.`, true); return; }
      delete d.id; delete d.modifie;
      try {
        const r = estNouveau ? await api("POST", "/api/bestiaire", d) : await api("PUT", `/api/bestiaire/${f.id}`, d);
        toast(estNouveau ? "Monstre créé." : "Monstre enregistré."); await charger(); ouvrir(r); aller(r.id);
      } catch (err) { toast(err.message, true); }
    };
    if (!estNouveau) {
      $("#b-dup", fiche).onclick = () => ouvrir({ ...lire(), id: undefined, nom: lire().nom + " (copie)" }, true);
      armer($("#b-suppr", fiche), "Confirmer la suppression", () => supprimer(f));
    } else form.nom.focus();
  }

  // ---------------------------------------------------------------- import
  const importer = $("#b-importer", el);
  if (importer) importer.onclick = async () => {
    const fichier = $("#b-fichier", el).files[0], msg = $("#b-import-msg", el);
    if (!fichier) { toast("Choisissez d'abord un fichier.", true); return; }
    let contenu;
    try { contenu = JSON.parse(await fichier.text()); } catch { toast("Ce fichier n'est pas un JSON valide.", true); return; }
    msg.textContent = "Import en cours…";
    try {
      const d = await api("POST", "/api/bestiaire/import", contenu);
      msg.innerHTML = `${d.crees} ajouté${d.crees > 1 ? "s" : ""}, ${d.mis_a_jour} mis à jour${d.rejets.length ? `, ${d.rejets.length} rejeté${d.rejets.length > 1 ? "s" : ""} :<br>` + d.rejets.slice(0, 10).map(x => `ligne ${esc(x.ligne)} ${esc(x.nom || "")} : ${esc(x.motif)}`).join("<br>") : "."}`;
      toast("Import terminé.");
      await charger();
    } catch (e) { msg.textContent = ""; toast(e.message, true); }
  };

  const nouveau = $("#b-nouveau", el);
  if (nouveau) nouveau.onclick = () => { ouvrir(vide(), true); aller(); };
  $("#b-liste", el).onclick = e => {
    const b = e.target.closest("[data-id]");
    const m = b && data.find(x => x.id === +b.dataset.id);
    if (m) { ouvrir(m); aller(m.id); }
  };

  await charger();
  if (arg === "nouveau" && !lecture) ouvrir(vide(), true);
  else if (arg) { const m = data.find(x => x.id === +arg); if (m) ouvrir(m); }
}
