// Routeur et cadre de l'application.
// Routes : #/  #/t/<type>  #/f/<id>  #/f/<id>/modifier  #/nouveau/<type>?nom=…  #/nouveau?nom=…
//          #/bestiaire[/<id>|/nouveau]  #/createur
//          #/voies[/<profils|peuples>]  #/aller/<nom>  #/table[/<quête>]  #/preparer/<séance>  #/apres/<séance>  #/graphe[/<id>]  #/chronologie  #/liens  #/journal  #/parametres[/<bloc>]
import { $, $$, esc, api, toast, delai, universOnglet, choisirUniversOnglet, enTetesUnivers } from "./ui.js";
import { chargerSchema, schema, typeDe, vignette, badgeType, actif as fonctionActive } from "./schema.js";
import * as tableau from "./views/tableau.js";
import * as table from "./views/table.js";
import * as liste from "./views/liste.js";
import * as fiche from "./views/fiche.js";
import * as graphe from "./views/graphe.js";
import * as chronologie from "./views/chronologie.js";
import * as outils from "./views/outils.js";
import * as secrets from "./views/secrets.js";
import * as bestiaire from "./views/bestiaire.js";
import * as voies from "./views/voies.js";
import * as combat from "./views/combat.js";
import * as createur from "./views/createur.js";
import * as synchro from "./views/synchro.js";
import * as preparation from "./views/preparation.js";
import * as apres from "./views/apres.js";
import { initFichier } from "./fichier.js";
import { initEtatLien } from "./connexion.js";
import { initBadgeMaj } from "./majour.js";
import { initDes } from "./des.js";

const GROUPES = [
  ["Monde", ["pnj", "lieu", "faction", "objet", "lore", "evenement"]],
  ["Campagnes", ["campagne", "quete", "rencontre", "seance", "pj"]],
  ["Secrets", ["secret"]],
  ["Joueurs", ["aide"]],
  ["Règles", ["regle"]],
];

let nettoyage = null;

function lireHash() {
  const [chemin, requete] = location.hash.replace(/^#\/?/, "").split("?");
  return { parts: chemin.split("/").filter(Boolean).map(decodeURIComponent), params: new URLSearchParams(requete || "") };
}

async function route() {
  const { parts, params } = lireHash();
  const [nom, a, b] = parts;
  if (nom === "aller") {
    try {
      const f = await api("GET", "/api/resoudre?nom=" + encodeURIComponent(a));
      location.replace("#/f/" + f.id);
    } catch { location.replace("#/nouveau?nom=" + encodeURIComponent(a)); }
    return;
  }
  // lecture seule (auditeur) : ni création, ni modification
  if (window.cofAcces?.lecture_seule && (nom === "nouveau" || nom === "apres" || b === "modifier")) {
    toast("Votre compte lit cet univers sans pouvoir le modifier.", true);
    location.replace(nom === "f" ? "#/f/" + a : "#/");
    return;
  }
  // écrans d'une fonctionnalité coupée dans cet univers
  const ecrans = { table: "ecran_mj", preparer: "seances", apres: "seances", graphe: "graphe", combat: "combat", secrets: "secrets", chronologie: "chronologie" };
  if (ecrans[nom] && !fonctionActive(ecrans[nom])) {
    toast("Cette fonctionnalité est désactivée dans cet univers (Paramètres → Fonctionnalités).", true);
    location.replace("#/");
    return;
  }
  if (typeof nettoyage === "function") nettoyage();
  nettoyage = null;
  $("#apercu").hidden = true;
  $("#app").classList.remove("menu-ouvert");
  // écran du MJ et écran de combat : toute la fenêtre, menu replié
  $("#app").classList.toggle("plein", nom === "table" || (nom === "combat" && !!a));
  const ancien = $("#vue");
  const el = ancien.cloneNode(false);
  ancien.replaceWith(el);
  let actif = "";
  try {
    if (!nom) { actif = "accueil"; nettoyage = await tableau.mount(el); }
    else if (nom === "t" && typeDe(a)) { actif = a; nettoyage = await liste.mount(el, a, params); }
    else if (nom === "f" && +a) { nettoyage = await fiche.mount(el, +a, b === "modifier"); actif = fiche.typeCourant(); }
    else if (nom === "nouveau") { actif = a || ""; nettoyage = await fiche.creer(el, a, params); }
    else if (nom === "table") { actif = "table"; nettoyage = await table.mount(el, a); }
    else if (nom === "preparer" && +a) { actif = "seance"; nettoyage = await preparation.mount(el, +a); }
    else if (nom === "apres" && +a) { actif = "seance"; nettoyage = await apres.mount(el, +a); }
    else if (nom === "graphe") { actif = "graphe"; nettoyage = await graphe.mount(el, +a || 0); }
    else if (nom === "bestiaire") { actif = "bestiaire"; nettoyage = await bestiaire.mount(el, a); }
    else if (nom === "voies") { actif = "voies"; nettoyage = await voies.mount(el, a); }
    else if (nom === "createur") { actif = "bestiaire"; nettoyage = await createur.mount(el); }
    else if (nom === "combat") { actif = "combat"; nettoyage = await combat.mount(el, a); }
    else if (nom === "secrets") { actif = "secrets"; nettoyage = await secrets.mount(el, params); }
    else if (nom === "chronologie") { actif = "chronologie"; nettoyage = await chronologie.mount(el, params); }
    else if (nom === "synchro" && !window.cofAcces?.serveur) { nettoyage = await synchro.mount(el); }
    else if (["liens", "journal", "parametres"].includes(nom)) { actif = nom; nettoyage = await outils[nom](el, a); }
    else { location.replace("#/"); return; }
  } catch (e) {
    el.innerHTML = `<div class="erreur">${esc(e.message)}</div>`;
  }
  marquerActif(actif);
  window.scrollTo(0, 0);
}

// ---------------------------------------------------------------- navigation latérale
// Chaque catégorie du menu se replie ; les catégories repliées sont gardées sur cet appareil. Celle
// de la page ouverte se déplie d'office le temps qu'on y est (sans changer ce qui est gardé) ; repliée
// à la main, elle garde au moins le lien de la page ouverte.
const CLE_REPLIES = "cof-menu-replie";
const lireReplies = () => { try { return new Set(JSON.parse(localStorage.getItem(CLE_REPLIES) || "[]")); } catch { return new Set(); } };
const garderReplies = s => { try { localStorage.setItem(CLE_REPLIES, JSON.stringify([...s])); } catch { /* stockage indisponible */ } };
const cleGroupe = g => g.normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase();

function replier(g, replie) {
  g.classList.toggle("replie", replie);
  g.querySelector(".groupe-titre").setAttribute("aria-expanded", String(!replie));
}

function marquerActif(actif) {
  $$("#nav a").forEach(x => x.classList.toggle("on", x.dataset.cle === actif));
  const replies = lireReplies();
  $$("#nav .groupe").forEach(g => replier(g, replies.has(g.dataset.groupe) && !g.querySelector("a.on")));
}

$("#nav").addEventListener("click", e => {
  const b = e.target.closest(".groupe-titre");
  if (!b) return;
  const g = b.closest(".groupe"), replie = !g.classList.contains("replie");
  replier(g, replie);
  const s = lireReplies();
  if (replie) s.add(g.dataset.groupe); else s.delete(g.dataset.groupe);
  garderReplies(s);
});

async function dessinerNav() {
  let compte = {};
  try { compte = (await api("GET", "/api/stats")).compte; } catch { /* hors ligne */ }
  const lien = (cle, href, icone, nom, n) =>
    `<a href="${href}" data-cle="${cle}"${typeDe(cle) ? ` data-type="${cle}"` : ""}><span class="ic">${icone}</span>${esc(nom)}${n ? `<span class="n">${n}</span>` : ""}</a>`;
  const groupe = (cle, titre, liens) => !liens ? "" :
    `<div class="groupe" data-groupe="${cle}"><h4><button type="button" class="groupe-titre" aria-expanded="true" aria-controls="nav-${cle}">` +
    `<span>${esc(titre)}</span><span class="chevron" aria-hidden="true"></span></button></h4><div class="groupe-liens" id="nav-${cle}">${liens}</div></div>`;
  $("#nav").innerHTML =
    lien("accueil", "#/", "🏠", "Tableau de bord") +
    (!fonctionActive("ecran_mj") ? "" : lien("table", "#/table", "🧭", "Écran du MJ")) +
    GROUPES.map(([g, cles]) => {
      const connus = cles.filter(typeDe); // types des fonctionnalités coupées : hors du schéma
      // les voies, profils et peuples de l'univers vont avec les règles
      const voies = g === "Règles" ? lien("voies", "#/voies", "📖", "Voies et profils") : "";
      return groupe(cleGroupe(g), g, connus.map(c => {
        const t = typeDe(c);
        return lien(c, `#/t/${c}`, t.icone, t.pluriel, compte[c]);
      }).join("") + voies + (g === "Secrets" && fonctionActive("secrets") ? lien("secrets", "#/secrets", "👁", "Qui sait quoi") : ""));
    }).join("") +
    groupe("combat", fonctionActive("combat") ? "Combat" : "Bestiaire",
      (fonctionActive("combat") ? lien("combat", "#/combat", "⚔", "Combats") : "") + lien("bestiaire", "#/bestiaire", "🐉", "Bestiaire")) +
    groupe("outils", "Outils",
      (fonctionActive("graphe") ? lien("graphe", "#/graphe", "🕸", "Graphe") : "") +
      (fonctionActive("chronologie") ? lien("chronologie", "#/chronologie", "📅", "Chronologie") : "") +
      lien("liens", "#/liens", "⛓", "Liens morts") +
      lien("journal", "#/journal", "📒", "Journal") +
      lien("parametres", "#/parametres", "⚙", "Paramètres"));
  const { parts } = lireHash();
  marquerActif(!parts[0] ? "accueil" : parts[0] === "t" ? parts[1] : parts[0] === "f" ? fiche.typeCourant() : parts[0]);
}
window.addEventListener("cof-change", () => { dessinerNav(); apercus.clear(); });

// ---------------------------------------------------------------- recherche rapide
/** Ouvre une fiche trouvée : dans sa page, ou là où la vue en cours le demande (tiroir de l'écran du MJ). */
const ouvrirFiche = id => { if (window.cofOuvrirFiche) window.cofOuvrirFiche(id); else location.hash = "#/f/" + id; };

function ouvrirRecherche(initial = "") {
  if ($(".voile")) return;
  const voile = document.createElement("div");
  voile.className = "voile";
  voile.innerHTML = `<div class="boite" role="dialog" aria-label="Rechercher">
    <input id="rr" placeholder="Rechercher une fiche…" autocomplete="off" spellcheck="false">
    <div class="resultats menu" id="rr-res"></div>
    <div class="pied"><span><kbd>↑</kbd><kbd>↓</kbd> choisir</span><span><kbd>Entrée</kbd> ouvrir</span><span><kbd>Échap</kbd> fermer</span><span><code>#tag</code> <code>"phrase"</code></span></div>
  </div>`;
  document.body.append(voile);
  const input = $("#rr", voile), res = $("#rr-res", voile);
  let items = [], sel = 0;
  const fermer = () => voile.remove();
  const peindre = () => {
    res.innerHTML = items.length ? items.map((f, i) => `<button data-i="${i}" class="${i === sel ? "actif" : ""}">
        ${vignette(f, 30)}<span style="min-width:0;flex:1"><b>${esc(f.nom)}</b>${f.extrait ? `<span class="x">${extrait(f.extrait)}</span>` : ""}</span>${badgeType(f.type)}</button>`).join("")
      : `<p class="vide" style="padding:10px">${input.value.trim() ? "Aucun résultat." : "Tapez un nom, un mot du texte, un #tag…"}</p>`;
  };
  const chercher = delai(async () => {
    const q = input.value.trim();
    items = q ? await api("GET", "/api/entites?limite=12&q=" + encodeURIComponent(q)).catch(() => []) : [];
    sel = 0; peindre();
  }, 120);
  input.oninput = chercher;
  input.onkeydown = e => {
    if (e.key === "ArrowDown") { e.preventDefault(); sel = Math.min(sel + 1, items.length - 1); peindre(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); sel = Math.max(sel - 1, 0); peindre(); }
    else if (e.key === "Enter" && items[sel]) { ouvrirFiche(items[sel].id); fermer(); }
    else if (e.key === "Escape") fermer();
  };
  res.onclick = e => { const b = e.target.closest("button[data-i]"); if (b) { ouvrirFiche(items[+b.dataset.i].id); fermer(); } };
  voile.onclick = e => { if (e.target === voile) fermer(); };
  input.value = initial;
  input.focus();
  peindre();
  if (initial) chercher();
}

/** Extrait de recherche : syntaxe Markdown retirée, termes trouvés surlignés (« »). */
export function extrait(x) {
  const propre = x.replace(/\[\[([^\]|]*?)(?:\\?\|([^\]]*))?\]\]/g, (_, c, t) => t || c)
    .replace(/[*_=~`]{1,2}/g, "").replace(/(^|\s)#{1,6}\s/g, "$1").replace(/\s+/g, " ");
  return esc(propre).replace(/«/g, "<mark>").replace(/»/g, "</mark>");
}

// ---------------------------------------------------------------- aperçu au survol des liens
const apercus = new Map();
let survole = null;
document.addEventListener("mouseover", async e => {
  const a = e.target.closest("a.wl[data-nom], a.lien-fiche[data-id]");
  if (!a || a === survole) return;
  survole = a;
  const cle = a.dataset.id ? "id:" + a.dataset.id : "nom:" + a.dataset.nom.toLowerCase();
  if (!apercus.has(cle)) {
    apercus.set(cle, (a.dataset.id
      ? api("GET", "/api/entites/" + a.dataset.id).then(f => ({ ...f, resume: f.data.resume || f.data.impression || f.data.but || f.data.pitch || "" }))
      : api("GET", "/api/resoudre?nom=" + encodeURIComponent(a.dataset.nom))).catch(() => null));
  }
  const f = await apercus.get(cle);
  if (survole !== a || !f) return;
  const box = $("#apercu");
  box.innerHTML = `${vignette(f, 54)}<div style="min-width:0"><b>${esc(f.nom)}</b>${badgeType(f.type)}${f.resume ? `<p>${esc(String(f.resume).slice(0, 220))}</p>` : ""}</div>`;
  box.hidden = false;
  const r = a.getBoundingClientRect();
  const h = box.offsetHeight, w = box.offsetWidth;
  box.style.left = Math.max(8, Math.min(r.left, innerWidth - w - 8)) + "px";
  box.style.top = (r.bottom + h + 12 > innerHeight ? r.top - h - 6 : r.bottom + 6) + "px";
});
document.addEventListener("mouseout", e => {
  if (survole && !survole.contains(e.relatedTarget)) { survole = null; $("#apercu").hidden = true; }
});

/** La version, en petit à côté de Thème : « v1.2.0 », une version d'essai mise en évidence
 * (« v1.2.1-test.4 »), ou la version de développement et son commit. Sur le bureau, elle mène aux
 * mises à jour. */
async function afficherVersion(bureau) {
  let v;
  try { v = (await api("GET", "/api/sante")).version || ""; } catch { return; }
  const el = $("#version-app");
  const git = /^0\.0\.0-git([0-9a-f]+)$/.exec(v);
  const test = /-test\./.test(v);
  el.textContent = git ? `dev ${git[1]}` : /^\d/.test(v) ? "v" + v : v;
  el.classList.toggle("test", test);
  el.title = git ? `Version de développement (commit ${git[1]})` : test ? `Version d'essai ${v}` : `Version ${v}`;
  if (bureau) { el.href = "#/parametres/maj"; el.title += " — mises à jour dans Paramètres"; }
  el.hidden = !v;
}

// ---------------------------------------------------------------- démarrage
document.addEventListener("keydown", e => {
  const saisie = e.target.closest("input,textarea,select,[contenteditable]");
  if ((e.key === "/" && !saisie) || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k")) { e.preventDefault(); ouvrirRecherche(); }
});
$("#ouvrir-recherche").onclick = () => ouvrirRecherche();
$("#burger").onclick = () => $("#app").classList.toggle("menu-ouvert");
$("#theme").onclick = () => {
  const r = document.documentElement;
  r.dataset.theme = r.dataset.theme === "clair" ? "" : "clair";
  try { localStorage.setItem("cof-theme", r.dataset.theme); } catch { /* navigation privée */ }
};
window.addEventListener("hashchange", route);
window.cofRecherche = ouvrirRecherche;

// ---------------------------------------------------------------- accès depuis un autre appareil
/** Écran de connexion d'un appareil du réseau (tablette du MJ) : mot de passe MJ. */
function ecranConnexion(acces) {
  $("#app").classList.add("plein", "ferme");
  document.title = "Connexion — COF DM";
  if (acces.serveur) { ecranCompte(acces); return; }
  $("#vue").innerHTML = `<section class="panneau connexion">
      <a class="marque" href="#/"><span class="sceau">DM</span><b>COF <span>DM</span></b></a>
      ${acces.mdp_defini ? `<p class="hint">Cet écran est réservé au MJ. Saisissez le mot de passe MJ.</p>
        <form id="connexion" class="colonne" style="gap:12px">
          <label class="champ"><span>Mot de passe MJ</span><input type="password" id="mdp" autocomplete="current-password" required></label>
          <div><button class="primary" type="submit">Entrer</button></div>
        </form>`
      : `<p class="alerte">Aucun mot de passe MJ n'est défini.</p>
        <p class="hint">Sur l'ordinateur du MJ, ouvrez <b>Paramètres → Tablette et réseau</b> et créez le mot de passe, puis rechargez cette page.</p>
        <div><button id="recharger">Recharger</button></div>`}
    </section>`;
  const recharger = $("#recharger");
  if (recharger) recharger.onclick = () => location.reload();
  const form = $("#connexion");
  if (!form) return;
  $("#mdp").focus();
  form.onsubmit = async e => {
    e.preventDefault();
    try { await api("POST", "/api/acces/connexion", { mdp: $("#mdp").value }); location.reload(); }
    catch (err) { toast(err.message, true); $("#mdp").select(); }
  };
}

/** Sur un serveur : passer d'un univers à l'autre, quand le compte a un rôle dans plusieurs. */
async function choixUnivers() {
  const m = await api("GET", "/api/mes-univers").catch(() => null);
  const sel = $("#choix-univers");
  // l'onglet s'attache à l'univers où il s'ouvre : un choix fait ensuite dans un autre onglet ne
  // le fera pas changer de monde au prochain rechargement
  if (m?.courant && !universOnglet()) choisirUniversOnglet(m.courant);
  if (!m || m.univers.length < 2) return;
  const roles = { mj: "MJ", auditeur: "auditeur", joueur: "joueur" };
  sel.innerHTML = m.univers.map(u => `<option value="${esc(u.id)}" ${u.id === m.courant ? "selected" : ""}>🌍 ${esc(u.nom)} — ${roles[u.role] || esc(u.role)}</option>`).join("");
  sel.hidden = false;
  // le choix vaut pour cet onglet ; il devient aussi celui des prochains onglets (cookie)
  sel.onchange = async () => {
    try {
      await api("POST", "/api/mes-univers/courant", { id: sel.value });
      choisirUniversOnglet(sel.value);
      // là où il est joueur, le compte n'a que sa page de joueur
      if (m.univers.find(u => u.id === sel.value)?.role === "joueur") location.href = "/joueur.html";
      else { location.hash = "#/"; location.reload(); }
    }
    catch (e) { toast(e.message, true); sel.value = m.courant; }
  };
}

/** Écran de connexion d'un serveur : chacun entre avec son compte. */
function ecranCompte(acces) {
  const marque = `<a class="marque" href="#/"><span class="sceau">DM</span><b>COF <span>DM</span></b></a>`;
  if (acces.connecte) { // un compte sans rôle dans l'univers ouvert
    $("#vue").innerHTML = `<section class="panneau connexion">${marque}
      <p class="alerte">Le compte « ${esc(acces.compte?.nom)} » n'a de rôle dans aucun univers de ce serveur.</p>
      <p class="hint">Demandez à l'administrateur du serveur de vous en donner un.</p>
      <div><button id="sortir">Changer de compte</button></div></section>`;
    $("#sortir").onclick = async () => { await api("POST", "/api/acces/deconnexion").catch(() => {}); location.reload(); };
    return;
  }
  $("#vue").innerHTML = `<section class="panneau connexion">${marque}
      <form id="connexion" class="colonne" style="gap:12px">
        <label class="champ"><span>Nom d'utilisateur</span><input id="nom" autocomplete="username" autocapitalize="none" spellcheck="false" required></label>
        <label class="champ"><span>Mot de passe</span><input type="password" id="mdp" autocomplete="current-password" required></label>
        <div><button class="primary" type="submit">Entrer</button></div>
      </form></section>`;
  $("#nom").focus();
  $("#connexion").onsubmit = async e => {
    e.preventDefault();
    try { await api("POST", "/api/acces/connexion", { nom: $("#nom").value, mdp: $("#mdp").value }); location.reload(); }
    catch (err) { toast(err.message, true); $("#mdp").select(); }
  };
}

(async () => {
  // l'univers que garde cet onglet a pu disparaître, ou être fermé à ce compte : il revient alors
  // au choix du navigateur
  if (universOnglet()) {
    const m = await fetch("/api/mes-univers", { headers: enTetesUnivers() }).then(r => r.ok ? r.json() : null).catch(() => null);
    if (!m?.univers?.some(u => u.id === universOnglet())) choisirUniversOnglet("");
  }
  // sur l'ordinateur du MJ, rien n'est demandé ; ailleurs, le mot de passe MJ
  const acces = await api("GET", "/api/acces").catch(() => ({ mj: false, mdp_defini: true }));
  if (!acces.mj) { ecranConnexion(acces); return; }
  // un compte joueur n'a que sa page de joueur, dans l'univers de cet onglet
  if (acces.joueur) { location.replace("/joueur.html"); return; }
  window.cofAcces = acces;
  window.addEventListener("cof-session", async () => {
    const a = await fetch("/api/acces", { headers: enTetesUnivers() }).then(r => r.json()).catch(() => null);
    if (a && !a.mj) location.reload();
  });
  if (!acces.local) {
    $("#deconnexion").hidden = false;
    $("#deconnexion").onclick = async () => { await api("POST", "/api/acces/deconnexion").catch(() => {}); choisirUniversOnglet(""); location.reload(); };
  }
  if (acces.serveur) {
    $("#deconnexion").title = `Fermer la session de ${acces.compte.nom}`;
    $("#app").classList.toggle("lecture-seule", !!acces.lecture_seule);
    if (acces.lecture_seule) $(".side .bas").insertAdjacentHTML("beforebegin", `<p class="lecture-avis">👁 Lecture seule</p>`);
  }
  if (acces.serveur) choixUnivers();
  // le menu Fichier : sur l'ordinateur du MJ, ou pour l'administrateur d'un serveur
  if (acces.local || acces.compte?.admin) initFichier();
  // en bas à gauche : local, connecté à un serveur ou hors connexion (application de bureau)
  if (acces.local && !acces.serveur) { initEtatLien(); initBadgeMaj(); }
  afficherVersion(acces.local && !acces.serveur);
  try { await chargerSchema(); }
  catch (e) { $("#vue").innerHTML = `<div class="erreur">${esc(e.message)}</div>`; return; }
  await dessinerNav();
  if (fonctionActive("des")) initDes({ lecture: !!acces.lecture_seule });
  route();
})();
export { schema };
