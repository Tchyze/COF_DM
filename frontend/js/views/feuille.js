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
  api("GET", `/api/reference?pj=${fiche.id}`).then(r => { REF = r; }, () => {});
  const R = () => F.regles || {};
  const pvActuel = () => F.pv_actuel ?? F.pv;

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

  // ---------------------------------------------------------------- lecture
  function lire() {
    const compteur = (k, nom) => (F[k + "_max"] || F[k]) ? `<span class="fp-res" data-res="${k}"><b>${nom}</b> ${peutEcrire ? `<button class="ghost petit" data-d="-1" aria-label="${nom} −1">−</button>` : ""}<span>${F[k]}/${F[k + "_max"]}</span>${peutEcrire ? `<button class="ghost petit" data-d="1" aria-label="${nom} +1">+</button>` : ""}</span>` : "";
    const identite = [["Sexe", F.sexe], ["Âge", F.age], ["Taille", F.taille_physique], ["Poids", F.poids], ["Divinité", F.divinite], ["Langues", F.langues]].filter(x => x[1]);
    const bourse = ["po", "pa", "pc"].filter(k => F.bourse?.[k]).map(k => `${F.bourse[k]} ${k}`).join(", ");
    box.innerHTML = `<h3>Feuille de personnage <span class="sp"></span><span class="hint" style="text-transform:none;letter-spacing:0">niveau ${esc(F.niveau)}</span>
        ${peutEcrire ? `<button class="petit" id="fp-modifier">✎ Modifier</button>` : ""}</h3>
      ${F.existe ? "" : `<p class="vide">Feuille vierge${peutEcrire ? " : cliquez sur « Modifier » pour la remplir" : ""}.</p>`}
      <div class="bloc-combat">
        <div class="ligne" style="gap:6px 14px">
          <span class="fp-res" data-res="pv"><b>PV</b> ${peutEcrire ? `<button class="ghost petit" data-d="-1" aria-label="PV −1">−</button>` : ""}<span>${pvActuel()}/${F.pv}</span>${peutEcrire ? `<button class="ghost petit" data-d="1" aria-label="PV +1">+</button>` : ""}</span>
          <span><b>DEF</b> ${F.defense}</span><span><b>Init</b> ${F.init}</span>${F.rd ? `<span><b>RD</b> ${F.rd}</span>` : ""}${F.dv ? `<span><b>DV</b> ${esc(F.dv)}</span>` : ""}
          ${compteur("pc", "PC")}${compteur("pr", "PR")}${compteur("pm", "PM")}
        </div>
        <div class="carac">${CARACS.map(k => `<span><small>${k}</small>${F.valeurs[k]} (${signe(F.carac[k])})</span>`).join("")}</div>
        ${F.attaques.length ? `<ul class="attaques">${F.attaques.map(a => `<li><b>${esc(a.nom)}</b> ${signe(a.bonus)}${a.dm ? `, DM ${esc(a.dm)}` : ""}${a.portee ? ` · ${esc(a.portee)}` : ""}${a.notes ? ` <span class="hint">${esc(a.notes)}</span>` : ""}</li>`).join("")}</ul>` : ""}
        ${peutEcrire && R().repos?.length ? `<div class="ligne">${R().repos.map((r, i) => `<button class="petit" data-repos="${i}" title="${esc(r.aide || "")}">${esc(r.libelle)}</button>`).join("")}</div>` : ""}
      </div>
      ${F.voies.length ? `<h3 style="margin-top:12px">Voies</h3><div class="colonne" style="gap:6px">${F.voies.map(v => `<div><b>${esc(v.nom)}</b> <span class="hint">${esc(v.origine || "")}${v.origine ? " · " : ""}rang ${v.rang}</span>
          ${v.rang ? `<div class="hint">${v.capacites.slice(0, v.rang).filter(Boolean).map((c, i) => `${i + 1}. ${esc(c)}`).join(" · ")}</div>` : ""}</div>`).join("")}</div>` : ""}
      ${F.capacites ? `<h3 style="margin-top:12px">Capacités</h3><div class="bst-texte">${esc(F.capacites)}</div>` : ""}
      ${F.equipement || bourse || F.armure?.nom || F.bouclier?.nom ? `<h3 style="margin-top:12px">Équipement</h3>
        ${[F.armure?.nom && `${F.armure.nom} (${signe(F.armure.bonus)} DEF)`, F.bouclier?.nom && `${F.bouclier.nom} (${signe(F.bouclier.bonus)} DEF)`].filter(Boolean).map(x => `<div>${esc(x)}</div>`).join("")}
        ${F.equipement ? `<div class="bst-texte">${esc(F.equipement)}</div>` : ""}${bourse ? `<p class="hint">Bourse : ${esc(bourse)}</p>` : ""}` : ""}
      ${identite.length ? `<dl class="proprietes" style="margin-top:12px">${identite.map(([n, v]) => `<div><dt>${n}</dt><dd>${esc(v)}</dd></div>`).join("")}</dl>` : ""}
      ${!joueur && F.notes ? `<h3 style="margin-top:12px">Notes du MJ</h3><div class="bst-texte">${esc(F.notes)}</div>` : ""}`;
    if (!peutEcrire) return;
    $("#fp-modifier", box).onclick = editer;
    // PV et ressources : un clic, enregistré aussitôt
    $$(".fp-res button", box).forEach(b => b.onclick = async () => {
      const k = b.closest(".fp-res").dataset.res, d = +b.dataset.d;
      const f = structuredClone(F);
      if (k === "pv") f.pv_actuel = Math.max(0, Math.min(F.pv, pvActuel() + d));
      else f[k] = Math.max(0, Math.min(F[k + "_max"], F[k] + d));
      if (await enregistrer(f)) lire();
    });
    $$("[data-repos]", box).forEach(b => b.onclick = () => reposer(R().repos[+b.dataset.repos]));
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
      f.pv_actuel = Math.min(F.pv, avant + gain);
      detail = ` : ${termes.map(x => x[0]).join(" ")} = ${gain} PV (${avant} → ${f.pv_actuel})`;
    }
    if (r.pm_max) f.pm = F.pm_max;
    if (await enregistrer(f, `${r.libelle}${detail}`)) lire();
  }

  // ---------------------------------------------------------------- édition
  function editer() {
    const f = structuredClone(F), rg = R();
    const nombre = (nom, chemin, plus = "") => `<label class="champ"><span>${nom}</span><input type="number" data-p="${chemin}" value="${esc(chemin.split(".").reduce((x, k) => x?.[k], f) ?? 0)}" ${plus}></label>`;
    const texte = (nom, chemin, plus = "") => `<label class="champ"><span>${nom}</span><input data-p="${chemin}" value="${esc(chemin.split(".").reduce((x, k) => x?.[k], f) ?? "")}" ${plus}></label>`;
    const choix = (nom, chemin, liste, valeur) => liste?.length
      ? `<label class="champ"><span>${nom}</span><select data-p="${chemin}">${liste.map(x => `<option${x === valeur ? " selected" : ""}>${esc(x)}</option>`).join("")}</select></label>`
      : texte(nom, chemin);
    const profil = REF.profils.find(p => cle(p.nom) === cle(fiche.data.profil));
    box.innerHTML = `<h3>Feuille de personnage</h3>
      <form id="fp-form" class="colonne" style="gap:12px" novalidate>
        <div class="bst-grille">${CARACS.map(k => nombre(k, "valeurs." + k)).join("")}</div>
        <div class="bst-grille">
          ${nombre("PV max", "pv", 'min="1"')}${choix("Dé de vie", "dv", rg.des_de_vie, f.dv)}${nombre("RD", "rd", 'min="0"')}${nombre("Critique sur", "crit", 'min="2" max="20"')}
          ${nombre("PC max", "pc_max", 'min="0"')}${nombre("PR max", "pr_max", 'min="0"')}${nombre("PM max", "pm_max", 'min="0"')}
          ${choix("Carac. de magie", "carac_magie", rg.caracs_magie?.length ? rg.caracs_magie : CARACS, f.carac_magie)}
        </div>
        <h3>Défense et initiative</h3>
        <div class="bst-grille">
          ${texte("Armure", "armure.nom")}${nombre("Bonus d'armure", "armure.bonus", 'min="0"')}${texte("Bouclier", "bouclier.nom")}${nombre("Bonus de bouclier", "bouclier.bonus", 'min="0"')}
          ${nombre("DEF, divers", "def_divers")}${nombre("Init, divers", "init_divers")}
        </div>
        <h3>Attaques</h3>
        <div class="bst-grille">${nombre("Contact, divers", "att_divers.C")}${nombre("Distance, divers", "att_divers.D")}${nombre("Magique, divers", "att_divers.M")}</div>
        <div class="colonne" style="gap:6px" id="fp-atks"></div>
        <div><button type="button" class="petit" id="fp-atk">＋ Attaque</button></div>
        <p class="hint">Le bonus d'attaque est calculé : niveau + modificateur + divers + bonus propre à l'arme.</p>
        <h3>Voies</h3>
        <div class="colonne" style="gap:10px" id="fp-voies"></div>
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
    const blocVoie = v => `<div class="fp-voie" data-ref="${esc(v.ref || "")}">
        <div class="ligne"><input data-k="nom" value="${esc(v.nom)}" placeholder="Nom de la voie" aria-label="Nom de la voie" style="flex:1">
          ${rg.origines_voie?.length ? `<select data-k="origine" aria-label="Origine">${rg.origines_voie.map(o => `<option${o === v.origine ? " selected" : ""}>${esc(o)}</option>`).join("")}</select>` : `<input data-k="origine" value="${esc(v.origine)}" placeholder="Origine" aria-label="Origine">`}
          <label class="hint">Rang <input data-k="rang" type="number" min="0" value="${esc(v.rang)}" style="width:60px"></label>
          <button type="button" class="ghost petit" data-retirer aria-label="Retirer">✕</button></div>
        <textarea data-k="capacites" rows="${Math.max(2, (v.capacites || []).length)}" placeholder="Une capacité par ligne, dans l'ordre des rangs" aria-label="Capacités">${esc((v.capacites || []).join("\n"))}</textarea></div>`;
    const depuisRef = v => ({ nom: v.nom, origine: v.type || "", rang: 0, capacites: (v.rangs || []).map(nomCap), ref: v.ref || "" });
    atks.innerHTML = f.attaques.map(ligneAtk).join("");
    voies.innerHTML = f.voies.map(blocVoie).join("");
    const retirer = e => { const b = e.target.closest("[data-retirer]"); if (b) b.closest(".fp-atk, .fp-voie").remove(); };
    atks.onclick = retirer; voies.onclick = retirer;
    $("#fp-atk", box).onclick = () => atks.insertAdjacentHTML("beforeend", ligneAtk({ nom: "", type: "C", mod: 0, dm: "", portee: "", notes: "" }));
    $("#fp-voie", box).onclick = () => voies.insertAdjacentHTML("beforeend", blocVoie({ nom: "", origine: rg.origines_voie?.[0] || "", rang: 0, capacites: [] }));
    const selRef = $("#fp-voie-ref", box);
    if (selRef) selRef.onchange = () => { if (selRef.value !== "") voies.insertAdjacentHTML("beforeend", blocVoie(depuisRef(REF.voies[+selRef.value]))); selRef.value = ""; };
    const btnProfil = $("#fp-profil", box);
    if (btnProfil) btnProfil.onclick = () => {
      // le profil de la bibliothèque donne son dé de vie, sa caractéristique de magie et ses voies (au rang 0)
      for (const [champ, v] of [["dv", profil.dv], ["carac_magie", profil.carac_magie]]) { const i = $(`[data-p="${champ}"]`, box); if (v && i) i.value = v; }
      const deja = new Set($$(".fp-voie [data-k=nom]", voies).map(i => cle(i.value)));
      let n = 0;
      for (const nom of profil.voies || []) {
        const v = REF.voies.find(x => cle(x.nom) === cle(nom));
        if (!deja.has(cle(nom))) { voies.insertAdjacentHTML("beforeend", blocVoie(v ? depuisRef(v) : { nom, origine: rg.origines_voie?.[0] || "", rang: 0, capacites: [] })); n++; }
      }
      toast(`Profil appliqué : ${n} voie${n > 1 ? "s" : ""} ajoutée${n > 1 ? "s" : ""}. Enregistrez pour valider.`);
    };
    $("#fp-annuler", box).onclick = lire;
    $("#fp-form", box).onsubmit = async e => {
      e.preventDefault();
      for (const i of $$("[data-p]", box)) {
        const ks = i.dataset.p.split("."), dernier = ks.pop();
        const cible = ks.reduce((x, k) => (x[k] ??= {}), f);
        cible[dernier] = i.type === "number" ? (+i.value || 0) : i.value;
      }
      f.attaques = $$(".fp-atk", atks).map(r => ({ nom: $("[data-k=nom]", r).value.trim(), type: $("[data-k=type]", r).value, mod: +$("[data-k=mod]", r).value || 0,
        dm: $("[data-k=dm]", r).value.trim(), portee: $("[data-k=portee]", r).value.trim(), notes: $("[data-k=notes]", r).value.trim() }));
      f.voies = $$(".fp-voie", voies).map(r => ({ nom: $("[data-k=nom]", r).value.trim(), origine: $("[data-k=origine]", r).value.trim(), rang: +$("[data-k=rang]", r).value || 0,
        capacites: $("[data-k=capacites]", r).value.split("\n").map(s => s.trim()), ref: r.dataset.ref || undefined })).filter(v => v.nom);
      for (const v of f.voies) while (v.capacites.length && !v.capacites.at(-1)) v.capacites.pop();
      if (f.pv_actuel != null) f.pv_actuel = Math.min(f.pv_actuel, f.pv);
      if (await enregistrer(f, "Feuille enregistrée.")) lire();
    };
  }

  lire();
}
