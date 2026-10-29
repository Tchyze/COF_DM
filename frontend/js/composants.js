// Composants de formulaire : sélecteur de fiches, saisie de date en jeu, complétion des [[liens]].
import { $, $$, esc, api, toast, delai } from "./ui.js";
import { schema, typeDe, vignette, badgeType } from "./schema.js";
import { listerMonstres, libelleMonstre } from "./monstres.js";

/** Cherche des fiches parmi certains types (tous si cibles est vide). */
export async function chercherFiches(q, cibles = [], limite = 12) {
  if (q.trim()) {
    const l = await api("GET", `/api/entites?limite=60&q=${encodeURIComponent(q)}`);
    return l.filter(f => !cibles.length || cibles.includes(f.type)).slice(0, limite);
  }
  const listes = await Promise.all((cibles.length ? cibles : [""]).map(t =>
    api("GET", `/api/entites?limite=${limite}${t ? "&type=" + t : ""}`)));
  return listes.flat().slice(0, limite);
}

/**
 * Sélecteur d'une ou plusieurs fiches. valeur : id ou liste d'ids ; refs : id → résumé (noms connus).
 * Propose de créer la fiche si le nom tapé n'existe pas.
 */
export function selecteur(conteneur, { cibles, multiple, valeur, refs = {}, auChangement }) {
  let ids = multiple ? [...(valeur || [])] : (valeur ? [valeur] : []);
  const connus = { ...refs };
  conteneur.classList.add("selecteur");
  conteneur.innerHTML = `<div class="choisis"></div><input type="search" placeholder="${multiple ? "Ajouter…" : "Choisir…"}" autocomplete="off"><div class="menu" hidden></div>`;
  const choisis = $(".choisis", conteneur), input = $("input", conteneur), menu = $(".menu", conteneur);
  let items = [], sel = 0;
  const peindreChoix = () => {
    choisis.innerHTML = ids.map(id => {
      const f = connus[id] || { id, nom: "fiche n°" + id, type: cibles[0] };
      return `<span class="badge type" data-type="${esc(f.type)}">${vignette(f, 18)} ${esc(f.nom)}<button type="button" data-retirer="${id}" aria-label="Retirer">✕</button></span>`;
    }).join("");
    input.hidden = !multiple && ids.length > 0;
  };
  const peindreMenu = () => {
    const q = input.value.trim();
    const exact = items.some(f => f.nom.toLowerCase() === q.toLowerCase());
    menu.innerHTML = items.map((f, i) => `<button type="button" data-i="${i}" class="${i === sel ? "actif" : ""}">${vignette(f, 24)}<span>${esc(f.nom)}</span><small>${esc(typeDe(f.type)?.nom || "")}</small></button>`).join("")
      + (q && !exact ? cibles.map(t => `<button type="button" data-creer="${t}">＋ Créer « ${esc(q)} » (${esc(typeDe(t).nom)})</button>`).join("") : "")
      || `<p class="vide" style="padding:8px">Aucune fiche.</p>`;
    menu.hidden = false;
  };
  const chercher = delai(async () => {
    items = (await chercherFiches(input.value, cibles).catch(() => [])).filter(f => !ids.includes(f.id));
    sel = 0; peindreMenu();
  }, 120);
  const choisir = f => {
    connus[f.id] = f;
    ids = multiple ? [...ids, f.id] : [f.id];
    input.value = ""; menu.hidden = true;
    peindreChoix();
    auChangement?.();
    if (multiple) input.focus();
  };
  input.addEventListener("focus", chercher);
  input.addEventListener("input", chercher);
  input.addEventListener("keydown", e => {
    if (menu.hidden) return;
    if (e.key === "ArrowDown") { e.preventDefault(); sel = Math.min(sel + 1, items.length - 1); peindreMenu(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); sel = Math.max(0, sel - 1); peindreMenu(); }
    else if (e.key === "Enter") { e.preventDefault(); if (items[sel]) choisir(items[sel]); }
    else if (e.key === "Escape") { e.stopPropagation(); menu.hidden = true; }
  });
  input.addEventListener("blur", () => setTimeout(() => { menu.hidden = true; }, 180));
  menu.addEventListener("mousedown", e => e.preventDefault());
  menu.addEventListener("click", async e => {
    const b = e.target.closest("button");
    if (!b) return;
    if (b.dataset.i) return choisir(items[+b.dataset.i]);
    if (b.dataset.creer) {
      try {
        const f = await api("POST", "/api/entites", { type: b.dataset.creer, nom: input.value.trim(), data: {} });
        toast(`Fiche « ${f.nom} » créée.`);
        window.dispatchEvent(new Event("cof-change"));
        choisir(f);
      } catch (err) { toast(err.message, true); }
    }
  });
  choisis.addEventListener("click", e => {
    const b = e.target.closest("button[data-retirer]");
    if (!b) return;
    ids = ids.filter(x => x !== +b.dataset.retirer);
    peindreChoix();
    auChangement?.();
  });
  peindreChoix();
  return { valeur: () => multiple ? ids : (ids[0] ?? null) };
}

/** Saisie d'une date du calendrier du monde : jour, mois, an. */
export function saisieDate(conteneur, valeur, auChangement) {
  const cal = schema.calendrier;
  conteneur.className = "date-jeu";
  conteneur.innerHTML = `<input type="number" min="1" placeholder="jour" aria-label="Jour" value="${valeur?.jour ?? ""}">
    <select aria-label="Mois"><option value=""></option>${cal.mois.map((m, i) => `<option value="${i + 1}" ${valeur?.mois === i + 1 ? "selected" : ""}>${esc(m.nom)}</option>`).join("")}</select>
    <input type="number" placeholder="an" aria-label="An" value="${valeur?.an ?? ""}">`;
  const [j, m, a] = [$("input", conteneur), $("select", conteneur), $$("input", conteneur)[1]];
  [j, m, a].forEach(x => x.addEventListener("input", () => auChangement?.()));
  return {
    valeur: () => (j.value === "" && m.value === "" && a.value === "") ? null
      : { jour: +j.value || 0, mois: +m.value || 0, an: a.value === "" ? NaN : +a.value },
  };
}

/** Complétion des [[liens]] dans un champ de texte : taper [[ ouvre la liste des fiches. */
export function completerLiens(zone) {
  const menu = document.createElement("div");
  menu.className = "menu";
  menu.hidden = true;
  menu.style.position = "absolute";
  zone.parentElement.style.position = "relative";
  zone.after(menu);
  let items = [], sel = 0, debut = -1;
  const fermer = () => { menu.hidden = true; debut = -1; };
  const peindre = () => {
    if (!items.length) { fermer(); return; }
    menu.innerHTML = items.map((f, i) => `<button type="button" data-i="${i}" class="${i === sel ? "actif" : ""}">${vignette(f, 22)}<span>${esc(f.nom)}</span>${badgeType(f.type)}</button>`).join("");
    menu.style.left = "0"; menu.style.right = "0"; menu.style.top = (zone.offsetTop + zone.offsetHeight + 4) + "px";
    menu.hidden = false;
  };
  const chercher = delai(async q => { items = await chercherFiches(q, [], 8).catch(() => []); sel = 0; peindre(); }, 100);
  const inserer = f => {
    const fin = zone.selectionStart;
    zone.setRangeText(f.nom + "]]", debut, fin, "end");
    zone.dispatchEvent(new Event("input", { bubbles: true }));
    fermer();
    zone.focus();
  };
  zone.addEventListener("input", () => {
    const avant = zone.value.slice(0, zone.selectionStart);
    const i = avant.lastIndexOf("[[");
    if (i < 0 || avant.slice(i).includes("]]") || avant.slice(i).includes("\n") || avant.length - i > 60) { fermer(); return; }
    debut = i + 2;
    chercher(avant.slice(debut));
  });
  zone.addEventListener("keydown", e => {
    if (menu.hidden) return;
    if (e.key === "ArrowDown") { e.preventDefault(); sel = Math.min(sel + 1, items.length - 1); peindre(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); sel = Math.max(0, sel - 1); peindre(); }
    else if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); inserer(items[sel]); }
    else if (e.key === "Escape") { e.stopPropagation(); fermer(); }
  });
  zone.addEventListener("blur", () => setTimeout(fermer, 180));
  menu.addEventListener("mousedown", e => e.preventDefault());
  menu.addEventListener("click", e => { const b = e.target.closest("button[data-i]"); if (b) inserer(items[+b.dataset.i]); });
}

/** Ajuste la hauteur d'un textarea à son contenu. */
export function hauteurAuto(t) {
  const f = () => { t.style.height = "auto"; t.style.height = Math.min(Math.max(t.scrollHeight + 2, 90), 640) + "px"; };
  t.addEventListener("input", f);
  requestAnimationFrame(f);
}

// ---------------------------------------------------------------- bestiaire de l'univers

const plier = t => String(t).normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase();

/** Sélecteur d'un monstre du bestiaire de l'univers. */
export function selecteurMonstre(conteneur, { valeur, auChangement }) {
  let id = valeur || null, nom = "", items = [], sel = 0;
  conteneur.classList.add("selecteur");
  conteneur.innerHTML = `<div class="choisis"></div><input type="search" placeholder="Chercher dans le bestiaire…" autocomplete="off"><div class="menu" hidden></div>`;
  const choisis = $(".choisis", conteneur), input = $("input", conteneur), menu = $(".menu", conteneur);
  const peindre = () => {
    choisis.innerHTML = id ? `<span class="badge" data-nom="${esc(nom)}">${esc(nom || "monstre n°" + id)}<button type="button" data-retirer aria-label="Retirer">✕</button></span>` : "";
    input.hidden = !!id;
  };
  if (id) listerMonstres().then(l => { const m = l.find(x => x.id === id); nom = m ? libelleMonstre(m) : "monstre retiré du bestiaire"; peindre(); }).catch(() => {});
  const chercher = delai(async () => {
    try {
      const q = plier(input.value.trim());
      items = (await listerMonstres()).filter(m => !q || plier(m.nom).includes(q)).sort((a, b) => a.nom.localeCompare(b.nom, "fr")).slice(0, 30);
    } catch (e) { items = []; menu.innerHTML = `<p class="vide" style="padding:8px">${esc(e.message)}</p>`; menu.hidden = false; return; }
    sel = 0;
    menu.innerHTML = items.map((m, i) => `<button type="button" data-i="${i}" class="${i === sel ? "actif" : ""}"><span>${esc(libelleMonstre(m))}</span><small>${esc(m.source === "officiel" ? "officiel" : "maison")}</small></button>`).join("")
      || `<p class="vide" style="padding:8px">Aucun monstre de ce nom dans le <a href="#/bestiaire">bestiaire</a>.</p>`;
    menu.hidden = false;
  }, 150);
  const choisir = m => { id = m.id; nom = libelleMonstre(m); input.value = ""; menu.hidden = true; peindre(); auChangement?.(); };
  input.addEventListener("focus", chercher);
  input.addEventListener("input", chercher);
  input.addEventListener("blur", () => setTimeout(() => { menu.hidden = true; }, 180));
  input.addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); if (items[sel]) choisir(items[sel]); } });
  menu.addEventListener("mousedown", e => { if (!e.target.closest("a")) e.preventDefault(); });
  menu.addEventListener("click", e => { const b = e.target.closest("button[data-i]"); if (b) choisir(items[+b.dataset.i]); });
  choisis.addEventListener("click", e => { if (e.target.closest("button[data-retirer]")) { id = null; nom = ""; peindre(); auChangement?.(); } });
  peindre();
  return { valeur: () => id, nom: () => nom.replace(/ · NC .*/, "") };
}

/** Créatures d'une rencontre : monstres du bestiaire de l'univers, quantité, camp. */
export function editeurAdversaires(conteneur, valeur, auChangement) {
  conteneur.innerHTML = `<div class="liste-adv"></div><div><button type="button" class="petit">＋ Créature</button></div>`;
  const liste = $(".liste-adv", conteneur);
  const ligne = (a = { monstre: 0, nom: "", qte: 1, camp: "ennemi" }) => {
    const d = document.createElement("div");
    d.className = "adv";
    d.innerHTML = `<div class="m"></div>
      <input type="number" min="1" max="99" value="${a.qte}" aria-label="Nombre">
      <select aria-label="Camp"><option value="ennemi">ennemi</option><option value="allie" ${a.camp === "allie" ? "selected" : ""}>allié</option></select>
      <button type="button" class="ghost petit" aria-label="Retirer">✕</button>`;
    const s = selecteurMonstre($(".m", d), { valeur: a.monstre || null, auChangement });
    d.lire = () => ({ monstre: s.valeur(), qte: +$("input[type=number]", d).value || 1, camp: $("select", d).value, nom: s.nom() || a.nom });
    $("button.ghost", d).onclick = () => { d.remove(); auChangement?.(); };
    liste.append(d);
  };
  (valeur || []).forEach(a => ligne(a));
  $("button.petit", conteneur).onclick = () => { ligne(); auChangement?.(); };
  return { valeur: () => { const l = [...liste.children].map(d => d.lire()).filter(a => a.monstre); return l.length ? l : null; } };
}

// ---------------------------------------------------------------- chiffres d'un objet

let emplacementsConnus = null;
/** Les emplacements de l'univers (fichier de règles, sinon la liste générique), lus une fois. */
export const listeEmplacements = () => (emplacementsConnus ||= api("GET", "/api/emplacements").catch(() => { emplacementsConnus = null; return []; }));

const TYPES_ARME = { C: "Contact", D: "Distance", M: "Magique" };
/** Ce qu'un objet porté peut augmenter : clé → libellé (mêmes cibles que les bonus des capacités). */
export const CIBLES_OBJET = { def: "DEF", init: "Init", rd: "RD", pv: "PV max", pm: "PM max", att: "Attaque", att_c: "Contact", att_d: "Distance", att_m: "Magique",
  FOR: "FOR", DEX: "DEX", CON: "CON", INT: "INT", SAG: "SAG", CHA: "CHA" };
const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);

/** Les chiffres d'un objet en une ligne : « DEF +2, Contact 1d8 +1 · 30 m » (sans l'emplacement). */
export function texteChiffres(c) {
  if (!c) return "";
  const a = c.arme;
  return [...(c.bonus || []).map(b => `${CIBLES_OBJET[b.cible] || b.cible} ${signe(b.valeur)}`),
    a && `${TYPES_ARME[a.type] || a.type}${a.dm ? ` ${a.dm}` : ""}${a.mod ? ` ${signe(a.mod)}` : ""}${a.portee ? ` · ${a.portee}` : ""}`].filter(Boolean).join(", ");
}

/** Saisie des chiffres d'une fiche « Objet » : emplacement, bonus, arme. valeur() : null si rien. */
export function editeurChiffres(conteneur, valeur, auChangement) {
  const v = valeur || {};
  const ligneBonus = b => `<span class="fp-inv-bonus"><select data-b="cible" aria-label="Valeur augmentée">${Object.entries(CIBLES_OBJET).map(([k, n]) => `<option value="${k}"${k === b.cible ? " selected" : ""}>${esc(n)}</option>`).join("")}</select><input data-b="valeur" type="number" min="-20" max="20" value="${esc(b.valeur ?? 1)}" aria-label="Bonus"><button type="button" class="ghost petit" data-retirer-bonus aria-label="Retirer le bonus">✕</button></span>`;
  conteneur.innerHTML = `<div class="colonne" style="gap:8px">
      <label class="champ"><span>Emplacement <small>où il se porte ; un seul objet par emplacement</small></span><select data-e="emplacement"><option value="">— aucun —</option></select></label>
      <div class="ligne" data-bonus>${(v.bonus || []).map(ligneBonus).join("")}<button type="button" class="petit" data-ajout-bonus>＋ Bonus</button></div>
      <div class="ligne fp-inv-arme${v.arme ? " on" : ""}"><label class="t-coche"><input type="checkbox" data-a="arme"${v.arme ? " checked" : ""}> Arme</label>
        <select data-a="type" aria-label="Type d'attaque">${Object.entries(TYPES_ARME).map(([k, n]) => `<option value="${k}"${(v.arme?.type || "C") === k ? " selected" : ""}>${n}</option>`).join("")}</select>
        <input data-a="dm" value="${esc(v.arme?.dm || "")}" placeholder="DM : 1d8" aria-label="Dégâts" style="width:9em">
        <input data-a="mod" type="number" value="${esc(v.arme?.mod || 0)}" aria-label="Bonus propre à l'arme" title="Bonus propre à l'arme" style="width:5em">
        <input data-a="portee" value="${esc(v.arme?.portee || "")}" placeholder="Portée" aria-label="Portée" style="width:8em"></div>
    </div>`;
  const sel = $("[data-e=emplacement]", conteneur), a = k => $(`[data-a=${k}]`, conteneur);
  listeEmplacements().then(l => {
    const connu = l.some(e => e.cle === v.emplacement);
    sel.insertAdjacentHTML("beforeend", l.map(e => `<option value="${esc(e.cle)}">${esc(e.nom)}</option>`).join("") + (v.emplacement && !connu ? `<option value="${esc(v.emplacement)}">${esc(v.emplacement)}</option>` : ""));
    sel.value = v.emplacement || "";
  });
  conteneur.addEventListener("click", e => {
    if (e.target.closest("[data-retirer-bonus]")) { e.target.closest(".fp-inv-bonus").remove(); auChangement?.(); }
    else if (e.target.closest("[data-ajout-bonus]")) { e.target.closest("[data-ajout-bonus]").insertAdjacentHTML("beforebegin", ligneBonus({ cible: "def", valeur: 1 })); auChangement?.(); }
  });
  conteneur.addEventListener("change", e => { if (e.target.dataset.a === "arme") e.target.closest(".fp-inv-arme").classList.toggle("on", e.target.checked); });
  return {
    valeur() {
      const c = { emplacement: sel.value || undefined,
        bonus: $$(".fp-inv-bonus", conteneur).map(b => ({ cible: $("[data-b=cible]", b).value, valeur: +$("[data-b=valeur]", b).value || 0 })).filter(b => b.valeur),
        arme: a("arme").checked ? { type: a("type").value, dm: a("dm").value.trim(), mod: +a("mod").value || 0, portee: a("portee").value.trim() } : undefined };
      if (!c.bonus.length) delete c.bonus;
      return c.emplacement || c.bonus || c.arme ? c : null;
    },
  };
}
