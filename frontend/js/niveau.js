// Montée de niveau d'un PJ, en assistant : dés de vie (lancés en vrai, saisis), rangs de voie, PM du
// profil, étapes en plus que prévoient les règles à ce niveau, récapitulatif. Le MJ la lance : il la
// fait lui-même, ou l'accorde et le joueur la prend depuis sa page (la fenêtre s'y ouvre seule).
// Tout ce qui tient au jeu vient des règles de l'univers (personnage.montee) ; sans elles, PV saisis
// et rangs libres. Le serveur vérifie tout : l'assistant ne fait que préparer.
import { $, $$, esc, api, toast, dialogue } from "./ui.js";
import { voieLiee } from "./capacites.js";
import { actif } from "./schema.js";

const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
const terme = n => `${n >= 0 ? "+" : "−"} ${Math.abs(n)}`;
const pluriel = n => (n > 1 ? "s" : "");

// coût d'un rang et niveau qu'il demande, comme internal/regles (Montee.Cout, Montee.NiveauPour)
export function coutRang(m, origine, r) {
  const c = m?.couts_origine?.[origine]?.length ? m.couts_origine[origine] : m?.couts;
  return !c?.length ? 1 : (c[r - 1] ?? c.at(-1));
}
export const niveauPour = (m, r) => m?.niveau_min?.[r - 1] || 0;
const formule = m => !!(m && (m.pv?.length || m.pv_fixe));

/** Une ligne de l'historique en clair : PV et PM gagnés, rangs pris, capacités débloquées, étapes. */
export function texteMontee(m) {
  const morceaux = [`${signe(m.pv)} PV${m.jet ? ` (${m.jet})` : ""}`];
  if (m.pm) morceaux.push(`${signe(m.pm)} PM`);
  for (const r of m.rangs || []) morceaux.push(`${r.voie} rang ${r.de} → ${r.a}${r.capacites?.length ? ` (${r.capacites.join(", ")})` : ""}`);
  if (m.points) morceaux.push(`${m.points} point${pluriel(m.points)} de capacité dépensé${pluriel(m.points)}`);
  for (const e of m.etapes || []) morceaux.push(`${e.libelle} : ${e.texte}`);
  return morceaux.join(" ; ");
}

/**
 * Ouvre l'assistant de montée de niveau du PJ `id` (nom : pour le titre). `apres(feuille)` est
 * appelé une fois la montée faite (ou accordée), avec la feuille renvoyée par le serveur. `joueur` :
 * le joueur prend le niveau que le MJ lui a accordé ; `plusTard` : appelé s'il remet à plus tard.
 */
export async function monterNiveau(id, nom, apres, { joueur = false, plusTard } = {}) {
  if (document.getElementById("mn")) return; // déjà ouvert
  let F, REF;
  try { [F, REF] = await Promise.all([api("GET", `/api/entites/${id}/feuille`), api("GET", `/api/reference?pj=${id}`).catch(() => ({ voies: [] }))]); }
  catch (e) { toast(e.message, true); return; }
  if (!F.existe) { toast("Ce PJ n'a pas encore de feuille : remplissez-la d'abord.", true); return; }
  const R = F.regles || {}, M = R.montee || null, niveau = F.niveau + 1;
  if (R.niveau_max && niveau > R.niveau_max) { toast(`Niveau ${R.niveau_max} au plus.`, true); return; }
  if (document.getElementById("mn")) return;
  const parPoints = M?.points > 0, dispo = parPoints ? M.points + (F.points_capacite || 0) : 0;
  const rangMax = v => R.rang_max || Math.max(5, v.capacites.length, v.rang + 1);
  const capacites = v => voieLiee(REF, v)?.rangs || v.capacites.map(nom => ({ nom }));
  const faces = +(F.dv || "").slice(1) || 0, nbDes = (M?.pv || []).filter(t => t === "dv").length;
  const extra = (M?.etapes || []).filter(e => e.niveaux.includes(niveau));

  // ce que le joueur choisit, d'une fenêtre à l'autre
  const choix = { des: Array(nbDes).fill(""), pv: "", vise: F.voies.map(v => v.rang), etapes: Object.fromEntries(extra.map(e => [e.cle, ""])) };
  const etapes = [
    { cle: "des", titre: formule(M) && nbDes ? "Dés de vie" : "Points de vie" },
    { cle: "rangs", titre: "Rangs de voie" },
    ...(F.montee_pm ? [{ cle: "pm", titre: "Points de mana" }] : []),
    ...extra.map(e => ({ cle: "etape", etape: e, titre: e.libelle })),
    { cle: "recap", titre: "Récapitulatif" },
  ];
  let i = 0;

  // PV gagnés, comme le serveur les calcule
  function pvGagnes() {
    if (!formule(M)) return choix.pv === "" ? null : +choix.pv;
    let total = 0, k = 0;
    const det = [];
    for (const t of M.pv) {
      if (t === "dv") { const v = +choix.des[k++]; total += v; det.push(`${F.dv}[${v}]`); }
      else if (t === "con") { total += F.carac.CON; det.push(`${terme(F.carac.CON)} (CON)`); }
      else { total += niveau; det.push(`${terme(niveau)} (niveau)`); }
    }
    if (M.pv_fixe) { total += M.pv_fixe; det.push(terme(M.pv_fixe)); }
    return { total: Math.max(0, total, M.pv_min || 0), detail: det.join(" ").replace(/^\+ /, "") };
  }
  const cout = j => { let c = 0; for (let r = F.voies[j].rang + 1; r <= choix.vise[j]; r++) c += coutRang(M, F.voies[j].origine, r); return c; };
  const depense = () => F.voies.reduce((s, _, j) => s + cout(j), 0);
  const nouvelles = j => { const v = F.voies[j], caps = capacites(v); return [...Array(choix.vise[j] - v.rang).keys()].map(k => ({ r: v.rang + k + 1, c: caps[v.rang + k] || {} })); };

  // le MJ peut laisser le joueur faire ses choix depuis sa page
  const accorde = F.niveau_accorde === niveau;
  const accord = joueur || !actif("page_joueurs") ? "" : `<div class="mn-accord">
      <p>${accorde ? `Niveau ${niveau} accordé : le joueur fait ses choix depuis sa page.` : "Laissez le joueur faire ses choix depuis sa page :"}</p>
      <button type="button" id="mn-accorder"${accorde ? ` class="ghost"` : ""}>${accorde ? "Retirer l'accord" : `Accorder le niveau ${niveau}`}</button></div>
    <p class="hint">Ou faites la montée vous-même :</p>`;

  const corps = {
    des() {
      const tete = (i === 0 ? accord : "") + (M?.aide ? `<p class="hint">${esc(M.aide)}</p>` : "");
      if (!formule(M)) return tete + `<p>Les règles de l'univers ne donnent pas la formule des PV : ${joueur ? "entrez les PV que le MJ vous accorde" : "saisissez les PV gagnés"}.</p>
        <label class="champ"><span>PV gagnés</span><input type="number" id="mn-pv" min="0" max="999" inputmode="numeric" value="${esc(choix.pv)}"></label>`;
      if (nbDes && !faces) return tete + `<p class="alerte">La feuille n'a pas de dé de vie${joueur ? " : demandez au MJ de le choisir." : " : choisissez-le d'abord en la modifiant."}</p>`;
      const autres = (M.pv || []).filter(t => t !== "dv").map(t => t === "con" ? `${terme(F.carac.CON)} (CON)` : `${terme(niveau)} (niveau)`).concat(M.pv_fixe ? [terme(M.pv_fixe)] : []);
      return tete + (nbDes ? `<p>Lancez ${nbDes > 1 ? `${nbDes} dés de vie (${esc(F.dv)})` : `votre dé de vie (${esc(F.dv)})`} et entrez le résultat.</p>
        <div class="mn-des">${choix.des.map((v, k) => `<label class="champ"><span>Dé de vie${nbDes > 1 ? ` ${k + 1}` : ""} (${esc(F.dv)})</span>
          <input type="number" data-dv="${k}" min="1" max="${faces}" inputmode="numeric" value="${esc(v)}" placeholder="1 à ${faces}"></label>`).join("")}</div>` : "")
        + (autres.length ? `<p class="hint">Ajouté au dé : ${esc(autres.join(" "))}${M.pv_min ? `, au moins ${M.pv_min} PV` : ""}.</p>` : "")
        + `<p class="mn-total" id="mn-total"></p>`;
    },
    rangs() {
      if (!F.voies.length) return `<p class="vide">Aucune voie sur la feuille${joueur ? " : demandez au MJ d'en ajouter." : " : ajoutez-en d'abord en la modifiant."}</p>`;
      return `${parPoints ? `<p>${M.points} point${pluriel(M.points)} de capacité à ce niveau${F.points_capacite ? `, plus ${F.points_capacite} gardé${pluriel(F.points_capacite)}` : ""}${M.report ? " ; ce qui n'est pas dépensé se garde." : "."} <b id="mn-points"></b></p>` : `<p class="hint">Les règles ne comptent pas de points : prenez les rangs librement.</p>`}
        <p class="hint">Touchez une case pour prendre la capacité (et celles d'avant dans la voie) ; touchez-la de nouveau pour y renoncer.</p>
        <div id="mn-grille"></div><p class="hint" id="mn-detail"></p>`;
    },
    pm: () => `<p>Votre profil vous donne <b>${signe(F.montee_pm)} PM max</b> à chaque niveau : ${F.pm_max} → ${F.pm_max + F.montee_pm}.</p>`,
    etape: e => `${e.aide ? `<p class="hint">${esc(e.aide)}</p>` : ""}
      <label class="champ"><span>${esc(e.libelle)}</span><textarea data-etape="${esc(e.cle)}" maxlength="200" rows="3">${esc(choix.etapes[e.cle])}</textarea></label>
      <p class="hint">Noté dans les capacités de la feuille.</p>`,
    recap() {
      const pv = pvGagnes(), lignes = [];
      lignes.push(`<b>Niveau ${F.niveau} → ${niveau}</b>`);
      lignes.push(formule(M) ? `${signe(pv.total)} PV <span class="hint">(${esc(pv.detail)})</span>` : `${signe(pv)} PV`);
      lignes.push(`Attaques ${signe(1)} <span class="hint">(le niveau compte dans chaque attaque)</span>`);
      if (F.montee_pm) lignes.push(`${signe(F.montee_pm)} PM max`);
      F.voies.forEach((v, j) => { if (choix.vise[j] > v.rang) lignes.push(`${esc(v.nom)} : rang ${v.rang} → ${choix.vise[j]}${nouvelles(j).length ? ` <span class="hint">(${nouvelles(j).map(x => esc(x.c.nom || `rang ${x.r}`)).join(", ")})</span>` : ""}`); });
      if (parPoints) { const reste = dispo - depense(); lignes.push(`${depense()} point${pluriel(depense())} de capacité dépensé${pluriel(depense())}${M.report && reste ? `, ${reste} gardé${pluriel(reste)}` : !M.report && reste ? ` <span class="fp-epuise">(${reste} perdu${pluriel(reste)})</span>` : ""}`); }
      for (const e of extra) lignes.push(`${esc(e.libelle)} : ${esc(choix.etapes[e.cle])}`);
      return `<ul class="mn-recap">${lignes.map(l => `<li>${l}</li>`).join("")}</ul>`;
    },
  };

  const { voile, fermer } = dialogue(joueur ? `Passer au niveau ${niveau}` : `${nom} passe au niveau ${niveau}`, `
    <form class="mn" id="mn" novalidate>
      <div class="mn-fil" id="mn-fil"></div>
      <div id="mn-corps"></div>
      <p class="alerte" id="mn-err" hidden></p>
      <div class="ligne"><button type="button" class="ghost" id="mn-non">${joueur ? "Plus tard" : "Annuler"}</button><span class="sp"></span>
        <button type="button" id="mn-prec">← Précédent</button><button class="primary" type="submit" id="mn-oui"></button></div>
    </form>`);
  // fermer, d'une façon ou d'une autre (bouton, clic à côté, Échap), c'est remettre à plus tard
  const quitter = () => { fermer(); if (joueur) plusTard?.(); };
  voile.onclick = e => { if (e.target === voile) quitter(); };
  voile.onkeydown = e => { if (e.key === "Escape") quitter(); };
  $("#mn-non", voile).onclick = quitter;
  const err = $("#mn-err", voile), boite = $("#mn-corps", voile);
  const erreurDe = m => { err.textContent = m; err.hidden = !m; };

  // ce qui empêche de passer à la fenêtre suivante
  function bloquant() {
    const e = etapes[i];
    if (e.cle === "des") {
      if (!formule(M)) return choix.pv === "" || +choix.pv < 0 || +choix.pv > 999 ? "Entrez les PV gagnés (0 à 999)." : "";
      if (nbDes && !faces) return "La feuille n'a pas de dé de vie.";
      if (choix.des.some(v => v === "" || !Number.isInteger(+v) || +v < 1 || +v > faces)) return `Entrez le résultat du dé de vie : de 1 à ${faces}.`;
    }
    if (e.cle === "rangs" && parPoints && depense() > dispo) return `Trop de rangs : ${depense()} points pour ${dispo}.`;
    if (e.cle === "etape" && !choix.etapes[e.etape.cle].trim()) return `${e.etape.libelle} : à remplir.`;
    return "";
  }
  function majDes() {
    const t = $("#mn-total", voile);
    if (t) { const pv = pvGagnes(); t.innerHTML = choix.des.every(v => v !== "") ? `PV gagnés : <b>${pv.total}</b> <span class="hint">(${esc(pv.detail)})</span>` : ""; }
  }
  // le tableau des voies, comme sur la feuille : une colonne par voie, une ligne par rang ; les rangs
  // acquis en surbrillance, ceux choisis à ce niveau marqués, ceux qu'interdit le niveau grisés
  function grille() {
    const parBloc = boite.clientWidth && boite.clientWidth < 520 ? 2 : 3;
    const caps = F.voies.map(capacites);
    let html = "";
    for (let d = 0; d < F.voies.length; d += parBloc) {
      const idx = [...Array(parBloc).keys()].map(k => d + k);
      const nb = Math.max(...idx.filter(j => j < F.voies.length).map(j => rangMax(F.voies[j])));
      const cellule = (j, r) => {
        if (j >= F.voies.length) return `<div class="fp-vcase vide"></div>`;
        const v = F.voies[j], c = caps[j][r] || {}, rang = r + 1;
        if (rang > rangMax(v)) return `<div class="fp-vcase vide"></div>`;
        const etat = rang <= v.rang ? "acquise" : rang <= choix.vise[j] ? "choisie" : niveauPour(M, rang) > niveau ? "bloquee" : "";
        const aide = [c.nom || `Rang ${rang}`, c.texte, etat === "bloquee" ? `Demande le niveau ${niveauPour(M, rang)}.` : parPoints && etat !== "acquise" ? `${coutRang(M, v.origine, rang)} point${pluriel(coutRang(M, v.origine, rang))}` : ""].filter(Boolean).join("\n");
        return `<button type="button" class="fp-vcase ${etat}" data-v="${j}" data-r="${r}" title="${esc(aide)}"${etat === "acquise" || etat === "bloquee" ? ` aria-disabled="true"` : ""}>${esc(c.nom || "")}</button>`;
      };
      html += `<div class="fp-vbloc" style="grid-template-columns:1.8em repeat(${idx.length},minmax(0,1fr))">
        <div class="fp-vcoin"></div>${idx.map(j => `<div class="fp-vtete">${j < F.voies.length ? esc(F.voies[j].nom) : ""}</div>`).join("")}
        <div class="fp-vcoin2"></div>${idx.map(j => `<div class="fp-vsous">${j < F.voies.length ? `rang ${F.voies[j].rang}${choix.vise[j] > F.voies[j].rang ? ` → <b>${choix.vise[j]}</b>` : ""}` : ""}</div>`).join("")}
        ${[...Array(nb).keys()].map(r => `<div class="fp-vrang">${r + 1}</div>${idx.map(j => cellule(j, r)).join("")}`).join("")}
      </div>`;
    }
    return `<div class="fp-voies">${html}</div>`;
  }
  function majRangs() {
    const g = $("#mn-grille", voile);
    if (g) g.innerHTML = grille();
    const p = $("#mn-points", voile);
    if (p) { p.textContent = `${depense()} / ${dispo} point${pluriel(dispo)}`; p.classList.toggle("fp-epuise", depense() > dispo); }
  }
  // une case touchée : on prend ce rang (et ceux d'avant), ou on y renonce s'il était choisi
  function toucher(j, r) {
    const v = F.voies[j], rang = r + 1, c = capacites(v)[r] || {};
    const d = $("#mn-detail", voile);
    if (d) d.innerHTML = `<b>${esc(c.nom || `Rang ${rang}`)}</b>${c.texte ? ` : ${esc(c.texte)}` : ""}`;
    if (rang <= v.rang) return;
    if (rang <= choix.vise[j]) choix.vise[j] = rang - 1;
    else {
      const bloque = [...Array(rang - v.rang).keys()].map(k => v.rang + k + 1).find(x => niveauPour(M, x) > niveau);
      if (bloque) { erreurDe(`Le rang ${bloque} demande le niveau ${niveauPour(M, bloque)}.`); return; }
      choix.vise[j] = rang;
    }
    erreurDe("");
    majRangs();
  }
  function peindre() {
    const e = etapes[i];
    $("#mn-fil", voile).innerHTML = `<span class="hint">Étape ${i + 1} sur ${etapes.length}</span> <b>${esc(e.titre)}</b>`;
    boite.innerHTML = e.cle === "etape" ? corps.etape(e.etape) : corps[e.cle]();
    $("#mn-prec", voile).hidden = i === 0;
    $("#mn-oui", voile).textContent = e.cle === "recap" ? `Valider le niveau ${niveau}` : "Suivant →";
    erreurDe("");
    boite.oninput = () => erreurDe(""); // une correction efface le message
    const bAccord = $("#mn-accorder", voile);
    if (bAccord) bAccord.onclick = async () => { if (await accorderNiveau(id, nom, !accorde, F.modifie, apres)) fermer(); };
    $$("[data-dv]", voile).forEach(x => x.oninput = () => { choix.des[+x.dataset.dv] = x.value; majDes(); });
    const pv = $("#mn-pv", voile);
    if (pv) pv.oninput = () => { choix.pv = pv.value; };
    const g = $("#mn-grille", voile);
    if (g) g.onclick = ev => { const x = ev.target.closest("[data-v][data-r]"); if (x) toucher(+x.dataset.v, +x.dataset.r); };
    $$("[data-etape]", voile).forEach(x => x.oninput = () => { choix.etapes[x.dataset.etape] = x.value; });
    majDes(); majRangs();
    ($("input, textarea", boite) || $("#mn-oui", voile)).focus();
  }
  $("#mn-prec", voile).onclick = () => { if (i > 0) { i--; peindre(); } };
  $("#mn", voile).onsubmit = async ev => {
    ev.preventDefault();
    const b = bloquant();
    if (b) { erreurDe(b); return; }
    if (etapes[i].cle !== "recap") { i++; peindre(); return; }
    const envoi = { rangs: F.voies.map((v, j) => ({ voie: j, rang: choix.vise[j] })).filter((x, j) => x.rang > F.voies[j].rang), modifie: F.modifie || undefined };
    if (formule(M)) envoi.des = choix.des.map(Number); else envoi.pv = +choix.pv;
    if (extra.length) envoi.etapes = Object.fromEntries(extra.map(e => [e.cle, choix.etapes[e.cle].trim()]));
    $("#mn-oui", voile).disabled = true;
    try {
      const r = await api("POST", joueur ? `/api/joueur/pj/${id}/niveau` : `/api/feuilles/${id}/niveau`, envoi);
      fermer();
      toast(`${joueur ? "Vous passez" : `${nom} passe`} au niveau ${r.niveau} : ${texteMontee(r.montee)}.`);
      apres?.(r);
    } catch (x) {
      erreurDe(x.status === 409 ? "La feuille a été modifiée ailleurs : fermez et recommencez." : x.message);
      $("#mn-oui", voile).disabled = false;
    }
  };
  peindre();
}

/** Accorde au PJ `id` le niveau suivant (`oui`), ou retire l'accord ; `apres(feuille)` une fois fait. */
export async function accorderNiveau(id, nom, oui, modifie, apres) {
  try {
    const r = await api("POST", `/api/feuilles/${id}/niveau/accorder`, { accorde: oui, modifie: modifie || undefined });
    toast(oui ? `Niveau ${r.niveau_accorde} accordé : ${nom} fera ses choix depuis sa page.` : "Accord retiré.");
    apres?.(r);
    return true;
  } catch (e) { toast(e.message, true); return false; }
}

/** Annule la dernière montée du PJ `id` ; `apres(feuille)` une fois faite. */
export async function annulerMontee(id, modifie, apres) {
  try {
    const r = await api("POST", `/api/feuilles/${id}/niveau/annuler`, { modifie: modifie || undefined });
    toast(`Montée au niveau ${r.annulee.niveau} annulée : retour au niveau ${r.niveau}.`);
    apres?.(r);
  } catch (e) { toast(e.message, true); }
}
