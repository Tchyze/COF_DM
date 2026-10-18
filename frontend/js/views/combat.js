// Combat (repris de tracker_COF) : liste des combats et écran de jeu, suivi en temps réel. Le
// serveur fait foi : chaque action part en POST, l'état revient par la réponse et par le WebSocket.
// Les valeurs (dé, critique, point de chance) et les états viennent des règles de l'univers ; sans
// elles, le combat est minimal.
import { $, $$, esc, api, toast, armer, lien } from "../ui.js";
import { recapHTML, recapMarkdown } from "../recap.js";

const md = s => esc(s).replace(/\*\*(.+?)\*\*/g, "<b>$1</b>");
const CAMP = { pj: "PJ", allie: "Allié", ennemi: "Ennemi" };
const TYPE = { C: "Contact", D: "Distance", M: "Magique" };
const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
const lectureSeule = () => !!window.cofAcces?.lecture_seule;

/** Remplace le contenu d'un élément en gardant la saisie et le focus des champs qui ont un id. */
function patch(el, html) {
  const vals = {};
  $$("input[id],select[id],textarea[id]", el).forEach(i => { vals[i.id] = i.type === "checkbox" ? i.checked : i.value; });
  const focus = document.activeElement && el.contains(document.activeElement) ? document.activeElement.id : null;
  el.innerHTML = html;
  for (const [id, v] of Object.entries(vals)) {
    const i = el.querySelector("#" + CSS.escape(id));
    if (!i || i.dataset.keep === "no") continue;
    if (i.type === "checkbox") i.checked = v; else if (i.tagName !== "SELECT") i.value = v;
  }
  if (focus) { const f = el.querySelector("#" + CSS.escape(focus)); if (f) f.focus(); }
}

async function copier(texte) {
  try { await navigator.clipboard.writeText(texte); return true; }
  catch {
    const ta = document.createElement("textarea");
    ta.value = texte; ta.style.position = "fixed"; ta.style.opacity = "0";
    document.body.append(ta); ta.select();
    let ok = false; try { ok = document.execCommand("copy"); } catch { /* rien */ }
    ta.remove(); return ok;
  }
}

/** Lance un combat et dit ce qui n'a pas pu y entrer. Renvoie son résumé. */
export async function lancerCombat(corps) {
  const r = await api("POST", "/api/combats", corps);
  if (r.absents?.length) toast("Restés hors du combat : " + r.absents.join(" ; ") + ".", true);
  return r.combat;
}

export async function mount(el, arg) {
  if (arg) {
    el.innerHTML = `<div class="combat-hote page"></div>`;
    return ecran($(".combat-hote", el), +arg);
  }
  return liste(el);
}

/* ------------------------------------------------------------ liste */
async function liste(el) {
  let combats = [], rencontres = [], campagnes = [];
  try {
    [combats, rencontres, campagnes] = await Promise.all([api("GET", "/api/combats"), api("GET", "/api/entites?type=rencontre"), api("GET", "/api/entites?type=campagne")]);
  } catch (e) { toast(e.message, true); }
  const date = t => t ? new Date(t * 1000).toLocaleString("fr-FR", { dateStyle: "short", timeStyle: "short" }) : "";
  const lecture = lectureSeule();
  el.innerHTML = `
    <div class="combat liste"><div class="col">
      ${lecture ? "" : `<section class="panel">
        <h1>Nouveau combat</h1>
        <div class="row">
          <label>Rencontre préparée<select id="nRen"><option value="">Aucune : combat improvisé</option>${rencontres.map(r => `<option value="${r.id}">${esc(r.nom)}</option>`).join("")}</select></label>
          <label>PJ de la campagne<select id="nCamp"><option value="">Celle de la rencontre, sinon aucun PJ</option>${campagnes.map(c => `<option value="${c.id}">${esc(c.nom)}</option>`).join("")}</select></label>
          <label>Nom (facultatif)<input id="nNom" placeholder="Nom de la rencontre par défaut"></label>
        </div>
        <p class="hint">Une rencontre amène ses créatures, ses PNJ présents qui ont un profil de combat et les PJ en jeu de sa campagne. Un PJ entre en combat avec sa feuille de personnage.</p>
        <div><button class="primary big" id="nGo">Lancer le combat</button></div>
      </section>`}
      <section class="panel">
        <h2>Combats</h2>
        <div class="cards" id="cards">${combats.length ? combats.map(c => `
          <article class="ccard${c.termine ? " done" : ""}">
            <h3>${esc(c.nom)}</h3>
            <p class="hint">${c.termine ? ({ victoire: "Victoire", defaite: "Défaite" }[c.issue] || "Terminé") : "En cours"} · round ${c.round} · ${c.pj} PJ, ${c.ennemis} adversaire${c.ennemis > 1 ? "s" : ""} debout</p>
            <p class="hint">Dernière action : ${date(c.updated)}</p>
            <div class="actions-bar"><button class="${c.termine ? "" : "primary"}" data-open="${c.id}">${c.termine ? "Revoir" : lecture ? "Suivre" : "Reprendre"}</button><span class="sp"></span>${lecture ? "" : `<button class="danger" data-del="${c.id}">Supprimer</button>`}</div>
          </article>`).join("") : `<p class="empty">Aucun combat pour l'instant.</p>`}</div>
      </section>
    </div></div>`;
  $$("[data-del]", el).forEach(b => armer(b, "Confirmer", async () => {
    try { await api("DELETE", `/api/combats/${b.dataset.del}`); b.closest(".ccard").remove(); toast("Combat supprimé."); }
    catch (e) { toast(e.message, true); }
  }));
  $$("[data-open]", el).forEach(b => { b.onclick = () => { location.hash = `#/combat/${b.dataset.open}`; }; });
  if ($("#nGo", el)) $("#nGo", el).onclick = async () => {
    const ren = +$("#nRen", el).value, camp = +$("#nCamp", el).value;
    try {
      const cb = await lancerCombat({ nom: $("#nNom", el).value, ...(ren ? { rencontre: ren } : {}), ...(camp ? { campagne: camp } : {}) });
      location.hash = `#/combat/${cb.id}`;
    } catch (e) { toast(e.message, true); }
  };
}

/* ------------------------------------------------------------ écran */
// Dés réels (lancés à la table, saisis ici) ou virtuels : réglage gardé sur l'appareil.
const CLE_DES = "cof-des";
function lireDes() {
  try { const v = localStorage.getItem(CLE_DES); return ["reels", "pnj", "tous"].includes(v) ? v : "reels"; }
  catch { return "reels"; }
}

// L'écran tient dans la fenêtre, sans défilement de page : ordre d'initiative et action en cours
// restent sous les yeux ; tout le reste (test, états, ajout, journal, fiche d'un combattant, règles,
// options) s'ouvre dans une fenêtre secondaire.
// integre : l'écran est logé dans une autre page (écran du MJ) ; auRetour y remplace le lien vers
// la liste des combats.
export async function ecran(el, cid, { integre = false, auRetour = null } = {}) {
  const RG = (await api("GET", "/api/regles").catch(() => ({}))).combat || null;
  const ETATS = Object.fromEntries((RG?.etats || []).map(e => [e.nom, e]));
  const DE = RG?.de || 20, ECHEC = RG?.echec_naturel || 0, CRIT = RG?.critique_dm || 0, CHANCE = RG?.chance || 0, BUDGET = !!RG?.budget_actions;
  const POSTURES = (RG?.etats || []).filter(e => e.action);
  const libres = Object.keys(ETATS).filter(e => !ETATS[e].action); // les états qui se posent sur un combattant
  const uneAction = c => etatsDe(c).some(e => ETATS[e]?.une_action);
  const retour = () => { if (auRetour) auRetour(); else location.hash = "#/combat"; };
  let st = null, annulable = 0, ws = null, retry = 0, ferme = false, cleTour = null;
  let monstres = [], pjs = [], pnjs = [];
  const ui = { tMode: "diff", tWho: null, tOpp: null, att: null, tgt: null, atk: 0, act: "A", actAuto: true, pane: "ordre", fiche: null, resFerme: 0, addTab: "monstre", delArm: null, dmAuto: true, des: lireDes() };
  // Qui lance les dés d'un combattant : la table (dés réels, saisis ici) ou l'application.
  const virtuel = c => ui.des === "tous" || (ui.des === "pnj" && c?.camp !== "pj");
  const fenetre = (id, titre, corps) => `<div class="fen" id="${id}" hidden><div class="fen-boite" role="dialog" aria-label="${titre}">
      <div class="fen-tete"><h2>${titre}</h2><button type="button" class="icon ghost" data-fermer aria-label="Fermer">✕</button></div>
      <div class="fen-corps">${corps}</div></div></div>`;

  el.innerHTML = `<div class="combat">
    <div class="ctop">
      <a href="#/combat" class="hint" id="cRetour" title="Liste des combats">◀<span class="lib"> Combats</span></a>
      <h1 id="cNom">Chargement…</h1>
      <span class="conn" id="conn" hidden></span>
      <div class="round"><span class="lbl">Round</span><span class="n num" id="roundN">–</span></div>
      <div class="tools">
        <button id="bUndo" class="ghost" title="Annuler la dernière action">↶<span class="lib"> Annuler</span><span class="kbd">Z</span></button>
        <button id="bPrev" class="ghost" title="Revenir au tour précédent">◀<span class="lib"> Précédent</span></button>
        <button id="bNext" class="primary big">Fin du tour ▶<span class="kbd">N</span></button>
        <button class="ghost" data-ouvre="fMenu" title="Ajouter, test, état, journal, options…">☰<span class="lib"> Menu</span></button>
      </div>
    </div>
    <div id="banner"></div>
    <div class="panes"><div class="seg" role="tablist" id="paneSeg">
      <button data-pane="ordre" aria-pressed="true">Ordre</button><button data-pane="tour" aria-pressed="false">Tour</button><button data-pane="action" aria-pressed="false">Action</button>
    </div></div>
    <div class="cgrid" id="grid" data-pane="ordre">
      <section class="col c-ordre">
        <div class="panel p-ordre">
          <div class="panel-head"><h2>Ordre d'initiative</h2><span class="hint" id="countInfo"></span></div>
          <ol class="order" id="order"></ol>
          <button type="button" class="ghost" id="bTous" data-ouvre="fTous" hidden></button>
        </div>
      </section>
      <section class="col c-act">
        <div class="panel p-carte"><div id="activeCard" class="active-card"></div></div>
        <div class="panel p-atk">
          <div class="panel-head"><h2>Résoudre une attaque</h2><button type="button" class="ghost icon" id="modeInfo" data-ouvre="fOptions" title="Changer dans les options"></button></div>
          <div id="atkForm" class="col">
            <div class="selecteurs">
              <div class="row deux">
                <label>Attaquant<select id="aAtt"></select></label>
                <label>Cible<select id="aTgt"></select></label>
              </div>
              <div class="row deux">
                <label>Attaque<select id="aAtk"></select></label>
                <label>Action<select id="aAct">
                  <option value="A">Attaque (A)</option>
                  <option value="L">Capacité limitée (L)</option>
                  <option value="LS">Suite de la capacité (L)</option>
                  <option value="G">Gratuite (riposte, capacité…)</option>
                </select></label>
              </div>
            </div>
            <div class="row trois">
              <label class="s" id="natWrap">Dé lancé<input id="aNat" type="number" min="1" max="${DE}" inputmode="numeric" placeholder="d${DE}"></label>
              <label class="s">Bonus divers<input id="aDiv" type="number" value="0"></label>
              <label class="s" id="pmWrap" hidden>Coût PM<input id="aPm" type="number" min="0" value="0" inputmode="numeric"></label>
              <label>Dégâts<input id="aDm" placeholder="1d8+2"></label>
            </div>
            <div class="vs" id="vs"></div>
            <p class="hint ligne2" id="mods"></p>
            <p class="hint ligne1" id="actInfo"></p>
            <div><button type="button" class="primary big" id="bAtk">Lancer l'attaque</button></div>
          </div>
          <div id="result"></div>
        </div>
        <div class="raccourcis">
          <button type="button" data-ouvre="fTest">🎲 Test</button><button type="button" data-ouvre="fEtat">État</button>
          <button type="button" data-ouvre="fAjout">＋ Ajouter</button><button type="button" class="b-journal" data-ouvre="fJournal">Journal</button>
        </div>
      </section>
      <section class="col c-log">
        <div class="panel p-log">
          <div class="panel-head"><h2>Journal</h2><button type="button" class="icon ghost" data-ouvre="fJournal">Tout voir</button></div>
          <div class="logapercu" id="logApercu"></div>
        </div>
      </section>
    </div>

    ${fenetre("fMenu", "Menu du combat", `<div class="menu-combat">
        <button type="button" data-ouvre="fAjout">＋ Ajouter au combat</button>
        <button type="button" data-ouvre="fTest">🎲 Test de caractéristique</button>
        <button type="button" data-ouvre="fEtat">Appliquer un état</button>
        <button type="button" data-ouvre="fJournal">Journal du combat</button>
        <button type="button" data-ouvre="fTous">Tous les combattants</button>
        <button type="button" id="bSort" data-fermer>Trier par initiative</button>
        <button type="button" data-ouvre="fRegles">Rappel des règles appliquées</button>
        <button type="button" data-ouvre="fOptions">Options : dés réels ou virtuels</button>
        <button type="button" id="bEnd" data-fermer>Terminer le combat</button>
      </div>`)}
    ${fenetre("fTous", "Tous les combattants", `<ol class="order" id="orderTous"></ol>`)}
    ${fenetre("fCbt", "Combattant", `<div id="ficheCbt" class="col"></div>`)}
    ${fenetre("fTest", "Test de caractéristique", `<div class="col" id="testPanel">
        <div class="seg" aria-label="Type de test"><button type="button" id="tDiff" aria-pressed="true">Difficulté</button><button type="button" id="tOpp" aria-pressed="false">Opposition</button></div>
        <div class="row">
          <label>Combattant<select id="tWho"></select></label>
          <label class="s">Carac.<select id="tCar">${["FOR", "DEX", "CON", "INT", "SAG", "CHA"].map(c => `<option>${c}</option>`).join("")}</select></label>
          <label class="s" id="tDiffWrap">Difficulté<input type="number" id="tDiffV" value="10" min="0" inputmode="numeric"></label>
        </div>
        <div class="row" id="tOppWrap" hidden>
          <label>Opposant (défenseur)<select id="tOppWho"></select></label>
          <label class="s">Carac.<select id="tOppCar">${["FOR", "DEX", "CON", "INT", "SAG", "CHA"].map(c => `<option>${c}</option>`).join("")}</select></label>
        </div>
        <div class="row">
          <label>En cas d'échec<input id="tEtat" list="dlEtats" maxlength="40" placeholder="Aucun état"></label>
          <label class="s">Durée<input type="number" id="tDuree" min="1" placeholder="∞" inputmode="numeric"></label>
          <label class="s" id="tNatWrap" hidden>Dé gardé<input type="number" id="tNat" min="1" max="${DE}" inputmode="numeric" placeholder="d${DE}"></label>
          <label class="s" id="tNatOppWrap" hidden>Dé opposant<input type="number" id="tNatOpp" min="1" max="${DE}" inputmode="numeric" placeholder="d${DE}"></label>
        </div>
        <p class="hint" id="tInfo"></p>
        <div><button type="button" class="primary" id="bTest">Lancer le test</button></div>
        <div id="tRes"></div>
      </div>`)}
    ${fenetre("fEtat", "Appliquer un état", `<div class="col">
        <div class="row">
          <label>Combattant<select id="eWho"></select></label>
          <label>État<input id="eWhat" list="dlEtats" maxlength="40" placeholder="${libres.length ? "Choisir ou saisir" : "Nom de l'état"}"></label>
          <label class="s">Durée (rounds)<input id="eDur" type="number" min="1" placeholder="∞"></label>
        </div>
        <p class="hint" id="eDesc"></p>
        <div><button type="button" class="primary" id="bEtat">Appliquer</button></div>
      </div>`)}
    ${fenetre("fAjout", "Ajouter au combat", `<div class="col" id="addBox">
        <div class="seg" id="addSeg"><button data-t="monstre" aria-pressed="true">Monstre</button><button data-t="pj" aria-pressed="false">Personnage</button><button data-t="libre" aria-pressed="false">Libre</button></div>
        <div id="addBody"></div>
      </div>`)}
    ${fenetre("fJournal", "Journal du combat", `<div class="row"><button type="button" id="bCopy" class="icon">Copier pour Obsidian</button><button type="button" id="bClear" class="icon ghost">Vider</button></div>
      <div class="logwrap" id="log"></div>`)}
    ${fenetre("fRegles", "Règles appliquées", `<div class="ref">
        <b>Initiative</b><span>La plus haute d'abord. À égalité, les PJ agissent avant les autres ; entre deux PJ, la plus haute SAG.</span>
        <b>Attaque</b><span>d${DE} + bonus ≥ DEF de la cible : touché. La RD de la cible est soustraite aux dégâts.</span>
        ${CRIT ? `<b>Critique</b><span>Le maximum du dé (ou dès « critique sur ») : touche toujours, dégâts × ${CRIT}, bonus inclus. Avec un dé réduit par un état, pas de critique.</span>` : ""}
        ${ECHEC ? `<b>${ECHEC} naturel</b><span>Échec critique : l'action échoue même si le total suffit.</span>` : ""}
        ${BUDGET ? `<b>Tour</b><span>Une action d'attaque (A) et une de mouvement (M), ou deux M, ou une action limitée (L).</span>` : ""}
        ${CHANCE ? `<b>Point de chance</b><span>+${CHANCE} au dé, dépensé après avoir vu le résultat.</span>` : ""}
        <b>Points de mana</b><span>Un sort coûte ses PM au moment où il est lancé, qu'il touche ou non (« Coût PM » dans l'attaque) ; « Annuler » les rend. Les PM d'un PJ sont reportés sur sa feuille.</span>
        <b>0 PV</b><span>Inconscient, ne peut plus agir : ses tours sont sautés jusqu'à ce qu'il soit soigné.</span>
        <b>États</b><span>Leurs effets s'appliquent seuls ; la durée baisse à la fin du tour du combattant. Un état saisi librement n'a pas d'effet : c'est une note.</span>
        ${(RG?.etats || []).map(e => `<b>${esc(e.nom)}</b><span>${esc(e.description || "")}</span>`).join("")}
      </div>
      ${RG ? "" : `<p class="hint">Cet univers n'a pas de règles de combat : ni états prédéfinis, ni critique, ni point de chance, ni budget d'actions. Importez un fichier de règles dans Paramètres → Bestiaire et règles.</p>`}`)}
    ${fenetre("fOptions", "Options du combat", `<div class="col" id="optDes">
        <p class="hint">Qui lance les dés ? Avec les dés réels, vous saisissez ce que la table a lancé : le dé d'attaque, puis le total des dés de dégâts. L'application fait le reste (bonus, états, critique, RD).</p>
        ${[["reels", "Dés réels pour tous", "Joueurs et MJ lancent leurs dés à la table."],
           ["pnj", "Dés virtuels pour les adversaires et les alliés", "Les joueurs lancent leurs dés ; l'application lance ceux des autres."],
           ["tous", "Dés virtuels pour tous", "L'application lance tous les dés."]].map(([v, t, d]) =>
          `<label class="choix"><input type="radio" name="des" value="${v}"><span><b>${t}</b><small>${d}</small></span></label>`).join("")}
        <p class="hint">Réglage propre à cet appareil.</p>
      </div>`)}
    <datalist id="dlEtats">${libres.map(e => `<option value="${esc(e)}">`).join("")}</datalist>
    <dialog id="dlg"><form method="dialog" id="dlgForm"></form></dialog>
    <dialog id="dlgFin" class="dlg-recap"><div id="finBody"></div></dialog>
  </div>`;

  const $e = s => $(s, el);
  if (integre) $e("#cRetour").innerHTML = "◀";
  $e("#cRetour").onclick = e => { e.preventDefault(); retour(); };
  function fermerFenetres() { $$(".fen", el).forEach(f => { f.hidden = true; }); ui.fiche = null; }
  function ouvrir(id) {
    fermerFenetres();
    $e("#" + id).hidden = false;
    if (id === "fAjout") rendreAjout();
    if (id === "fTous") rendreOrdre();
  }
  // Une action part en POST ; le nouvel état revient dans la réponse (et par le WebSocket, pour les
  // autres écrans). Elles se suivent une à une, dans l'ordre où elles ont été tapées.
  let timerReco = null, timerPing = null, dernierPong = Date.now(), file = Promise.resolve();
  const send = a => {
    if (ferme) return;
    if (lectureSeule()) { toast("Votre compte suit ce combat sans pouvoir y agir.", true); return; }
    file = file.then(async () => {
      try { recevoir(await api("POST", `/api/combats/${cid}/actions`, a)); }
      catch (e) { if (!ferme) toast(e.message, true); }
    });
  };
  function reconnecterVite() {
    if (ferme || (ws && ws.readyState <= 1)) return;
    clearTimeout(timerReco); retry = 0; connecter();
  }
  const cur = () => st && st.ordre.length ? st.cbts[st.ordre[st.tour]] : null;
  const foes = (a, b) => (a.camp === "ennemi") !== (b.camp === "ennemi");
  const etatsDe = c => Object.keys(c.etats || {});

  /* --- connexion temps réel --- */
  function connecter() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const sock = new WebSocket(`${proto}://${location.host}${lien(`/api/combats/${cid}/direct`)}`);
    ws = sock;
    sock.onopen = () => { retry = 0; dernierPong = Date.now(); $e("#conn").hidden = true; };
    sock.onmessage = e => {
      if (ws !== sock) return;
      const m = JSON.parse(e.data);
      dernierPong = Date.now();
      if (m.type === "etat") recevoir(m);
      else if (m.type === "supprime") { ferme = true; toast("Ce combat n'existe plus.", true); retour(); }
    };
    sock.onclose = () => {
      if (ferme || ws !== sock) return;
      const c = $e("#conn"); c.hidden = false; c.textContent = "Reconnexion…";
      clearTimeout(timerReco);
      timerReco = setTimeout(connecter, Math.min(3000, 300 * 2 ** retry++));
    };
  }
  // Une connexion peut mourir sans prévenir (tablette en veille, Wi-Fi qui décroche) :
  // on interroge le serveur régulièrement et on se reconnecte s'il ne répond plus.
  timerPing = setInterval(() => {
    if (!ws || ws.readyState !== 1) return;
    if (Date.now() - dernierPong > 45000) { try { ws.close(); } catch { /* rien */ } ws.onclose?.(); return; }
    ws.send(JSON.stringify({ type: "ping" }));
  }, 15000);
  const reveil = () => { if (document.visibilityState === "visible") { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: "ping" })); else reconnecterVite(); } };
  document.addEventListener("visibilitychange", reveil);
  window.addEventListener("online", reconnecterVite);
  window.addEventListener("focus", reveil);

  function recevoir(m) {
    // à l'ouverture, une attaque déjà réglée ne masque pas le formulaire ; des dégâts en attente, si
    if (!st && m.etat.resultat && (m.etat.resultat.verdict === "miss" || m.etat.resultat.applique !== null)) ui.resFerme = m.etat.resultat.evt;
    st = m.etat; annulable = m.annulable;
    const c = cur();
    const cle = `${st.round}:${st.tour}:${c ? c.id : ""}`;
    if (cle !== cleTour) { cleTour = cle; defautsAttaque(); }
    rendre();
    if (ui.addTab === "pj" && !$e("#fAjout").hidden) rendreAjout();
  }

  function defautsAttaque() {
    const c = cur(); if (!c) return;
    ui.att = c.id; ui.atk = 0; ui.dmAuto = true; ui.tgtAuto = true; ui.actAuto = true;
    const t = st.cbts[c.cible];
    if (t && t.pv > 0) ui.tgt = t.id;
    else { const f = st.ordre.map(i => st.cbts[i]).find(o => foes(c, o) && o.pv > 0); ui.tgt = f ? f.id : null; }
  }

  /* --- budget d'actions, si les règles en ont un : 1 L, ou 1 M + 1 A, ou 2 M ; le serveur fait foi --- */
  const nbM = c => (c.act?.M ? 1 : 0) + (c.act?.M2 ? 1 : 0);
  function empeche(c) {
    if (c.pv <= 0) return c.camp === "pj" ? `${c.nom} est inconscient.` : `${c.nom} est hors de combat.`;
    const b = etatsDe(c).find(e => ETATS[e]?.bloque);
    return b ? `${c.nom} est ${b.toLowerCase()} : aucune action.` : "";
  }
  function possible(c, k) {
    const r = empeche(c); if (r) return r;
    if (!BUDGET) return "";
    const ral = uneAction(c), m = nbM(c), a = c.act || {};
    if (k === "L") return ral ? "Une seule action ce tour : pas d'action limitée." : a.L ? "Action limitée déjà utilisée." : (a.A || m) ? "Une action limitée occupe tout le tour." : "";
    if (k === "A") return a.L ? "Action limitée utilisée : plus d'attaque." : a.A ? "Action d'attaque déjà utilisée." : m >= 2 ? "Deux mouvements faits : plus d'attaque." : (ral && m) ? "Une seule action ce tour." : "";
    if (k === "M") return a.L ? "Action limitée utilisée : plus de mouvement." : m >= 2 ? "Deux mouvements déjà faits." : (m && a.A) ? "Attaque et mouvement faits : tour complet." : (ral && (m || a.A)) ? "Une seule action ce tour." : "";
    return "";
  }
  function resteActions(c) {
    if (empeche(c)) return "aucune action";
    if (!BUDGET) return "libre";
    const ral = uneAction(c), rien = !c.act.A && !c.act.L && nbM(c) === 0;
    if (rien) return ral ? "1 attaque ou 1 mouvement" : "1 action limitée, ou 1 attaque et 1 mouvement, ou 2 mouvements";
    const a = !possible(c, "A"), m = !possible(c, "M");
    return a && m ? "1 attaque ou 1 mouvement" : a ? "1 attaque" : m ? "1 mouvement" : "tour complet";
  }
  // type d'action d'une attaque : raison du refus, ou ""
  function possibleAttaque(A, k) {
    const c = cur();
    if (k === "G") return empeche(A);
    if (!c || c.id !== A.id) return `Ce n'est pas le tour de ${A.nom} : attaque hors tour = action gratuite.`;
    if (!BUDGET) return k === "A" ? empeche(A) : "Sans budget d'actions dans les règles, toute attaque est une simple attaque.";
    if (k === "LS") return empeche(A) || (A.act?.LA ? "" : "Aucune capacité limitée en cours.");
    return possible(A, k);
  }
  function choisirAction(A) {
    if (!A) return;
    const ok = k => !possibleAttaque(A, k);
    if (ui.actAuto || !ok(ui.act)) ui.act = ["A", "LS", "L"].find(ok) || (cur()?.id === A.id ? "A" : "G"), ui.actAuto = true;
  }

  /* --- rendu --- */
  /* --- fin de combat : cadre de récap --- */
  let finVue = null, etaitTermine = null;
  function ouvrirFin(mode) {
    const d = $e("#dlgFin");
    const boutons = mode === "proposition"
      ? `<button type="button" class="primary big" data-a="finTerminer">Terminer le combat</button>
         <button type="button" class="ghost" data-a="finContinuer">Continuer (fuite, renforts, soins…)</button>`
      : `<button type="button" class="ghost" data-a="copierRecap">Copier pour Obsidian</button><button type="button" class="primary" data-a="finFermer">Fermer</button>`;
    $e("#finBody").innerHTML = `${mode === "proposition" ? `<p class="hint">${st.fin === "victoire" ? "Tous les adversaires sont à terre." : "Tous les PJ sont à terre."} Le combat peut s'arrêter ici : le récap sera enregistré.</p>` : ""}
      ${recapHTML(st.recap)}
      <div class="row fin-actions">${boutons}</div>`;
    if (!d.open) d.showModal();
  }
  function gererFin() {
    const d = $e("#dlgFin");
    if (st.fin && !st.termine) {
      const cle = st.fin + ":" + (st.recap?.date || "");
      if (finVue !== cle) { finVue = cle; ouvrirFin("proposition"); }
      else if (d.open) ouvrirFin("proposition");
    } else if (st.termine && st.recap && etaitTermine === false) {
      ouvrirFin("final"); // vient d'être terminé sur cet écran ou un autre
    } else if (d.open && !st.termine) {
      d.close();
    }
    if (!st.fin) finVue = null;
    etaitTermine = st.termine;
  }

  function rendre() {
    if (!st) return;
    $e("#cNom").textContent = st.nom;
    $e("#roundN").textContent = st.round;
    $e("#bUndo").disabled = !annulable;
    for (const id of ["#bPrev", "#bNext", "#bSort", "#bAtk", "#bEtat"]) $e(id).disabled = st.termine;
    $e("#bEnd").textContent = st.termine ? "Rouvrir le combat" : "Terminer le combat";
    $e("#banner").innerHTML = st.termine
      ? `<div class="banner">Combat terminé. Les PV et les points de chance des PJ sont sur leur feuille.${st.recap ? ` <button type="button" class="ghost" data-a="voirRecap">Voir le récap</button>` : ""}</div>`
      : st.fin ? `<div class="banner fin ${st.fin}">${st.fin === "victoire" ? "Tous les adversaires sont à terre." : "Tous les PJ sont à terre."}
          <button type="button" class="primary" data-a="finTerminer">Terminer le combat</button><button type="button" class="ghost" data-a="finContinuer">Continuer</button></div>` : "";
    gererFin();
    rendreOrdre(); rendreActif(); rendreSelects(); rendreVs(); rendreResultat(); rendreTest(); rendreJournal();
  }

  // Une ligne par combattant, de hauteur constante. Le détail (attaques, capacités, états à retirer,
  // déplacement dans l'ordre) est dans sa fenêtre, ouverte en touchant son nom.
  function ligne(id) {
    const c = st.cbts[id], courant = st.ordre[st.tour] === id, down = c.pv <= 0;
    const [, base, numero] = /^(.*?)(\s+\d+)?$/.exec(c.nom);
    const pct = Math.round(c.pv / c.pvMax * 100), cls = pct <= 25 ? "low" : pct <= 50 ? "mid" : "";
    const chips = etatsDe(c).map(e => `<span class="chip" title="${esc(ETATS[e]?.description || "")}">${esc(e)}${typeof c.etats[e] === "number" ? " " + c.etats[e] + "r" : ""}</span>`).join("");
    return `<li class="cbt ${c.camp}${courant ? " current" : ""}${down ? " down" : ""}" data-pv>
      <div class="init num">${c.init}</div>
      <div class="main">
        <div class="name"><button data-a="fiche" data-id="${id}" title="${esc(c.nom)} (${CAMP[c.camp]}) : attaques, capacités, états…"><span class="base">${esc(base)}</span>${numero ? `<span>${esc(numero)}</span>` : ""}</button>${courant ? `<span class="flag">Joue</span>` : ""}${down ? `<span class="tag">${c.camp === "pj" ? "Inconscient" : "Hors combat"}</span>` : ""}${chips}</div>
        <div class="hp"><div class="bar"><i class="${cls}" style="width:${pct}%"></i></div><span class="val">${c.pv} / ${c.pvMax}</span><span class="stats">DEF ${c.defense}${c.rd ? " · RD " + c.rd : ""}${c.pmMax ? ` · <span class="pm">PM ${c.pm}/${c.pmMax}</span>` : ""}</span></div>
      </div>
      <span class="quick"><input type="number" min="1" inputmode="numeric" class="q" placeholder="PV" aria-label="Valeur pour ${esc(c.nom)}"><button class="icon" data-a="dmg" data-id="${id}" title="Infliger des dégâts" aria-label="Dégâts">−</button><button class="icon" data-a="heal" data-id="${id}" title="Soigner" aria-label="Soins">+</button></span>
      <button class="icon ghost" data-a="viser" data-id="${id}" title="Choisir comme cible" aria-label="Cibler ${esc(c.nom)}">🎯</button>
    </li>`;
  }
  // Remplace les lignes en gardant ce qui est en cours de saisie dans une case de PV.
  function poserLignes(ol, liste) {
    const saisie = {}, actif = document.activeElement;
    let focus = null;
    $$("li.cbt", ol).forEach(li => {
      const q = $(".q", li), id = $("[data-a=fiche]", li)?.dataset.id;
      if (q?.value) saisie[id] = q.value;
      if (q && q === actif) focus = id;
    });
    ol.innerHTML = liste.map(ligne).join("") || `<li class="empty">Aucun combattant : ajoutez-en par le menu.</li>`;
    $$("li.cbt", ol).forEach(li => {
      const id = $("[data-a=fiche]", li).dataset.id, q = $(".q", li);
      if (saisie[id]) q.value = saisie[id];
      if (focus === id) q.focus();
    });
  }
  // Combien de lignes tiennent dans la hauteur du panneau.
  function capacite(ol) {
    const h = ol.querySelector("li.cbt")?.offsetHeight || 54, ecart = 6;
    return Math.max(1, Math.floor((ol.clientHeight + ecart) / (h + ecart)));
  }
  function rendreOrdre() {
    const ids = st.ordre, ol = $e("#order"), plus = $e("#bTous");
    const debout = ids.filter(i => st.cbts[i].pv > 0 || st.cbts[i].camp === "pj").length;
    $e("#countInfo").textContent = ids.length ? `${debout} en lice sur ${ids.length}` : "";
    // Tout tient : l'ordre d'initiative tel quel. Sinon : sans les adversaires à terre ; et s'il en
    // reste trop, à partir de celui qui joue. La liste complète est dans sa fenêtre.
    let liste = ids;
    plus.hidden = true;
    if (!ol.querySelector("li.cbt") && ids.length) poserLignes(ol, ids.slice(0, 1)); // une ligne pour la mesurer
    if (ids.length > capacite(ol)) {
      plus.hidden = false;
      const n = capacite(ol), utiles = ids.filter(i => st.cbts[i].pv > 0 || st.cbts[i].camp === "pj");
      liste = utiles.length <= n ? utiles : [...ids.slice(st.tour), ...ids.slice(0, st.tour)].filter(i => utiles.includes(i) || i === ids[st.tour]).slice(0, n);
      plus.textContent = `＋ ${ids.length - liste.length} autre${ids.length - liste.length > 1 ? "s" : ""} · tout voir`;
    }
    poserLignes(ol, liste);
    if (!$e("#fTous").hidden) poserLignes($e("#orderTous"), ids);
    if (ui.fiche) rendreFiche();
  }

  // Fenêtre d'un combattant : tout ce que sa ligne ne montre pas.
  function rendreFiche() {
    const c = st.cbts[ui.fiche];
    if (!c) { fermerFenetres(); return; }
    const id = c.id, t = st.cbts[c.cible];
    const by = st.ordre.map(i => st.cbts[i]).filter(o => o.cible === id && o.pv > 0).map(o => o.nom);
    const chips = etatsDe(c).map(e => `<span class="chip" title="${esc(ETATS[e]?.description || "")}">${esc(e)}${typeof c.etats[e] === "number" ? " " + c.etats[e] + "r" : ""}<button data-a="rmEtat" data-id="${id}" data-e="${esc(e)}" aria-label="Retirer ${esc(e)}">×</button></span>`).join("");
    $e("#fCbt h2").textContent = c.nom;
    patch($e("#ficheCbt"), `<div class="stats">${CAMP[c.camp]} · Init ${c.init} · ${c.pv} / ${c.pvMax} PV · DEF ${c.defense}${c.rd ? " · RD " + c.rd : ""}${c.camp === "pj" ? " · PC " + c.pc : ""}${c.pmMax ? ` · <span class="pm">PM ${c.pm} / ${c.pmMax}</span>` : ""}${CRIT && c.crit >= 2 && c.crit < DE ? ` · critique sur ${c.crit}–${DE}` : ""}</div>
      <div class="quick" data-pv><input type="number" min="1" inputmode="numeric" class="q" id="fichePv" placeholder="PV" aria-label="Valeur pour ${esc(c.nom)}"><button data-a="dmg" data-id="${id}">− Dégâts</button><button data-a="heal" data-id="${id}">+ Soins</button></div>
      ${c.pmMax ? `<div class="quick" data-pm><input type="number" min="1" inputmode="numeric" class="q" id="fichePm" placeholder="PM" aria-label="Points de mana pour ${esc(c.nom)}"><button data-a="pmMoins" data-id="${id}">− PM</button><button data-a="pmPlus" data-id="${id}">+ PM</button></div>` : ""}
      ${chips ? `<div class="chips">${chips}</div>` : ""}
      ${t || by.length ? `<div class="target">${t ? `Dernière cible : <b>${esc(t.nom)}</b>` : ""}${t && by.length ? " · " : ""}${by.length ? `Ciblé par <b>${esc(by.join(", "))}</b>` : ""}</div>` : ""}
      <div class="stats">${(c.attaques || []).map(a => `${esc(a.nom)} (${TYPE[a.type] || "?"}) ${signe(a.bonus)} · DM ${esc(a.dm || "—")}`).join("<br>") || "Aucune attaque"}</div>
      ${c.capacites ? `<div class="capa">${esc(c.capacites)}</div>` : ""}
      <div class="usages">
        <span class="hint">Capacités à usage limité : touchez celle qui vient d'être utilisée dans ce combat.</span>
        <div class="chips">${[...new Set([...(c.voies || []), ...(c.utilise || [])])].map(n => {
          const u = (c.utilise || []).includes(n);
          return `<button type="button" class="ghost${u ? " used" : ""}" data-a="usage" data-id="${id}" data-cap="${esc(n)}" data-u="${u ? 1 : 0}" title="${u ? "Utilisée : toucher pour la rendre" : "Marquer comme utilisée"}">${u ? "✓ " : ""}${esc(n)}</button>`;
        }).join("")}</div>
        <div class="row"><input id="ficheCap" maxlength="80" placeholder="Autre capacité utilisée" aria-label="Autre capacité utilisée"><button type="button" data-a="usageLibre" data-id="${id}">Noter</button></div>
      </div>
      <div class="menu-combat deux">
        <button type="button" data-a="viser" data-id="${id}">🎯 Prendre pour cible</button>
        <button type="button" data-a="tester" data-id="${id}">🎲 Test de caractéristique</button>
        <button type="button" data-a="etatPour" data-id="${id}">Appliquer un état</button>
        <button type="button" data-a="edit" data-id="${id}">✎ Modifier</button>
        <button type="button" data-a="up" data-id="${id}">▲ Monter dans l'ordre</button>
        <button type="button" data-a="down" data-id="${id}">▼ Descendre dans l'ordre</button>
      </div>`);
  }

  function rendreActif() {
    const c = cur(), box = $e("#activeCard");
    if (!c) { box.className = "active-card"; box.innerHTML = `<p class="empty">Ajoutez des combattants pour commencer.</p>`; return; }
    box.className = "active-card " + c.camp;
    const ee = etatsDe(c), warn = [];
    if (c.pv <= 0) warn.push(c.camp === "pj" ? "Inconscient : aucune action." : "Hors de combat.");
    const bloq = ee.filter(e => ETATS[e]?.bloque);
    if (bloq.length) warn.push(`${bloq.join(", ")} : ne peut pas agir ce tour.`);
    ee.filter(e => ETATS[e]?.description && !ETATS[e].bloque && !ETATS[e].action).forEach(e => warn.push(`${e} : ${ETATS[e].description}`));
    const by = st.ordre.map(i => st.cbts[i]).filter(o => o.cible === c.id && o.pv > 0).map(o => o.nom);
    const t = st.cbts[c.cible];
    const L = { A: "Attaque (A)", M: "Mouvement (M)", L: "Limitée (L)" };
    box.innerHTML = `<div class="active-tete"><button type="button" class="active-name" data-a="fiche" data-id="${c.id}" title="Attaques, capacités, états…">${esc(c.nom)}</button>
        <span class="stats">${CAMP[c.camp]} · ${c.pv}/${c.pvMax} PV · DEF ${c.defense}${c.rd ? " · RD " + c.rd : ""}${c.camp === "pj" ? " · PC " + c.pc : ""}${c.pmMax ? ` · <span class="pm">PM ${c.pm}/${c.pmMax}</span>` : ""}</span></div>
      ${warn.length ? `<div class="warnbox">${warn.map(esc).join("<br>")}</div>` : ""}
      <div class="acts">${["A", "M", "L"].map(k => {
        const used = k === "M" ? nbM(c) : c.act[k] ? 1 : 0, r = possible(c, k);
        const lbl = k === "M" ? `<span class="long">Mouvement</span><span class="court">Mouv.</span> (M)${used ? ` ${used}/2` : ""}` : L[k];
        return `<button type="button" class="act${used ? " used" : ""}" data-act="${k}"${r || st.termine ? " disabled" : ""} title="${esc(r || "Marquer cette action comme utilisée")}">${used && k !== "M" ? "✓ " : ""}${lbl}</button>`;
      }).join("")}
        ${POSTURES.map(p => `<button class="act" data-a="defense" data-e="${esc(p.nom)}"${st.termine || possible(c, p.action) || ee.includes(p.nom) ? " disabled" : ""} title="${esc(possible(c, p.action) || p.description || "")}">${esc(p.nom)}${p.def ? ` ${signe(p.def)}` : ""} (${p.action})</button>`).join("")}
      </div>
      <p class="hint ligne2 reste" title="Une action utilisée ne se rend pas : « Annuler » corrige une erreur.">Reste : <b>${esc(resteActions(c))}</b>${t ? ` · dernière cible : <b>${esc(t.nom)}</b>` : ""}${by.length ? ` · ciblé par <b>${esc(by.join(", "))}</b>` : ""}</p>`;
  }

  const option = (c, sel) => `<option value="${c.id}"${c.id === sel ? " selected" : ""}>${esc(c.nom)}${c.pv <= 0 ? " (à terre)" : ""} — ${CAMP[c.camp]}</option>`;

  function rendreSelects() {
    const tous = st.ordre.map(i => st.cbts[i]);
    if (!st.cbts[ui.att]) ui.att = cur()?.id ?? null;
    const A = st.cbts[ui.att];
    $e("#aAtt").innerHTML = tous.map(c => option(c, ui.att)).join("");
    const cand = A ? tous.filter(o => o.id !== A.id).sort((x, y) => foes(A, y) - foes(A, x)) : tous;
    // cible choisie automatiquement : on préfère toujours un adversaire debout s'il en existe un
    const T = st.cbts[ui.tgt];
    const adversaire = A && cand.find(o => foes(A, o) && o.pv > 0);
    if (!T || ui.tgt === ui.att || (ui.tgtAuto && adversaire && !foes(A, T))) { ui.tgt = adversaire ? adversaire.id : cand[0]?.id ?? null; ui.tgtAuto = true; }
    $e("#aTgt").innerHTML = cand.map(c => option(c, ui.tgt)).join("");
    const atks = A?.attaques || [];
    if (ui.atk >= atks.length) ui.atk = atks.length ? 0 : -1;
    $e("#aAtk").innerHTML = atks.map((a, i) => `<option value="${i}"${i === ui.atk ? " selected" : ""}>${esc(a.nom)} (${TYPE[a.type] || "?"} ${signe(a.bonus)})</option>`).join("")
      + `<option value="-1"${ui.atk === -1 ? " selected" : ""}>Attaque libre (bonus via « divers »)</option>`;
    if (ui.dmAuto) { $e("#aDm").value = atks[ui.atk]?.dm || ""; ui.dmAuto = false; }
    if (A) {
      choisirAction(A);
      [...$e("#aAct").options].forEach(o => { const r = possibleAttaque(A, o.value); o.disabled = !!r; o.title = r; });
      $e("#aAct").value = ui.act;
      const r = possibleAttaque(A, ui.act);
      $e("#actInfo").textContent = r || (!BUDGET ? "" : { A: "Consomme l'action d'attaque du tour.", L: "Consomme tout le tour (capacité limitée).", LS: "Attaque supplémentaire de la capacité limitée déjà lancée ce tour.", G: "Ne consomme rien : à réserver aux attaques gratuites prévues par une capacité ou une règle (riposte…)." }[ui.act]);
      $e("#bAtk").disabled = !!r || st.termine;
    }
    const eWho = $e("#eWho"), ew = st.cbts[eWho.value] ? eWho.value : ui.tgt;
    eWho.innerHTML = tous.map(c => option(c, ew)).join("");
    const reel = !virtuel(A);
    $e("#natWrap").hidden = !reel;
    $e("#pmWrap").hidden = !A?.pmMax;
    $e("#pmWrap").title = A?.pmMax ? `${A.nom} : ${A.pm} PM sur ${A.pmMax}` : "";
    $e("#modeInfo").textContent = reel ? "🎲 dés réels" : "dés virtuels";
    $e("#bAtk").textContent = reel ? "Résoudre l'attaque" : "Lancer l'attaque";
    $e("#eDesc").textContent = descEtat($e("#eWhat").value);
  }
  const descEtat = nom => !nom.trim() ? "" : ETATS[nom.trim()]?.description || "État libre : il n'a pas d'effet, c'est une note sur le combattant.";

  function rendreVs() {
    const a = st?.cbts[ui.att], t = st?.cbts[ui.tgt];
    if (!a || !t) { $e("#vs").innerHTML = ""; $e("#mods").textContent = ""; return; }
    const atk = a.attaques?.[ui.atk];
    const dist = atk?.type === "D";
    const m = [];
    let malusA = 0, malusT = 0, de = DE, autoCrit = false;
    etatsDe(a).forEach(e => {
      const E = ETATS[e] || {};
      const v = dist && E.att_distance ? E.att_distance : (E.att || 0);
      if (v) { malusA += v; m.push(`${a.nom} ${e.toLowerCase()} : ${v} en attaque${dist && E.att_distance ? " à distance" : ""}`); }
      if (E.de && E.de < de) { de = E.de; m.push(`${a.nom} ${e.toLowerCase()} : d${E.de} au lieu du d${DE}`); }
    });
    etatsDe(t).forEach(e => {
      const E = ETATS[e] || {};
      if (E.def) { malusT += E.def; m.push(`${t.nom} ${e.toLowerCase()} : ${E.def > 0 ? "+" : ""}${E.def} en DEF`); }
      if (E.critique_automatique) { autoCrit = true; m.push(`${t.nom} ${e.toLowerCase()} : touché automatiquement${CRIT ? ", critique" : ""}`); }
    });
    const d12 = de !== DE;
    const tot = (atk?.bonus || 0) + malusA + (+$e("#aDiv").value || 0);
    $e("#aNat").max = de; $e("#aNat").placeholder = "d" + de;
    $e("#vs").innerHTML = `<div><span class="who">${esc(a.nom)}</span> <span class="num hint">d${de} ${signe(tot)}</span></div>
      <div class="arrow">→</div>
      <div><span class="who">${esc(t.nom)}</span> <span class="num hint">DEF ${t.defense + malusT}${t.rd ? " · RD " + t.rd : ""} · ${t.pv} PV</span></div>`;
    if (CRIT && a.crit >= 2 && a.crit < DE && !d12 && !autoCrit) m.push(`Critique sur ${a.crit}–${DE}`);
    if (CHANCE && a.camp === "pj" && a.pc > 0) m.push(`${a.pc} point${a.pc > 1 ? "s" : ""} de chance : à dépenser après le jet si besoin`);
    $e("#mods").textContent = m.join(" · ");
  }

  // Le résultat prend la place du formulaire : ni fenêtre, ni hauteur en plus. En dés réels, on y
  // saisit le total des dés de dégâts ; critique et RD sont appliqués par le moteur.
  function rendreResultat() {
    const r = st.resultat, box = $e("#result");
    const montre = !!r && ui.resFerme !== r.evt;
    $e("#atkForm").hidden = montre;
    if (!montre) { box.innerHTML = ""; return; }
    const t = st.cbts[r.cible], a = st.cbts[r.att];
    const titre = { hit: "Touché", crit: "Coup critique !", miss: ECHEC && r.nat === ECHEC ? "Échec critique" : "Raté" }[r.verdict];
    const attente = r.verdict !== "miss" && t && r.applique === null;
    let h = `<div class="result ${r.verdict}"><div class="verdict">${titre}</div>
      <div class="calc">${esc(a?.nom || "?")} → ${esc(t?.nom || "?")} · ${esc(r.attaque)}</div><div class="calc">${esc(r.calc)}</div>`;
    if (r.verdict !== "miss" && t) {
      if (r.dm_reel && attente) {
        h += `<div class="calc">Dégâts à lancer : ${esc(r.dm_expr || "pas de formule")}${r.verdict === "crit" && CRIT > 1 ? ` · critique : total × ${CRIT}` : ""}${r.rd ? ` · RD ${r.rd}` : ""}</div>
          <div class="dmrow des"><label class="s">Total des dés<input type="number" min="0" id="rDes" data-keep="no" inputmode="numeric" aria-label="Total des dés de dégâts, bonus compris"></label>
            <button class="primary big" data-a="applyDes" title="Appliquer à ${esc(t.nom)}">Appliquer</button><button class="ghost" data-a="skipDm">Ignorer</button>
            <span class="num apercu" id="rApercu"></span></div>`;
      } else {
        if (r.sans_dm) h += `<p class="hint">Pas de formule de dégâts : saisissez les DM à appliquer.</p>`;
        else if (r.brut !== null) h += `<div class="calc">DM ${esc(r.dm_detail)} = ${r.brut}${r.rd ? ` − RD ${r.rd} = ${r.final}` : ""}</div>`;
        if (r.applique !== null) h += `<p class="hint">${r.applique} DM appliqués à ${esc(t.nom)}.</p>`;
        else h += `<div class="dmrow"><input type="number" min="0" id="rDm" data-keep="no" value="${r.final ?? ""}" aria-label="Dégâts à appliquer"><button class="primary big" data-a="applyDm">Appliquer à ${esc(t.nom)}</button><button class="ghost" data-a="skipDm">Ignorer</button></div>`;
      }
    }
    if (CHANCE && r.verdict === "miss" && r.nat !== ECHEC && !r.chance && a?.camp === "pj" && a.pc > 0 && !st.termine) {
      const manque = r.defense - r.total;
      h += `<div class="dmrow"><button data-a="chance">Dépenser un point de chance (+${CHANCE})</button><span class="hint">${manque <= CHANCE ? `Il manque ${manque} : le point de chance fait toucher.` : `Il manque ${manque} : même avec +${CHANCE}, l'attaque échoue.`} Reste ${a.pc} PC.</span></div>`;
    }
    if (!attente) h += `<div><button type="button" class="${r.verdict === "miss" ? "" : "primary"}" data-a="nouvelleAttaque">Nouvelle attaque</button></div>`;
    const avant = box.innerHTML;
    patch(box, h + `</div>`);
    const des = $e("#rDes");
    if (des) {
      const apercu = () => {
        const v = des.value === "" ? null : Math.max(0, +des.value || 0);
        const mult = r.verdict === "crit" && CRIT > 1 ? CRIT : 1;
        const brut = v === null ? null : v * mult;
        $e("#rApercu").textContent = brut === null ? "" : `${mult > 1 ? `${v} × ${mult} = ${brut}` : brut}${r.rd ? ` − RD ${r.rd}` : ""} → ${Math.max(0, brut - r.rd)} DM`;
      };
      des.oninput = apercu;
      des.onkeydown = e => { if (e.key === "Enter") { e.preventDefault(); appliquerDes(); } };
      apercu();
      if (!avant.includes('id="rDes"')) des.focus(); // le résultat vient d'arriver : prêt à saisir
    }
  }
  function appliquerDes() {
    const des = $e("#rDes");
    if (!des || des.value === "" || +des.value < 0) { des?.focus(); toast("Saisissez le total des dés de dégâts lancés à la table."); return; }
    send({ type: "appliquer_dm", cle: "des", valeur: Math.floor(+des.value) });
  }

  function rendreTest() {
    const tous = st.ordre.map(i => st.cbts[i]);
    const w = $e("#tWho"); let who = st.cbts[ui.tWho] ? ui.tWho : (st.cbts[ui.tgt] ? ui.tgt : cur()?.id);
    w.innerHTML = tous.map(c => option(c, who)).join("");
    const o = $e("#tOppWho"); let opp = st.cbts[ui.tOpp] && ui.tOpp !== who ? ui.tOpp : (tous.find(c => c.id !== who && st.cbts[who] && foes(st.cbts[who], c))?.id || tous.find(c => c.id !== who)?.id);
    o.innerHTML = tous.filter(c => c.id !== who).map(c => option(c, opp)).join("");
    const opposition = ui.tMode === "opp";
    $e("#tDiff").setAttribute("aria-pressed", !opposition); $e("#tOpp").setAttribute("aria-pressed", opposition);
    $e("#tDiffWrap").hidden = opposition; $e("#tOppWrap").hidden = !opposition;
    const C = st.cbts[who], O = st.cbts[opp];
    $e("#tNatWrap").hidden = virtuel(C); $e("#tNatOppWrap").hidden = !opposition || virtuel(O);
    if (C) {
      const car = $e("#tCar").value, m = C.carac?.[car] ?? 0, s = (C.superieures || []).includes(car);
      const reduit = Math.min(DE, ...etatsDe(C).map(e => ETATS[e]?.de || DE));
      $e("#tNat").max = reduit; $e("#tNat").placeholder = "d" + reduit;
      $e("#tInfo").textContent = `${C.nom} : ${car} ${signe(m)}${s ? " (supérieure : deux dés, meilleur gardé)" : ""}${reduit !== DE ? ` · d${reduit} à cause de son état` : ""}.${opposition ? " Égalité : le défenseur l'emporte." : ""}${virtuel(C) ? "" : " Dés réels : saisissez le dé conservé."}`;
    }
    $e("#bTest").disabled = st.termine;
    const r = st.test, box = $e("#tRes");
    if (!r) { box.innerHTML = ""; return; }
    const j = r.jet, cj = st.cbts[j.id];
    const calc = x => `d${x.de} ${x.nat}${x.des.length > 1 ? ` [${x.des.join(", ")}]` : ""} ${signe(x.mod)} ${x.carac}${x.divers ? " " + signe(x.divers) : ""}${x.chance ? ` + ${CHANCE} PC` : ""} = ${x.total}`;
    let h = `<div class="result ${r.reussi ? "hit" : "miss"}"><div class="verdict">${r.reussi ? "Test réussi" : "Test raté"}${j.critique === "reussite" ? " (critique)" : j.critique === "echec" ? " (échec critique)" : ""}</div>
      <div class="calc">${esc(j.nom)} · ${esc(calc(j))}</div>
      ${r.opposant ? `<div class="calc">${esc(r.opposant.nom)} · ${esc(calc(r.opposant))}</div>` : ""}
      <div class="hint">${esc(r.explication)}${r.etat_applique ? ` ${esc(j.nom)} est ${esc(r.etat_echec.toLowerCase())}${r.duree ? ` (${r.duree} round${r.duree > 1 ? "s" : ""})` : ""}.` : ""}</div>`;
    if (CHANCE && !r.reussi && !j.chance && j.critique !== "echec" && cj?.camp === "pj" && cj.pc > 0 && !st.termine)
      h += `<div class="dmrow"><button data-a="chanceTest">Dépenser un point de chance (+${CHANCE})</button><span class="hint">Reste ${cj.pc} PC.</span></div>`;
    h += `<div><button class="ghost icon" data-a="fermerTest">Fermer</button></div>`;
    patch(box, h + `</div>`);
  }

  // Journal complet dans sa fenêtre ; à côté, sur un grand écran, les dernières lignes seulement.
  function rendreJournal() {
    const L = st.log;
    if (!L.length) { $e("#log").innerHTML = $e("#logApercu").innerHTML = `<p class="empty">Les attaques, dégâts et états apparaîtront ici.</p>`; return; }
    let h = "", r = null;
    for (let i = L.length - 1; i >= 0; i--) {
      const e = L[i];
      if (e.r !== r) { h += `<div class="logr">Round ${e.r}</div>`; r = e.r; }
      h += `<div class="loge ${e.k}">${md(e.t)}</div>`;
    }
    $e("#log").innerHTML = h;
    $e("#logApercu").innerHTML = L.slice(-14).reverse().map(e => `<div class="loge ${e.k}">${md(e.t)}</div>`).join("");
  }

  /* --- panneau d'ajout --- */
  function rendreAjout() {
    $$("#addSeg button", el).forEach(b => b.setAttribute("aria-pressed", b.dataset.t === ui.addTab));
    const body = $e("#addBody");
    if (ui.addTab === "monstre") {
      body.innerHTML = `<div class="row"><input type="search" id="adQ" placeholder="Rechercher dans le bestiaire…" aria-label="Rechercher un monstre" style="flex:1">
          <label class="s">Qté<input type="number" id="adN" min="1" max="30" value="1"></label>
          <label class="s">Camp<select id="adC"><option value="ennemi">Ennemi</option><option value="allie">Allié</option></select></label></div>
        <div class="picker" id="adList"></div>`;
      const liste = () => {
        const q = $e("#adQ").value.trim().toLowerCase();
        const l = monstres.filter(m => !q || [m.nom, m.categorie, ...(m.tags || [])].join(" ").toLowerCase().includes(q)).sort((a, b) => a.nc - b.nc || a.nom.localeCompare(b.nom, "fr"));
        if (!monstres.length) { $e("#adList").innerHTML = `<p class="empty">Le bestiaire de cet univers est vide.</p>`; return; }
        $e("#adList").innerHTML = l.slice(0, 50).map(m => `<div class="pick"><span>${esc(m.nom)} <span class="hint num">NC ${m.nc} · PV ${m.pv} · Init ${m.init}</span></span><button class="icon" data-addm="${m.id}">Ajouter</button></div>`).join("") || `<p class="empty">Aucun monstre trouvé.</p>`;
      };
      $e("#adQ").oninput = liste; liste();
    } else if (ui.addTab === "pj") {
      const presents = k => new Set(Object.values(st?.cbts || {}).filter(c => c.ref?.kind === k).map(c => c.ref.id));
      const ici = presents("pj"), la = presents("pnj");
      const l = pjs.filter(p => !ici.has(p.id)), n = pnjs.filter(p => !la.has(p.id));
      const q0 = $e("#adPnj")?.value || "";
      body.innerHTML = (l.length ? `<div class="picker">${l.map(p => `<div class="pick"><span>${esc(p.nom)} <span class="hint">PJ${p.carte?.statut && p.carte.statut !== "actif" ? " · " + esc(p.carte.statut) : ""}</span></span><button class="icon" data-addp="${p.id}">Ajouter</button></div>`).join("")}</div>`
        : `<p class="empty">Tous les PJ sont déjà dans le combat.</p>`)
        + `<p class="hint">Un PJ entre avec sa feuille de personnage. Un PNJ du monde entre avec son profil de combat et garde son nom.</p>
        <div class="row"><input type="search" id="adPnj" placeholder="Chercher un PNJ…" aria-label="Chercher un PNJ" style="flex:1">
          <label class="s">Camp<select id="adPnjC"><option value="ennemi">Ennemi</option><option value="allie">Allié</option></select></label></div>
        <div class="picker" id="adPnjList"></div>`;
      const listePnj = () => {
        const q = $e("#adPnj").value.trim().toLowerCase();
        $e("#adPnjList").innerHTML = !q ? "" : n.filter(p => p.nom.toLowerCase().includes(q)).slice(0, 20).map(p => `<div class="pick"><span>${esc(p.nom)} <span class="hint">PNJ</span></span><button class="icon" data-addn="${p.id}">Ajouter</button></div>`).join("") || `<p class="empty">Aucun PNJ de ce nom.</p>`;
      };
      $e("#adPnj").value = q0; $e("#adPnj").oninput = listePnj; listePnj();
    } else {
      body.innerHTML = `<div class="formgrid">
          <label class="w2">Nom<input id="lNom" placeholder="Garde de la ville"></label>
          <label>Camp<select id="lCamp"><option value="ennemi">Ennemi</option><option value="allie">Allié</option><option value="pj">PJ</option></select></label>
          <label>Qté<input id="lQte" type="number" min="1" max="30" value="1"></label>
          <label>Init<input id="lInit" type="number" value="10"></label>
          <label>DEF<input id="lDef" type="number" value="12"></label>
          <label>PV<input id="lPv" type="number" min="1" value="8"></label>
          <label>Bonus att.<input id="lAtt" type="number" value="2"></label>
          <label class="w2">Dégâts<input id="lDm" value="1d6"></label>
        </div><div><button class="primary" id="lGo">Ajouter</button></div>`;
      $e("#lGo").onclick = () => {
        const nom = $e("#lNom").value.trim();
        if (!nom) { toast("Donnez un nom au combattant.", true); return; }
        send({ type: "ajouter", camp: $e("#lCamp").value, qte: +$e("#lQte").value || 1, fiche: {
          nom, init: +$e("#lInit").value || 0, defense: +$e("#lDef").value || 0, pv: Math.max(1, +$e("#lPv").value || 1),
          attaques: [{ nom: "Attaque", type: "C", bonus: +$e("#lAtt").value || 0, dm: $e("#lDm").value.trim() }] } });
        $e("#lNom").value = "";
      };
    }
  }

  /* --- édition d'un combattant --- */
  function editer(id) {
    const c = st.cbts[id]; if (!c) return;
    const f = $e("#dlgForm");
    f.innerHTML = `<h2>Modifier ${esc(c.nom)}</h2>
      <div class="formgrid">
        <label class="w2">Nom<input name="nom" value="${esc(c.nom)}"></label>
        <label>Camp<select name="camp">${Object.entries(CAMP).map(([k, v]) => `<option value="${k}"${c.camp === k ? " selected" : ""}>${v}</option>`).join("")}</select></label>
        <label>Init<input name="init" type="number" value="${c.init}"></label>
        <label>DEF<input name="defense" type="number" value="${c.defense}"></label>
        <label>PV<input name="pv" type="number" min="0" value="${c.pv}"></label>
        <label>PV max<input name="pvMax" type="number" min="1" value="${c.pvMax}"></label>
        <label>RD<input name="rd" type="number" min="0" value="${c.rd}"></label>
        <label>Critique sur<input name="crit" type="number" min="0" max="${DE}" value="${c.crit}"></label>
        <label>Pts chance<input name="pc" type="number" min="0" value="${c.pc}"></label>
        <label>PM<input name="pm" type="number" min="0" value="${c.pm || 0}"></label>
        <label>PM max<input name="pmMax" type="number" min="0" value="${c.pmMax || 0}"></label>
      </div>
      <label>Capacités<textarea name="capacites" rows="3">${esc(c.capacites)}</textarea></label>
      <p class="hint">Les modifications ne concernent que ce combat, pas la fiche du monstre ni la feuille du personnage (sauf les PV, les points de chance et les points de mana d'un PJ, reportés sur sa feuille).</p>
      <div class="actions-bar"><button value="ok" class="primary">Enregistrer</button><button value="annuler" formnovalidate>Fermer</button><span class="sp"></span><button type="button" class="danger" id="dDel">Retirer du combat</button></div>`;
    armer($("#dDel", f), "Confirmer le retrait", () => { send({ type: "retirer", id }); $e("#dlg").close(); });
    const dlg = $e("#dlg");
    dlg.onclose = () => {
      if (dlg.returnValue !== "ok") return;
      const g = n => f.elements[n].value;
      send({ type: "modifier", id, champs: { nom: g("nom").trim() || c.nom, camp: g("camp"), init: +g("init"), defense: +g("defense"),
        pv: +g("pv"), pvMax: +g("pvMax"), rd: +g("rd"), crit: +g("crit") || 0, pc: +g("pc"), pmMax: +g("pmMax") || 0, pm: +g("pm") || 0, capacites: g("capacites") } });
    };
    dlg.returnValue = "";
    dlg.showModal();
  }

  function journalMarkdown() {
    let out = `## ${st.nom}\n`, r = null;
    for (const e of st.log) {
      if (e.r !== r) { out += `\n### Round ${e.r}\n`; r = e.r; }
      out += e.k === "turn" ? `*${e.t.replace(/\*\*/g, "")}*\n` : `- ${e.t}\n`;
    }
    return out;
  }

  /* --- événements --- */
  el.addEventListener("click", e => {
    const b = e.target.closest("[data-a]"); if (!b || !st) return;
    const id = b.dataset.id, a = b.dataset.a;
    if (a === "dmg" || a === "heal") {
      const q = b.closest("[data-pv]").querySelector(".q"), v = +q.value;
      if (!v || v < 0) { q.focus(); toast("Tapez d'abord le nombre de PV dans la case, puis − ou +."); return; }
      send({ type: "pv", id, valeur: v, sens: a === "dmg" ? "degats" : "soins" });
      q.value = "";
    } else if (a === "pmMoins" || a === "pmPlus") {
      const q = b.closest("[data-pm]").querySelector(".q"), v = +q.value;
      if (!v || v < 0) { q.focus(); toast("Tapez d'abord le nombre de PM dans la case, puis − ou +."); return; }
      send({ type: "pm", id, valeur: v, sens: a === "pmMoins" ? "depense" : "gain" });
      q.value = "";
    } else if (a === "usage") send({ type: "usage", id, cle: b.dataset.cap, valeur: b.dataset.u !== "1" });
    else if (a === "usageLibre") {
      const q = $e("#ficheCap");
      if (!q.value.trim()) { q.focus(); toast("Nommez la capacité utilisée."); return; }
      send({ type: "usage", id, cle: q.value.trim(), valeur: true });
      q.value = "";
    } else if (a === "up" || a === "down") send({ type: "deplacer", id, sens: a === "up" ? "haut" : "bas" });
    else if (a === "edit") { fermerFenetres(); editer(id); }
    else if (a === "fiche") { ouvrir("fCbt"); ui.fiche = id; rendreFiche(); }
    else if (a === "etatPour") { ouvrir("fEtat"); $e("#eWho").value = id; }
    else if (a === "viser") { fermerFenetres(); ui.tgt = id; ui.tgtAuto = false; ui.resFerme = st.resultat?.evt || 0; if (ui.att === id) ui.att = cur()?.id; rendreSelects(); rendreVs(); rendreResultat(); setPane("action"); toast(`Cible : ${st.cbts[id].nom}`); }
    else if (a === "rmEtat") send({ type: "retirer_etat", id, etat: b.dataset.e });
    else if (a === "applyDm") send({ type: "appliquer_dm", valeur: +$e("#rDm").value || 0 });
    else if (a === "applyDes") appliquerDes();
    else if (a === "nouvelleAttaque") { ui.resFerme = st.resultat?.evt || 0; rendreResultat(); (virtuel(st.cbts[ui.att]) ? $e("#bAtk") : $e("#aNat")).focus(); }
    else if (a === "skipDm") send({ type: "ignorer_dm" });
    else if (a === "chance") send({ type: "chance" });
    else if (a === "chanceTest") send({ type: "chance", cle: "test" });
    else if (a === "fermerTest") send({ type: "fermer_test" });
    else if (a === "tester") { ui.tWho = id; ouvrir("fTest"); rendreTest(); }
    else if (a === "defense") send({ type: "defense", etat: b.dataset.e });
    else if (a === "finTerminer") { $e("#dlgFin").close(); send({ type: "terminer" }); }
    else if (a === "finContinuer") { $e("#dlgFin").close(); send({ type: "continuer" }); }
    else if (a === "finFermer") $e("#dlgFin").close();
    else if (a === "voirRecap") ouvrirFin("final");
    else if (a === "copierRecap") copier(recapMarkdown(st.recap)).then(ok => toast(ok ? "Récap copié en Markdown : collez-le dans une note Obsidian." : "Copie impossible depuis ce navigateur."));
  });
  el.addEventListener("click", e => {
    const b = e.target.closest("button[data-act]");
    if (b && !b.disabled) send({ type: "action_utilisee", cle: b.dataset.act, valeur: true });
  });
  // fenêtres secondaires : un bouton les ouvre, ✕, Échap ou un clic à côté les ferment
  el.addEventListener("click", e => {
    const o = e.target.closest("[data-ouvre]");
    if (o) { ouvrir(o.dataset.ouvre); return; }
    if (e.target.closest("[data-fermer]") || e.target.classList.contains("fen")) fermerFenetres();
  });
  $$("#optDes input", el).forEach(i => { i.checked = i.value === ui.des; });
  $e("#optDes").onchange = e => {
    ui.des = e.target.value;
    try { localStorage.setItem(CLE_DES, ui.des); } catch { /* stockage indisponible */ }
    if (st) { rendreSelects(); rendreTest(); }
  };
  $e("#addBody").addEventListener("click", e => {
    const m = e.target.closest("[data-addm]"), p = e.target.closest("[data-addp]"), n = e.target.closest("[data-addn]");
    if (m) send({ type: "ajouter", kind: "monstre", ref_id: +m.dataset.addm, qte: +$e("#adN").value || 1, camp: $e("#adC").value });
    if (p) send({ type: "ajouter", kind: "pj", ref_id: +p.dataset.addp, camp: "pj" });
    if (n) send({ type: "ajouter", kind: "pnj", ref_id: +n.dataset.addn, camp: $e("#adPnjC").value });
  });
  $e("#addSeg").onclick = e => { const b = e.target.closest("button"); if (b) { ui.addTab = b.dataset.t; rendreAjout(); } };

  $e("#aAtt").onchange = e => {
    ui.att = e.target.value; ui.atk = 0; ui.dmAuto = true; ui.actAuto = true;
    const A = st.cbts[ui.att], t = A && st.cbts[A.cible];
    ui.tgt = t && t.pv > 0 && t.id !== A.id ? t.id : null; ui.tgtAuto = !ui.tgt;
    rendreSelects(); rendreVs();
  };
  $e("#aTgt").onchange = e => { ui.tgt = e.target.value; ui.tgtAuto = false; rendreVs(); };
  $e("#aAct").onchange = e => { ui.act = e.target.value; ui.actAuto = false; rendreSelects(); };
  $e("#aAtk").onchange = e => { ui.atk = +e.target.value; ui.dmAuto = true; rendreSelects(); rendreVs(); };
  $e("#aDiv").oninput = rendreVs;
  const attaquer = () => {
    const reel = !virtuel(st?.cbts[ui.att]), nat = $e("#aNat").value;
    if (reel && nat === "") { $e("#aNat").focus(); toast("Saisissez d'abord le dé lancé à la table."); return; }
    ui.resFerme = 0;
    send({ type: "attaque", att: ui.att, cible: ui.tgt, attaque: ui.atk, mode: reel ? "reel" : "auto", nat: reel ? +nat : null,
      divers: +$e("#aDiv").value || 0, dm: $e("#aDm").value.trim(), act: ui.act, cout_pm: st.cbts[ui.att]?.pmMax ? +$e("#aPm").value || 0 : 0 });
    $e("#aNat").value = ""; $e("#aPm").value = "0";
  };
  $e("#bAtk").onclick = attaquer;
  $e("#aNat").addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); attaquer(); } });
  $e("#eWhat").oninput = () => { $e("#eDesc").textContent = descEtat($e("#eWhat").value); };
  $e("#bEtat").onclick = () => {
    if (!$e("#eWhat").value.trim()) { $e("#eWhat").focus(); toast("Choisissez ou saisissez un état."); return; }
    send({ type: "etat", id: $e("#eWho").value, etat: $e("#eWhat").value.trim(), duree: +$e("#eDur").value || 0 });
    $e("#eDur").value = ""; $e("#eWhat").value = ""; $e("#eDesc").textContent = "";
  };

  $e("#tDiff").onclick = () => { ui.tMode = "diff"; rendreTest(); };
  $e("#tOpp").onclick = () => { ui.tMode = "opp"; rendreTest(); };
  $e("#tWho").onchange = e => { ui.tWho = e.target.value; rendreTest(); };
  $e("#tOppWho").onchange = e => { ui.tOpp = e.target.value; rendreTest(); };
  $e("#tCar").onchange = () => { if (!ui.tOppCarManuel) $e("#tOppCar").value = $e("#tCar").value; rendreTest(); };
  $e("#tOppCar").onchange = () => { ui.tOppCarManuel = true; };
  $e("#bTest").onclick = () => {
    const opp = ui.tMode === "opp", n = $e("#tNat").value, no = $e("#tNatOpp").value;
    // dés réels : sans le dé saisi, le moteur lancerait un dé virtuel
    const reel = !virtuel(st.cbts[$e("#tWho").value]), reelOpp = opp && !virtuel(st.cbts[$e("#tOppWho").value]);
    if (reel && n === "") { $e("#tNat").focus(); toast("Saisissez le dé lancé à la table."); return; }
    if (reelOpp && no === "") { $e("#tNatOpp").focus(); toast("Saisissez le dé de l'opposant."); return; }
    send({ type: "test", id: $e("#tWho").value, carac: $e("#tCar").value, difficulte: +$e("#tDiffV").value || 0,
      opposant: opp ? $e("#tOppWho").value : "", carac_opp: $e("#tOppCar").value,
      nat: reel ? +n : null, nat_opp: reelOpp ? +no : null,
      etat_echec: $e("#tEtat").value.trim(), duree: +$e("#tDuree").value || 0 });
    $e("#tNat").value = ""; $e("#tNatOpp").value = "";
  };
  $e("#bNext").onclick = () => send({ type: "suivant" });
  $e("#bPrev").onclick = () => send({ type: "precedent" });
  $e("#bSort").onclick = () => send({ type: "trier" });
  $e("#bUndo").onclick = () => send({ type: "annuler" });
  $e("#bEnd").onclick = () => send({ type: st?.termine ? "rouvrir" : "terminer" });
  $e("#bCopy").onclick = async () => { if (st) toast(await copier(journalMarkdown()) ? "Journal copié en Markdown." : "Copie impossible depuis ce navigateur.", false); };
  armer($e("#bClear"), "Confirmer", () => send({ type: "vider_journal" }));

  function setPane(p) {
    ui.pane = p;
    $e("#grid").dataset.pane = p;
    $$("#paneSeg button", el).forEach(b => b.setAttribute("aria-pressed", b.dataset.pane === p));
  }
  $e("#paneSeg").onclick = e => { const b = e.target.closest("button"); if (b) setPane(b.dataset.pane); };

  const clavier = e => {
    const ouverte = $$(".fen", el).some(f => !f.hidden);
    if (e.key === "Escape" && ouverte) { fermerFenetres(); return; }
    if (e.target.closest("input,select,textarea") || e.ctrlKey || e.metaKey || e.altKey || $e("#dlg").open || ouverte) return;
    if (e.key === "n" || e.key === "N") { e.preventDefault(); send({ type: "suivant" }); }
    else if (e.key === "z" || e.key === "Z") { e.preventDefault(); send({ type: "annuler" }); }
  };
  document.addEventListener("keydown", clavier);
  // la hauteur disponible change (fenêtre, onglet, rotation de la tablette) : on recompte les lignes
  let trame = 0;
  const mesure = new ResizeObserver(() => { cancelAnimationFrame(trame); trame = requestAnimationFrame(() => { if (st && !ferme) rendreOrdre(); }); });
  mesure.observe($e("#order"));

  try { [monstres, pjs, pnjs] = await Promise.all([api("GET", "/api/bestiaire"), api("GET", "/api/entites?type=pj"), api("GET", "/api/entites?type=pnj")]); }
  catch (e) { toast(e.message, true); }
  connecter();

  return () => {
    ferme = true; clearInterval(timerPing); clearTimeout(timerReco);
    document.removeEventListener("visibilitychange", reveil); window.removeEventListener("online", reconnecterVite); window.removeEventListener("focus", reveil);
    document.removeEventListener("keydown", clavier); try { ws?.close(); } catch { /* rien */ }
    mesure.disconnect(); cancelAnimationFrame(trame); };
}
