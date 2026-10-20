// Voies, profils et peuples de l'univers : liste filtrée et éditeur, ouvert depuis Paramètres →
// Bestiaire et règles. Aucun contenu n'est fourni : le MJ saisit ou importe le sien. L'étiquette
// (officielle ou maison) sert à trier, elle ne verrouille rien.
import { $, $$, esc, api, toast, armer } from "../ui.js";

const GENRES = { voies: ["voie", "Voies"], profils: ["profil", "Profils"], peuples: ["peuple", "Peuples"] };
const CARACS = ["FOR", "DEX", "CON", "INT", "SAG", "CHA"];
const CIBLES = [["pv", "PV max"], ["pm", "PM max"], ["def", "DEF"], ["init", "Initiative"], ["att", "Toutes les attaques"], ["att_c", "Attaque au contact"],
  ["att_d", "Attaque à distance"], ["att_m", "Attaque magique"], ["rd", "RD"], ...CARACS.map(c => [c, `${c} (valeur)`])];
const ACTIONS = [["", "Pas d'action en combat"], ["sort", "Sort"], ["attaque", "Attaque spéciale"], ["soin", "Soin"], ["etat", "État"], ["autre", "Autre action"]];
const ATTAQUES = [["", "Pas de jet d'attaque"], ["C", "Contact"], ["D", "Distance"], ["M", "Magique"]];
const TYPES_VOIE = ["profil", "peuple", "prestige", "autre"];
const RANGS_MAX = 20;

const opts = (liste, val) => liste.map(([v, l]) => `<option value="${esc(v)}"${v === (val ?? "") ? " selected" : ""}>${esc(l)}</option>`).join("");
const simples = liste => liste.map(x => [x, x]);
const pluriel = (n, mot) => `${n} ${mot}${n > 1 ? "s" : ""}`;

function vide(genre, rangs) {
  const base = { nom: "", ref: "", etiquette: "maison", description: "", livre: "", page: 0 };
  if (genre === "voies") return { ...base, type: "profil", profils: [], prerequis: "", rangs: Array.from({ length: rangs }, () => ({ nom: "", texte: "" })) };
  if (genre === "profils") return { ...base, famille: "", dv: "", carac_magie: "", armes: "", armures: "", equipement: "", voies: [] };
  return { ...base, caracs: "", modificateurs: {}, voies: [] };
}

export async function mount(el, arg) {
  const genre = GENRES[arg] ? arg : "voies", [unite] = GENRES[genre];
  const lecture = !!window.cofAcces?.lecture_seule; // auditeur : consulter sans modifier
  let regles = {};
  try { regles = await api("GET", "/api/regles"); } catch { /* sans règles : saisie libre */ }
  const repos = regles.personnage?.repos || [], etats = (regles.combat?.etats || []).map(e => e.nom);
  const rangsDefaut = Math.min(RANGS_MAX, regles.personnage?.rang_max || 5);
  const PAR = [["combat", "par combat"], ["jour", "par jour"], ...repos.map(r => [`repos:${r.cle}`, `par ${r.libelle.toLowerCase()}`]), ["main", "à remettre à zéro à la main"]];
  let ref = { voies: [], profils: [], peuples: [] }, sel = null, filtre = { q: "", etiquette: "" };

  el.innerHTML = `
    <div class="entete"><h1>Voies, profils et peuples</h1><span class="sp"></span>
      <a href="#/parametres"><button class="ghost">← Paramètres</button></a>
      ${lecture ? "" : `<button class="primary" id="v-nouveau">＋ Nouveau ${unite}</button>`}</div>
    <div class="ligne" role="tablist" style="margin-bottom:12px">${Object.entries(GENRES).map(([k, [, titre]]) =>
      `<a href="#/voies/${k}" role="tab" aria-selected="${k === genre}"><button class="${k === genre ? "primary" : ""}">${titre}</button></a>`).join("")}</div>
    <div class="bst">
      <section class="panneau colonne" style="gap:8px">
        <input type="search" id="v-q" placeholder="${genre === "voies" ? "Nom de la voie ou d'une capacité…" : "Nom…"}" aria-label="Rechercher">
        <select id="v-etiquette" aria-label="Étiquette"><option value="">Officielles et maison</option><option value="officielle">Officielles</option><option value="maison">Maison</option></select>
        <p class="hint" id="v-compte"></p>
        <div class="bst-liste" id="v-liste"></div>
      </section>
      <section class="panneau" id="v-fiche"><p class="vide">Choisissez un ${unite} dans la liste${lecture ? "" : " ou créez-en un"}.</p></section>
    </div>`;
  const fiche = $("#v-fiche", el);
  $("#v-q", el).oninput = e => { filtre.q = e.target.value; rendreListe(); };
  $("#v-etiquette", el).onchange = e => { filtre.etiquette = e.target.value; rendreListe(); };
  if (!lecture) $("#v-nouveau", el).onclick = () => { sel = null; rendreListe(); formulaire(vide(genre, rangsDefaut), []); };

  async function charger() {
    try { ref = await api("GET", "/api/reference"); } catch (e) { toast(e.message, true); }
    rendreListe();
  }

  function rendreListe() {
    const tous = ref[genre] || [], mots = filtre.q.trim().toLowerCase().split(/\s+/).filter(Boolean);
    const l = tous.filter(x => (!filtre.etiquette || x.etiquette === filtre.etiquette) &&
      mots.every(m => [x.nom, ...(x.rangs || []).map(c => c.nom)].join(" ").toLowerCase().includes(m)));
    $("#v-compte", el).textContent = `${pluriel(l.length, unite)}${l.length !== tous.length ? ` sur ${tous.length}` : ""}`;
    $("#v-liste", el).innerHTML = l.length ? l.map(x => `
      <button type="button" class="${x.id === sel ? "actif" : ""}" data-id="${x.id}">
        <span class="t"><b>${esc(x.nom)}</b> <span class="badge">${esc(x.etiquette)}</span>${genre === "voies" ? `<span class="nc">${esc(x.type)}</span>` : ""}</span>
        <span class="hint">${genre === "voies" ? (x.rangs || []).map(c => c.nom).filter(Boolean).map(esc).join(" · ") || "Aucune capacité nommée"
          : (x.voies || []).map(esc).join(" · ") || "Aucune voie"}</span>
      </button>`).join("")
      : `<p class="vide">${tous.length ? "Rien ne correspond aux filtres." : `Aucun ${unite}. Importez un fichier depuis Paramètres → Bestiaire et règles${lecture ? "" : `, ou créez-en un`}.`}</p>`;
    $$("#v-liste [data-id]", el).forEach(b => b.onclick = () => ouvrir(+b.dataset.id));
  }

  async function ouvrir(id) {
    try {
      const d = await api("GET", `/api/reference/${genre}/${id}`);
      sel = id; rendreListe();
      formulaire(d.element, d.utilisee_par);
    } catch (e) { toast(e.message, true); }
  }

  // ---------------------------------------------------------------- formulaire
  const champ = (nom, html, plus = "") => `<label class="champ"${plus}><span>${nom}</span>${html}</label>`;
  const texte = (nom, k, v, plus = "") => champ(nom, `<input data-k="${k}" value="${esc(v ?? "")}" ${plus}>`);
  const zone = (nom, k, v, lignes = 3) => champ(nom, `<textarea data-k="${k}" rows="${lignes}">${esc(v ?? "")}</textarea>`);
  const choix = (nom, k, liste, v) => champ(nom, `<select data-k="${k}">${opts(liste, v)}</select>`);

  function ligneBonus(b = { cible: "def", valeur: 1 }) {
    return `<div class="v-bonus">
      <select data-b="cible" aria-label="Ce qui augmente">${opts(CIBLES, b.cible)}</select>
      <input data-b="valeur" type="number" value="${esc(b.valeur ?? 0)}" aria-label="Valeur" style="width:5em">
      <label class="hint"><input type="checkbox" data-b="par_rang"${b.par_rang ? " checked" : ""}> × rang</label>
      <label class="hint"><input type="checkbox" data-b="niveau"${b.niveau ? " checked" : ""}> + niveau</label>
      <select data-b="carac" aria-label="Plus le modificateur de">${opts([["", "+ mod. : aucun"], ...CARACS.map(c => [c, `+ mod. ${c}`])], b.carac)}</select>
      <button type="button" class="ghost petit" data-retirer-bonus aria-label="Retirer le bonus">✕</button></div>`;
  }

  function blocRang(c, r) {
    const a = c.action || {}, u = c.utilisations;
    const etat = etats.length ? `<select data-a="etat">${opts([["", "Aucun état"], ...simples(etats)], a.etat)}</select>` : `<input data-a="etat" value="${esc(a.etat || "")}" placeholder="Nom de l'état">`;
    return `<details class="v-rang" data-r="${r}"${c.nom ? "" : " open"}>
      <summary><b>Rang ${r + 1}</b> <span class="v-rang-nom">${esc(c.nom || "")}</span></summary>
      <div class="colonne" style="gap:8px;margin-top:8px">
        <div class="ligne">${champ("Nom", `<input data-c="nom" value="${esc(c.nom || "")}">`, ' style="flex:1;min-width:12em"')}
          <label class="hint"><input type="checkbox" data-c="limitee"${c.limitee ? " checked" : ""}> Action limitée (L)</label>
          <label class="hint"><input type="checkbox" data-c="sort"${c.sort ? " checked" : ""}> Sort (*)</label></div>
        ${champ("Texte", `<textarea data-c="texte" rows="3">${esc(c.texte || "")}</textarea>`)}
        <div><b class="hint">Bonus permanents</b> <span class="hint">ajoutés à la feuille quand le PJ a ce rang</span></div>
        <div class="colonne v-bonus-liste" style="gap:4px">${(c.bonus || []).map(ligneBonus).join("")}</div>
        <div><button type="button" class="petit" data-ajouter-bonus>＋ Bonus</button></div>
        <div class="ligne">${champ("En combat", `<select data-a="genre">${opts(ACTIONS, a.genre)}</select>`)}</div>
        <div class="bst-grille v-action"${a.genre ? "" : " hidden"}>
          ${champ("Coût en PM", `<input data-a="cout_pm" type="number" min="0" value="${esc(a.cout_pm ?? 0)}">`)}
          ${champ("Jet d'attaque", `<select data-a="attaque">${opts(ATTAQUES, a.attaque)}</select>`)}
          ${champ("Bonus au jet", `<input data-a="bonus" type="number" value="${esc(a.bonus ?? 0)}">`)}
          ${champ("Dégâts", `<input data-a="degats" value="${esc(a.degats || "")}" placeholder="1d6">`)}
          ${champ("Soin", `<input data-a="soin" value="${esc(a.soin || "")}" placeholder="1d8">`)}
          ${champ("État", etat)}
        </div>
        <div class="ligne">${champ("Utilisations", `<input data-u="fois" type="number" min="1" max="99" value="${esc(u?.fois ?? "")}" placeholder="sans limite" style="width:8em">`)}
          ${champ("fois", `<select data-u="par">${opts(PAR, u?.par || "combat")}</select>`)}</div>
      </div></details>`;
  }

  function formulaire(x, utilisee) {
    const nouveau = !x.id;
    const usage = utilisee?.length ? `<p class="hint">${genre === "voies" ? "Sur la feuille de" : "Fiche de"} : ${utilisee.map(u =>
      `<a href="#/f/${u.id}">${esc(u.nom)}</a>${genre === "voies" ? ` (rang ${u.rang})` : ""}`).join(", ")}.</p>` : "";
    let corps;
    if (genre === "voies") corps = `
      <div class="bst-grille">${choix("Type", "type", simples(TYPES_VOIE), x.type)}${texte("Livre", "livre", x.livre)}${texte("Page", "page", x.page || "", 'type="number" min="0"')}</div>
      ${texte("Profils", "profils", (x.profils || []).join(", "), 'placeholder="Séparés par des virgules"')}
      ${texte("Prérequis", "prerequis", x.prerequis)}
      ${zone("Description", "description", x.description, 2)}
      <h3>Rangs</h3>
      <div class="colonne" style="gap:6px" id="v-rangs">${(x.rangs || []).map(blocRang).join("")}</div>
      <div class="ligne"><button type="button" class="petit" id="v-rang-plus">＋ Rang</button><button type="button" class="petit ghost" id="v-rang-moins">Retirer le dernier rang</button></div>`;
    else if (genre === "profils") corps = `
      <div class="bst-grille">${texte("Famille", "famille", x.famille)}${choix("Dé de vie", "dv", [["", "—"], ...simples(["d4", "d6", "d8", "d10", "d12"])], x.dv)}
        ${choix("Carac. de magie", "carac_magie", [["", "—"], ...simples(["INT", "SAG", "CHA"])], x.carac_magie)}${texte("Livre", "livre", x.livre)}${texte("Page", "page", x.page || "", 'type="number" min="0"')}</div>
      ${texte("Armes", "armes", x.armes)}${texte("Armures", "armures", x.armures)}${zone("Équipement", "equipement", x.equipement, 2)}
      ${zone("Voies", "voies", (x.voies || []).join("\n"), 5).replace("<span>Voies</span>", "<span>Voies <small>une par ligne</small></span>")}
      ${zone("Description", "description", x.description)}`;
    else corps = `
      ${texte("Caractéristiques (texte)", "caracs", x.caracs, 'placeholder="Texte libre"')}
      <div><b class="hint">Modificateurs</b> <span class="hint">affichés et proposés à la création d'un PJ, jamais ajoutés d'office à une feuille</span></div>
      <div class="bst-grille">${CARACS.map(c => champ(c, `<input data-m="${c}" type="number" value="${esc(x.modificateurs?.[c] ?? 0)}">`)).join("")}</div>
      <div class="bst-grille">${texte("Livre", "livre", x.livre)}${texte("Page", "page", x.page || "", 'type="number" min="0"')}</div>
      ${zone("Voies", "voies", (x.voies || []).join("\n"), 3).replace("<span>Voies</span>", "<span>Voies <small>une par ligne</small></span>")}
      ${zone("Description", "description", x.description)}`;
    fiche.innerHTML = `<form id="v-form" class="colonne" style="gap:10px" novalidate>
      <fieldset class="colonne" style="gap:10px;border:0;padding:0;margin:0"${lecture ? " disabled" : ""}>
        <div class="ligne"><h2 style="flex:1;margin:0">${nouveau ? `Nouveau ${unite}` : esc(x.nom)}</h2>${x.etiquette ? `<span class="badge">${esc(x.etiquette)}</span>` : ""}</div>
        ${usage}
        <div class="bst-grille">${texte("Nom", "nom", x.nom, "required")}${choix("Étiquette", "etiquette", [["officielle", "Officielle"], ["maison", "Maison"]], x.etiquette || "maison")}
          ${texte("Référence", "ref", x.ref, 'title="Identifiant stable, qui sert à reconnaître l\'élément d\'un fichier à l\'autre" placeholder="facultative"')}</div>
        ${corps}
      </fieldset>
      ${lecture ? "" : `<div class="ligne"><button type="submit" class="primary">Enregistrer</button>
        ${nouveau ? "" : `<button type="button" class="danger" id="v-retirer">Retirer</button>`}</div>`}
      <div id="v-msg" class="alerte" hidden></div>
    </form>`;
    if (lecture) return;
    if (genre === "voies") armerRangs();
    const retirer = $("#v-retirer", fiche);
    if (retirer) {
      if (genre === "voies" && utilisee?.length) retirer.title = `${pluriel(utilisee.length, "feuille")} en ${utilisee.length > 1 ? "gardent" : "garde"} une copie, qui devient une voie libre.`;
      armer(retirer, genre === "voies" && utilisee?.length ? `Confirmer (${pluriel(utilisee.length, "feuille")} en ${utilisee.length > 1 ? "gardent" : "garde"} une copie)` : "Confirmer", async () => {
        try {
          await api("DELETE", `/api/reference/${genre}/${x.id}`);
          toast(`« ${x.nom} » retiré${genre === "voies" ? "e" : ""} de la bibliothèque.`);
          sel = null; fiche.innerHTML = `<p class="vide">Choisissez un ${unite} dans la liste.</p>`; charger();
        } catch (e) { toast(e.message, true); }
      });
    }
    $("#v-form", fiche).onsubmit = async e => {
      e.preventDefault();
      const el2 = lire(x);
      try {
        const d = nouveau ? await api("POST", `/api/reference/${genre}`, el2) : await api("PUT", `/api/reference/${genre}/${x.id}`, el2);
        toast("Enregistré.");
        await charger();
        await ouvrir(d.element.id);
        if (d.avertissements?.length) {
          const m = $("#v-msg", fiche);
          m.hidden = false;
          m.innerHTML = `Bonus à vérifier : ces feuilles ont déjà une valeur saisie à la main au même endroit, qui risque de compter deux fois.<ul>${d.avertissements.map(a =>
            `<li><a href="#/f/${a.id}">${esc(a.nom)}</a> : « ${esc(a.capacite)} » et ${esc(a.champ)}</li>`).join("")}</ul>`;
          m.scrollIntoView({ block: "nearest" });
        }
      } catch (err) { toast(err.message, true); }
    };
  }

  function armerRangs() {
    const rangs = $("#v-rangs", fiche);
    rangs.onclick = e => {
      if (e.target.closest("[data-ajouter-bonus]")) e.target.closest(".v-rang").querySelector(".v-bonus-liste").insertAdjacentHTML("beforeend", ligneBonus());
      const r = e.target.closest("[data-retirer-bonus]");
      if (r) r.closest(".v-bonus").remove();
    };
    rangs.onchange = e => {
      if (e.target.dataset.a === "genre") e.target.closest(".v-rang").querySelector(".v-action").hidden = !e.target.value;
    };
    rangs.oninput = e => {
      if (e.target.dataset.c === "nom") e.target.closest(".v-rang").querySelector(".v-rang-nom").textContent = e.target.value;
    };
    $("#v-rang-plus", fiche).onclick = () => {
      const n = $$(".v-rang", rangs).length;
      if (n >= RANGS_MAX) { toast(`${RANGS_MAX} rangs au plus.`, true); return; }
      rangs.insertAdjacentHTML("beforeend", blocRang({ nom: "", texte: "" }, n));
    };
    $("#v-rang-moins", fiche).onclick = () => {
      const l = $$(".v-rang", rangs);
      if (l.length > 5) l.at(-1).remove(); else toast("Une voie a au moins 5 rangs.", true);
    };
  }

  // l'élément tel que le formulaire le décrit (les champs inconnus du formulaire sont gardés)
  function lire(x) {
    const out = { ...x };
    delete out.id;
    for (const i of $$("[data-k]", fiche)) out[i.dataset.k] = i.value.trim();
    out.page = +out.page || 0;
    if (genre === "voies") {
      out.profils = out.profils.split(",").map(s => s.trim()).filter(Boolean);
      out.rangs = $$(".v-rang", fiche).map(b => {
        const c = { nom: $("[data-c=nom]", b).value.trim(), texte: $("[data-c=texte]", b).value.trim(),
          limitee: $("[data-c=limitee]", b).checked, sort: $("[data-c=sort]", b).checked };
        const bonus = $$(".v-bonus", b).map(r => ({ cible: $("[data-b=cible]", r).value, valeur: +$("[data-b=valeur]", r).value || 0,
          par_rang: $("[data-b=par_rang]", r).checked, niveau: $("[data-b=niveau]", r).checked, carac: $("[data-b=carac]", r).value }));
        if (bonus.length) c.bonus = bonus;
        const genreA = $("[data-a=genre]", b).value;
        if (genreA) {
          const v = k => $(`[data-a=${k}]`, b).value.trim();
          c.action = { genre: genreA, cout_pm: +v("cout_pm") || 0, attaque: v("attaque"), bonus: +v("bonus") || 0, degats: v("degats"), soin: v("soin"), etat: v("etat") };
        }
        const fois = +$("[data-u=fois]", b).value;
        if (fois) c.utilisations = { fois, par: $("[data-u=par]", b).value };
        return c;
      });
    } else {
      out.voies = out.voies.split("\n").map(s => s.trim()).filter(Boolean);
      if (genre === "peuples") out.modificateurs = Object.fromEntries(CARACS.map(c => [c, +$(`[data-m=${c}]`, fiche).value || 0]).filter(([, n]) => n));
    }
    return out;
  }

  await charger();
}
