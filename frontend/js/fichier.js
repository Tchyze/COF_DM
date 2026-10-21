// Menu Fichier : l'univers ouvert est un fichier .cofdm de cet ordinateur. Nouvel univers, ouvrir,
// univers récents, enregistrer une copie, sauvegarder. Réservé à l'ordinateur du MJ.
import { actif } from "./schema.js";
import { $, esc, api, toast, choisirUniversOnglet, lien, dialogue } from "./ui.js";
import { ouvrirPanneau } from "./connexion.js";

/** Taille lisible d'un fichier. */
export function taille(octets) {
  if (!octets) return "";
  const unites = ["o", "Ko", "Mo", "Go"];
  let n = octets, i = 0;
  while (n >= 1024 && i < unites.length - 1) { n /= 1024; i++; }
  return (i ? n.toFixed(n < 10 ? 1 : 0).replace(".", ",") : n) + " " + unites[i];
}

/** L'univers a changé : tout est à relire. Sur un serveur, l'onglet suit l'univers que le serveur
 * vient de donner au navigateur (créé, ouvert ou déposé). */
function recharger() {
  choisirUniversOnglet("");
  location.hash = "#/";
  location.reload();
}

async function ouvrir(chemin) {
  try { await api("POST", "/api/univers/ouvrir", { chemin }); recharger(); }
  catch (e) { toast(e.message, true); }
}

/** Boîte de dialogue du système ; renvoie "" si le MJ annule ou s'il n'y en a pas. */
async function parcourir(enregistrer, nom = "") {
  try { return (await api("POST", "/api/univers/parcourir", { enregistrer, nom })).chemin; }
  catch (e) { toast(e.message, true); return ""; }
}


const ligneUnivers = u => `<button type="button" data-chemin="${esc(u.chemin)}" ${u.present ? "" : "disabled"}>
    <span class="ic">🌍</span><span class="u"><b>${esc(u.nom)}</b><span class="x">${esc(u.chemin)}</span></span>
    <small>${u.present ? taille(u.taille) : "introuvable"}</small></button>`;

async function dialogueOuvrir() {
  const etat = await api("GET", "/api/univers");
  const connus = [...etat.recents, ...etat.autres];
  const { voile, fermer } = dialogue("Ouvrir un univers", `
    ${connus.length ? `<div class="menu liste-univers">${connus.map(ligneUnivers).join("")}</div>`
      : `<p class="vide">Aucun autre univers connu sur cet ordinateur.</p>`}
    <label class="champ"><span>Fichier d'univers <small>(.cofdm)</small></span><input id="u-chemin" placeholder="${esc(etat.dossier)}" autocomplete="off" spellcheck="false"></label>
    <div class="ligne">${etat.parcourir ? `<button id="u-parcourir">Parcourir…</button>` : ""}<button class="primary" id="u-ouvrir">Ouvrir</button><button class="ghost" id="u-annuler">Annuler</button></div>`);
  const champ = $("#u-chemin", voile);
  $("#u-annuler", voile).onclick = fermer;
  $("#u-ouvrir", voile).onclick = () => { if (champ.value.trim()) ouvrir(champ.value.trim()); else champ.focus(); };
  champ.onkeydown = e => { if (e.key === "Enter") $("#u-ouvrir", voile).click(); };
  const liste = $(".liste-univers", voile);
  if (liste) liste.onclick = e => { const b = e.target.closest("button[data-chemin]"); if (b) ouvrir(b.dataset.chemin); };
  const bouton = $("#u-parcourir", voile);
  if (bouton) bouton.onclick = async () => { const c = await parcourir(false); if (c) ouvrir(c); };
  champ.focus();
}

async function dialogueNouveau() {
  const etat = await api("GET", "/api/univers");
  const { voile, fermer } = dialogue("Nouvel univers", `
    <p class="hint">Un univers est un fichier : fiches, images et journal y sont rangés ensemble. L'univers ouvert en ce moment n'est pas modifié.</p>
    <label class="champ"><span>Nom de l'univers</span><input id="u-nom" autocomplete="off" maxlength="80"></label>
    <p class="hint">Créé dans <code>${esc(etat.dossier)}</code>${etat.parcourir ? ", ou à l'emplacement de votre choix" : ""}.</p>
    <div class="ligne"><button class="primary" id="u-creer">Créer</button>${etat.parcourir ? `<button id="u-ou">Choisir l'emplacement…</button>` : ""}<button class="ghost" id="u-annuler">Annuler</button></div>`);
  const champ = $("#u-nom", voile);
  const creer = async corps => {
    try { await api("POST", "/api/univers/nouveau", corps); recharger(); }
    catch (e) { toast(e.message, true); }
  };
  $("#u-annuler", voile).onclick = fermer;
  $("#u-creer", voile).onclick = () => { if (champ.value.trim()) creer({ nom: champ.value.trim() }); else champ.focus(); };
  champ.onkeydown = e => { if (e.key === "Enter") $("#u-creer", voile).click(); };
  const bouton = $("#u-ou", voile);
  if (bouton) bouton.onclick = async () => { const c = await parcourir(true, champ.value.trim() || "Nouvel univers"); if (c) creer({ chemin: c }); };
  champ.focus();
}

/** Serveur : l'administrateur y ajoute un univers existant en déposant son fichier. */
function dialogueDeposer() {
  const { voile, fermer } = dialogue("Déposer un univers", `
    <p class="hint">Ajoute au serveur un univers existant : son fichier <code>.cofdm</code> est contrôlé, puis rangé avec les autres. Vous en êtes MJ ; donnez un rôle aux autres comptes dans Paramètres.</p>
    <label class="champ"><span>Fichier d'univers <small>(.cofdm)</small></span><input type="file" id="u-fichier" accept=".cofdm"></label>
    <label class="champ"><span>Nom sur le serveur</span><input id="u-nom" autocomplete="off" maxlength="80"></label>
    <div class="ligne"><button class="primary" id="u-deposer">Déposer</button><button class="ghost" id="u-annuler">Annuler</button></div>`);
  const fichier = $("#u-fichier", voile), nom = $("#u-nom", voile), bouton = $("#u-deposer", voile);
  fichier.onchange = () => { if (fichier.files[0] && !nom.value.trim()) nom.value = fichier.files[0].name.replace(/\.cofdm$/i, ""); };
  $("#u-annuler", voile).onclick = fermer;
  bouton.onclick = async () => {
    if (!fichier.files[0]) { fichier.focus(); return; }
    if (!nom.value.trim()) { nom.focus(); return; }
    bouton.disabled = true;
    bouton.textContent = "Envoi…";
    try { await api("POST", "/api/univers/deposer?nom=" + encodeURIComponent(nom.value.trim()), fichier.files[0]); recharger(); }
    catch (e) { toast(e.message, true); bouton.disabled = false; bouton.textContent = "Déposer"; }
  };
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
    <div class="ligne"><button class="primary" id="u-copier">Enregistrer</button><button class="ghost" id="u-annuler">Annuler</button></div>`);
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

/** Export vers Obsidian : un zip de notes Markdown, avec ou sans ce qui est réservé au MJ. */
export function dialogueObsidian() {
  const { voile, fermer } = dialogue("Exporter vers Obsidian", `
    <p class="hint">Une note Markdown par fiche, rangée comme un vault de campagne (<i>01 - Monde</i>, <i>02 - PNJ</i>, <i>05 - Campagnes</i>…), avec les images, la chronologie et les liens <code>[[Nom]]</code>. Décompressez le zip dans votre vault : tout est dans un seul dossier, rien n'écrase vos notes.</p>
    <label class="t-coche"><input type="checkbox" id="o-secrets" checked><span>Inclure ce qui est réservé au MJ<small id="o-detail"></small></span></label>
    <div class="ligne"><a id="o-lien" download><button class="primary" type="button">⬇ Télécharger le zip</button></a><button class="ghost" id="o-annuler">Annuler</button></div>`);
  const secrets = $("#o-secrets", voile), a = $("#o-lien", voile);
  const maj = () => {
    a.href = lien("/api/export/obsidian" + (secrets.checked ? "" : "?secrets=0"));
    $("#o-detail", voile).textContent = secrets.checked
      ? "Secrets, notes, fiches non partagées et archivées, récapitulatifs des combats, journal."
      : "L'export ne contient que ce que voient les joueurs : les fiches partagées, sans leurs champs du MJ.";
  };
  secrets.onchange = maj;
  maj();
  a.onclick = () => setTimeout(fermer, 0);
  $("#o-annuler", voile).onclick = fermer;
}

async function sauvegarder() {
  try { const r = await api("POST", "/api/sauvegarde"); toast("Sauvegarde : " + r.fichier); }
  catch (e) { toast(e.message, true); }
}

function ouvrirMenu(bouton, etat) {
  const menu = document.createElement("div");
  menu.className = "menu menu-fichier";
  menu.setAttribute("role", "menu");
  const recents = etat.recents.filter(u => u.present).slice(0, 5);
  menu.innerHTML = `
    <button data-a="nouveau"><span class="ic">✚</span>Nouvel univers…</button>
    <button data-a="ouvrir"><span class="ic">📂</span>Ouvrir un univers…</button>
    ${etat.serveur ? `<button data-a="deposer"><span class="ic">⬆</span>Déposer un univers…</button>` : ""}
    ${recents.length ? `<h4>Récents</h4>${recents.map(u => `<button data-chemin="${esc(u.chemin)}" title="${esc(u.chemin)}"><span class="ic">🌍</span>${esc(u.nom)}</button>`).join("")}<hr>` : "<hr>"}
    <button data-a="copie"><span class="ic">⎘</span>Enregistrer une copie…</button>
    <button data-a="sauver"><span class="ic">💾</span>Sauvegarder maintenant</button>
    <button data-a="sauvegardes"><span class="ic">🕘</span>Sauvegardes…</button>
    ${actif("obsidian") ? `<button data-a="obsidian"><span class="ic">📝</span>Exporter vers Obsidian…</button>` : ""}
    ${etat.serveur ? "" : `<hr>
    <button data-a="serveur"><span class="ic">🌐</span>Serveur et synchronisation…</button>`}`;
  const r = bouton.getBoundingClientRect();
  menu.style.left = r.left + "px";
  menu.style.top = r.bottom + 4 + "px";
  menu.style.minWidth = r.width + "px";
  document.body.append(menu);
  const fermer = () => { menu.remove(); document.removeEventListener("mousedown", dehors, true); document.removeEventListener("keydown", echap, true); };
  const dehors = e => { if (!menu.contains(e.target)) fermer(); };
  const echap = e => { if (e.key === "Escape") fermer(); };
  document.addEventListener("mousedown", dehors, true);
  document.addEventListener("keydown", echap, true);
  menu.onclick = e => {
    const b = e.target.closest("button");
    if (!b) return;
    fermer();
    if (b.dataset.chemin) { ouvrir(b.dataset.chemin); return; }
    const action = { nouveau: dialogueNouveau, ouvrir: dialogueOuvrir, deposer: dialogueDeposer, copie: enregistrerCopie, sauver: sauvegarder,
      sauvegardes: () => { location.hash = "#/parametres/donnees"; }, obsidian: dialogueObsidian,
      serveur: ouvrirPanneau }[b.dataset.a];
    Promise.resolve(action()).catch(err => toast(err.message, true));
  };
}

/** Affiche le bouton Fichier dans le menu latéral et le nom de l'univers ouvert. */
export async function initFichier() {
  const bouton = $("#fichier");
  let etat;
  try { etat = await api("GET", "/api/univers"); } catch { return; }
  if (!etat.courant) return;
  $("#univers-nom").textContent = etat.courant.nom;
  bouton.title = etat.courant.chemin;
  bouton.hidden = false;
  document.title = etat.courant.nom + " — COF DM";
  bouton.onclick = async () => {
    if ($(".menu-fichier")) return;
    try { ouvrirMenu(bouton, await api("GET", "/api/univers")); }
    catch (e) { toast(e.message, true); }
  };
  if (etat.avertissement) toast(etat.avertissement, true);
}
