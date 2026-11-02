// Fenêtre « Mes univers » de l'application de bureau : les univers de cet ordinateur et leurs actions
// (créer, ouvrir, renommer, archiver, désarchiver, supprimer), et ce qui porte sur l'univers ouvert
// (copie, sauvegardes, export, serveur). Sans univers ouvert, elle tient lieu d'écran d'accueil. Sur
// un serveur, rien de tout cela : voir fichier.js.
import { actif } from "./schema.js";
import { $, esc, api, toast, choisirUniversOnglet, dialogue, taille } from "./ui.js";
import { ouvrirPanneau } from "./connexion.js";
import { dialogueObsidian } from "./fichier.js";

const GESTION = "/api/univers/gestion";

function recharger() {
  choisirUniversOnglet("");
  location.hash = "#/";
  location.reload();
}

const pluriel = (n, s, p) => `${n} ${n > 1 ? p : s}`;

/** Boîte de dialogue du système ; renvoie "" si le MJ annule ou s'il n'y en a pas. */
async function parcourir(enregistrer, nom = "") {
  try { return (await api("POST", "/api/univers/parcourir", { enregistrer, nom })).chemin; }
  catch (e) { toast(e.message, true); return ""; }
}

async function ouvrir(chemin) {
  try { await api("POST", "/api/univers/ouvrir", { chemin }); recharger(); }
  catch (e) { toast(e.message, true); }
}

async function creer(corps) {
  try { await api("POST", "/api/univers/nouveau", corps); recharger(); }
  catch (e) { toast(e.message, true); }
}

async function dialogueOuvrir(etat) {
  const { voile, fermer } = dialogue("Ouvrir un fichier", `
    <label class="champ"><span>Fichier d'univers <small>(.cofdm)</small></span><input id="u-chemin" placeholder="${esc(etat.dossier)}" autocomplete="off" spellcheck="false"></label>
    <div class="ligne">${etat.parcourir ? `<button id="u-parcourir">Parcourir…</button>` : ""}<button class="primary" id="u-ouvrir">Ouvrir</button><button class="ghost" id="u-annuler">Annuler</button></div>`, { piege: true });
  const champ = $("#u-chemin", voile);
  $("#u-annuler", voile).onclick = fermer;
  $("#u-ouvrir", voile).onclick = () => { if (champ.value.trim()) ouvrir(champ.value.trim()); else champ.focus(); };
  champ.onkeydown = e => { if (e.key === "Enter") $("#u-ouvrir", voile).click(); };
  const bouton = $("#u-parcourir", voile);
  if (bouton) bouton.onclick = async () => { const c = await parcourir(false); if (c) ouvrir(c); };
  champ.focus();
}

function dialogueNouveau(etat) {
  const { voile, fermer } = dialogue("Nouvel univers", `
    <p class="hint">Un univers est un fichier : fiches, images et journal y sont rangés ensemble. L'univers ouvert en ce moment n'est pas modifié.</p>
    <label class="champ"><span>Nom de l'univers</span><input id="u-nom" autocomplete="off" maxlength="80"></label>
    <p class="hint">Créé dans <code>${esc(etat.dossier)}</code>${etat.parcourir ? ", ou à l'emplacement de votre choix" : ""}.</p>
    <div class="ligne"><button class="primary" id="u-creer">Créer</button>${etat.parcourir ? `<button id="u-ou">Choisir l'emplacement…</button>` : ""}<button class="ghost" id="u-annuler">Annuler</button></div>`, { piege: true });
  const champ = $("#u-nom", voile);
  $("#u-annuler", voile).onclick = fermer;
  $("#u-creer", voile).onclick = () => { if (champ.value.trim()) creer({ nom: champ.value.trim() }); else champ.focus(); };
  champ.onkeydown = e => { if (e.key === "Enter") $("#u-creer", voile).click(); };
  const bouton = $("#u-ou", voile);
  if (bouton) bouton.onclick = async () => { const c = await parcourir(true, champ.value.trim() || "Nouvel univers"); if (c) creer({ chemin: c }); };
  champ.focus();
}

async function enregistrerCopie() {
  const etat = await api("GET", "/api/univers");
  const copier = async (chemin, remplacer) => {
    const r = await api("POST", "/api/univers/copie", { chemin, remplacer });
    toast("Copie enregistrée : " + r.chemin);
  };
  if (etat.parcourir) {
    // la boîte du système a déjà demandé s'il fallait remplacer un fichier existant
    const c = await parcourir(true, etat.courant.nom + " (copie)");
    if (c) await copier(c, true).catch(e => toast(e.message, true));
    return;
  }
  const { voile, fermer } = dialogue("Enregistrer une copie", `
    <p class="hint">Une copie complète de « ${esc(etat.courant.nom)} », à ranger où vous voulez. L'univers ouvert reste le même.</p>
    <label class="champ"><span>Fichier de la copie</span><input id="u-chemin" autocomplete="off" spellcheck="false"></label>
    <div class="ligne"><button class="primary" id="u-copier">Enregistrer</button><button class="ghost" id="u-annuler">Annuler</button></div>`, { piege: true });
  const champ = $("#u-chemin", voile), bouton = $("#u-copier", voile);
  champ.value = etat.courant.chemin.replace(/\.cofdm$/i, "") + " (copie).cofdm";
  let remplacer = false;
  champ.oninput = () => { remplacer = false; bouton.textContent = "Enregistrer"; };
  $("#u-annuler", voile).onclick = fermer;
  bouton.onclick = async () => {
    try { await copier(champ.value.trim(), remplacer); fermer(); }
    catch (e) {
      if (e.status === 409) { remplacer = true; bouton.textContent = "Remplacer le fichier existant"; }
      toast(e.message, true);
    }
  };
  champ.focus();
}

async function sauvegarder() {
  try { const r = await api("POST", "/api/sauvegarde"); toast("Sauvegarde : " + r.fichier); }
  catch (e) { toast(e.message, true); }
}

/** Ce que l'action fait sur une copie reliée : le serveur n'est jamais touché. */
const NOTE_COPIE = {
  renommer: "Sur cet ordinateur seulement : le nom sur le serveur ne change pas, et la synchronisation continue.",
  archiver: "Le serveur n'est pas touché. La copie ne se synchronise plus jusqu'au désarchivage ; ses changements attendent dans le fichier.",
  supprimer: "Seule la copie de cet ordinateur est supprimée : le serveur garde son univers, et vous pourrez en tirer une nouvelle copie.",
};

const hote = url => { try { return new URL(url).host; } catch { return url || ""; } };

/** Les risques du bilan, à lire et à confirmer. */
function risques(b, action) {
  const garde = action === "archiver";
  const l = [];
  if (b.attente) l.push(`${pluriel(b.attente, "changement n'est pas encore envoyé", "changements ne sont pas encore envoyés")} au serveur : ${garde ? "ils attendront dans le fichier" : "ils ne resteront que dans la sauvegarde"}.`);
  if (b.conflits) l.push(`${pluriel(b.conflits, "conflit attend", "conflits attendent")} votre choix : ${garde ? "ils attendront dans le fichier" : "ils ne resteront que dans la sauvegarde"}.`);
  if (b.recus) l.push(`${pluriel(b.recus, "objet reçu du serveur attend", "objets reçus du serveur attendent")} une mise à jour de l'application.`);
  if (b.combats) l.push(`${pluriel(b.combats, "combat lancé ici n'est pas terminé", "combats lancés ici ne sont pas terminés")} : ${garde ? "ils restent dans le fichier" : "ils ne resteront que dans la sauvegarde"}.`);
  if (b.inconnu) l.push(`Ce fichier ne se laisse pas lire : impossible de dire ce qu'il contient${garde ? "" : ", et sa sauvegarde n'en sera qu'une simple copie"}.`);
  return l;
}

const aRisque = b => risques(b, "supprimer").length > 0;

/** La partie commune d'archiver et de supprimer : note de copie, bilan, univers suivant, case à cocher. */
function blocAvertissements(f, info, action) {
  return `
    ${f.copie ? `<p class="alerte">${esc(NOTE_COPIE[action])} <small>(${esc(hote(f.copie.url))})</small></p>` : ""}
    <div id="u-risques"></div>
    ${info.bilan.direct ? `<p class="hint">${pluriel(info.bilan.direct, "combat joué en direct continue", "combats joués en direct continuent")} sur le serveur : seul l'écran de ce poste s'arrête.</p>` : ""}
    ${f.ouvert ? (info.suivants.length
      ? `<label class="champ"><span>Univers à ouvrir ensuite</span><select id="u-suivant">${info.suivants.map(s => `<option value="${esc(s.cle)}">${esc(s.nom)}</option>`).join("")}</select></label>`
      : `<p class="hint">C'est votre dernier univers : l'application vous proposera d'en créer un.</p>`) : ""}`;
}

/** Montre les risques et rend la case à cocher ; renvoie la fonction qui les met à jour. */
function brancherRisques(voile, action, maj) {
  const zone = $("#u-risques", voile);
  return b => {
    const l = risques(b, action);
    zone.innerHTML = l.length ? `<div class="alerte"><ul class="uv-risques">${l.map(x => `<li>${esc(x)}</li>`).join("")}</ul></div>
      <label class="t-coche"><input type="checkbox" id="u-accepte"><span>J'ai compris, continuer quand même</span></label>` : "";
    const c = $("#u-accepte", zone);
    if (c) c.onchange = maj;
    maj();
  };
}

/** Une boîte dont le bilan est calculé d'abord : compter peut prendre un moment sur un gros fichier. */
async function bilanDe(f) {
  return api("POST", `${GESTION}/${encodeURIComponent(f.cle)}/bilan`);
}

function dialogueRenommer(f, apres) {
  const { voile, fermer } = dialogue(`Renommer « ${f.nom} »`, `
    ${f.copie ? `<p class="alerte">${esc(NOTE_COPIE.renommer)}</p>` : ""}
    <label class="champ"><span>Nouveau nom</span><input id="u-nom" autocomplete="off" maxlength="80" value="${esc(f.nom)}"></label>
    <p class="hint">Le fichier change de nom, à la même place. Les sauvegardes et les récents suivent.</p>
    <div class="ligne"><button class="primary" id="u-ok">Renommer</button><button class="ghost" id="u-annuler">Annuler</button></div>`, { piege: true });
  const champ = $("#u-nom", voile), ok = $("#u-ok", voile);
  $("#u-annuler", voile).onclick = fermer;
  ok.onclick = async () => {
    ok.disabled = true;
    try {
      await api("POST", `${GESTION}/${encodeURIComponent(f.cle)}/renommer`, { nom: champ.value });
      if (f.ouvert) { recharger(); return; }
      fermer();
      apres();
    } catch (e) { ok.disabled = false; toast(e.message, true); champ.focus(); }
  };
  champ.onkeydown = e => { if (e.key === "Enter") ok.click(); };
  champ.select();
}

async function dialogueArchiver(f, apres) {
  const info = await bilanDe(f);
  const { voile, fermer } = dialogue(`Archiver « ${f.nom} »`, `
    <p class="hint">L'univers sort de la liste courante et ne s'ouvre plus jusqu'au désarchivage. Le fichier reste entier, sans changement, dans le dossier des archives.</p>
    ${blocAvertissements(f, info, "archiver")}
    <div class="ligne"><button class="primary" id="u-ok">Archiver</button><button class="ghost" id="u-annuler">Annuler</button></div>`, { piege: true });
  const ok = $("#u-ok", voile);
  let bilan = info.bilan;
  const maj = () => { ok.disabled = aRisque(bilan) && !$("#u-accepte", voile)?.checked; };
  const montrer = brancherRisques(voile, "archiver", maj);
  montrer(bilan);
  $("#u-annuler", voile).onclick = fermer;
  ok.onclick = async () => {
    ok.disabled = true;
    try {
      const r = await api("POST", `${GESTION}/${encodeURIComponent(f.cle)}/archiver`, { suivant: $("#u-suivant", voile)?.value || "", bilan_vu: bilan });
      if (f.ouvert) { recharger(); return; }
      toast(`« ${f.nom} » est archivé.`);
      fermer();
      apres(r);
    } catch (e) {
      toast(e.message, true);
      if (e.data?.bilan) { bilan = e.data.bilan; montrer(bilan); } else maj();
    }
  };
}

async function dialogueSupprimer(f, apres) {
  const info = await bilanDe(f);
  const { voile, fermer } = dialogue(`Supprimer « ${f.nom} »`, `
    <p class="hint">Une sauvegarde est faite avant. Si elle échoue, rien n'est supprimé.</p>
    ${blocAvertissements(f, info, "supprimer")}
    <label class="champ"><span>Retapez le nom de l'univers pour confirmer</span><input id="u-nom" autocomplete="off" spellcheck="false"></label>
    <div class="ligne"><button class="danger" id="u-ok" disabled>Supprimer</button><button class="ghost" id="u-annuler">Annuler</button></div>`, { piege: true });
  const ok = $("#u-ok", voile), champ = $("#u-nom", voile);
  let bilan = info.bilan;
  const maj = () => { ok.disabled = champ.value.trim() !== f.nom || (aRisque(bilan) && !$("#u-accepte", voile)?.checked); };
  const montrer = brancherRisques(voile, "supprimer", maj);
  montrer(bilan);
  champ.oninput = maj;
  $("#u-annuler", voile).onclick = fermer;
  ok.onclick = async () => {
    ok.disabled = true;
    try {
      const r = await api("POST", `${GESTION}/${encodeURIComponent(f.cle)}/supprimer`, { nom: champ.value, suivant: $("#u-suivant", voile)?.value || "", bilan_vu: bilan });
      fermer();
      resultatSuppression(f, r, apres);
    } catch (e) {
      toast(e.message, true);
      if (e.data?.bilan) { bilan = e.data.bilan; montrer(bilan); } else maj();
    }
  };
  champ.focus();
}

/** Où est la sauvegarde, et comment effacer vraiment. */
function resultatSuppression(f, r, apres) {
  const { voile, fermer } = dialogue(`« ${f.nom} » est supprimé`, `
    <p class="hint">Sa sauvegarde${r.verifiee ? "" : ", une simple copie du fichier abîmé, non vérifiée,"} se trouve ici :</p>
    <p><code class="uv-chemin">${esc(r.sauvegarde)}</code></p>
    <p class="hint">Pour l'effacer vraiment, supprimez vous-même le dossier <code class="uv-chemin">${esc(r.dossier_sauvegardes)}</code> : il contient aussi ses sauvegardes automatiques. Sans cela, cet univers reste sur votre ordinateur.</p>
    ${f.copie ? `<p class="hint">Le serveur <code>${esc(hote(f.copie.url))}</code> n'a pas été touché.</p>` : ""}
    <div class="ligne"><button class="primary" id="u-ok">Compris</button></div>`, { piege: true });
  const suite = () => { fermer(); if (f.ouvert) recharger(); else apres(r); };
  $("#u-ok", voile).onclick = suite;
  $("#u-ok", voile).focus();
}

function ligne(f, actions) {
  const details = [];
  if (f.copie) details.push(`🔗 Copie de ${esc(hote(f.copie.url))}${f.copie.nom && f.copie.nom !== f.nom ? ` (« ${esc(f.copie.nom)} » sur le serveur)` : ""}${f.copie.etat === "pause" ? ", en pause" : ""}`);
  if (f.lieu === "ailleurs" || f.lieu === "introuvable") details.push(`<span class="uv-chemin">${esc(f.chemin)}</span>`);
  if (f.lieu === "ailleurs") details.push("Hors du dossier des univers");
  if (f.interrompu) details.push("Une écriture a été interrompue : ouvrez-le une fois pour la réparer");
  if (f.abime) details.push("Ce fichier est abîmé : il ne s'ouvre plus");
  if (!f.gerable && !f.abime && !f.interrompu && f.lieu !== "introuvable") details.push("Ce fichier ne se gère pas ici");
  const boutons = actions.map(([a, texte, classe]) => `<button type="button" class="petit ${classe || ""}" data-a="${a}">${texte}</button>`).join("");
  return `<div class="uv-ligne" data-cle="${esc(f.cle)}">
    <div class="uv-tete">
      <span class="ic" aria-hidden="true">${f.ouvert ? "●" : f.lieu === "archive" ? "🗄" : "○"}</span>
      <span class="uv-nom"><b>${esc(f.nom)}</b>${f.ouvert ? `<span class="badge">ouvert</span>` : ""}</span>
      <small class="uv-taille">${f.lieu === "introuvable" ? "introuvable" : taille(f.taille)}</small>
      ${boutons ? `<button type="button" class="ghost petit uv-plus" aria-expanded="false" aria-label="Actions de ${esc(f.nom)}">⋯</button>` : ""}
    </div>
    ${details.length ? `<p class="hint uv-detail">${details.join(" · ")}</p>` : ""}
    ${boutons ? `<div class="uv-actions ligne">${boutons}</div>` : ""}
  </div>`;
}

const LIBELLES = { ouvrir: ["Ouvrir"], renommer: ["Renommer"], archiver: ["Archiver"], desarchiver: ["Désarchiver"], oublier: ["Oublier", "ghost"], supprimer: ["Supprimer", "danger"] };
const ligneDe = f => ligne(f, f.actions.map(a => [a, LIBELLES[a][0], LIBELLES[a][1]]));

/** La fenêtre. Sans univers ouvert (accueil), elle ne se ferme pas : il n'y a rien derrière. */
export async function fenetreUnivers({ bouton = null, accueil = false } = {}) {
  const etat = await api("GET", GESTION);
  const { voile, fermer } = dialogue("Mes univers", `<div id="uv-corps" class="colonne"></div>`, { piege: true, rendre: bouton, bloque: accueil, classe: "boite-univers", croix: !accueil });
  const corps = $("#uv-corps", voile);

  const peindre = async (donnees, garder) => {
    const d = donnees || await api("GET", GESTION);
    const courants = d.univers.filter(f => f.lieu === "dossier" || f.lieu === "ailleurs");
    const archives = d.univers.filter(f => f.lieu === "archive"), perdus = d.univers.filter(f => f.lieu === "introuvable");
    const ouvert = d.univers.find(f => f.ouvert);
    corps.innerHTML = `
      ${accueil ? `<p class="hint">Aucun univers n'est ouvert. Un univers est un fichier : fiches, images et journal y sont rangés ensemble.</p>
        <div class="uv-creer">
          <label class="champ"><span>Créez votre univers</span><input id="uv-nom" autocomplete="off" maxlength="80" placeholder="Nom de l'univers"></label>
          <button class="primary" id="uv-creer">Créer</button>
        </div>` : ""}
      <div class="ligne">
        ${accueil ? "" : `<button type="button" class="primary" data-g="nouveau">+ Nouvel univers</button>`}
        <button type="button" data-g="ouvrir">Ouvrir un fichier…</button>
      </div>
      ${courants.length ? `<section class="uv-groupe"><h4>Sur cet ordinateur</h4>${courants.map(ligneDe).join("")}</section>` : ""}
      ${archives.length ? `<details class="uv-groupe"><summary>Archivés (${archives.length})</summary><p class="hint">Ils ne s'ouvrent plus jusqu'au désarchivage. Dossier : <code class="uv-chemin">${esc(d.dossier_archives)}</code></p>${archives.map(ligneDe).join("")}</details>` : ""}
      ${perdus.length ? `<details class="uv-groupe"><summary>Introuvables (${perdus.length})</summary><p class="hint">Le fichier n'est plus à cet endroit. Oublier le retire de la liste.</p>${perdus.map(ligneDe).join("")}</details>` : ""}
      ${ouvert ? `<section class="uv-groupe uv-ouvert"><h4>L'univers ouvert : « ${esc(ouvert.nom)} »</h4>
        <div class="ligne">
          <button type="button" data-g="copie">Enregistrer une copie…</button>
          <button type="button" data-g="sauver">Sauvegarder maintenant</button>
          <button type="button" data-g="sauvegardes">Sauvegardes…</button>
          ${actif("obsidian") ? `<button type="button" data-g="obsidian">Exporter vers Obsidian…</button>` : ""}
        </div></section>` : ""}
      <div class="ligne"><button type="button" data-g="serveur">Serveur et synchronisation…</button>
        ${accueil ? "" : `<button type="button" class="ghost" data-g="fermer">Fermer</button>`}</div>`;
    if (garder) ($(`[data-cle="${garder}"] button`, corps) || $("button", corps))?.focus();
    return d;
  };

  const etatUnivers = () => api("GET", "/api/univers");
  corps.onclick = async e => {
    const b = e.target.closest("button");
    if (!b) return;
    try {
      if (b.classList.contains("uv-plus")) {
        const l = b.closest(".uv-ligne"), ouvert = l.classList.toggle("deplie");
        b.setAttribute("aria-expanded", ouvert);
        return;
      }
      if (b.id === "uv-creer") {
        const nom = $("#uv-nom", corps);
        if (nom.value.trim()) creer({ nom: nom.value.trim() }); else nom.focus();
        return;
      }
      const ligneU = b.closest(".uv-ligne");
      if (ligneU && b.dataset.a) {
        const d = await api("GET", GESTION);
        const f = d.univers.find(x => x.cle === ligneU.dataset.cle);
        if (!f) { toast("Cet univers n'est plus dans la liste.", true); await peindre(d); return; }
        const apres = () => peindre(null, null).then(() => $("button", corps)?.focus());
        const actions = {
          ouvrir: () => ouvrir(f.chemin),
          renommer: () => dialogueRenommer(f, apres),
          archiver: () => dialogueArchiver(f, apres),
          supprimer: () => dialogueSupprimer(f, apres),
          desarchiver: async () => { await api("POST", `${GESTION}/${encodeURIComponent(f.cle)}/desarchiver`); toast(`« ${f.nom} » est de retour.`); await apres(); },
          oublier: async () => { await api("POST", `${GESTION}/${encodeURIComponent(f.cle)}/oublier`); toast(`« ${f.nom} » est retiré de la liste. Aucun fichier n'a été touché.`); await apres(); },
        };
        await actions[b.dataset.a]();
        return;
      }
      const generales = {
        nouveau: async () => dialogueNouveau(await etatUnivers()),
        ouvrir: async () => dialogueOuvrir(await etatUnivers()),
        copie: enregistrerCopie,
        sauver: sauvegarder,
        sauvegardes: () => { fermer(); location.hash = "#/parametres/donnees"; },
        obsidian: () => dialogueObsidian(),
        serveur: () => { if (!accueil) fermer(); return ouvrirPanneau(); },
        fermer,
      };
      if (generales[b.dataset.g]) await generales[b.dataset.g]();
    } catch (err) { toast(err.message, true); }
  };
  corps.onkeydown = e => { if (e.key === "Enter" && e.target.id === "uv-nom") $("#uv-creer", corps).click(); };
  await peindre(etat);
  ($("#uv-nom", corps) || $("button", corps))?.focus();
}
