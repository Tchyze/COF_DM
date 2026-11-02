// Le bouton en haut du menu latéral. Sur l'ordinateur du MJ : « Mes univers », qui ouvre la fenêtre de
// ses univers (univers.js). Sur un serveur : « Fichier », un petit menu où le MJ de l'univers importe ses
// données (l'administrateur crée les univers).
import { $, esc, api, toast, choisirUniversOnglet, lien, dialogue, taille } from "./ui.js";

export { taille };

/** L'univers a changé : tout est à relire. Sur un serveur, l'onglet suit l'univers que le serveur
 * vient de donner au navigateur (créé, ouvert ou déposé). */
function recharger() {
  choisirUniversOnglet("");
  location.hash = "#/";
  location.reload();
}

/** Serveur : le MJ verse dans son univers (créé par l'administrateur) les données d'un fichier
 * d'univers ; un univers qui a déjà des fiches n'est remplacé qu'après confirmation. */
function dialogueImporter(etat) {
  const { voile, fermer } = dialogue("Importer un fichier d'univers", `
    <p class="hint">Verse dans « ${esc(etat.courant.nom)} » les données d'un fichier <code>.cofdm</code> préparé ailleurs : fiches, relations, images, bestiaire, règles, feuilles. Le nom de l'univers, ses comptes et leurs rôles ne changent pas.</p>
    <label class="champ"><span>Fichier d'univers <small>(.cofdm)</small></span><input type="file" id="u-fichier" accept=".cofdm"></label>
    <div class="ligne"><button class="primary" id="u-importer">Importer</button><button class="ghost" id="u-annuler">Annuler</button></div>`);
  const fichier = $("#u-fichier", voile), bouton = $("#u-importer", voile);
  let remplacer = false;
  fichier.onchange = () => { remplacer = false; bouton.textContent = "Importer"; bouton.className = "primary"; };
  $("#u-annuler", voile).onclick = fermer;
  bouton.onclick = async () => {
    if (!fichier.files[0]) { fichier.focus(); return; }
    const texte = bouton.textContent;
    bouton.disabled = true;
    bouton.textContent = "Envoi…";
    try {
      const r = await api("POST", "/api/import/univers" + (remplacer ? "?remplacer=1" : ""), fichier.files[0]);
      toast(`${r.fiches} fiche${r.fiches > 1 ? "s" : ""} importée${r.fiches > 1 ? "s" : ""}.`);
      recharger();
    } catch (e) {
      bouton.disabled = false;
      bouton.textContent = texte;
      if (e.status === 409 && !remplacer) { remplacer = true; bouton.textContent = "Remplacer tout le contenu"; bouton.className = "danger"; }
      toast(e.message, true);
    }
  };
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

function ouvrirMenu(bouton, etat) {
  const menu = document.createElement("div");
  menu.className = "menu menu-fichier";
  menu.setAttribute("role", "menu");
  // l'univers existe déjà (l'administrateur l'a créé) : son MJ y importe ses données
  menu.innerHTML = `
    <button data-a="importer"><span class="ic">⬆</span>Importer un fichier d'univers…</button>
    <a href="${esc(lien("/api/export"))}" download role="menuitem"><button tabindex="-1"><span class="ic">⬇</span>Exporter en JSON</button></a>
    <hr>`;
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
    if (e.target.closest("a[download]")) { setTimeout(fermer, 0); return; } // l'export : le lien télécharge
    const b = e.target.closest("button");
    if (!b) return;
    fermer();
    if (b.dataset.a === "importer") dialogueImporter(etat);
  };
}

/** Affiche le bouton dans le menu latéral et le nom de l'univers ouvert. */
export async function initFichier() {
  const bouton = $("#fichier");
  let etat;
  try { etat = await api("GET", "/api/univers"); } catch { return; }
  if (etat.serveur && !etat.courant) return;
  $("#univers-nom").textContent = etat.courant?.nom || "Aucun ouvert";
  bouton.title = etat.courant?.chemin || etat.courant?.nom || "";
  bouton.hidden = false;
  if (etat.courant) document.title = etat.courant.nom + " — COF DM";
  if (etat.serveur) {
    bouton.onclick = async () => {
      if ($(".menu-fichier")) return;
      try { ouvrirMenu(bouton, await api("GET", "/api/univers")); }
      catch (e) { toast(e.message, true); }
    };
  } else {
    $("small", bouton).textContent = "Mes univers";
    bouton.setAttribute("aria-haspopup", "dialog");
    $(".fl", bouton).textContent = "▸";
    if (!bouton.title) bouton.title = "Vos univers sur cet ordinateur";
    bouton.onclick = async () => {
      try { (await import("./univers.js")).fenetreUnivers({ bouton }); }
      catch (e) { toast(e.message, true); }
    };
  }
  if (etat.avertissement) toast(etat.avertissement, true);
}
