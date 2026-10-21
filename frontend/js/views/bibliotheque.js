// Paramètres → Bestiaire et règles : profils, peuples et voies de l'univers, et reprise de la
// bibliothèque (bestiaire et règles) d'un autre univers.
import { $, esc, api, toast, armer, lien } from "../ui.js";

/** Lit un fichier JSON choisi par l'utilisateur ; null (avec un message) s'il est illisible. */
async function lireJSON(input) {
  const fichier = input.files[0];
  input.value = "";
  if (!fichier) return null;
  try { return JSON.parse(await fichier.text()); }
  catch { toast("Ce fichier n'est pas un JSON valide.", true); return null; }
}

const bilanMonstres = m => `${m.crees} monstre${m.crees > 1 ? "s" : ""} ajouté${m.crees > 1 ? "s" : ""}, ${m.mis_a_jour} mis à jour${m.rejets.length ? `, ${m.rejets.length} rejeté${m.rejets.length > 1 ? "s" : ""}` : ""}`;

export async function panneauBibliotheque(box, { peutReprendre }) {
  const lecture = !!window.cofAcces?.lecture_seule;
  const [ref, monstres, regles] = await Promise.all([api("GET", "/api/reference"), api("GET", "/api/bestiaire"), api("GET", "/api/regles")]);
  const listes = regles.listes || {};
  const nbRef = ref.profils.length + ref.peuples.length + ref.voies.length;
  const maison = [...ref.profils, ...ref.peuples, ...ref.voies].filter(x => x.etiquette === "maison").length;
  const nbListes = Object.values(listes).reduce((n, l) => n + (l?.length || 0), 0);
  const aDesRegles = nbListes > 0 || !!regles.creature || !!regles.personnage || !!regles.rencontre || !!regles.combat;
  // ce que le fichier apporte, partie par partie : il ne crée aucune fiche, on ne le voit qu'ici
  const partie = (presente, nom) => `<li>${presente ? "✔" : "✗"} ${nom}</li>`;
  const etats = regles.combat?.etats?.length || 0;
  const parties = `<ul class="acces-etat">
      ${partie(nbListes > 0, `Listes de choix des fiches${nbListes ? ` : ${listes.peuples?.length || 0} peuples, ${listes.profils?.length || 0} profils, ${listes.tailles?.length || 0} tailles, ${listes.categories?.length || 0} catégories` : ""}`)}
      ${partie(!!regles.creature, `Créateur de monstre`)}
      ${partie(!!regles.personnage, `Feuille de personnage${regles.personnage?.repos?.length ? ` : bornes, ${regles.personnage.repos.length} repos` : ""}`)}
      ${partie(!!regles.rencontre, `Difficulté des rencontres`)}
      ${partie(!!regles.combat, `Combat${regles.combat ? ` : ${etats} état${etats > 1 ? "s" : ""}` : ""}`)}
    </ul>`;
  let autres = [];
  if (peutReprendre && !lecture) {
    const u = await api("GET", "/api/univers").catch(() => null);
    if (u) autres = [...(u.recents || []), ...(u.autres || [])].filter(x => x.present);
  }
  box.innerHTML = `<h3>Bestiaire et règles</h3>
    <p class="hint">Rangés dans cet univers : <b>${monstres.length}</b> monstre${monstres.length > 1 ? "s" : ""} (<a href="#/bestiaire">bestiaire</a>),
      <b>${ref.profils.length}</b> profil${ref.profils.length > 1 ? "s" : ""}, <b>${ref.peuples.length}</b> peuple${ref.peuples.length > 1 ? "s" : ""}, <b>${ref.voies.length}</b> voie${ref.voies.length > 1 ? "s" : ""}${nbRef ? ` (${nbRef - maison} officiel${nbRef - maison > 1 ? "s" : ""}, ${maison} maison)` : ""}.
      Le contenu des livres n'est pas fourni avec l'application : il s'importe depuis vos fichiers.</p>
    <p class="hint"><b>Fichier de règles</b> : ${aDesRegles ? `${regles.titre ? `« ${esc(regles.titre)} »` : "importé"}. Il règle le fonctionnement de l'application :`
      : "aucun. Sans lui, peuples, profils, tailles et catégories se saisissent librement, le créateur de monstre et la difficulté des rencontres ne sont pas proposés, et le combat est minimal."}</p>
    ${aDesRegles ? parties : ""}
    <p class="hint">Ce fichier ne crée aucune fiche : la liste <a href="#/t/regle">Règles</a> du menu contient des fiches d'aide-mémoire, qui s'importent par « Importer des fiches », sur cette page. Les monstres s'importent depuis le <a href="#/bestiaire">bestiaire</a>.</p>
    <div class="ligne"><a href="#/voies"><button>✎ ${lecture ? "Consulter" : "Modifier"} les voies, profils et peuples</button></a></div>
    ${lecture ? "" : `
    <div class="ligne">
      <button id="bib-regles">⬆ Importer un fichier de règles (JSON)…</button><input type="file" id="bib-regles-f" accept="application/json,.json" hidden>
      ${aDesRegles ? `<button class="danger" id="bib-regles-vider">Retirer le fichier de règles</button>` : ""}
    </div>
    <div class="ligne">
      <button id="bib-ref">⬆ Importer des profils et voies (JSON)…</button><input type="file" id="bib-ref-f" accept="application/json,.json" hidden>
      <select id="bib-ref-etiquette" style="width:auto" aria-label="Étiquette des éléments importés" title="Étiquette des éléments importés : une simple marque pour trier, rien n'est verrouillé">
        <option value="">Étiquette du fichier</option><option value="officielle">Tout en officielle</option><option value="maison">Tout en maison</option>
      </select>
      ${nbRef ? `<a href="${lien("/api/reference/export")}" download><button>⬇ Exporter les profils et voies</button></a>
      <button class="danger" id="bib-vider">Retirer les profils et voies</button>` : ""}
    </div>
    <h3 style="margin-top:14px">D'un univers à l'autre</h3>
    <p class="hint">Reprend le bestiaire, le fichier de règles et les profils et voies d'un autre univers. Rien n'est retiré : les fiches officielles déjà présentes sont mises à jour, un monstre maison du même nom n'est pas ajouté deux fois.</p>
    ${autres.length ? `<div class="ligne"><select id="bib-source" aria-label="Univers">${autres.map(x => `<option value="${esc(x.chemin)}">${esc(x.nom)}</option>`).join("")}</select><button id="bib-reprendre">Reprendre depuis cet univers</button></div>` : ""}
    <div class="ligne">
      <a href="${lien("/api/bibliotheque")}" download><button>⬇ Exporter la bibliothèque</button></a>
      <button id="bib-imp">⬆ Importer une bibliothèque…</button><input type="file" id="bib-imp-f" accept="application/json,.json" hidden>
    </div>`}
    <div id="bib-msg" class="alerte" hidden></div>`;
  if (lecture) return;
  const maj = () => panneauBibliotheque(box, { peutReprendre });
  const dire = async html => { await maj(); const m = $("#bib-msg", box); m.hidden = false; m.innerHTML = html; };
  const recu = d => `${bilanMonstres(d.monstres)}${d.deja_presents ? `, ${d.deja_presents} déjà présent${d.deja_presents > 1 ? "s" : ""}` : ""}.` +
    (d.total ? ` Profils et voies : ${d.total.profils} profils, ${d.total.peuples} peuples, ${d.total.voies} voies.` : "") +
    (d.regles ? " Fichier de règles repris." : "");

  $("#bib-regles", box).onclick = () => $("#bib-regles-f", box).click();
  $("#bib-regles-f", box).onchange = async e => {
    const contenu = await lireJSON(e.target);
    if (!contenu) return;
    try {
      const d = await api("POST", "/api/regles/import", contenu);
      window.dispatchEvent(new Event("cof-regles"));
      dire(`Fichier de règles importé${d.titre ? ` : ${esc(d.titre)}` : ""}. Ce qu'il apporte est détaillé ci-dessus ; il ne crée pas de fiche dans la liste « Règles ». Rechargez la page pour retrouver ses listes dans les formulaires.`);
    } catch (err) { toast(err.message, true); }
  };
  const viderRegles = $("#bib-regles-vider", box);
  if (viderRegles) armer(viderRegles, "Confirmer", async () => {
    try { await api("DELETE", "/api/regles"); toast("Fichier de règles retiré."); maj(); }
    catch (err) { toast(err.message, true); }
  });
  $("#bib-ref", box).onclick = () => $("#bib-ref-f", box).click();
  $("#bib-ref-f", box).onchange = async e => {
    const contenu = await lireJSON(e.target);
    if (!contenu) return;
    try {
      const etiquette = $("#bib-ref-etiquette", box).value;
      const d = await api("POST", `/api/reference/import${etiquette ? `?etiquette=${etiquette}` : ""}`, contenu);
      dire(`Profils et voies importés : ${d.total.profils} profils, ${d.total.peuples} peuples, ${d.total.voies} voies.` +
        (d.alertes?.length ? `<br>${d.alertes.slice(0, 8).map(esc).join("<br>")}` : ""));
    } catch (err) { toast(err.message, true); }
  };
  const vider = $("#bib-vider", box);
  if (vider) armer(vider, "Confirmer", async () => {
    try { await api("DELETE", "/api/reference"); toast("Profils et voies retirés."); maj(); }
    catch (err) { toast(err.message, true); }
  });
  $("#bib-imp", box).onclick = () => $("#bib-imp-f", box).click();
  $("#bib-imp-f", box).onchange = async e => {
    const contenu = await lireJSON(e.target);
    if (!contenu) return;
    try { dire(esc(recu(await api("POST", "/api/bibliotheque", contenu)))); }
    catch (err) { toast(err.message, true); }
  };
  const reprendre = $("#bib-reprendre", box);
  if (reprendre) reprendre.onclick = async () => {
    try { dire(esc(recu(await api("POST", "/api/univers/bibliotheque", { source: $("#bib-source", box).value })))); }
    catch (err) { toast(err.message, true); }
  };
}
