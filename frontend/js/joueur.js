// Page des joueurs : la feuille de son personnage, le combat en cours (l'ordre d'initiative et les
// états, sans aucun chiffre), les dés (ses jets et ceux de la table) et les hauts faits. À la maison, la page est libre quand le MJ l'a
// ouverte et chacun y choisit son PJ ; sur un serveur, on y entre par son compte, avec son PJ.
import { $, $$, esc, api, toast, lien } from "./ui.js";
import { panneauFeuille, CARACS } from "./views/feuille.js";
import { recapHTML, recapMarkdown, dateCourte } from "./recap.js";
import { ligneJet, DES_RAPIDES } from "./des.js";

const CLE = "cof-joueur-pj";
const etat = { serveur: false, edition: false, combat: null, pj: null, pjs: [], vue: "fiche", de: 20, jets: [], dernierJet: 0, crit: 20 };
const vue = $("#vue");

const lireChoix = () => { try { return +localStorage.getItem(CLE) || 0; } catch { return 0; } };
const garderChoix = id => { try { id ? localStorage.setItem(CLE, id) : localStorage.removeItem(CLE); } catch { /* stockage indisponible */ } };
const moi = p => !!p && !!etat.pj && p.pj === etat.pj.id;

async function copier(texte) {
  try { await navigator.clipboard.writeText(texte); return true; } catch { return false; }
}

/* ---------------------------------------------------------- choix du personnage */
function vueChoix() {
  vue.className = "combat";
  vue.innerHTML = `<section class="panel pj-choix">
      <h1>Quel est votre personnage ?</h1>
      <p class="hint">Le choix est gardé sur cet appareil. Vous pourrez en changer.</p>
      <div class="cartes-pj">${etat.pjs.map(p => `<button class="carte-pj${etat.pj?.id === p.id ? " sel" : ""}" data-pj="${p.id}">
          <span class="nom">${esc(p.nom)}</span>
          <span class="hint">${esc([p.peuple, p.profil, p.niveau ? "niveau " + p.niveau : ""].filter(Boolean).join(" · "))}</span>
          ${p.joueur ? `<span class="hint">joué par ${esc(p.joueur)}</span>` : ""}
          ${p.statut === "absent" ? `<span class="tag">absent</span>` : ""}
        </button>`).join("") || `<p class="empty">Aucun personnage pour l'instant : le MJ doit d'abord créer les PJ.</p>`}</div>
    </section>`;
  $$("[data-pj]", vue).forEach(b => {
    b.onclick = () => {
      etat.pj = etat.pjs.find(p => p.id === +b.dataset.pj);
      garderChoix(etat.pj.id);
      location.hash = "#/";
      route();
    };
  });
}

/* ---------------------------------------------------------- ma fiche */
async function vueFiche() {
  const p = etat.pj;
  vue.className = "";
  vue.innerHTML = `<div class="pj-fiche">
      <div class="pj-tete"><h1>${esc(p.nom)}</h1>
        <span class="hint">${esc([p.peuple, p.profil, p.niveau ? "niveau " + p.niveau : ""].filter(Boolean).join(" · "))}</span>
        <span class="badge">${etat.edition ? "🔓 feuille modifiable" : "🔒 feuille verrouillée par le MJ"}</span></div>
      <section class="panneau" id="p-feuille"></section>
    </div>`;
  await panneauFeuille($("#p-feuille", vue), { id: p.id, nom: p.nom, data: { profil: p.profil } }, { peutEcrire: etat.edition, joueur: true });
}
// La feuille a changé ailleurs (combat, MJ) : on la relit, sauf pendant une saisie.
function rafraichirFiche() {
  if (etat.vue === "fiche" && etat.pj && !$("#fp-form", vue)) vueFiche();
}

/* ---------------------------------------------------------- combat */
const CAMPS = { pj: "PJ", allie: "Allié", ennemi: "Adversaire" };
function vueCombat() {
  vue.className = "combat";
  const c = etat.combat;
  if (!c || !c.ordre.length) {
    vue.innerHTML = `<section class="panel suivi"><h1>Combat</h1><p class="empty">Aucun combat en cours. L'ordre d'initiative s'affichera ici dès que le MJ lancera un combat.</p></section>`;
    return;
  }
  const courant = c.ordre.find(p => p.courant), suivant = c.ordre.find(p => p.suivant);
  vue.innerHTML = `<section class="panel suivi">
    <div class="suivi-tete"><h1>${esc(c.nom)}</h1><span class="hint">Round</span> <span class="round-n num">${c.round}</span></div>
    <div class="tour-de ${moi(courant) ? "moi" : ""}">
      <span class="hint">C'est au tour de</span>
      <span class="qui">${courant ? esc(courant.nom) : "—"}</span>
      ${moi(courant) ? `<span class="flag-moi">C'est à vous !</span>` : ""}
    </div>
    ${suivant ? `<div class="ensuite ${moi(suivant) ? "moi" : ""}"><span class="hint">Ensuite</span> <b>${esc(suivant.nom)}</b>${moi(suivant) ? ` <span class="flag-moi petit">préparez-vous</span>` : ""}</div>` : ""}
    <ol class="ordre-joueurs">${c.ordre.map(p => `
      <li class="${p.camp}${p.courant ? " courant" : ""}${p.suivant ? " suivant" : ""}${moi(p) ? " moi" : ""}${p.a_terre ? " terre" : ""}">
        <span class="qui-etats"><span class="nom">${esc(p.nom)}</span>${p.etats.map(e => `<span class="chip">${esc(e)}</span>`).join("")}</span>
        <span class="etiquettes"><span class="tag ${p.camp}">${CAMPS[p.camp] || ""}</span>${p.a_terre ? `<span class="tag">À terre</span>` : ""}${p.courant ? `<span class="flag">Joue</span>` : p.suivant ? `<span class="tag">Ensuite</span>` : ""}</span>
      </li>`).join("")}</ol>
  </section>`;
}

/* ---------------------------------------------------------- dés */
const signe = n => n > 0 ? "+" + n : n < 0 ? String(n) : "";
let modeDe = "normal"; // dé bonus : garder le meilleur de deux ; dé malus : le pire

async function vueDes() {
  vue.className = "combat";
  vue.innerHTML = `<section class="panel des-joueur">
      <h1>Dés</h1>
      <div class="des-mode" role="group" aria-label="Dé du test">
        ${[["normal", "Normal"], ["bonus", "Dé bonus"], ["malus", "Dé malus"]].map(([k, t]) => `<button type="button" class="petit${modeDe === k ? " on" : ""}" data-mode="${k}" aria-pressed="${modeDe === k}">${t}</button>`).join("")}
      </div>
      <div class="des-tests" id="des-tests"><p class="hint">Chargement de la feuille…</p></div>
      <form class="des-form" autocomplete="off">
        <div class="des-saisie">
          <input name="formule" placeholder="1d20+3, 2d6…" aria-label="Formule" spellcheck="false" required>
          <button class="primary" type="submit">Lancer</button>
        </div>
        <input name="libelle" placeholder="Pour quoi ? (facultatif)" aria-label="Libellé" maxlength="60">
        <div class="des-rapides">${DES_RAPIDES.map(f => `<button type="button" class="petit" data-formule="1d${f}">d${f}</button>`).join("")}</div>
      </form>
      <h2>À la table</h2>
      <ol class="des-liste" id="des-liste"></ol>
    </section>`;
  peindreJets();
  const form = $(".des-form", vue);
  form.onsubmit = e => { e.preventDefault(); lancerJet(form.formule.value, form.libelle.value); };
  vue.onclick = e => {
    const m = e.target.closest("[data-mode]");
    if (m) { modeDe = m.dataset.mode; $$("[data-mode]", vue).forEach(b => { b.classList.toggle("on", b === m); b.setAttribute("aria-pressed", b === m); }); return; }
    const b = e.target.closest("[data-formule]");
    if (b) lancerJet(b.dataset.test ? formuleTest(+b.dataset.test) : b.dataset.formule, b.dataset.libelle || form.libelle.value);
  };
  // les tests tout faits, lus sur la feuille
  let f = null;
  try { f = await api("GET", `/api/entites/${etat.pj.id}/feuille`); } catch { /* sans feuille : formules libres */ }
  if (etat.vue !== "des") return;
  const box = $("#des-tests", vue);
  if (!f || !f.existe) { box.innerHTML = `<p class="hint">Les tests tout faits apparaîtront quand le MJ aura rempli votre feuille.</p>`; return; }
  etat.crit = f.crit || etat.de;
  const caracs = CARACS.filter(k => k in (f.carac || {})).map(k => [k, f.carac[k]]);
  box.innerHTML = `
    <div class="des-groupe"><span class="hint">Tests</span>${caracs.map(([k, v]) => `<button type="button" data-formule="" data-test="${v}" data-libelle="Test de ${esc(k)}">${esc(k)} <b>${signe(v) || "+0"}</b></button>`).join("")}</div>
    ${(f.attaques || []).length ? `<div class="des-groupe"><span class="hint">Attaques</span>${f.attaques.map(a => `
      <button type="button" data-formule="" data-test="${a.bonus || 0}" data-libelle="${esc(a.nom)}">${esc(a.nom)} <b>${signe(a.bonus) || "+0"}</b></button>
      ${a.dm ? `<button type="button" class="ghost" data-formule="${esc(a.dm)}" data-libelle="Dégâts : ${esc(a.nom)}">DM ${esc(a.dm)}</button>` : ""}`).join("")}</div>` : ""}`;
}

/** Le dé du test (avec le dé bonus ou malus choisi) et son modificateur. */
function formuleTest(mod) {
  const de = { normal: `1d${etat.de}`, bonus: `2d${etat.de}kh1`, malus: `2d${etat.de}kl1` }[modeDe];
  return de + signe(mod);
}

async function lancerJet(formule, libelle) {
  try {
    const j = await api("POST", "/api/joueur/des", { formule, libelle, pj: etat.pj.id });
    ajouterJets([j]);
    toast(`🎲 ${j.total}${libelle ? " — " + libelle : ""}`);
    if (modeDe !== "normal") { modeDe = "normal"; $$("[data-mode]", vue).forEach(b => { b.classList.toggle("on", b.dataset.mode === "normal"); b.setAttribute("aria-pressed", b.dataset.mode === "normal"); }); }
  } catch (e) { toast(e.message, true); }
}

function ajouterJets(l) {
  for (const j of l) if (!etat.jets.some(x => x.id === j.id)) etat.jets.push(j);
  etat.jets.sort((a, b) => a.id - b.id);
  etat.jets.splice(0, Math.max(0, etat.jets.length - 100));
  etat.dernierJet = Math.max(etat.dernierJet, ...l.map(j => j.id));
  peindreJets();
}

function peindreJets() {
  const box = $("#des-liste", vue);
  if (!box) return;
  box.innerHTML = etat.jets.length ? etat.jets.slice().reverse().map(j => {
    const mien = !!etat.pj && j.pj === etat.pj.id;
    return ligneJet(j, { moi: mien, critique: mien && j.faces === etat.de ? etat.crit : 0 });
  }).join("") : `<li class="vide">Aucun jet pour l'instant.</li>`;
}

async function relireJets() {
  try {
    const d = await api("GET", "/api/joueur/des?depuis=" + etat.dernierJet);
    if (d.dernier < etat.dernierJet) { etat.jets = []; etat.dernierJet = 0; } // l'application a redémarré
    ajouterJets(d.jets);
  } catch { /* hors ligne */ }
}

/* ---------------------------------------------------------- hauts faits */
async function vueHautsFaits() {
  vue.className = "combat";
  vue.innerHTML = `<section class="panel hauts-faits" id="hf"><p class="hint">Chargement…</p></section>`;
  const box = $("#hf", vue);
  let d;
  try { d = await api("GET", "/api/joueur/hauts-faits"); } catch (e) { box.innerHTML = `<p class="empty">${esc(e.message)}</p>`; return; }
  if (etat.vue !== "hauts-faits") return;
  const mien = r => !!etat.pj && r.pj === etat.pj.id;
  const recs = [...(d.records || [])].sort((a, b) => mien(b) - mien(a));
  const titres = r => Object.entries(r.titres || {}).sort((a, b) => b[1] - a[1]).map(([t, n]) => `<span class="tag">${esc(t)}${n > 1 ? " ×" + n : ""}</span>`).join(" ");
  const carte = r => `<article class="record${mien(r) ? " moi" : ""}">
      <h3>${esc(r.nom)}${mien(r) ? ` <span class="flag-moi petit">vous</span>` : ""}</h3>
      <div class="record-stats">
        <div><b class="num">${r.combats}</b><span>combat${r.combats > 1 ? "s" : ""}</span></div>
        <div><b class="num">${r.victoires}</b><span>victoire${r.victoires > 1 ? "s" : ""}</span></div>
        <div><b class="num">${r.abattus}</b><span>abattus</span></div>
        <div><b class="num">${r.critiques}</b><span>critiques</span></div>
        <div><b class="num">${r.infliges}</b><span>DM infligés</span></div>
        <div><b class="num">${r.tentatives ? Math.round(r.touches / r.tentatives * 100) + "%" : "—"}</b><span>précision</span></div>
      </div>
      ${r.meilleur_coup ? `<p class="record-coup">Meilleur coup : <b>${esc(r.meilleur_txt)}</b> <span class="hint">— ${esc(r.meilleur_ou)}</span></p>` : ""}
      ${titres(r) ? `<p class="record-titres">${titres(r)}</p>` : ""}
    </article>`;
  box.innerHTML = `
    <h1>Hauts faits</h1>
    ${d.combats.length ? "" : `<p class="empty">Aucun combat terminé pour l'instant. Le récap de chaque combat apparaîtra ici dès que le MJ le clôturera.</p>`}
    ${recs.length ? `<h2>Records</h2><div class="records">${recs.map(carte).join("")}</div>` : ""}
    ${d.combats.length ? `<h2>Combats</h2><div class="combats-passes">${d.combats.map((c, i) => `
      <details${i === 0 ? " open" : ""}><summary><b>${esc(c.recap.nom)}</b> <span class="tag ${c.recap.issue === "victoire" ? "allie" : c.recap.issue === "defaite" ? "ennemi" : ""}">${{ victoire: "Victoire", defaite: "Défaite" }[c.recap.issue] || "Terminé"}</span> <span class="hint">${dateCourte(c.recap.date)} · ${c.recap.rounds} round${c.recap.rounds > 1 ? "s" : ""}</span></summary>
        ${recapHTML(c.recap, { titre: false })}
        <div class="row"><button type="button" class="ghost" data-copier="${i}">Copier en Markdown</button></div></details>`).join("")}</div>` : ""}`;
  box.onclick = async e => {
    const b = e.target.closest("[data-copier]"); if (!b) return;
    toast(await copier(recapMarkdown(d.combats[+b.dataset.copier].recap)) ? "Récap copié en Markdown." : "Copie impossible depuis ce navigateur.");
  };
}

/* bandeau permanent, hors de la vue du combat : à qui le tour */
function rendreBandeau() {
  const b = $("#bandeau"), c = etat.combat;
  const visible = !!c && c.ordre.length > 0 && etat.vue !== "combat" && !!etat.pj;
  b.hidden = !visible;
  if (!visible) return;
  const courant = c.ordre.find(p => p.courant), suivant = c.ordre.find(p => p.suivant);
  b.className = moi(courant) ? "moi" : "";
  b.innerHTML = `<a href="#/combat">Round ${c.round} · ${moi(courant) ? "<b>C'est à vous !</b>" : `Au tour de <b>${esc(courant?.nom || "—")}</b>`}${suivant ? ` · ensuite ${moi(suivant) ? "<b>vous</b>" : esc(suivant.nom)}` : ""}</a>`;
}

/* ---------------------------------------------------------- temps réel */
let ws = null, essais = 0, dernier = Date.now(), arrete = false;
function connecter() {
  if (arrete) return;
  const sock = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${lien("/api/joueur/direct")}`);
  ws = sock;
  sock.onopen = () => { essais = 0; dernier = Date.now(); $("#conn").hidden = true; };
  sock.onmessage = e => {
    dernier = Date.now();
    const m = JSON.parse(e.data);
    if (m.type === "jet") { ajouterJets([m.jet]); return; }
    if (m.type === "etat") { relireJets(); etat.combat = m.combat; if (etat.edition !== m.edition) { etat.edition = m.edition; rafraichirFiche(); } }
    else if (m.type === "combat") etat.combat = m.combat;
    else if (m.type === "edition") { etat.edition = m.active; toast(m.active ? "Le MJ a déverrouillé les feuilles : vous pouvez modifier la vôtre." : "Le MJ a verrouillé les feuilles."); if (etat.vue === "fiche" && etat.pj) vueFiche(); }
    else if (m.type === "fiche" && etat.pj && m.id === etat.pj.id) rafraichirFiche();
    else if (m.type === "hauts_faits") { if (etat.vue === "hauts-faits") vueHautsFaits(); }
    if (etat.vue === "combat") vueCombat();
    rendreBandeau();
  };
  sock.onclose = () => {
    if (ws !== sock || arrete) return;
    $("#conn").hidden = false; $("#conn").textContent = "Reconnexion…";
    setTimeout(connecter, Math.min(3000, 300 * 2 ** essais++));
  };
}
setInterval(() => {
  if (!ws || ws.readyState !== 1) return;
  if (Date.now() - dernier > 45000) { ws.close(); return; }
  ws.send(JSON.stringify({ type: "ping" }));
}, 15000);
document.addEventListener("visibilitychange", () => { if (document.visibilityState === "visible" && (!ws || ws.readyState > 1)) connecter(); });

/* ---------------------------------------------------------- routeur */
async function route() {
  const nom = location.hash.replace(/^#\/?/, "") || "fiche";
  etat.vue = !etat.pj || nom === "choix" ? "choix" : ["combat", "des", "hauts-faits"].includes(nom) ? nom : "fiche";
  $$("[data-nav]").forEach(a => a.classList.toggle("on", a.dataset.nav === etat.vue));
  $("#pj-nav").hidden = !etat.pj;
  $("#pj-changer").hidden = etat.serveur || !etat.pj || etat.vue === "choix";
  rendreBandeau();
  vue.onclick = null;
  window.scrollTo(0, 0);
  if (etat.vue === "choix") vueChoix();
  else if (etat.vue === "combat") vueCombat();
  else if (etat.vue === "des") await vueDes();
  else if (etat.vue === "hauts-faits") await vueHautsFaits();
  else await vueFiche();
}

function fermee(titre, texte) {
  arrete = true;
  $("#pj-nav").hidden = true; $("#bandeau").hidden = true;
  vue.className = "combat";
  vue.innerHTML = `<section class="panel pj-choix"><h1>${esc(titre)}</h1><p class="hint">${texte}</p><div><button class="primary" id="pj-reessayer">Réessayer</button></div></section>`;
  $("#pj-reessayer").onclick = () => location.reload();
}

async function demarrer() {
  let e;
  try { e = await api("GET", "/api/joueur/etat"); }
  catch (err) {
    if (err.status === 401) fermee("La page des joueurs est fermée", `Demandez au MJ de l'ouvrir dans Paramètres → Tablette et réseau. Sur un serveur, <a href="/">connectez-vous</a> d'abord avec votre compte.`);
    else if (err.status === 403) fermee("Pas d'accès", "Votre compte n'a pas de rôle dans cet univers.");
    else fermee("COF DM ne répond pas", esc(err.message));
    return;
  }
  Object.assign(etat, { serveur: e.serveur, edition: e.edition, combat: e.combat, de: e.de || 20 });
  $("#pj-univers").textContent = e.univers || "";
  etat.pjs = await api("GET", "/api/joueur/pj").catch(() => []);
  // sur un serveur, le PJ est celui du compte ; à la maison, celui choisi sur cet appareil
  const voulu = e.serveur && e.pj ? e.pj : lireChoix();
  etat.pj = etat.pjs.find(p => p.id === voulu) || null;
  if (e.serveur && !etat.pj && etat.pjs.length === 1) etat.pj = etat.pjs[0];
  if (e.serveur && !etat.pj && !etat.pjs.length) { fermee("Aucun personnage", "Aucun PJ n'est rattaché à votre compte, ou sa fiche n'est pas partagée. Demandez au MJ."); return; }
  if (!etat.pj) garderChoix(0);
  $("#pj-changer").onclick = () => { location.hash = "#/choix"; };
  window.addEventListener("hashchange", route);
  connecter();
  route();
}
demarrer();
