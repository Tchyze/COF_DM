// Afficheur de table : une seconde fenêtre du MJ, placée sur une télé (en HDMI) et lue de loin par
// les joueurs. Elle suit le combat en cours par le même WebSocket que l'écran de combat du MJ (donc
// aussi quand le combat se joue sur le serveur), et n'en montre que ce que la table peut voir :
// l'ordre d'initiative, le round, qui joue et qui suit, les PV et PM des PJ, les états de chacun.
// D'un adversaire ou d'un allié, rien que vivant ou hors de combat : aucun indice sur sa santé.
// Elle ne fait qu'afficher : aucune action, aucun POST.
import { $, esc, api, lien } from "./ui.js";
import { chargerSchema, actif } from "./schema.js";

const CANAL = "cof-afficheur"; // l'écran de combat du MJ y annonce le combat qu'il ouvre
const params = new URLSearchParams(location.search);
const scene = $("#scene"), zoneJets = $("#jets"), conn = $("#conn");

let cid = +params.get("combat") || 0; // combat suivi (0 : le plus récent en cours)
let st = null, ws = null, retry = 0, timerReco = null, dernierPong = Date.now();
const portraits = new Map(); // fiche → adresse de son image principale ("" : aucune)

/* ------------------------------------------------------------ ce que la table voit d'un combat */

const CAMPS = { pj: "PJ", allie: "Allié", ennemi: "Adversaire" };
const etatsDe = c => Object.keys(c.etats || {}).sort((a, b) => a.localeCompare(b, "fr"));
const tombe = c => c.pv <= 0;

/** La place d'un combattant, réduite à ce qui s'affiche : jamais les PV d'un adversaire ou d'un allié. */
export function vueCombattant(c, i, st) {
  const pj = c.camp === "pj";
  return {
    id: c.id,
    nom: c.nom,
    camp: c.camp,
    fiche: c.ref && (c.ref.kind === "pj" || c.ref.kind === "pnj") ? c.ref.id : 0,
    courant: i === st.tour,
    tombe: tombe(c),
    etats: etatsDe(c),
    pv: pj ? { n: Math.max(0, c.pv), max: c.pvMax } : null,
    pm: pj && c.pmMax > 0 ? { n: Math.max(0, c.pm || 0), max: c.pmMax } : null,
  };
}

/** L'ordre d'initiative tel que la table le voit, et qui joue ensuite (le premier encore debout). */
export function vueOrdre(st) {
  const ordre = st.ordre.map((id, i) => st.cbts[id] && vueCombattant(st.cbts[id], i, st)).filter(Boolean);
  const cur = ordre.findIndex(p => p.courant);
  let suivant = null;
  if (cur >= 0) {
    for (let k = 1; k < ordre.length; k++) {
      const p = ordre[(cur + k) % ordre.length];
      if (!p.tombe) { suivant = p; break; }
    }
  }
  return { ordre, courant: ordre[cur] || null, suivant };
}

/* ------------------------------------------------------------ rendu */

const jauge = (v, cls) => {
  const part = v.max > 0 ? Math.max(0, Math.min(100, Math.round(100 * v.n / v.max))) : 0;
  return `<span class="aff-jauge ${cls}"><span class="aff-jauge-val num">${v.n} / ${v.max}</span><span class="aff-jauge-barre"><span style="width:${part}%"></span></span></span>`;
};

function portrait(p, grand = false) {
  const src = p.fiche ? portraits.get(p.fiche) : "";
  const init = esc((p.nom.trim()[0] || "?").toUpperCase());
  return `<span class="aff-portrait ${p.camp}${grand ? " grand" : ""}">${src ? `<img src="${esc(src)}" alt="">` : `<span>${init}</span>`}</span>`;
}

function ligne(p) {
  const tag = p.tombe ? (p.camp === "pj" ? "À terre" : "Hors de combat") : "";
  return `<li class="aff-ligne ${p.camp}${p.courant ? " courant" : ""}${p.tombe ? " tombe" : ""}">
    ${portrait(p)}
    <span class="aff-qui">
      <span class="aff-nom">${esc(p.nom)}</span>
      <span class="aff-etiquettes"><span class="tag ${p.camp}">${CAMPS[p.camp] || ""}</span>${tag ? `<span class="tag aff-tombe">${tag}</span>` : ""}${p.etats.map(e => `<span class="chip">${esc(e)}</span>`).join("")}</span>
    </span>
    ${p.pv ? `<span class="aff-chiffres">${jauge(p.pv, "pv")}${p.pm ? jauge(p.pm, "pm") : ""}</span>` : ""}
  </li>`;
}

function rendre() {
  if (!st || st.termine || !st.ordre.length) {
    scene.innerHTML = `<section class="aff-attente"><span class="sceau">DM</span>
      <h1>${st?.termine ? "Combat terminé" : "Pas de combat en cours"}</h1>
      <p class="hint">L'afficheur suivra le prochain combat dès que le MJ l'ouvrira.</p></section>`;
    return;
  }
  const { ordre, courant, suivant } = vueOrdre(st);
  scene.innerHTML = `<section class="aff-combat">
    <header class="aff-tete">
      <h1>${esc(st.nom)}</h1>
      <span class="aff-round"><span class="lbl">Round</span><span class="n num">${st.round}</span></span>
    </header>
    <div class="aff-tour ${courant?.camp || ""}">
      ${courant ? portrait(courant, true) : ""}
      <span class="aff-tour-texte"><span class="lbl">C'est au tour de</span><span class="aff-tour-nom">${courant ? esc(courant.nom) : "—"}</span>
      ${suivant ? `<span class="aff-ensuite"><span class="lbl">Ensuite</span> <b>${esc(suivant.nom)}</b></span>` : ""}</span>
    </div>
    <ol class="aff-ordre${ordre.length > 12 ? " tres-serre" : ordre.length > 8 ? " serre" : ""}" style="--lignes:${Math.ceil(ordre.length / 2)}">${ordre.map(ligne).join("")}</ol>
  </section>`;
  chargerPortraits(ordre);
}

// Les portraits : l'image principale de la fiche du PJ ou du PNJ (les monstres du bestiaire n'en ont
// pas). Lus une fois par fiche ; une fiche absente ou sans image garde l'initiale.
async function chargerPortraits(ordre) {
  const manquants = [...new Set(ordre.map(p => p.fiche).filter(id => id && !portraits.has(id)))];
  if (!manquants.length) return;
  manquants.forEach(id => portraits.set(id, ""));
  await Promise.all(manquants.map(async id => {
    try {
      const f = await api("GET", `/api/entites/${id}`);
      const m = (f.medias || []).find(x => x.principal) || (f.medias || [])[0];
      if (m) portraits.set(id, lien(`/medias/${encodeURIComponent(m.fichier)}`));
    } catch { /* fiche absente : l'initiale suffit */ }
  }));
  if (manquants.some(id => portraits.get(id))) rendre();
}

/* ------------------------------------------------------------ suivi du combat */

// Sans combat désigné (ou le sien fini), l'afficheur prend le plus récent en cours.
async function choisirCombat() {
  if (cid && st && !st.termine) return;
  try {
    const liste = await api("GET", "/api/combats?en_cours=1");
    const c = liste.find(x => !x.termine);
    if (c && c.id !== cid) suivre(c.id);
    else if (!c && !cid) rendre();
  } catch { /* l'application ne répond pas : on réessaie */ }
}

function suivre(id) {
  if (id === cid && ws && ws.readyState <= 1) return;
  cid = id; st = null;
  try { ws?.close(); } catch { /* rien */ }
  ws = null; retry = 0;
  api("GET", `/api/combats/${cid}`).then(m => { if (m.etat && id === cid) { st = m.etat; rendre(); } }).catch(() => {});
  connecter();
}

function connecter() {
  if (!cid) return;
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const id = cid;
  const sock = new WebSocket(`${proto}://${location.host}${lien(`/api/combats/${id}/direct`)}`);
  ws = sock;
  sock.onopen = () => { retry = 0; dernierPong = Date.now(); conn.hidden = true; };
  sock.onmessage = e => {
    if (ws !== sock) return;
    const m = JSON.parse(e.data);
    dernierPong = Date.now();
    if (m.type === "etat") { st = m.etat; rendre(); }
    else if (m.type === "connexion") { conn.hidden = m.etat !== "perdue"; conn.textContent = "Connexion au serveur perdue…"; }
    else if (m.type === "supprime") { cid = 0; st = null; rendre(); choisirCombat(); }
  };
  sock.onclose = () => {
    if (ws !== sock) return;
    conn.hidden = false; conn.textContent = "Reconnexion…";
    clearTimeout(timerReco);
    timerReco = setTimeout(connecter, Math.min(5000, 300 * 2 ** retry++));
  };
}

// une connexion peut mourir sans prévenir : on interroge l'application, et on se reconnecte
setInterval(() => {
  if (!ws || ws.readyState !== 1) return;
  if (Date.now() - dernierPong > 45000) { try { ws.close(); } catch { /* rien */ } ws.onclose?.(); return; }
  ws.send(JSON.stringify({ type: "ping" }));
}, 15000);

/* ------------------------------------------------------------ dés de la table */

// Les jets publics seulement (un jet caché du MJ ne s'affiche jamais), et seulement si les dés
// virtuels sont activés dans l'univers.
const jets = [];
let dernierJet = 0, premierPassage = true;
async function relireJets() {
  if (!actif("des")) { zoneJets.hidden = true; return; }
  try {
    const d = await api("GET", "/api/des?depuis=" + dernierJet);
    if (d.dernier < dernierJet) jets.length = 0; // l'application a redémarré : nouvelle partie
    dernierJet = d.dernier ?? dernierJet;
    const nouveaux = (d.jets || []).filter(j => !j.cache);
    jets.push(...nouveaux);
    jets.splice(0, Math.max(0, jets.length - 5));
    if (nouveaux.length || premierPassage) rendreJets(premierPassage ? 0 : nouveaux.at(-1)?.id);
    premierPassage = false;
  } catch (e) { if (e.status === 404) zoneJets.hidden = true; /* 404 : dés coupés ; sinon hors ligne, on réessaie */ }
}

function rendreJets(neuf) {
  zoneJets.hidden = !jets.length;
  zoneJets.innerHTML = `<h2>Dés</h2><ol>${[...jets].reverse().map(j => `
    <li class="aff-jet${j.id === neuf ? " neuf" : ""}"><span class="aff-jet-total num">${j.total}</span>
      <span><b>${esc(j.qui)}</b>${j.libelle ? ` · ${esc(j.libelle)}` : ""}<span class="hint"> ${esc(j.detail)}</span></span></li>`).join("")}</ol>`;
}

/* ------------------------------------------------------------ fenêtre */

$("#plein").onclick = () => basculerPleinEcran();
document.addEventListener("dblclick", () => basculerPleinEcran());
function basculerPleinEcran() {
  if (document.fullscreenElement) document.exitFullscreen?.().catch(() => {});
  else document.documentElement.requestFullscreen?.().catch(() => {});
}
// les outils et le curseur s'effacent quand la souris ne bouge plus
let timerSouris = null;
document.addEventListener("mousemove", () => {
  document.body.classList.add("souris");
  clearTimeout(timerSouris);
  timerSouris = setTimeout(() => document.body.classList.remove("souris"), 2500);
});

try {
  const canal = new BroadcastChannel(CANAL);
  canal.onmessage = e => { if (e.data?.combat) suivre(+e.data.combat); };
} catch { /* sans BroadcastChannel, l'afficheur suit le plus récent combat en cours */ }

async function demarrer() {
  try { await chargerSchema(); } catch { /* sans schéma : tout est tenu pour activé */ }
  if (!actif("combat")) {
    scene.innerHTML = `<section class="aff-attente"><span class="sceau">DM</span><h1>Le combat est désactivé</h1><p class="hint">Activez-le dans Paramètres → Fonctionnalités.</p></section>`;
    return;
  }
  if (cid) suivre(cid); else await choisirCombat();
  rendre();
  setInterval(choisirCombat, 5000);
  relireJets();
  setInterval(relireJets, 3000);
}
demarrer();
