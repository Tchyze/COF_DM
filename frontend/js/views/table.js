// Écran du MJ : la partie en une page, sans défilement. Joueurs et scène à gauche, quête et séance
// au centre, le combat à droite (l'écran de combat, logé dans le volet). Une fiche touchée s'ouvre dans un
// tiroir, sans quitter l'écran.
import { $, $$, esc, api, toast, armer } from "../ui.js";
import { typeDe, champDe, vignette, lienFiche, badgeType, valeurCourte, actif as fonctionActive } from "../schema.js";
import { rendu, inline, resolveur } from "../md.js";
import { ecran as ecranCombat, lancerCombat } from "./combat.js";
import { lireMonstre, blocMonstre } from "../monstres.js";
import { COURTS, RESUMES } from "./fiche.js";
import { aideEditionJoueurs } from "./comptes.js";
import { monterNiveau } from "../niveau.js";
import { textePieces } from "../composants.js";

const PREFS = "cof-table";
const SONDE = 5000; // relecture de ce qui bouge (PV des PJ, combat en cours)

function lirePrefs() {
  try { return { chrono: false, debut: null, cumul: 0, rappels: [], ...JSON.parse(localStorage.getItem(PREFS) || "{}") }; }
  catch { return { chrono: false, debut: null, cumul: 0, rappels: [] }; }
}

/** Durée h:mm:ss. */
function duree(ms) {
  const s = Math.max(0, Math.floor(ms / 1000));
  const deux = n => String(n).padStart(2, "0");
  return `${Math.floor(s / 3600)}:${deux(Math.floor(s / 60) % 60)}:${deux(s % 60)}`;
}

/** Remplace le contenu seulement s'il a changé : un bouton armé ou une liste déroulée restent en l'état. */
function maj(box, html) {
  if (box._html === html) return false;
  box._html = html;
  box.innerHTML = html;
  return true;
}

/** Phrase de présentation d'une fiche (un résumé long, comme celui d'une séance, reste dans le corps). */
const phrase = f => RESUMES.filter(k => champDe(f.type, k)?.genre !== "long").map(k => f.data[k]).find(Boolean);

/** Corps d'une fiche en lecture, construit avec le schéma : propriétés, étapes, textes. */
function corps(f, sauf = []) {
  const t = typeDe(f.type), res = resolveur(f.mentions), refs = f.refs || {};
  const rempli = c => f.data[c.cle] != null && f.data[c.cle] !== "" && !sauf.includes(c.cle);
  const courts = t.champs.filter(c => COURTS.has(c.genre) && !RESUMES.includes(c.cle) && rempli(c));
  const etapes = t.champs.find(c => c.genre === "etapes" && f.data[c.cle]?.length);
  const longs = t.champs.filter(c => c.genre === "long" && rempli(c));
  if (!courts.length && !etapes && !longs.length) return `<p class="vide">Rien d'autre sur cette fiche.</p>`;
  return `
    ${courts.length ? `<dl class="t-props">${courts.map(c => `<div><dt>${esc(c.nom)}</dt><dd>${f.dates?.[c.cle] ? esc(f.dates[c.cle]) : valeurCourte(f.type, c.cle, f.data[c.cle], refs)}</dd></div>`).join("")}</dl>` : ""}
    ${etapes ? `<section><h4>${esc(etapes.nom)} <span class="hint" data-compte>${f.data[etapes.cle].filter(e => e.fait).length} / ${f.data[etapes.cle].length}</span></h4>
      <ul class="etapes" data-fiche="${f.id}" data-champ="${esc(etapes.cle)}">${f.data[etapes.cle].map((e, i) => `<li><label><input type="checkbox" data-i="${i}" ${e.fait ? "checked" : ""}><span class="${e.fait ? "fait" : ""}">${inline(e.texte, res)}</span></label></li>`).join("")}</ul></section>` : ""}
    ${longs.map(c => `<section class="${c.secret ? "secret" : ""}"><h4>${esc(c.nom)}</h4><div class="md">${rendu(f.data[c.cle], res)}</div></section>`).join("")}`;
}

export async function mount(el, arg) {
  document.title = "Écran du MJ — COF DM";
  el.innerHTML = `<div class="table" data-volet="quete">
    <header class="t-barre">
      <button class="ghost" id="t-menu" aria-label="Menu">☰</button>
      <select id="t-quete" aria-label="Quête affichée"></select>
      <span class="t-contexte" id="t-contexte"></span>
      <span class="t-pot" id="t-pot"></span>
      <span class="t-rappels" id="t-rappels"></span>
      <button class="ghost num" id="t-chrono" title="Chronomètre de séance et rappels" hidden></button>
      <button class="ghost" id="t-cherche" title="Rechercher une fiche ( / )" aria-label="Rechercher">🔍</button>
      <button class="ghost" id="t-options" title="Options de l'écran" aria-label="Options">⚙</button>
      <button class="ghost" id="t-plein" title="Plein écran" aria-label="Plein écran"><svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4" fill="none" stroke="currentColor" stroke-width="1.6"/></svg></button>
    </header>
    <section class="t-volet t-gauche">
      <div class="t-bloc" id="t-pj"></div>
      <div class="t-bloc t-etire" id="t-scene"></div>
    </section>
    <section class="t-volet t-centre t-bloc" id="t-centre"></section>
    <section class="t-volet t-droite t-bloc">
      <div class="t-tete" id="t-combat"></div>
      <div class="t-rencs" id="t-rencs" hidden></div>
      <div class="t-cadre" id="t-cadre"></div>
    </section>
    <nav class="t-onglets" aria-label="Volets">
      <button data-v="joueurs">🛡 Joueurs</button><button data-v="quete">✦ Quête</button><button data-v="combat">⚔ Combat</button>
    </nav>
    <aside class="t-tiroir t-bloc" id="t-tiroir" hidden></aside>
    <div class="t-pop t-bloc" id="t-pop" hidden></div>
  </div>`;
  const racine = $(".table", el);
  const fiches = new Map(); // fiches affichées (pour cocher leurs étapes)
  let d = null, brut = "", trk = null, onglet = "quete", vivant = true, enVol = false;
  let aides = null, aidesBrut = ""; // aides de jeu de la campagne (onglet « Aides »)
  // combat affiché dans le volet : son identifiant, de quoi le refermer, et celui que le MJ a écarté
  let combatVu = 0, fermerCombat = null, combatEcarte = 0;
  const lecture = !!window.cofAcces?.lecture_seule;
  const prefs = lirePrefs();
  const garder = () => { try { localStorage.setItem(PREFS, JSON.stringify(prefs)); } catch { /* stockage indisponible */ } };

  // ---------------------------------------------------------------- barre

  function peindreBarre() {
    const sel = $("#t-quete", el);
    const liste = [...d.quetes];
    if (d.quete && !liste.some(q => q.id === d.quete.id)) liste.unshift(d.quete);
    sel.innerHTML = liste.length ? liste.map(q => `<option value="${q.id}">✦ ${esc(q.nom)}</option>`).join("") : `<option value="">Aucune quête en cours</option>`;
    sel.disabled = liste.length < 2;
    if (d.quete) sel.value = d.quete.id;
    const c = d.campagne;
    const lieu = c?.refs?.[c.data.lieu_groupe];
    maj($("#t-contexte", el), c ? `${lienFiche(c)}${c.dates?.date_actuelle ? `<span class="hint">${esc(c.dates.date_actuelle)}</span>` : ""}${lieu ? `<span class="hint">📍</span>${lienFiche(lieu, false)}` : ""}` : "");
  }

  // ---------------------------------------------------------------- joueurs

  function cartePJ(p) {
    const carte = p.fiche.carte || {};
    const sous = [carte.joueur, [carte.profil, carte.niveau ? "niv. " + carte.niveau : ""].filter(Boolean).join(" ")].filter(Boolean).join(" · ");
    const e = trk?.pj?.[p.fiche.id] || p.vie;
    let vie = "";
    if (e) {
      const pct = e.pv_max > 0 ? Math.max(0, Math.min(100, Math.round(e.pv / e.pv_max * 100))) : 0;
      const ton = e.pv <= 0 ? "ko" : pct <= 25 ? "bas" : pct <= 50 ? "moyen" : "haut";
      vie = `<span class="t-pv ${ton}"><i style="width:${pct}%"></i><span>${e.pv <= 0 ? "à terre · " : ""}${e.pv} / ${e.pv_max} PV</span></span>
        <span class="t-stats"><span>DEF <b>${e.defense}</b></span><span>Init <b>${e.init}</b></span><span>PC <b>${e.pc}</b>/${e.pc_max}</span><span>PR <b>${e.pr}</b>/${e.pr_max}</span>${e.pm_max ? `<span>PM <b>${e.pm}</b>/${e.pm_max}</span>` : ""}</span>`;
    } else if (fonctionActive("feuilles")) vie = `<span class="t-stats">pas encore de feuille de personnage</span>`;
    const niv = +(carte.niveau || 1) + 1;
    const monter = e && !lecture && fonctionActive("feuilles") ? `<button type="button" class="petit t-niveau" data-niveau="${p.fiche.id}" title="Passer au niveau ${niv} : PV gagnés, rangs de voie" aria-label="${esc(p.fiche.nom)} : passer au niveau ${niv}">⬆</button>` : "";
    return `<div class="t-pj-l"><a class="t-pj${p.statut === "absent" ? " absent" : ""}" href="#/f/${p.fiche.id}" data-type="pj">
      ${vignette(p.fiche, 36)}
      <span class="c"><span class="n"><b>${esc(p.fiche.nom)}</b>${sous ? `<small>${esc(sous)}</small>` : ""}${p.statut === "absent" ? `<span class="badge">absent</span>` : ""}</span>
        ${vie}</span></a>${monter}</div>`;
  }

  function peindrePJ() {
    const jour = !lecture && fonctionActive("feuilles") && d.pj.length;
    maj($("#t-pj", el), `<h3>Joueurs${jour ? ` <span class="sp"></span><button type="button" class="petit ghost" id="t-jour" title="Les capacités « par jour » des PJ de la table redeviennent utilisables">☀ Nouveau jour</button>` : ""}</h3>
      <div class="t-defile t-pjs">${d.pj.length ? d.pj.map(cartePJ).join("") : `<p class="vide">Aucun PJ${d.campagne ? " dans cette campagne" : ""}. <a href="#/nouveau/pj${d.campagne ? "?campagne=" + d.campagne.id : ""}">Créer un PJ</a></p>`}</div>`);
    $$("[data-niveau]", el).forEach(x => x.onclick = () => {
      const p = d.pj.find(y => y.fiche.id === +x.dataset.niveau);
      monterNiveau(p.fiche.id, p.fiche.nom, () => charger(d.quete?.id || 0));
    });
    const b = $("#t-jour", el);
    if (b && !b.dataset.arme) {
      b.dataset.arme = "1";
      armer(b, "Confirmer le nouveau jour", async () => {
        try {
          const r = await api("POST", "/api/feuilles/nouveau-jour", { pj: d.pj.map(p => p.fiche.id) });
          toast(r.feuilles ? `Nouveau jour : capacités « par jour » rendues sur ${r.feuilles} feuille${r.feuilles > 1 ? "s" : ""}.` : "Nouveau jour : aucune capacité « par jour » n'avait servi.");
        } catch (e) { toast(e.message, true); }
      });
    }
  }

  // ---------------------------------------------------------------- scène

  function peindreScene() {
    const groupes = new Map();
    for (const x of d.scene) {
      if (!groupes.has(x.fiche.type)) groupes.set(x.fiche.type, []);
      groupes.get(x.fiche.type).push(x);
    }
    maj($("#t-scene", el), `<h3>Scène<span class="sp"></span><span class="hint">${d.scene.length || ""}</span></h3>
      <div class="t-defile">${d.scene.length ? [...groupes].map(([type, l]) => `<h4 data-type="${esc(type)}">${esc(typeDe(type)?.pluriel || type)}</h4>` + l.map(x => `
        <a class="t-item" href="#/f/${x.fiche.id}" data-type="${esc(type)}" title="${esc(x.raison)}">${vignette(x.fiche, 30)}
          <span class="c"><span class="n"><b>${esc(x.fiche.nom)}</b>${x.secret ? `<span title="Des secrets du MJ">🔒</span>` : ""}${x.statut ? `<span class="statut-${esc(x.statut.replace(/ /g, "."))} etat">${esc(x.statut)}</span>` : ""}</span>
            <small>${esc(x.resume || x.raison)}</small></span></a>`).join("")).join("")
        : `<p class="vide">${d.quete ? "Rien n'est encore lié à cette quête : commanditaire, lieu, relations, rencontres et [[liens]] de ses textes apparaîtront ici." : "Les PNJ, lieux et secrets de la quête en cours apparaîtront ici."}</p>`}</div>`);
  }

  // ---------------------------------------------------------------- quête et séance

  function peindreCentre() {
    const dispo = [d.quete, d.seance].filter(Boolean);
    const box = $("#t-centre", el);
    if (avecAides && (onglet === "aides" || (!dispo.length && aides?.length))) { peindreAides(box, dispo); return; }
    if (!dispo.length) {
      maj(box, `<h3>Quête</h3><div class="t-defile"><p class="vide">Aucune quête en cours. Passez une quête au statut « en cours » pour la voir ici.</p>
        <div class="ligne" style="margin-top:10px"><a href="#/t/quete"><button>✦ Voir les quêtes</button></a><a href="#/nouveau/quete"><button class="primary">Nouvelle quête</button></a></div></div>`);
      return;
    }
    const f = dispo.find(x => x.type === onglet) || dispo[0];
    const t = typeDe(f.type), res = resolveur(f.mentions);
    const resume = phrase(f);
    const cartes = t.carte.filter(k => f.data[k] != null && f.data[k] !== "").map(k => f.dates?.[k] ? esc(f.dates[k]) : valeurCourte(f.type, k, f.data[k], f.refs || {}));
    fiches.set(f.id, f);
    const haut = box._fiche === f.id ? box.querySelector(".t-defile")?.scrollTop || 0 : 0;
    box._fiche = f.id;
    if (maj(box, `<div class="t-tete">${onglets(dispo, f.type)}
        <span class="sp"></span>${f.type === "seance" && avecSeances ? `<a href="#/preparer/${f.id}" title="Préparer cette séance"><button class="petit ghost">🧭 Préparer</button></a>${lecture ? "" : `<a href="#/apres/${f.id}" title="Reporter ce qui s'est passé"><button class="petit ghost">✅ Après</button></a>`}` : ""}<a href="#/f/${f.id}/modifier" title="Modifier la fiche"><button class="petit ghost" aria-label="Modifier">✎</button></a></div>
      <div class="t-defile">
        <div class="t-titre" data-type="${f.type}"><h2><a href="#/f/${f.id}" data-plein="1">${esc(f.nom)}</a></h2><div class="ligne">${cartes.map(c => `<span class="badge">${c}</span>`).join("")}</div></div>
        ${resume ? `<div class="resume">${inline(resume, res)}</div>` : ""}
        ${corps(f, t.carte)}
      </div>`)) box.querySelector(".t-defile").scrollTop = haut;
  }

  const avecAides = fonctionActive("aides"), avecCombat = fonctionActive("combat"), avecSeances = fonctionActive("seances");
  if (!avecCombat) racine.dataset.sansCombat = "";
  function onglets(dispo, courant) {
    return `<div class="t-tabs" role="tablist">${dispo.map(x => `<button role="tab" data-onglet="${x.type}" aria-selected="${x.type === courant}">${esc(typeDe(x.type).icone)} ${esc(typeDe(x.type).nom)}</button>`).join("")}${avecAides ? `<button role="tab" data-onglet="aides" aria-selected="${courant === "aides"}">${esc(typeDe("aide").icone)} Aides${aides?.length ? `<span class="nb">${aides.length}</span>` : ""}</button>` : ""}</div>`;
  }

  // ---------------------------------------------------------------- aides de jeu

  // Les aides de la campagne, celles prévues pour la séance d'abord. Un PJ touché la reçoit ; « Tous »
  // l'envoie aux PJ de la table qui ne l'ont pas. La séance de l'écran est notée avec l'envoi.
  function peindreAides(box, dispo) {
    const seance = d.seance?.id || 0, pjs = d.pj.map(p => p.fiche);
    const liste = [...(aides || [])].sort((a, b) => (b.prevue === seance && seance ? 1 : 0) - (a.prevue === seance && seance ? 1 : 0));
    const prevues = liste.filter(a => seance && a.prevue === seance).length;
    const nouvelle = `#/nouveau/aide?${new URLSearchParams({ ...(d.campagne ? { campagne: d.campagne.id } : {}), ...(seance ? { prevue: seance } : {}) })}`;
    const ligne = a => {
      const recus = new Set(a.envois.map(e => e.pj.id));
      const manquent = pjs.filter(p => !recus.has(p.id));
      return `<div class="t-aide" data-type="aide">
        <div class="n"><a href="#/f/${a.fiche.id}">${esc(typeDe("aide").icone)} ${esc(a.fiche.nom)}</a>${a.fiche.carte?.forme ? `<span class="badge">${esc(a.fiche.carte.forme)}</span>` : ""}${a.indice_de ? `<span class="hint" title="Indice du secret">🔒 ${esc(a.indice_de.nom)}</span>` : ""}</div>
        <div class="dest">${pjs.map(p => recus.has(p.id)
          ? `<button class="recu" disabled title="Reçue${a.envois.find(e => e.pj.id === p.id)?.seance ? " lors de " + esc(a.envois.find(e => e.pj.id === p.id).seance.nom) : ""}">✓ ${esc(p.nom)}</button>`
          : lecture ? `<button disabled>${esc(p.nom)}</button>` : `<button data-envoyer="${a.fiche.id}" data-qui="${p.id}" title="Envoyer à ${esc(p.nom)}">${esc(p.nom)}</button>`).join("")}
          ${!lecture && manquent.length > 1 ? `<button class="primary" data-envoyer="${a.fiche.id}" data-qui="${manquent.map(p => p.id).join(",")}">✉ Tous</button>` : ""}
          ${!pjs.length ? `<span class="hint">Aucun PJ à la table.</span>` : ""}</div>
      </div>`;
    };
    maj(box, `<div class="t-tete">${onglets(dispo, "aides")}<span class="sp"></span>${lecture ? "" : `<a href="${nouvelle}" title="Nouvelle aide de jeu"><button class="petit ghost">＋ Aide</button></a>`}</div>
      <div class="t-defile">${aides === null ? `<p class="hint">Chargement…</p>` : liste.length
        ? `${prevues ? `<h4>Prévues pour ${esc(d.seance.nom)}</h4>` : ""}${liste.slice(0, prevues).map(ligne).join("")}${prevues && liste.length > prevues ? `<h4>Autres aides</h4>` : ""}${liste.slice(prevues).map(ligne).join("")}`
        : `<p class="vide">Aucune aide de jeu${d.campagne ? " pour cette campagne" : ""}. Une lettre, une image, un indice : créez-la, puis touchez un PJ pour la lui envoyer en direct (onglet « Documents » de sa page).</p>`}</div>`);
  }

  async function chargerAides() {
    if (!avecAides) return;
    try {
      const r = await api("GET", "/api/aides" + (d?.campagne ? "?campagne=" + d.campagne.id : ""));
      const texte = JSON.stringify(r);
      if (!vivant || texte === aidesBrut) return;
      aidesBrut = texte; aides = r;
      if (d) peindreCentre();
    } catch { /* hors ligne : on garde la liste */ }
  }

  async function envoyerAide(id, qui) {
    try {
      const r = await api("POST", `/api/aides/${id}/envoyer`, { qui, seance: d.seance?.id || 0 });
      toast(`Envoyée à ${r.envoyes.length} PJ.${r.statut_secret ? ` Le secret passe à « ${r.statut_secret} ».` : ""}`);
      await chargerAides();
    } catch (e) { toast(e.message, true); }
  }

  // ---------------------------------------------------------------- combat

  // Le combat se joue dans le volet : l'écran de combat y est logé tel quel.
  async function montrer(id) {
    if (id === combatVu) return;
    fermerCombat?.();
    fermerCombat = null; combatVu = id; combatEcarte = 0;
    const hote = $("#t-cadre", el);
    hote._html = null;
    hote.className = "t-cadre combat-hote";
    hote.innerHTML = "";
    const fin = await ecranCombat(hote, id, { integre: true, auRetour: () => quitter(id) });
    if (vivant && combatVu === id) fermerCombat = fin; else fin();
  }
  // Le MJ referme le combat : le volet revient à l'accueil, et ce combat ne se rouvre pas tout seul.
  function quitter(id) {
    fermerCombat?.();
    fermerCombat = null; combatVu = 0; combatEcarte = id;
    $("#t-cadre", el).className = "t-cadre";
    peindreCombat();
  }

  function peindreCombat() {
    if (!avecCombat) return;
    const cb = trk?.combat;
    const etat = !trk ? `<span class="hint">…</span>`
      : cb ? `<button class="petit${cb.id === combatVu ? "" : " primary"}" data-combat="${cb.id}" title="Afficher ce combat">▶ ${esc(cb.nom)} · round ${cb.round}</button>`
      : `<span class="badge">aucun combat en cours</span>`;
    const rencontres = d.rencontres.map(r => `<span class="t-renc${r.fiche.carte?.statut && r.fiche.carte.statut !== "préparée" ? " passee" : ""}" title="${esc([r.lieu, r.resume, r.creatures].filter(Boolean).join(" — "))}">
        <a href="#/f/${r.fiche.id}">⚔ ${esc(r.fiche.nom)}</a>${lecture ? "" : `<button class="petit" data-lancer="${r.fiche.id}">Lancer</button>`}</span>`).join("");
    maj($("#t-combat", el), `<h3>Combat</h3>${etat}<span class="sp"></span>
      <button class="petit ghost" id="t-large" title="Agrandir ou réduire le combat" aria-label="Agrandir le combat">⇔</button>
      ${combatVu ? `<a href="#/combat/${combatVu}" data-plein="1" title="Ouvrir le combat en pleine page"><button class="petit ghost" aria-label="Ouvrir le combat en pleine page">↗</button></a>` : ""}`);
    const liste = $("#t-rencs", el);
    liste.hidden = !d.rencontres.length;
    if (maj(liste, `<span class="hint">Rencontres</span>${rencontres}`)) {
      $$("[data-lancer]", liste).forEach(b => armer(b, "Lancer ?", () => lancer({ rencontre: +b.dataset.lancer })));
    }
    if (combatVu) return;
    if (cb && cb.id !== combatEcarte) { montrer(cb.id).then(peindreCombat); return; }
    if (maj($("#t-cadre", el), `<div class="t-absent"><p class="vide">${cb ? `Le combat « ${esc(cb.nom)} » est en cours.` : "Aucun combat en cours."}</p>
        <p class="hint">${lecture ? "Un combat lancé par le MJ s'affiche ici." : "Lancez une rencontre de la quête ci-dessus, ou un combat improvisé avec les PJ de la campagne."}</p>
        <div class="ligne">${cb ? `<button class="petit primary" data-combat="${cb.id}">Reprendre</button>` : ""}${lecture ? "" : `<button class="petit" id="t-improvise">Combat improvisé</button>`}<a href="#/combat" data-plein="1"><button class="petit ghost">Tous les combats</button></a></div></div>`)) {
      const b = $("#t-improvise", el);
      if (b) b.onclick = () => lancer({ nom: "Combat improvisé", ...(d.campagne ? { campagne: d.campagne.id } : {}) });
    }
  }

  async function lancer(corps) {
    try {
      const c = await lancerCombat(corps);
      toast(`Combat « ${c.nom} » lancé avec ${c.pj} PJ.`);
      racine.dataset.volet = "combat";
      await montrer(c.id);
      sonder();
    } catch (e) { toast(e.message, true); }
  }

  // ---------------------------------------------------------------- pot commun de la campagne

  let potLu = 0;
  async function chargerPot(force) {
    const c = d?.campagne, box = $("#t-pot", el);
    if (!c || !fonctionActive("butin")) { maj(box, ""); return; }
    if (!force && Date.now() - potLu < 15000) return;
    potLu = Date.now();
    let p;
    try { p = await api("GET", `/api/campagnes/${c.id}/pot`); } catch { return; }
    if (!vivant) return;
    const pieces = textePieces(p.pieces), n = p.propositions.length, objets = p.objets.length;
    maj(box, pieces || objets || n ? `<a href="#/f/${c.id}" title="Pot commun de ${esc(c.nom)} : ${esc([pieces, objets ? objets + " objet" + (objets > 1 ? "s" : "") : ""].filter(Boolean).join(", ") || "vide")}">💰 <span>${esc(pieces || (objets ? objets + " obj." : "vide"))}</span>${n ? ` <span class="badge" title="Propositions de partage à valider">${n} à valider</span>` : ""}</a>` : "");
  }

  async function sonder() {
    if (!document.hidden) chargerPot();
    if (enVol || document.hidden) return;
    enVol = true;
    const r = await api("GET", "/api/table/etat" + (d?.campagne ? "?campagne=" + d.campagne.id : "")).catch(() => trk || { pj: {}, combat: null });
    enVol = false;
    if (!vivant) return;
    trk = r;
    if (d) { peindrePJ(); peindreCombat(); }
  }

  // ---------------------------------------------------------------- tiroir : une fiche sans quitter l'écran

  const pile = [];
  const tiroir = $("#t-tiroir", el);
  function fermerTiroir() { pile.length = 0; tiroir.hidden = true; }
  async function ouvrirTiroir(id, empiler = true) {
    let f;
    try { f = await api("GET", "/api/entites/" + id); } catch (e) { toast(e.message, true); return; }
    if (!vivant) return;
    if (empiler && pile.at(-1) !== id) pile.push(id);
    fiches.set(f.id, f);
    const res = resolveur(f.mentions);
    const resume = phrase(f);
    tiroir.innerHTML = `<div class="t-tete">
        ${pile.length > 1 ? `<button class="petit ghost" id="tt-retour" aria-label="Fiche précédente">←</button>` : ""}
        ${badgeType(f.type)}${f.archive ? `<span class="badge archive">archivée</span>` : ""}<span class="sp"></span>
        <a href="#/f/${f.id}" data-plein="1"><button class="petit">Fiche complète</button></a>
        <button class="petit ghost" id="tt-fermer" aria-label="Fermer">✕</button>
      </div>
      <div class="t-defile">
        <div class="t-ident" data-type="${f.type}">${vignette(f, 64)}<div><h2>${esc(f.nom)}</h2>${resume ? `<div class="resume">${inline(resume, res)}</div>` : ""}</div></div>
        <div id="tt-combat"></div>
        ${corps(f)}
        ${f.relations.length ? `<section><h4>Relations</h4><ul class="t-rel">${f.relations.map(r => `<li><span class="hint">${esc(r.libelle)}</span> ${lienFiche(r.autre)}${r.secrete ? ` <span title="Connue du MJ seulement">🔒</span>` : ""}${r.note && r.type !== "autre" ? ` <span class="hint">— ${esc(r.note)}</span>` : ""}</li>`).join("")}</ul></section>` : ""}
      </div>`;
    tiroir.hidden = false;
    $("#tt-fermer", tiroir).onclick = fermerTiroir;
    const retour = $("#tt-retour", tiroir);
    if (retour) retour.onclick = () => { pile.pop(); ouvrirTiroir(pile.at(-1), false); };
    if (f.type === "pnj" && f.data.profil_combat) {
      lireMonstre(f.data.profil_combat).then(m => {
        const zone = $("#tt-combat", tiroir);
        if (vivant && zone && pile.at(-1) === id) zone.innerHTML = blocMonstre(m);
      }).catch(() => { /* monstre retiré du bestiaire : la fiche s'affiche sans son profil */ });
    }
  }

  // ---------------------------------------------------------------- options, chronomètre et rappels

  const pop = $("#t-pop", el);
  const fermerPop = () => { pop.hidden = true; pop.dataset.quoi = ""; };
  function ouvrirPop(quoi) {
    if (!pop.hidden && pop.dataset.quoi === quoi) { fermerPop(); return; }
    pop.dataset.quoi = quoi;
    pop.hidden = false;
    peindrePop();
  }
  function peindrePop() {
    if (pop.dataset.quoi === "options") {
      pop.innerHTML = `<h3>Options de l'écran</h3><div class="t-defile">
        <label class="t-coche"><input type="checkbox" id="o-chrono" ${prefs.chrono ? "checked" : ""}><span>Chronomètre de séance et rappels<small>Durée de la partie et comptes à rebours (« torche : 120 min »), dans la barre du haut.</small></span></label>
        <p class="hint">Ce réglage est propre à cet appareil.</p>
        <h4>Joueurs</h4>
        ${fonctionActive("page_joueurs") ? `<label class="t-coche"><input type="checkbox" id="o-edition" ${window.cofAcces?.edition_joueurs ? "checked" : ""} ${window.cofAcces?.lecture_seule ? "disabled" : ""}>
          <span>Édition par les joueurs<small>${esc(aideEditionJoueurs())}</small></span></label>`
        : `<p class="hint">La page des joueurs est désactivée dans cet univers (Paramètres → Fonctionnalités) : ils ne modifient rien.</p>`}</div>`;
      $("#o-chrono", pop).onchange = e => { prefs.chrono = e.target.checked; garder(); battre(); };
      const edition = $("#o-edition", pop);
      if (edition) edition.onchange = async e => {
        try {
          const r = await api("PUT", "/api/joueurs/edition", { active: e.target.checked });
          window.cofAcces.edition_joueurs = r.edition_joueurs;
          toast(r.edition_joueurs ? "Les joueurs peuvent modifier leur feuille." : "Feuilles des joueurs verrouillées.");
        } catch (err) { toast(err.message, true); e.target.checked = !e.target.checked; }
      };
      return;
    }
    const enMarche = !!prefs.debut;
    pop.innerHTML = `<h3>Chronomètre de séance</h3><div class="t-defile">
      <div class="ligne"><button class="primary" id="c-marche">${enMarche ? "⏸ Pause" : "▶ Démarrer"}</button><button id="c-zero">↺ Remettre à zéro</button></div>
      <h4>Rappels</h4>
      <ul class="t-rel" id="c-rappels">${prefs.rappels.map(r => `<li><span>${esc(r.texte)}</span><span class="sp"></span><button class="petit ghost" data-retirer="${r.id}" aria-label="Retirer ce rappel">✕</button></li>`).join("") || `<li class="vide">Aucun rappel.</li>`}</ul>
      <form class="t-rappel-form" id="c-form"><input id="c-texte" placeholder="Torche, sortilège, renforts…" maxlength="40" required><input id="c-min" type="number" min="1" max="600" value="10" aria-label="Minutes" required><span class="hint">min</span><button>Ajouter</button></form></div>`;
    $("#c-marche", pop).onclick = () => {
      if (prefs.debut) { prefs.cumul += Date.now() - prefs.debut; prefs.debut = null; } else prefs.debut = Date.now();
      garder(); battre(); peindrePop();
    };
    armer($("#c-zero", pop), "Confirmer", () => { prefs.cumul = 0; if (prefs.debut) prefs.debut = Date.now(); garder(); battre(); });
    $("#c-rappels", pop).onclick = e => {
      const b = e.target.closest("[data-retirer]");
      if (!b) return;
      prefs.rappels = prefs.rappels.filter(r => String(r.id) !== b.dataset.retirer);
      garder(); battre(); peindrePop();
    };
    $("#c-form", pop).onsubmit = e => {
      e.preventDefault();
      const min = Math.max(1, Math.min(600, +$("#c-min", pop).value || 0));
      prefs.rappels.push({ id: Date.now(), texte: $("#c-texte", pop).value.trim(), fin: Date.now() + min * 60000 });
      garder(); battre(); peindrePop();
    };
  }

  /** Une fois par seconde : chronomètre et rappels de la barre. */
  function battre() {
    const bouton = $("#t-chrono", el);
    bouton.hidden = !prefs.chrono;
    const zone = $("#t-rappels", el);
    if (!prefs.chrono) { maj(zone, ""); return; }
    bouton.textContent = `⏱ ${duree(prefs.cumul + (prefs.debut ? Date.now() - prefs.debut : 0))}${prefs.debut ? "" : " ⏸"}`;
    let sonne = false;
    for (const r of prefs.rappels) {
      if (r.fin <= Date.now() && !r.sonne) { r.sonne = sonne = true; toast(`⏰ ${r.texte} : c'est l'heure.`, true); }
    }
    if (sonne) garder();
    maj(zone, prefs.rappels.map(r => r.fin <= Date.now()
      ? `<button class="petit t-rappel fini" data-fini="${r.id}" title="Toucher pour retirer">⏰ ${esc(r.texte)}</button>`
      : `<button class="petit t-rappel" title="Rappel en cours">⏳ ${esc(r.texte)} ${duree(r.fin - Date.now())}</button>`).join(""));
  }

  // ---------------------------------------------------------------- données

  async function charger(quete) {
    let r;
    try { r = await api("GET", "/api/table" + (quete ? "?quete=" + quete : "")); }
    catch (e) {
      if (quete && e.status === 404) return charger(0); // quête supprimée depuis
      if (!d) $(".t-centre", el).innerHTML = `<div class="t-defile"><div class="erreur">${esc(e.message)}</div></div>`;
      return;
    }
    const texte = JSON.stringify(r);
    if (!vivant || texte === brut) return;
    brut = texte;
    d = r;
    peindreBarre(); peindrePJ(); peindreScene(); peindreCentre(); peindreCombat();
    chargerAides(); chargerPot(true);
  }

  // ---------------------------------------------------------------- gestes

  $("#t-menu", el).onclick = e => { e.stopPropagation(); $("#app").classList.toggle("menu-ouvert"); };
  $("#t-cherche", el).onclick = () => window.cofRecherche();
  $("#t-options", el).onclick = e => { e.stopPropagation(); ouvrirPop("options"); };
  $("#t-chrono", el).onclick = e => { e.stopPropagation(); ouvrirPop("chrono"); };
  $("#t-rappels", el).onclick = e => {
    const b = e.target.closest("[data-fini]");
    if (b) { prefs.rappels = prefs.rappels.filter(r => String(r.id) !== b.dataset.fini); garder(); battre(); }
    else if (e.target.closest(".t-rappel")) { e.stopPropagation(); ouvrirPop("chrono"); }
  };
  const plein = $("#t-plein", el);
  plein.hidden = !document.fullscreenEnabled;
  plein.onclick = () => (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => toast("Plein écran refusé par le navigateur.", true));
  $("#t-quete", el).onchange = async e => {
    const id = +e.target.value;
    fermerTiroir();
    prefs.quete = id; garder();
    await charger(id);
    history.replaceState(null, "", "#/table/" + id); // sans recharger la page : le combat reste en place
  };
  $(".t-onglets", el).onclick = e => { const b = e.target.closest("[data-v]"); if (b) racine.dataset.volet = b.dataset.v; };

  racine.addEventListener("click", e => {
    $("#app").classList.remove("menu-ouvert");
    if (!pop.hidden && !e.target.closest("#t-pop")) fermerPop();
    const tab = e.target.closest("[data-onglet]");
    if (tab) { onglet = tab.dataset.onglet; peindreCentre(); return; }
    const envoi = e.target.closest("[data-envoyer]");
    if (envoi) { envoyerAide(+envoi.dataset.envoyer, envoi.dataset.qui.split(",").map(Number)); return; }
    const combat = e.target.closest("[data-combat]");
    if (combat) { montrer(+combat.dataset.combat).then(peindreCombat); racine.dataset.volet = "combat"; return; }
    if (e.target.closest("#t-large")) { racine.toggleAttribute("data-large"); return; }
    if (e.target.closest("#t-cadre.combat-hote")) return; // l'écran de combat gère ses propres clics
    // une fiche touchée s'ouvre dans le tiroir ; « Fiche complète » et « Modifier » quittent l'écran
    const a = e.target.closest('a[href^="#/f/"]');
    const id = a && !a.dataset.plein && /^#\/f\/(\d+)$/.exec(a.getAttribute("href"));
    if (id && !e.ctrlKey && !e.metaKey && !e.shiftKey) { e.preventDefault(); ouvrirTiroir(+id[1]); }
  });

  racine.addEventListener("change", async e => {
    const liste = e.target.closest("ul.etapes[data-fiche]");
    if (!liste || e.target.type !== "checkbox") return;
    const f = fiches.get(+liste.dataset.fiche), champ = liste.dataset.champ, i = +e.target.dataset.i;
    if (!f) return;
    const etapes = f.data[champ].map((x, j) => j === i ? { ...x, fait: e.target.checked } : x);
    try {
      const n = await api("PATCH", "/api/entites/" + f.id, { data: { [champ]: etapes } });
      Object.assign(f, n);
      e.target.nextElementSibling.classList.toggle("fait", e.target.checked);
      const compte = liste.parentElement.querySelector("[data-compte]");
      if (compte) compte.textContent = `${etapes.filter(x => x.fait).length} / ${etapes.length}`;
      brut = ""; // la prochaine relecture repeint avec la fiche à jour
    } catch (err) { toast(err.message, true); e.target.checked = !e.target.checked; }
  });

  const touche = e => {
    if (e.key !== "Escape") return;
    if (!pop.hidden) fermerPop(); else if (!tiroir.hidden) fermerTiroir();
  };
  const retour = () => { if (!document.hidden) { charger(d?.quete?.id || 0); sonder(); chargerAides(); } };
  document.addEventListener("keydown", touche);
  document.addEventListener("visibilitychange", retour);
  window.addEventListener("focus", retour);
  window.cofOuvrirFiche = id => ouvrirTiroir(id);
  const sonde = setInterval(sonder, SONDE);
  const battement = setInterval(battre, 1000);

  battre();
  sonder();
  // sans attendre (le nettoyage doit être rendu tout de suite) : la quête de l'adresse, sinon la
  // dernière choisie sur cet appareil si elle est toujours en cours, sinon celle du serveur
  if (+arg) charger(+arg);
  else charger(prefs.quete || 0).then(() => { if (d?.quete && !d.quetes.some(q => q.id === d.quete.id)) charger(0); });

  return () => {
    vivant = false;
    fermerCombat?.();
    clearInterval(sonde); clearInterval(battement);
    document.removeEventListener("keydown", touche);
    document.removeEventListener("visibilitychange", retour);
    window.removeEventListener("focus", retour);
    delete window.cofOuvrirFiche;
  };
}
