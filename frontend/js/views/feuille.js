// Feuille de personnage d'un PJ, dans sa fiche : caractéristiques, PV et ressources, attaques, voies,
// équipement. Le serveur calcule les valeurs dérivées (modificateurs, DEF, initiative, bonus
// d'attaque) ; les bornes, les valeurs de départ et les repos viennent des règles de l'univers,
// jointes à la feuille (f.regles).
import { $, $$, esc, api, toast } from "../ui.js";

export const CARACS = ["FOR", "DEX", "CON", "INT", "SAG", "CHA"];
const TYPES = { C: "Contact", D: "Distance", M: "Magique" };
const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
const cle = s => (s || "").toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[\s'’,-]+/g, " ").trim();
const nomCap = c => c.nom ? c.nom + (c.limitee ? " (L)" : "") + (c.sort ? "*" : "") : "";

/**
 * Monte la feuille du PJ `fiche` dans `box`. `peutEcrire` : le MJ, ou le joueur de ce PJ quand
 * l'édition par les joueurs est ouverte. `joueur` : vue du joueur (pas de notes du MJ).
 */
export async function panneauFeuille(box, fiche, { peutEcrire, joueur }) {
  let F, REF = { profils: [], peuples: [], voies: [] };
  const url = `/api/entites/${fiche.id}/feuille`;
  try { F = await api("GET", url); }
  catch (e) { box.innerHTML = `<h3>Feuille de personnage</h3><p class="alerte">${esc(e.message)}</p>`; return; }
  // le MJ lit toute la bibliothèque ; un joueur, les voies de ce PJ, son profil et son peuple
  let enLecture = false; // la vue de lecture est affichée (et non le formulaire)
  api("GET", `/api/reference?pj=${fiche.id}`).then(r => { REF = r; if (enLecture) lire(); }, () => {});
  const R = () => F.regles || {};
  // bonus des capacités atteintes, calculés par le serveur : « A + b », b en vert, détail au survol
  const E = () => F.effets || { base: {}, bonus: {}, detail: [] };
  const CLES = { att: ["att_C", "att_D", "att_M"], att_c: ["att_C"], att_d: ["att_D"], att_m: ["att_M"] };
  const NOMS_CIBLE = { def: "DEF", init: "Init", pv: "PV max", pm: "PM max", rd: "RD", att: "attaques", att_c: "contact", att_d: "distance", att_m: "magique" };
  function plus(k) {
    const b = E().bonus[k];
    if (!b) return "";
    const directs = E().detail.filter(d => (CLES[d.cible] || [d.cible]).includes(k));
    const lignes = directs.map(d => `${d.capacite} (${d.voie}, rang ${d.rang}) : ${signe(d.valeur)}`);
    const reste = b - directs.reduce((n, d) => n + d.valeur, 0);
    if (reste) lignes.push(`${signe(reste)} par une caractéristique augmentée`);
    return ` <span class="fp-bonus" title="${esc(lignes.join("\n"))}">${b > 0 ? "+" : "−"} ${Math.abs(b)}</span>`;
  }
  const AB = (k, total) => (E().bonus[k] ? `${esc(E().base[k])}${plus(k)}` : esc(total));
  const pvMax = () => F.pv + (E().bonus.pv || 0);
  const pmMax = () => F.pm_max + (E().bonus.pm || 0);
  const maxDe = k => (k === "pm" ? pmMax() : F[k + "_max"]);
  const pvActuel = () => F.pv_actuel ?? pvMax();
  // un bonus de capacité et un « divers » saisi à la main au même endroit : à vérifier
  function doublons() {
    const b = E().bonus, d = [];
    if (b.def && F.def_divers) d.push(`DEF, divers (${signe(F.def_divers)})`);
    if (b.init && F.init_divers) d.push(`Init, divers (${signe(F.init_divers)})`);
    for (const [t, nom] of [["C", "Contact"], ["D", "Distance"], ["M", "Magique"]]) {
      const direct = E().detail.some(x => (CLES[x.cible] || []).includes("att_" + t));
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
  const voieDeRef = v => REF.voies.find(x => (v.ref && x.ref === v.ref) || cle(x.nom) === cle(v.nom));
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

  const CIBLES = { pv: "PV max", pm: "PM max", def: "DEF", init: "Initiative", att: "Attaques", att_c: "Attaque au contact", att_d: "Attaque à distance", att_m: "Attaque magique", rd: "RD" };
  const GENRES = { sort: "Sort", attaque: "Attaque spéciale", soin: "Soin", etat: "État", autre: "Action" };
  function utilisations(u) {
    if (u.par === "main") return `${u.fois} fois, à remettre à zéro à la main`;
    if (u.par.startsWith("repos:")) { const c = u.par.slice(6); return `${u.fois} fois par ${R().repos?.find(r => r.cle === c)?.libelle || c}`; }
    return `${u.fois} fois par ${u.par}`;
  }
  function detail(c, voie, r) {
    const lignes = [];
    for (const b of c.bonus || []) lignes.push(`${CIBLES[b.cible] || b.cible} ${signe(b.valeur)}${b.par_rang ? " × rang" : ""}${b.niveau ? " + niveau" : ""}${b.carac ? ` + mod. ${b.carac}` : ""}`);
    if (c.action) {
      const a = c.action;
      lignes.push([GENRES[a.genre] || a.genre, a.cout_pm && `${a.cout_pm} PM`, a.attaque && `attaque ${(TYPES[a.attaque] || a.attaque).toLowerCase()}${a.bonus ? ` ${signe(a.bonus)}` : ""}`,
        a.degats && `DM ${a.degats}`, a.soin && `soigne ${a.soin}`, a.etat && `état : ${a.etat}`].filter(Boolean).join(" · "));
    }
    if (c.utilisations) lignes.push(utilisations(c.utilisations));
    if (c.limitee) lignes.push("Action limitée");
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
    box.innerHTML = `<h3>Feuille de personnage <span class="sp"></span><span class="hint" style="text-transform:none;letter-spacing:0">niveau ${esc(F.niveau)}</span>
        ${peutEcrire ? `<button class="petit" id="fp-modifier">✎ Modifier</button>` : ""}</h3>
      ${F.existe ? "" : `<p class="vide">Feuille vierge${peutEcrire ? " : cliquez sur « Modifier » pour la remplir" : ""}.</p>`}
      <div class="bloc-combat">
        <div class="ligne" style="gap:6px 14px">
          <span class="fp-res" data-res="pv"><b>PV</b> ${peutEcrire ? `<button class="ghost petit" data-d="-1" aria-label="PV −1">−</button>` : ""}<span>${pvActuel()}/${AB("pv", F.pv)}</span>${peutEcrire ? `<button class="ghost petit" data-d="1" aria-label="PV +1">+</button>` : ""}</span>
          <span><b>DEF</b> ${AB("def", F.defense)}</span><span><b>Init</b> ${AB("init", F.init)}</span>${F.rd || E().bonus.rd ? `<span><b>RD</b> ${AB("rd", F.rd)}</span>` : ""}${F.dv ? `<span><b>DV</b> ${esc(F.dv)}</span>` : ""}
          ${compteur("pc", "PC")}${compteur("pr", "PR")}${compteur("pm", "PM")}
        </div>
        <div class="carac">${CARACS.map(k => `<span><small>${k}</small>${AB(k, F.valeurs[k])} (${signe(F.carac[k])})</span>`).join("")}</div>
        ${F.attaques.length ? `<ul class="attaques">${F.attaques.map(a => `<li><b>${esc(a.nom)}</b> ${E().bonus["att_" + a.type] ? `${signe(a.bonus - E().bonus["att_" + a.type])}${plus("att_" + a.type)}` : signe(a.bonus)}${a.dm ? `, DM ${esc(a.dm)}` : ""}${a.portee ? ` · ${esc(a.portee)}` : ""}${a.notes ? ` <span class="hint">${esc(a.notes)}</span>` : ""}</li>`).join("")}</ul>` : ""}
        ${peutEcrire && doublons().length ? `<p class="hint fp-doublon">⚠ À vérifier : ${doublons().map(esc).join(", ")} s'ajoute${doublons().length > 1 ? "nt" : ""} aux bonus des capacités. Un bonus saisi à la main pour la même capacité compterait deux fois.</p>` : ""}
        ${peutEcrire && (R().repos?.length || comptees().length) ? `<div class="ligne">${(R().repos || []).map((r, i) => `<button class="petit" data-repos="${i}" title="${esc(r.aide || "")}">${esc(r.libelle)}</button>`).join("")}
          ${comptees().some(x => x.par === "jour") ? `<button class="petit" id="fp-jour" title="Les capacités « par jour » redeviennent utilisables">☀ Nouveau jour</button>` : ""}</div>` : ""}
      </div>
      ${F.voies.length ? `<h3 style="margin-top:12px">Voies</h3>${grilleLecture()}` : ""}
      ${utilisationsHTML()}
      ${F.capacites ? `<h3 style="margin-top:12px">Capacités</h3><div class="bst-texte">${esc(F.capacites)}</div>` : ""}
      ${F.equipement || bourse || F.armure?.nom || F.bouclier?.nom ? `<h3 style="margin-top:12px">Équipement</h3>
        ${[F.armure?.nom && `${F.armure.nom} (${signe(F.armure.bonus)} DEF)`, F.bouclier?.nom && `${F.bouclier.nom} (${signe(F.bouclier.bonus)} DEF)`].filter(Boolean).map(x => `<div>${esc(x)}</div>`).join("")}
        ${F.equipement ? `<div class="bst-texte">${esc(F.equipement)}</div>` : ""}${bourse ? `<p class="hint">Bourse : ${esc(bourse)}</p>` : ""}` : ""}
      ${identite.length ? `<dl class="proprietes" style="margin-top:12px">${identite.map(([n, v]) => `<div><dt>${n}</dt><dd>${esc(v)}</dd></div>`).join("")}</dl>` : ""}
      ${!joueur && F.notes ? `<h3 style="margin-top:12px">Notes du MJ</h3><div class="bst-texte">${esc(F.notes)}</div>` : ""}`;
    armerBulles(F.voies);
    if (!peutEcrire) return;
    $("#fp-modifier", box).onclick = editer;
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
  }

  // capacités dont les utilisations se comptent (calculées par le serveur), et leur remise à zéro
  const comptees = () => F.comptees || [];
  function libPar(p) {
    if (p === "combat") return "par combat";
    if (p === "jour") return "par jour";
    if (p === "main") return "à remettre à zéro à la main";
    const c = p.slice(6), r = R().repos?.find(x => x.cle === c);
    return `par ${(r?.libelle || c).toLowerCase()}`;
  }
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
    // à côté d'un champ saisi, ce que les capacités y ajoutent
    const nomB = (nom, ...cles) => { const b = cles.reduce((n, k) => n + (E().bonus[k] || 0), 0); return b && cles.some(k => E().detail.some(d => (CLES[d.cible] || [d.cible]).includes(k))) ? `${nom} <small class="fp-bonus">${signe(b)} capacités</small>` : nom; };
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
          ${texte("Armure", "armure.nom")}${nombre("Bonus d'armure", "armure.bonus", 'min="0"')}${texte("Bouclier", "bouclier.nom")}${nombre("Bonus de bouclier", "bouclier.bonus", 'min="0"')}
          ${nombre(nomB("DEF, divers", "def"), "def_divers")}${nombre(nomB("Init, divers", "init"), "init_divers")}
        </div>
        <h3>Attaques</h3>
        <div class="bst-grille">${nombre(nomB("Contact, divers", "att_C"), "att_divers.C")}${nombre(nomB("Distance, divers", "att_D"), "att_divers.D")}${nombre(nomB("Magique, divers", "att_M"), "att_divers.M")}</div>
        <div class="colonne" style="gap:6px" id="fp-atks"></div>
        <div><button type="button" class="petit" id="fp-atk">＋ Attaque</button></div>
        <p class="hint">Le bonus d'attaque est calculé : niveau + modificateur + divers + bonus propre à l'arme.</p>
        <h3>Voies</h3>
        <p class="hint">Une voie de la bibliothèque montre ses capacités : un clic sur une case fixe le rang. Une voie libre se remplit rang par rang.</p>
        <div id="fp-voies"></div>
        <div class="ligne"><button type="button" class="petit" id="fp-voie">＋ Voie</button>
          ${REF.voies.length ? `<select id="fp-voie-ref" aria-label="Voie de la bibliothèque"><option value="">＋ Voie de la bibliothèque…</option>${REF.voies.map((v, i) => `<option value="${i}">${esc(v.nom)}</option>`).join("")}</select>` : ""}
          ${profil ? `<button type="button" class="petit" id="fp-profil">Appliquer le profil « ${esc(profil.nom)} »</button>` : ""}</div>
        <label class="champ"><span>Capacités <small>hors voies</small></span><textarea data-p="capacites" rows="3">${esc(f.capacites)}</textarea></label>
        <label class="champ"><span>Équipement</span><textarea data-p="equipement" rows="4">${esc(f.equipement)}</textarea></label>
        <div class="bst-grille">${nombre("Pièces d'or", "bourse.po", 'min="0"')}${nombre("Pièces d'argent", "bourse.pa", 'min="0"')}${nombre("Pièces de cuivre", "bourse.pc", 'min="0"')}</div>
        <div class="bst-grille">${texte("Sexe", "sexe")}${texte("Âge", "age")}${texte("Taille", "taille_physique")}${texte("Poids", "poids")}${texte("Divinité", "divinite")}${texte("Langues", "langues")}</div>
        ${joueur ? "" : `<label class="champ"><span>Notes du MJ <small>jamais montrées au joueur</small></span><textarea data-p="notes" rows="3">${esc(f.notes)}</textarea></label>`}
        <div class="ligne"><button type="submit" class="primary">Enregistrer</button><button type="button" id="fp-annuler">Annuler</button></div>
      </form>`;
    const atks = $("#fp-atks", box), voies = $("#fp-voies", box);
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
    atks.innerHTML = f.attaques.map(ligneAtk).join("");
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
