// « Montrer à la table » : depuis une fiche, le MJ montre une de ses images (et, pour une aide de jeu,
// son texte remis) sur l'afficheur de table et sur la page des joueurs, jusqu'à ce qu'il la retire.
// Le titre montré est le nom de la fiche, que le MJ peut changer ou vider (un portrait sans nom).
import { $, $$, esc, api, toast, lien, dialogue } from "./ui.js";

const CANAL = "cof-afficheur"; // l'afficheur ouvert relit aussitôt ce qui est montré

function prevenirAfficheur() {
  try { const c = new BroadcastChannel(CANAL); c.postMessage({ montre: true }); c.close(); } catch { /* il relira seul */ }
}

/** Ce qui est montré à la table (null : rien). */
export async function montreActuel() {
  try { return (await api("GET", "/api/afficheur/montre")).montre; } catch { return null; }
}

/** Retire ce qui est montré. */
export async function retirerDeLaTable() {
  await api("POST", "/api/afficheur/retirer");
  prevenirAfficheur(); changement();
  toast("Plus rien n'est montré à la table.");
}

/** La pastille de l'écran du MJ : ce qui est montré à la table, et ✕ pour le retirer. Relue toutes
 * les quatre secondes, et aussitôt après un « Montrer » ou un retrait de cette fenêtre. */
export function initPastilleMontre() {
  const p = document.createElement("div");
  p.className = "pastille-montre";
  p.hidden = true;
  document.body.append(p);
  let seq = -1;
  const relire = async () => {
    if (document.visibilityState !== "visible") return;
    const m = await montreActuel();
    if ((m ? m.seq : 0) === seq) return;
    seq = m ? m.seq : 0;
    p.hidden = !m;
    if (!m) return;
    p.innerHTML = `<span title="Montré sur l'afficheur de table et la page des joueurs">📺 Montré : <b>${esc(m.titre || "(sans titre)")}</b></span><button class="ghost petit" aria-label="Retirer de la table" title="Retirer de la table">✕</button>`;
    p.querySelector("button").onclick = async () => { try { await retirerDeLaTable(); relire(); } catch (e) { toast(e.message, true); } };
  };
  relire();
  setInterval(relire, 4000);
  window.addEventListener("cof-montre", relire);
}
const changement = () => window.dispatchEvent(new Event("cof-montre"));

/** Le dialogue de « Montrer à la table » pour la fiche f (avec ses images, f.medias). */
export async function ouvrirMontrer(f) {
  const images = f.medias || [];
  const aide = f.type === "aide";
  const actuel = await montreActuel();
  const deCetteFiche = actuel && actuel.fiche === f.id;
  const { voile, fermer } = dialogue("Montrer à la table", `
    <p class="hint">Sur l'afficheur de table et la page des joueurs, jusqu'à ce que vous le retiriez.</p>
    ${images.length ? `<div class="montrer-images">${images.map((m, i) => `
      <label class="montrer-image"><input type="radio" name="m-image" value="${esc(m.fichier)}"${i === 0 ? " checked" : ""}>
        <img src="${esc(lien("/medias/" + encodeURIComponent(m.fichier)))}" alt="${esc(m.legende || m.nom_original || "")}"></label>`).join("")}</div>`
      : `<p class="hint">Cette fiche n'a pas d'image${aide ? " : seul son texte remis sera montré" : ""}.</p>`}
    <label class="champ"><span>Titre montré <small>vide : aucun titre, pour ne rien trahir</small></span><input id="m-titre" value="${esc(f.nom)}" maxlength="120"></label>
    ${aide ? `<label class="ligne"><input type="checkbox" id="m-texte" checked> Montrer aussi le texte remis aux joueurs</label>` : ""}
    ${actuel ? `<p class="hint">Déjà montré : <b>${esc(actuel.titre || "(sans titre)")}</b>${deCetteFiche ? " (cette fiche)" : ""}. Il sera remplacé.</p>` : ""}
    <div class="ligne"><button class="primary" id="m-ok">📺 Montrer</button>${actuel ? `<button id="m-retirer">Retirer de la table</button>` : ""}<span style="flex:1"></span><button class="ghost" id="m-annuler">Annuler</button></div>`,
    { piege: true, classe: "montrer" });
  $("#m-annuler", voile).onclick = fermer;
  if ($("#m-retirer", voile)) $("#m-retirer", voile).onclick = async () => {
    try { await retirerDeLaTable(); fermer(); } catch (e) { toast(e.message, true); }
  };
  $("#m-ok", voile).onclick = async () => {
    const choix = $$("input[name=m-image]", voile).find(x => x.checked)?.value || "";
    try {
      await api("POST", "/api/afficheur/montrer", { fiche: f.id, image: choix, titre: $("#m-titre", voile).value, ...(aide ? { texte: $("#m-texte", voile).checked } : {}) });
      prevenirAfficheur(); changement();
      toast("Montré à la table.");
      fermer();
    } catch (e) { toast(e.message, true); }
  };
}
