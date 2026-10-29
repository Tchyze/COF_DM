// Feuille de personnage d'un PJ, dans sa fiche : caractéristiques, PV et ressources, attaques, voies,
// équipement. Le serveur calcule les valeurs dérivées (modificateurs, DEF, initiative, bonus
// d'attaque) ; les bornes, les valeurs de départ et les repos viennent des règles de l'univers,
// jointes à la feuille (f.regles).
import { $, $$, esc, api, toast, armer, dateHeure } from "../ui.js";
import { lignesEffets, libellePar, cle, voieLiee } from "../capacites.js";
import { selecteur } from "../composants.js";
import { monterNiveau, annulerMontee, accorderNiveau, texteMontee } from "../niveau.js";

export const CARACS = ["FOR", "DEX", "CON", "INT", "SAG", "CHA"];
const TYPES = { C: "Contact", D: "Distance", M: "Magique" };
const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
const nomCap = c => c.nom ? c.nom + (c.limitee ? " (L)" : "") + (c.sort ? "*" : "") : "";

/**
 * Monte la feuille du PJ `fiche` dans `box`. `peutEcrire` : le MJ, ou le joueur de ce PJ quand
 * l'édition par les joueurs est ouverte. `joueur` : vue du joueur (pas de notes du MJ). `apres` :
 * appelé quand le niveau de la fiche change (montée de niveau, annulation), pour la repeindre.
 */
export async function panneauFeuille(box, fiche, { peutEcrire, joueur, apres }) {
  let F, REF = { profils: [], peuples: [], voies: [] };
  const url = `/api/entites/${fiche.id}/feuille`;
  try { F = await api("GET", url); }
  catch (e) { box.innerHTML = `<h3>Feuille de personnage</h3><p class="alerte">${esc(e.message)}</p>`; return; }
  // le MJ lit toute la bibliothèque ; un joueur, les voies de ce PJ, son profil et son peuple
  let enLecture = false; // la vue de lecture est affichée (et non le formulaire)
  api("GET", `/api/reference?pj=${fiche.id}`).then(r => { REF = r; if (enLecture) lire(); }, () => {});
  const R = () => F.regles || {};
  // bonus calculés par le serveur, des capacités atteintes et des objets portés : la valeur totale, en
  // bleu si un objet y est pour quelque chose (en vert sinon) ; au survol, la formule sur une ligne
  // (« DEF 16 = 10 + 6 ») et ce qui la change sur la suivante (« + 1 Anneau, + 5 Casque »)
  const E = () => F.effets || { base: {}, bonus: {}, detail: [] };
  const CLES = { att: ["att_C", "att_D", "att_M"], att_c: ["att_C"], att_d: ["att_D"], att_m: ["att_M"] };
  const LIBELLES = { def: "DEF", init: "Init", rd: "RD", pv: "PV max", pm: "PM max", att_C: "Contact", att_D: "Distance", att_M: "Magique" };
  const touche = k => d => (CLES[d.cible] || [d.cible]).includes(k);
  const terme = n => `${n >= 0 ? "+" : "−"} ${Math.abs(n)}`;
  function avecBonus(k, base = E().base[k], affiche = String) {
    const b = E().bonus[k];
    if (!b) return esc(affiche(base));
    const directs = E().detail.filter(touche(k));
    const sources = directs.map(d => `${terme(d.valeur)} ${d.objet || `${d.capacite} (${d.voie}, rang ${d.rang})`}`);
    // le reste vient de la caractéristique dont la valeur dépend, augmentée par un objet ou une capacité
    const reste = b - directs.reduce((n, d) => n + d.valeur, 0);
    const carac = { def: "DEX", init: "DEX", att_C: "FOR", att_D: "DEX", att_M: F.carac_magie }[k];
    const parCarac = E().detail.filter(d => d.cible === carac);
    if (reste) sources.push(`${terme(reste)} par ${carac || "une caractéristique"}${parCarac.length ? ` (${[...new Set(parCarac.map(d => d.objet || d.capacite))].join(", ")})` : ""}`);
    const objet = directs.some(d => d.objet) || (reste && parCarac.some(d => d.objet));
    const titre = `${LIBELLES[k] || k} ${affiche(base + b)} = ${affiche(base)} ${terme(b)}\n${sources.join(", ")}`;
    return `<span class="fp-avec ${objet ? "fp-objet" : "fp-bonus"}" title="${esc(titre)}">${esc(affiche(base + b))}</span>`;
  }
  const AB = (k, total) => (E().bonus[k] ? avecBonus(k) : esc(total));
  const pvMax = () => F.pv + (E().bonus.pv || 0);
  const pmMax = () => F.pm_max + (E().bonus.pm || 0);
  const maxDe = k => (k === "pm" ? pmMax() : F[k + "_max"]);
  const pvActuel = () => F.pv_actuel ?? pvMax();
  // un bonus de capacité et un « divers » saisi à la main au même endroit : à vérifier
  function doublons() {
    const parCapacite = k => E().detail.some(x => !x.objet && touche(k)(x)), d = [];
    if (parCapacite("def") && F.def_divers) d.push(`DEF, divers (${signe(F.def_divers)})`);
    if (parCapacite("init") && F.init_divers) d.push(`Init, divers (${signe(F.init_divers)})`);
    for (const [t, nom] of [["C", "Contact"], ["D", "Distance"], ["M", "Magique"]]) {
      const direct = parCapacite("att_" + t);
      if (direct && F.att_divers?.[t]) d.push(`${nom}, divers (${signe(F.att_divers[t])})`);
    }
    return d;
  }

  async function enregistrer(feuille, message) {
    try {
      F = await api("PUT", url, { ...feuille, regles: undefined, modifie: F.modifie || undefined });
      if (message) toast(message);
      return true;
    } catch (e) {
      toast(e.status === 409 ? "La feuille a été modifiée ailleurs : elle est rechargée." : e.message, true);
      if (e.status === 409) { F = await api("GET", url); lire(); }
      return false;
    }
  }

  // ---------------------------------------------------------------- voies
  // Une grille comme sur la feuille papier : une colonne par voie (trois par bloc, une seule en
  // étroit), une ligne par rang. Une voie liée à la bibliothèque (même ref, sinon même nom) en
  // montre les capacités ; une voie libre, celles saisies sur la feuille. Au survol (ou au toucher),
  // le texte du rang et ses effets.
  const voieDeRef = v => voieLiee(REF, v);
  const capacitesDe = v => voieDeRef(v)?.rangs || v.capacites.map(nom => ({ nom }));
  const parBloc = () => (box.clientWidth && box.clientWidth < 560 ? 1 : 3);
  let blocAffiche = 0;

  // grille de voies (lecture ou édition) : tete, sous et cellule donnent le contenu de l'en-tête,
  // de la ligne sous l'en-tête et d'une case (i : la voie, r : le rang, c : sa capacité)
  function grilleVoies(voies, { tete, sous, cellule }) {
    const k = blocAffiche = parBloc();
    const caps = voies.map(capacitesDe);
    let html = "";
    for (let d = 0; d < voies.length; d += k) {
      const idx = [...Array(k).keys()].map(i => d + i); // un bloc incomplet garde ses colonnes vides, comme sur papier
      const nbRangs = Math.max(R().rang_max || 5, ...idx.filter(i => i < voies.length).map(i => {
        const c = caps[i]; let n = c.length; while (n && !c[n - 1]?.nom) n--; return n;
      }));
      html += `<div class="fp-vbloc" style="grid-template-columns:1.8em repeat(${idx.length},minmax(0,1fr))">
        <div class="fp-vcoin"></div>${idx.map(i => i < voies.length ? tete(i) : `<div class="fp-vtete"></div>`).join("")}
        <div class="fp-vcoin2"></div>${idx.map(i => i < voies.length ? sous(i) : `<div class="fp-vsous"></div>`).join("")}
        ${[...Array(nbRangs).keys()].map(r => `<div class="fp-vrang">${r + 1}</div>${idx.map(i => i < voies.length ? cellule(i, r, caps[i][r]) : `<div class="fp-vcase vide"></div>`).join("")}`).join("")}
      </div>`;
    }
    return `<div class="fp-voies">${html}</div>`;
  }
  const origineRang = v => `${esc(v.origine || "")}${v.origine ? " · " : ""}rang ${v.rang}`;
  const grilleLecture = () => grilleVoies(F.voies, {
    tete: i => `<div class="fp-vtete">${esc(F.voies[i].nom)}</div>`,
    sous: i => `<div class="fp-vsous">${origineRang(F.voies[i])}</div>`,
    cellule: (i, r, c) => c?.nom ? `<div class="fp-vcase${r < F.voies[i].rang ? " acquise" : ""}" tabindex="0" data-v="${i}" data-r="${r}">${esc(nomCap(c))}</div>` : `<div class="fp-vcase vide"></div>`,
  });
  let redessiner = null; // ce qu'il faut repeindre quand la grille change de nombre de colonnes

  function detail(c, voie, r) {
    const lignes = lignesEffets(c, R().repos);
    return `<b>${esc(c.nom)}</b> <span class="hint">${esc(voie.nom)}, rang ${r + 1}${r < voie.rang ? " · acquis" : ""}</span>
      ${c.texte ? `<div class="bst-texte">${esc(c.texte)}</div>` : ""}
      ${lignes.length ? `<ul>${lignes.map(l => `<li>${esc(l)}</li>`).join("")}</ul>` : ""}`;
  }
  // une seule bulle pour la page, placée sous la case (au-dessus si la place manque), dans l'écran
  function bulle() {
    let b = document.getElementById("fp-bulle");
    if (!b) { b = document.createElement("div"); b.id = "fp-bulle"; b.className = "fp-bulle"; b.hidden = true; b.setAttribute("role", "tooltip"); document.body.append(b); }
    return b;
  }
  function armerBulles(voies) {
    const cacher = () => { bulle().hidden = true; };
    $$(".fp-vcase[data-v][data-r]", box).forEach(el => {
      const montrer = () => {
        const v = voies[+el.dataset.v], r = +el.dataset.r, b = bulle();
        b.innerHTML = detail(capacitesDe(v)[r], v, r);
        b.hidden = false;
        const c = el.getBoundingClientRect(), l = Math.min(360, innerWidth - 16);
        b.style.width = l + "px";
        b.style.left = Math.max(8, Math.min(c.left, innerWidth - l - 8)) + "px";
        const h = b.offsetHeight;
        b.style.top = (c.bottom + 6 + h > innerHeight && c.top - 6 - h > 0 ? c.top - 6 - h : c.bottom + 6) + "px";
      };
      el.addEventListener("pointerenter", montrer);
      el.addEventListener("focus", montrer);
      el.addEventListener("pointerleave", () => { if (document.activeElement !== el) cacher(); });
      el.addEventListener("blur", cacher);
    });
    addEventListener("scroll", cacher, { capture: true, passive: true, once: true });
  }
  // en passant de large à étroit (ou l'inverse), la grille change de nombre de colonnes
  new ResizeObserver(() => { if (redessiner && box.isConnected && blocAffiche && parBloc() !== blocAffiche) redessiner(); }).observe(box);

  // ---------------------------------------------------------------- lecture
  function lire() {
    enLecture = true;
    redessiner = lire;
    const compteur = (k, nom) => (maxDe(k) || F[k]) ? `<span class="fp-res" data-res="${k}"><b>${nom}</b> ${peutEcrire ? `<button class="ghost petit" data-d="-1" aria-label="${nom} −1">−</button>` : ""}<span>${F[k]}/${AB(k, F[k + "_max"])}</span>${peutEcrire ? `<button class="ghost petit" data-d="1" aria-label="${nom} +1">+</button>` : ""}</span>` : "";
    const identite = [["Sexe", F.sexe], ["Âge", F.age], ["Taille", F.taille_physique], ["Poids", F.poids], ["Divinité", F.divinite], ["Langues", F.langues]].filter(x => x[1]);
    const bourse = ["po", "pa", "pc"].filter(k => F.bourse?.[k]).map(k => `${F.bourse[k]} ${k}`).join(", ");
    box.innerHTML = `<h3 style="flex-wrap:wrap;row-gap:6px">Feuille de personnage <span class="sp"></span><span class="hint" style="text-transform:none;letter-spacing:0">niveau ${esc(F.niveau)}</span>
        ${accorde() && !joueur ? `<span class="badge" title="Le joueur fera ses choix depuis sa page">⏳ niveau ${F.niveau + 1} accordé</span>${mj ? `<button class="ghost petit" id="fp-retirer-accord" title="Retirer l'accord" aria-label="Retirer l'accord du niveau ${F.niveau + 1}">✕</button>` : ""}` : ""}
        ${accorde() && joueur ? `<button class="petit primary" id="fp-prendre">⬆ Passer au niveau ${F.niveau + 1}</button>` : ""}
        ${mj && F.existe && !(R().niveau_max && F.niveau >= R().niveau_max) ? `<button class="petit" id="fp-niveau" title="Passer au niveau ${F.niveau + 1} : PV gagnés, rangs de voie">⬆ Niveau ${F.niveau + 1}</button>` : ""}
        ${peutEcrire ? `<button class="petit" id="fp-modifier">✎ Modifier</button>` : ""}</h3>
      ${accorde() && joueur ? `<p class="alerte">Le MJ vous accorde le niveau ${F.niveau + 1} : lancez vos PV et choisissez vos rangs de voie.</p>` : ""}
      ${F.existe ? "" : `<p class="vide">Feuille vierge${peutEcrire ? " : cliquez sur « Modifier » pour la remplir" : ""}.</p>`}
      <div class="bloc-combat">
        <div class="ligne" style="gap:6px 14px">
          <span class="fp-res" data-res="pv"><b>PV</b> ${peutEcrire ? `<button class="ghost petit" data-d="-1" aria-label="PV −1">−</button>` : ""}<span>${pvActuel()}/${AB("pv", F.pv)}</span>${peutEcrire ? `<button class="ghost petit" data-d="1" aria-label="PV +1">+</button>` : ""}</span>
          <span><b>DEF</b> ${AB("def", F.defense)}</span><span><b>Init</b> ${AB("init", F.init)}</span>${F.rd || E().bonus.rd ? `<span><b>RD</b> ${AB("rd", F.rd)}</span>` : ""}${F.dv ? `<span><b>DV</b> ${esc(F.dv)}</span>` : ""}
          ${compteur("pc", "PC")}${compteur("pr", "PR")}${compteur("pm", "PM")}
        </div>
        <div class="carac">${CARACS.map(k => `<span><small>${k}</small>${AB(k, F.valeurs[k])} (${signe(F.carac[k])})</span>`).join("")}</div>
        ${F.attaques.length ? `<ul class="attaques">${F.attaques.map(a => `<li><b>${esc(a.nom)}</b>${a.objet ? ` <span class="hint" title="Arme portée, dans l'inventaire">🗡</span>` : ""} ${avecBonus("att_" + a.type, a.bonus - (E().bonus["att_" + a.type] || 0), signe)}${a.dm ? `, DM ${esc(a.dm)}` : ""}${a.portee ? ` · ${esc(a.portee)}` : ""}${a.notes ? ` <span class="hint">${esc(a.notes)}</span>` : ""}</li>`).join("")}</ul>` : ""}
        ${peutEcrire && doublons().length ? `<p class="hint fp-doublon">⚠ À vérifier : ${doublons().map(esc).join(", ")} s'ajoute${doublons().length > 1 ? "nt" : ""} aux bonus des capacités. Un bonus saisi à la main pour la même capacité compterait deux fois.</p>` : ""}
        ${peutEcrire && (R().repos?.length || comptees().length) ? `<div class="ligne">${(R().repos || []).map((r, i) => `<button class="petit" data-repos="${i}" title="${esc(r.aide || "")}">${esc(r.libelle)}</button>`).join("")}
          ${comptees().some(x => x.par === "jour") ? `<button class="petit" id="fp-jour" title="Les capacités « par jour » redeviennent utilisables">☀ Nouveau jour</button>` : ""}</div>` : ""}
      </div>
      ${F.voies.length ? `<h3 style="margin-top:12px">Voies</h3>${grilleLecture()}` : ""}
      ${utilisationsHTML()}
      ${niveauxHTML()}
      ${F.capacites ? `<h3 style="margin-top:12px">Capacités</h3><div class="bst-texte">${esc(F.capacites)}</div>` : ""}
      ${inventaire().length || bourse ? `<h3 style="margin-top:12px">Inventaire</h3>${inventaireHTML()}${bourse ? `<p class="hint">Bourse : ${esc(bourse)}</p>` : ""}` : ""}
      ${identite.length ? `<dl class="proprietes" style="margin-top:12px">${identite.map(([n, v]) => `<div><dt>${n}</dt><dd>${esc(v)}</dd></div>`).join("")}</dl>` : ""}
      ${!joueur && F.notes ? `<h3 style="margin-top:12px">Notes du MJ</h3><div class="bst-texte">${esc(F.notes)}</div>` : ""}`;
    armerBulles(F.voies);
    // montée de niveau et annulation : le niveau de la fiche change, toute la fiche se repeint
    const change = r => { F = r; if (apres) apres(); else lire(); };
    const prendre = $("#fp-prendre", box);
    if (prendre) prendre.onclick = () => monterNiveau(fiche.id, fiche.nom, change, { joueur: true });
    if (!peutEcrire) return;
    $("#fp-modifier", box).onclick = editer;
    const retirer = $("#fp-retirer-accord", box);
    if (retirer) retirer.onclick = () => accorderNiveau(fiche.id, fiche.nom, false, F.modifie, change);
    const niv = $("#fp-niveau", box);
    if (niv) niv.onclick = () => monterNiveau(fiche.id, fiche.nom, change);
    const defaire = $("#fp-annuler-niveau", box);
    if (defaire) armer(defaire, `Confirmer : retour au niveau ${F.niveau - 1}`, () => annulerMontee(fiche.id, F.modifie, change));
    // PV et ressources : un clic, enregistré aussitôt
    $$(".fp-res button", box).forEach(b => b.onclick = async () => {
      const k = b.closest(".fp-res").dataset.res, d = +b.dataset.d;
      const f = structuredClone(F);
      if (k === "pv") f.pv_actuel = Math.max(0, Math.min(pvMax(), pvActuel() + d));
      else f[k] = Math.max(0, Math.min(maxDe(k), F[k] + d));
      if (await enregistrer(f)) lire();
    });
    $$("[data-repos]", box).forEach(b => b.onclick = () => reposer(R().repos[+b.dataset.repos]));
    const jour = $("#fp-jour", box);
    if (jour) jour.onclick = async () => {
      const f = structuredClone(F);
      if (!remettre(f, ["jour"])) { toast("Aucune capacité « par jour » n'avait servi."); return; }
      if (await enregistrer(f, "Nouveau jour : capacités « par jour » rendues.")) lire();
    };
    // une capacité utilisée hors combat, ou remise à zéro
    $$("[data-util]", box).forEach(b => b.onclick = async () => {
      const x = comptees().find(c => c.cle === b.dataset.cle), f = structuredClone(F);
      f.utilisations = { ...(f.utilisations || {}) };
      const n = (f.utilisations[x.cle]?.n || 0) + (b.dataset.util === "plus" ? 1 : -Infinity);
      if (n > 0) f.utilisations[x.cle] = { par: x.par, n: Math.min(n, x.fois) }; else delete f.utilisations[x.cle];
      if (await enregistrer(f, b.dataset.util === "plus" ? `« ${x.nom} » utilisée.` : `« ${x.nom} » remise à zéro.`)) lire();
    });
    // l'inventaire : porter ou ôter un objet, en compter un de plus ou de moins, d'un clic
    $$("[data-porte]", box).forEach(c => c.onchange = async () => {
      const f = structuredClone(F), o = f.inventaire.find(x => x.id === +c.dataset.porte);
      o.porte = c.checked;
      await enregistrer(f, `« ${o.nom} » ${o.porte ? "porté" : "ôté"}.`);
      lire(); // refusé (emplacement pris) : la case revient
    });
    $$("[data-qte]", box).forEach(b => {
      const changer = async () => {
        const f = structuredClone(F), i = f.inventaire.findIndex(x => x.id === +b.dataset.qte), o = f.inventaire[i];
        o.quantite += +b.dataset.d;
        if (o.quantite < 1) f.inventaire.splice(i, 1);
        if (await enregistrer(f, o.quantite < 1 ? `« ${o.nom} » retiré de l'inventaire.` : "")) lire();
      };
      // le dernier exemplaire : deux clics, il quitte l'inventaire
      if (b.dataset.d === "-1" && F.inventaire.find(x => x.id === +b.dataset.qte)?.quantite === 1) armer(b, "Retirer ?", changer);
      else b.onclick = changer;
    });
  }

  // ---------------------------------------------------------------- inventaire
  const inventaire = () => F.inventaire || [];
  const emplacements = () => F.emplacements || [];
  const nomEmplacement = c => emplacements().find(e => e.cle === c)?.nom || c;
  const CIBLES = [...Object.keys(LIBELLES).filter(k => !k.startsWith("att_")), "att", "att_c", "att_d", "att_m", ...CARACS];
  const libCible = c => ({ att: "Attaque", att_c: "Contact", att_d: "Distance", att_m: "Magique" })[c] || LIBELLES[c] || c;
  const chiffres = o => [...(o.bonus || []).map(b => `${libCible(b.cible)} ${signe(b.valeur)}`),
    o.arme && `${TYPES[o.arme.type] || o.arme.type}${o.arme.dm ? ` ${o.arme.dm}` : ""}${o.arme.mod ? ` ${signe(o.arme.mod)}` : ""}${o.arme.portee ? ` · ${o.arme.portee}` : ""}`].filter(Boolean).join(", ");
  function inventaireHTML() {
    // ce qui est porté d'abord, dans l'ordre des emplacements ; puis le reste, dans l'ordre saisi
    const rang = o => (o.porte ? 0 : 1) * 1000 + Math.max(0, emplacements().findIndex(e => e.cle === o.emplacement));
    const l = inventaire().map((o, i) => [o, i]).sort((a, b) => rang(a[0]) - rang(b[0]) || a[1] - b[1]).map(x => x[0]);
    return `<ul class="fp-inv">${l.map(o => `<li class="${o.porte ? "porte" : ""}">
        ${o.emplacement && peutEcrire ? `<label class="fp-porte" title="${o.porte ? "Ôter" : "Porter"} (${esc(nomEmplacement(o.emplacement))})"><input type="checkbox" data-porte="${o.id}"${o.porte ? " checked" : ""} aria-label="Porter « ${esc(o.nom)} »"></label>` : `<span class="fp-porte">${o.porte ? "✔" : ""}</span>`}
        <span class="fp-inv-nom">${o.fiche && !joueur ? `<a href="#/f/${o.fiche}" title="Objet du monde : sa fiche"><b>${esc(o.nom)}</b> 🔗</a>` : `<b>${esc(o.nom)}</b>`}${o.quantite > 1 ? ` <span class="num">×${o.quantite}</span>` : ""}
          ${o.emplacement ? `<span class="hint">${esc(nomEmplacement(o.emplacement))}</span>` : ""}
          ${chiffres(o) ? `<span class="${o.porte ? "fp-objet" : "hint"}">${esc(chiffres(o))}</span>` : ""}
          ${o.notes ? `<span class="hint">${esc(o.notes)}</span>` : ""}</span>
        ${peutEcrire ? `<span class="fp-qte"><button class="ghost petit" data-qte="${o.id}" data-d="-1" aria-label="« ${esc(o.nom)} » −1">−</button><button class="ghost petit" data-qte="${o.id}" data-d="1" aria-label="« ${esc(o.nom)} » +1">+</button></span>` : ""}
      </li>`).join("")}</ul>`;
  }

  // historique des montées de niveau, la plus récente d'abord ; le MJ peut annuler la dernière
  const mj = peutEcrire && !joueur;
  const accorde = () => F.niveau_accorde === F.niveau + 1; // un niveau accordé par le MJ, que le joueur n'a pas pris
  function niveauxHTML() {
    const l = F.niveaux || [];
    if (!l.length) return "";
    const derniere = l.at(-1);
    return `<h3 style="margin-top:12px">Niveaux${F.points_capacite ? ` <span class="sp"></span><span class="hint" style="text-transform:none;letter-spacing:0">${F.points_capacite} point${F.points_capacite > 1 ? "s" : ""} de capacité gardé${F.points_capacite > 1 ? "s" : ""}</span>` : ""}</h3>
      <ul class="fp-niveaux">${l.slice().reverse().map(m => `<li><b>Niveau ${m.niveau}</b> <span class="hint">${esc(dateHeure(m.quand))}</span> · ${esc(texteMontee(m))}</li>`).join("")}</ul>
      ${mj && derniere.niveau === F.niveau ? `<div class="ligne"><button class="petit ghost" id="fp-annuler-niveau" title="Défait la montée au niveau ${derniere.niveau} : niveau, PV max et rangs reviennent à ce qu'ils étaient">↶ Annuler la montée au niveau ${derniere.niveau}</button></div>` : ""}`;
  }

  // capacités dont les utilisations se comptent (calculées par le serveur), et leur remise à zéro
  const comptees = () => F.comptees || [];
  const libPar = p => libellePar(p, R().repos);
  function remettre(f, pars) {
    let n = 0;
    for (const [k, u] of Object.entries(f.utilisations || {})) if (pars.includes(u.par)) { delete f.utilisations[k]; n++; }
    return n;
  }
  function utilisationsHTML() {
    const l = comptees();
    if (!l.length) return "";
    return `<h3 style="margin-top:12px">Utilisations</h3><ul class="fp-utils">${l.map(x => {
      const n = x.par === "combat" ? null : (F.utilisations?.[x.cle]?.n || 0);
      return `<li><b>${esc(x.nom)}</b> <span class="hint">${esc(x.voie)}, rang ${x.rang}</span> ·
        ${n === null ? `${x.fois} ${libPar(x.par)} <span class="hint">(comptées dans le combat)</span>` : `<span class="${n >= x.fois ? "fp-epuise" : ""}">${n}/${x.fois}</span> ${esc(libPar(x.par))}`}
        ${peutEcrire && n !== null ? `${n < x.fois ? `<button class="ghost petit" data-util="plus" data-cle="${esc(x.cle)}" title="Utilisée hors combat">＋1</button>` : ""}${n ? `<button class="ghost petit" data-util="zero" data-cle="${esc(x.cle)}" title="Remettre à zéro">↺</button>` : ""}` : ""}</li>`;
    }).join("")}</ul>`;
  }

  // un repos, tel que les règles de l'univers le décrivent : ce qu'il coûte, ce qu'il rend
  async function reposer(r) {
    if ((r.cout_pr || 0) > F.pr) { toast("Plus assez de points de récupération.", true); return; }
    const f = structuredClone(F), avant = pvActuel();
    f.pr = Math.min(F.pr_max, F.pr - (r.cout_pr || 0) + (r.gain_pr || 0));
    let detail = "";
    if (r.pv_max) f.pv_actuel = null;
    else if (r.pv?.length) {
      const faces = +(F.dv || "").slice(1) || 0;
      const termes = r.pv.map(t => {
        if (t === "dv") { const de = faces ? 1 + Math.floor(Math.random() * faces) : 0; return [`${F.dv || "dé"}[${de}]`, de]; }
        if (t === "con") return [`${signe(F.carac.CON)} CON`, F.carac.CON];
        return [`${signe(F.niveau)} niv.`, F.niveau];
      });
      const gain = Math.max(0, termes.reduce((s, x) => s + x[1], 0));
      f.pv_actuel = Math.min(pvMax(), avant + gain);
      detail = ` : ${termes.map(x => x[0]).join(" ")} = ${gain} PV (${avant} → ${f.pv_actuel})`;
    }
    if (r.pm_max) f.pm = pmMax();
    // il rend les capacités « par » ce repos, et celles « par jour » s'il vaut un nouveau jour
    const rendues = remettre(f, [`repos:${r.cle}`, ...(r.nouveau_jour ? ["jour"] : [])]);
    if (rendues) detail += ` ; ${rendues} capacité${rendues > 1 ? "s" : ""} de nouveau utilisable${rendues > 1 ? "s" : ""}`;
    if (await enregistrer(f, `${r.libelle}${detail}`)) lire();
  }

  // ---------------------------------------------------------------- édition
  function editer() {
    enLecture = false;
    bulle().hidden = true;
    const f = structuredClone(F), rg = R();
    // à côté d'un champ saisi, ce que les capacités et les objets portés y ajoutent
    const nomB = (nom, ...cles) => {
      const b = cles.reduce((n, k) => n + (E().bonus[k] || 0), 0), d = E().detail.filter(x => cles.some(k => touche(k)(x)));
      if (!b || !d.length) return nom;
      const qui = [d.some(x => !x.objet) && "capacités", d.some(x => x.objet) && "objets"].filter(Boolean).join(" et ");
      return `${nom} <small class="${d.every(x => x.objet) ? "fp-objet" : "fp-bonus"}">${signe(b)} ${qui}</small>`;
    };
    const nombre = (nom, chemin, plus = "") => `<label class="champ"><span>${nom}</span><input type="number" data-p="${chemin}" value="${esc(chemin.split(".").reduce((x, k) => x?.[k], f) ?? 0)}" ${plus}></label>`;
    const texte = (nom, chemin, plus = "") => `<label class="champ"><span>${nom}</span><input data-p="${chemin}" value="${esc(chemin.split(".").reduce((x, k) => x?.[k], f) ?? "")}" ${plus}></label>`;
    const choix = (nom, chemin, liste, valeur) => liste?.length
      ? `<label class="champ"><span>${nom}</span><select data-p="${chemin}">${liste.map(x => `<option${x === valeur ? " selected" : ""}>${esc(x)}</option>`).join("")}</select></label>`
      : texte(nom, chemin);
    const profil = REF.profils.find(p => cle(p.nom) === cle(fiche.data.profil));
    box.innerHTML = `<h3>Feuille de personnage</h3>
      <form id="fp-form" class="colonne" style="gap:12px" novalidate>
        <div class="bst-grille">${CARACS.map(k => nombre(nomB(k, k), "valeurs." + k)).join("")}</div>
        <div class="bst-grille">
          ${nombre(nomB("PV max", "pv"), "pv", 'min="1"')}${choix("Dé de vie", "dv", rg.des_de_vie, f.dv)}${nombre(nomB("RD", "rd"), "rd", 'min="0"')}${nombre("Critique sur", "crit", 'min="2" max="20"')}
          ${nombre("PC max", "pc_max", 'min="0"')}${nombre("PR max", "pr_max", 'min="0"')}${nombre(nomB("PM max", "pm"), "pm_max", 'min="0"')}
          ${choix("Carac. de magie", "carac_magie", rg.caracs_magie?.length ? rg.caracs_magie : CARACS, f.carac_magie)}
        </div>
        <h3>Défense et initiative</h3>
        <div class="bst-grille">
          ${nombre(nomB("DEF, divers", "def"), "def_divers")}${nombre(nomB("Init, divers", "init"), "init_divers")}
        </div>
        <p class="hint">L'armure, le bouclier et les objets qui protègent se portent dans l'inventaire : leurs bonus s'y saisissent.</p>
        <h3>Attaques</h3>
        <div class="bst-grille">${nombre(nomB("Contact, divers", "att_C"), "att_divers.C")}${nombre(nomB("Distance, divers", "att_D"), "att_divers.D")}${nombre(nomB("Magique, divers", "att_M"), "att_divers.M")}</div>
        <div class="colonne" style="gap:6px" id="fp-atks"></div>
        <div><button type="button" class="petit" id="fp-atk">＋ Attaque</button></div>
        <p class="hint">Le bonus d'attaque est calculé : niveau + modificateur + divers + bonus propre à l'arme. Une arme portée de l'inventaire ajoute sa ligne d'elle-même.</p>
        <h3>Voies</h3>
        <p class="hint">Une voie de la bibliothèque montre ses capacités : un clic sur une case fixe le rang. Une voie libre se remplit rang par rang.${joueur ? " Ajouter une voie de la bibliothèque reste au MJ." : ""}</p>
        <div id="fp-voies"></div>
        <div class="ligne"><button type="button" class="petit" id="fp-voie">＋ Voie</button>
          ${REF.voies.length && !joueur ? `<select id="fp-voie-ref" aria-label="Voie de la bibliothèque"><option value="">＋ Voie de la bibliothèque…</option>${REF.voies.map((v, i) => `<option value="${i}">${esc(v.nom)}</option>`).join("")}</select>` : ""}
          ${profil && !joueur ? `<button type="button" class="petit" id="fp-profil">Appliquer le profil « ${esc(profil.nom)} »</button>` : ""}</div>
        <label class="champ"><span>Capacités <small>hors voies</small></span><textarea data-p="capacites" rows="3">${esc(f.capacites)}</textarea></label>
        <h3>Inventaire</h3>
        <p class="hint">Un objet porté occupe son emplacement (un seul casque, une seule armure…).${joueur ? " Ses chiffres (bonus, arme) sont ceux du MJ." : " Ses bonus et son arme ne comptent que s'il est porté."}</p>
        <div class="colonne" style="gap:6px" id="fp-inv"></div>
        <div class="ligne"><button type="button" class="petit" id="fp-objet">＋ Objet</button>
          ${joueur ? "" : `<div id="fp-objet-monde" style="flex:1;min-width:220px" title="Une fiche « Objet » : le PJ en devient le détenteur"></div>`}</div>
        <div class="bst-grille">${nombre("Pièces d'or", "bourse.po", 'min="0"')}${nombre("Pièces d'argent", "bourse.pa", 'min="0"')}${nombre("Pièces de cuivre", "bourse.pc", 'min="0"')}</div>
        <div class="bst-grille">${texte("Sexe", "sexe")}${texte("Âge", "age")}${texte("Taille", "taille_physique")}${texte("Poids", "poids")}${texte("Divinité", "divinite")}${texte("Langues", "langues")}</div>
        ${joueur ? "" : `<label class="champ"><span>Notes du MJ <small>jamais montrées au joueur</small></span><textarea data-p="notes" rows="3">${esc(f.notes)}</textarea></label>`}
        <div class="ligne"><button type="submit" class="primary">Enregistrer</button><button type="button" id="fp-annuler">Annuler</button></div>
      </form>`;
    const atks = $("#fp-atks", box), voies = $("#fp-voies", box), inv = $("#fp-inv", box);
    const ligneAtk = a => `<div class="fp-atk">
        <input data-k="nom" value="${esc(a.nom)}" placeholder="Nom" aria-label="Nom de l'attaque">
        <select data-k="type" aria-label="Type">${Object.entries(TYPES).map(([k, v]) => `<option value="${k}"${a.type === k ? " selected" : ""}>${v}</option>`).join("")}</select>
        <input data-k="mod" type="number" value="${esc(a.mod ?? 0)}" aria-label="Bonus propre à l'arme" title="Bonus propre à l'arme">
        <input data-k="dm" value="${esc(a.dm)}" placeholder="1d8+2" aria-label="Dégâts">
        <input data-k="portee" value="${esc(a.portee)}" placeholder="Portée" aria-label="Portée">
        <input data-k="notes" value="${esc(a.notes)}" placeholder="Notes" aria-label="Notes">
        <button type="button" class="ghost petit" data-retirer aria-label="Retirer">✕</button></div>`;
    // les voies en grille, comme en lecture : ed garde ce qui est saisi entre deux dessins
    const ed = f.voies.map(v => ({ ...v, capacites: [...(v.capacites || [])] }));
    const nouvelleVoie = (nom = "") => ({ nom, origine: rg.origines_voie?.[0] || "", rang: 0, capacites: [] });
    const depuisRef = v => ({ nom: v.nom, origine: rg.origines_voie?.includes(v.type) ? v.type : (rg.origines_voie?.[0] || v.type || ""), rang: 0, capacites: (v.rangs || []).map(nomCap), ref: v.ref || "" });
    const origine = (v, i) => rg.origines_voie?.length
      ? `<select data-origine="${i}" aria-label="Origine">${rg.origines_voie.map(o => `<option${o === v.origine ? " selected" : ""}>${esc(o)}</option>`).join("")}</select>`
      : `<input data-origine="${i}" value="${esc(v.origine)}" placeholder="Origine" aria-label="Origine">`;
    function peindreVoies() {
      redessiner = peindreVoies;
      voies.innerHTML = ed.length ? grilleVoies(ed, {
        tete: i => `<div class="fp-vtete fp-vtete-ed"><input data-nom="${i}" value="${esc(ed[i].nom)}" placeholder="Nom de la voie" aria-label="Nom de la voie">
          <button type="button" class="ghost petit" data-retirer-voie="${i}" aria-label="Retirer la voie" title="Retirer la voie">✕</button></div>`,
        sous: i => `<div class="fp-vsous fp-vsous-ed">${voieDeRef(ed[i]) ? `<span title="Capacités de la bibliothèque">📖</span>` : ""}${origine(ed[i], i)}
          <label>rang <input data-rang="${i}" type="number" min="0" max="20" value="${esc(ed[i].rang)}"></label></div>`,
        cellule: (i, r, c) => {
          const acq = r < ed[i].rang ? " acquise" : "";
          if (voieDeRef(ed[i])) return c?.nom ? `<button type="button" class="fp-vcase${acq}" data-v="${i}" data-r="${r}" data-fixer="${i}:${r}" title="Rang ${r + 1}">${esc(nomCap(c))}</button>` : `<div class="fp-vcase vide"></div>`;
          return `<div class="fp-vcase fp-vcase-ed${acq}"><input data-cap="${i}:${r}" value="${esc(ed[i].capacites[r] || "")}" placeholder="Rang ${r + 1}" aria-label="${esc(ed[i].nom || "Voie")}, rang ${r + 1}"></div>`;
        },
      }) : `<p class="vide">Aucune voie.</p>`;
      armerBulles(ed);
    }
    voies.oninput = e => {
      const t = e.target, d = t.dataset;
      if (d.nom != null) ed[+d.nom].nom = t.value;
      else if (d.origine != null) ed[+d.origine].origine = t.value;
      else if (d.rang != null) ed[+d.rang].rang = Math.max(0, +t.value || 0);
      else if (d.cap != null) { const [i, r] = d.cap.split(":").map(Number); ed[i].capacites[r] = t.value; }
    };
    // un nom changé peut lier ou délier la voie de la bibliothèque ; un rang change la surbrillance
    voies.onchange = e => { if (e.target.dataset.nom != null || e.target.dataset.rang != null) peindreVoies(); };
    voies.onclick = e => {
      const x = e.target.closest("[data-retirer-voie], [data-fixer]");
      if (!x) return;
      if (x.dataset.retirerVoie != null) ed.splice(+x.dataset.retirerVoie, 1);
      else { const [i, r] = x.dataset.fixer.split(":").map(Number); ed[i].rang = ed[i].rang === r + 1 ? r : r + 1; }
      bulle().hidden = true;
      peindreVoies();
    };
    atks.innerHTML = f.attaques.filter(a => !a.objet).map(ligneAtk).join(""); // celles des armes portées se recalculent

    // l'inventaire : une ligne par objet ; ses chiffres (bonus, arme) dans un volet, pour le MJ
    const optionsEmpl = c => `<option value="">— aucun —</option>${emplacements().map(e => `<option value="${esc(e.cle)}"${e.cle === c ? " selected" : ""}>${esc(e.nom)}</option>`).join("")}${c && !emplacements().some(e => e.cle === c) ? `<option value="${esc(c)}" selected>${esc(c)}</option>` : ""}`;
    const ligneBonus = b => `<span class="fp-inv-bonus"><select data-b="cible" aria-label="Valeur augmentée">${CIBLES.map(c => `<option value="${c}"${c === b.cible ? " selected" : ""}>${esc(libCible(c))}</option>`).join("")}</select><input data-b="valeur" type="number" min="-20" max="20" value="${esc(b.valeur ?? 1)}" aria-label="Bonus"><button type="button" class="ghost petit" data-retirer-bonus aria-label="Retirer le bonus">✕</button></span>`;
    // un objet du monde (lié à une fiche « Objet ») : son nom et ses chiffres sont ceux de la fiche ;
    // l'ôter de l'inventaire lui retire son détenteur, ce qui reste au MJ
    const ligneObjet = o => `<div class="fp-inv-ed" data-id="${o.id || ""}" data-fiche="${o.fiche || ""}">
        <div class="fp-inv-ligne">
          <input data-o="nom" value="${esc(o.nom)}" placeholder="Objet" aria-label="Objet"${o.fiche ? ` readonly title="Le nom de la fiche de l'objet"` : ""}>
          <input data-o="quantite" type="number" min="1" value="${esc(o.quantite || 1)}" aria-label="Quantité" title="Quantité">
          <select data-o="emplacement" aria-label="Emplacement" title="Emplacement"${(joueur && o.id) || o.fiche ? " disabled" : ""}>${optionsEmpl(o.emplacement)}</select>
          <label class="t-coche"><input type="checkbox" data-o="porte"${o.porte ? " checked" : ""}> porté</label>
          ${joueur && o.fiche ? `<span></span>` : `<button type="button" class="ghost petit" data-retirer-objet aria-label="Retirer l'objet"${o.fiche ? ` title="Il n'aura plus de détenteur"` : ""}>✕</button>`}
        </div>
        ${o.fiche ? `<p class="hint">🔗 Objet du monde${joueur ? "" : ` : chiffres de <a href="#/f/${o.fiche}">sa fiche</a>`}${chiffres(o) ? ` · ${esc(chiffres(o))}` : ""}</p>`
        : joueur ? (chiffres(o) ? `<p class="hint">${esc(chiffres(o))}</p>` : "") : `<details class="fp-inv-chiffres"><summary>Chiffres${chiffres(o) ? ` : ${esc(chiffres(o))}` : ""}</summary>
          <div class="ligne" data-bonus>${(o.bonus || []).map(ligneBonus).join("")}<button type="button" class="petit" data-ajout-bonus>＋ Bonus</button></div>
          <div class="ligne fp-inv-arme${o.arme ? " on" : ""}"><label class="t-coche"><input type="checkbox" data-a="arme"${o.arme ? " checked" : ""}> Arme</label>
            <select data-a="type" aria-label="Type d'attaque">${Object.entries(TYPES).map(([k, v]) => `<option value="${k}"${(o.arme?.type || "C") === k ? " selected" : ""}>${v}</option>`).join("")}</select>
            <input data-a="dm" value="${esc(o.arme?.dm || "")}" placeholder="DM : 1d8" aria-label="Dégâts">
            <input data-a="mod" type="number" value="${esc(o.arme?.mod || 0)}" aria-label="Bonus propre à l'arme" title="Bonus propre à l'arme">
            <input data-a="portee" value="${esc(o.arme?.portee || "")}" placeholder="Portée" aria-label="Portée"></div>
        </details>`}
        <input data-o="notes" value="${esc(o.notes || "")}" placeholder="Notes" aria-label="Notes">
      </div>`;
    inv.innerHTML = (f.inventaire || []).map(ligneObjet).join("");
    inv.onchange = e => { if (e.target.dataset.a === "arme") e.target.closest(".fp-inv-arme").classList.toggle("on", e.target.checked); };
    inv.onclick = e => {
      const t = e.target;
      if (t.closest("[data-retirer-objet]")) t.closest(".fp-inv-ed").remove();
      else if (t.closest("[data-retirer-bonus]")) t.closest(".fp-inv-bonus").remove();
      else if (t.closest("[data-ajout-bonus]")) t.closest("[data-ajout-bonus]").insertAdjacentHTML("beforebegin", ligneBonus({ cible: "def", valeur: 1 }));
    };
    $("#fp-objet", box).onclick = () => { inv.insertAdjacentHTML("beforeend", ligneObjet({ nom: "", quantite: 1 })); $(".fp-inv-ed:last-child [data-o=nom]", inv).focus(); };
    // un objet du monde : choisi parmi les fiches « Objet » ; ses chiffres arrivent à l'enregistrement
    const zoneMonde = $("#fp-objet-monde", box);
    const armerMonde = () => { // un sélecteur vide, refait après chaque choix
      const choix = selecteur(zoneMonde, { cibles: ["objet"], multiple: false, auChangement: async () => {
        const id = choix.valeur();
        if (!id) return;
        if ($(`.fp-inv-ed[data-fiche="${id}"]`, inv)) toast("Cet objet est déjà dans l'inventaire.", true);
        else {
          try {
            const o = await api("GET", `/api/entites/${id}`);
            inv.insertAdjacentHTML("beforeend", ligneObjet({ fiche: o.id, nom: o.nom, quantite: 1, ...(o.data.equipement || {}) }));
          } catch (e) { toast(e.message, true); }
        }
        armerMonde();
      } });
      $("input", zoneMonde).placeholder = "＋ Objet du monde…";
    };
    if (zoneMonde) armerMonde();
    const lireInventaire = () => $$(".fp-inv-ed", inv).map(l => {
      const v = k => $(`[data-o=${k}]`, l), a = k => $(`[data-a=${k}]`, l);
      const ancien = (f.inventaire || []).find(o => String(o.id) === l.dataset.id) || {};
      const o = { ...ancien, id: +l.dataset.id || 0, fiche: +l.dataset.fiche || undefined, nom: v("nom").value.trim(), quantite: Math.max(1, +v("quantite").value || 1),
        emplacement: v("emplacement").value, porte: v("porte").checked, notes: v("notes").value.trim() };
      if (!joueur && !o.fiche) {
        o.bonus = $$(".fp-inv-bonus", l).map(b => ({ cible: $("[data-b=cible]", b).value, valeur: +$("[data-b=valeur]", b).value || 0 })).filter(b => b.valeur);
        o.arme = a("arme").checked ? { type: a("type").value, dm: a("dm").value.trim(), mod: +a("mod").value || 0, portee: a("portee").value.trim() } : undefined;
      }
      return o;
    }).filter(o => o.nom);
    peindreVoies();
    atks.onclick = e => { const b = e.target.closest("[data-retirer]"); if (b) b.closest(".fp-atk").remove(); };
    $("#fp-atk", box).onclick = () => atks.insertAdjacentHTML("beforeend", ligneAtk({ nom: "", type: "C", mod: 0, dm: "", portee: "", notes: "" }));
    $("#fp-voie", box).onclick = () => { ed.push(nouvelleVoie()); peindreVoies(); $(`[data-nom="${ed.length - 1}"]`, voies)?.focus(); };
    const selRef = $("#fp-voie-ref", box);
    if (selRef) selRef.onchange = () => { if (selRef.value !== "") { ed.push(depuisRef(REF.voies[+selRef.value])); peindreVoies(); } selRef.value = ""; };
    const btnProfil = $("#fp-profil", box);
    if (btnProfil) btnProfil.onclick = () => {
      // le profil de la bibliothèque donne son dé de vie, sa caractéristique de magie et ses voies (au rang 0)
      for (const [champ, v] of [["dv", profil.dv], ["carac_magie", profil.carac_magie]]) { const i = $(`[data-p="${champ}"]`, box); if (v && i) i.value = v; }
      const deja = new Set(ed.map(v => cle(v.nom)));
      let n = 0;
      for (const nom of profil.voies || []) {
        const v = REF.voies.find(x => cle(x.nom) === cle(nom) || (x.ref && x.ref === nom));
        if (!deja.has(cle(v?.nom || nom))) { ed.push(v ? depuisRef(v) : nouvelleVoie(nom)); n++; }
      }
      peindreVoies();
      toast(`Profil appliqué : ${n} voie${n > 1 ? "s" : ""} ajoutée${n > 1 ? "s" : ""}. Enregistrez pour valider.`);
    };
    $("#fp-annuler", box).onclick = () => { bulle().hidden = true; lire(); };
    $("#fp-form", box).onsubmit = async e => {
      e.preventDefault();
      for (const i of $$("[data-p]", box)) {
        const ks = i.dataset.p.split("."), dernier = ks.pop();
        const cible = ks.reduce((x, k) => (x[k] ??= {}), f);
        cible[dernier] = i.type === "number" ? (+i.value || 0) : i.value;
      }
      f.attaques = $$(".fp-atk", atks).map(r => ({ nom: $("[data-k=nom]", r).value.trim(), type: $("[data-k=type]", r).value, mod: +$("[data-k=mod]", r).value || 0,
        dm: $("[data-k=dm]", r).value.trim(), portee: $("[data-k=portee]", r).value.trim(), notes: $("[data-k=notes]", r).value.trim() }));
      f.inventaire = lireInventaire();
      // une voie liée garde une copie des noms de la bibliothèque, lisible sans elle
      f.voies = ed.filter(v => v.nom.trim()).map(v => {
        const lib = voieDeRef(v);
        return { nom: v.nom.trim(), origine: (v.origine || "").trim(), rang: v.rang, ref: v.ref || undefined,
          capacites: lib ? lib.rangs.map(nomCap) : v.capacites.map(c => (c || "").trim()) };
      });
      for (const v of f.voies) while (v.capacites.length > v.rang && !v.capacites.at(-1)) v.capacites.pop();
      if (f.pv_actuel != null) f.pv_actuel = Math.min(f.pv_actuel, f.pv + (E().bonus.pv || 0));
      if (await enregistrer(f, "Feuille enregistrée.")) lire();
    };
  }

  lire();
}
