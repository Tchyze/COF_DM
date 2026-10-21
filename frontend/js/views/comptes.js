// Mode serveur : « Mon compte » (mot de passe, jetons d'API) et, pour l'administrateur, les comptes
// du serveur et leur rôle dans l'univers ouvert.
import { $, $$, esc, api, toast, armer, dateHeure } from "../ui.js";
import { typeDe } from "../schema.js";

const ROLES = { mj: "MJ", auditeur: "Auditeur", joueur: "Joueur", "": "Aucun accès" };
const MDP_MIN = 10;

/** Deux saisies identiques d'un mot de passe assez long, sinon un message et null. */
function nouveauMdp(a, b) {
  if (a.value.length < MDP_MIN) { toast(`Le mot de passe doit faire au moins ${MDP_MIN} caractères.`, true); return null; }
  if (a.value !== b.value) { toast("Les deux mots de passe ne correspondent pas.", true); return null; }
  return a.value;
}

/** Ce qu'ouvre l'édition par les joueurs, et à quelles conditions : sur un serveur (ou depuis un
 * bureau relié), par les comptes des joueurs ; à la maison, sur la page libre. */
export function aideEditionJoueurs(acces = window.cofAcces || {}) {
  const voie = "Ajouter une voie de la bibliothèque reste au MJ.";
  if (!acces.serveur && !acces.relie) {
    return `Sur la page des joueurs, chacun modifie la feuille du PJ qu'il a choisi, sauf vos notes. ${voie} La page doit être ouverte au réseau local (Paramètres → Tablette et réseau).`;
  }
  const champs = (typeDe("pj")?.champs || []).filter(c => c.joueur).map(c => `« ${c.nom} »`);
  const fiche = champs.length ? `, et ${champs.length > 1 ? `les champs ${champs.slice(0, -1).join(", ")} et ${champs.at(-1)}` : `le champ ${champs[0]}`} de sa fiche` : "";
  return `Chaque joueur modifie, depuis sa page, toute la feuille de son PJ sauf les notes du MJ${fiche}. ${voie}
    Conditions : un compte au rôle joueur, le PJ rattaché à ce compte, la fiche du PJ partagée. Vaut pour tout l'univers${acces.relie ? " ; envoyé au serveur à la prochaine synchronisation" : ""}.`;
}

/** Mon compte : changer son mot de passe, créer et retirer ses jetons d'API (pas sur la page des
 * joueurs : jetons: false). */
export async function panneauCompte(box, { jetons = true } = {}) {
  const c = await api("GET", "/api/compte");
  const qui = c.admin ? "Administrateur du serveur" : `${ROLES[c.role || ""]} de cet univers`;
  box.innerHTML = `<h3>Mon compte</h3>
    <p class="hint">Connecté en tant que <b>${esc(c.nom)}</b> — ${esc(qui)}.</p>
    <form id="cpt-mdp" class="colonne" style="gap:10px">
      <label class="champ"><span>Mot de passe actuel</span><input type="password" id="cpt-ancien" autocomplete="current-password" required></label>
      <div class="org-form">
        <label class="champ"><span>Nouveau mot de passe</span><input type="password" id="cpt-nouveau" autocomplete="new-password" placeholder="${MDP_MIN} caractères au moins" required></label>
        <label class="champ"><span>Confirmation</span><input type="password" id="cpt-nouveau2" autocomplete="new-password" required></label>
      </div>
      <div><button class="primary" type="submit">Changer le mot de passe</button></div>
    </form>
    <p class="hint">Changer le mot de passe déconnecte vos autres appareils.</p>
    ${jetons ? `<h3 style="margin-top:14px">Jetons d'API</h3>
    <p class="hint">Pour un programme ou pour Claude (en-tête <code>Authorization: Bearer …</code>). Un jeton a les droits de ce compte, sauf changer le mot de passe et gérer les jetons ou les comptes.</p>
    ${c.jetons.length ? `<table class="tab"><tbody>${c.jetons.map(j => `<tr><td>${esc(j.nom || "sans nom")}</td><td class="num" style="white-space:nowrap">${dateHeure(j.cree)}</td>
      <td style="text-align:right"><button class="ghost petit" data-jeton="${esc(j.id)}">Retirer</button></td></tr>`).join("")}</tbody></table>` : `<p class="vide">Aucun jeton.</p>`}
    <form id="cpt-jeton" class="ligne"><input id="cpt-jeton-nom" placeholder="À quoi sert-il ? (ex. import Obsidian)" maxlength="60" style="flex:1;min-width:0"><button type="submit">＋ Créer un jeton</button></form>
    <div id="cpt-clair" class="alerte" hidden></div>` : ""}`;
  const maj = () => panneauCompte(box, { jetons });
  $("#cpt-mdp", box).onsubmit = async e => {
    e.preventDefault();
    const nouveau = nouveauMdp($("#cpt-nouveau", box), $("#cpt-nouveau2", box));
    if (!nouveau) return;
    try { await api("POST", "/api/compte/mdp", { ancien: $("#cpt-ancien", box).value, nouveau }); toast("Mot de passe changé."); maj(); }
    catch (err) { toast(err.message, true); }
  };
  if (jetons) $("#cpt-jeton", box).onsubmit = async e => {
    e.preventDefault();
    try {
      const j = await api("POST", "/api/compte/jetons", { nom: $("#cpt-jeton-nom", box).value });
      await maj();
      const clair = $("#cpt-clair", box);
      clair.hidden = false;
      clair.innerHTML = `<p style="margin:0 0 6px">Copiez ce jeton maintenant : il ne sera plus affiché.</p><div class="jeton">${esc(j.jeton)}</div><div style="margin-top:8px"><button id="cpt-copier">Copier</button></div>`;
      $("#cpt-copier", box).onclick = () => navigator.clipboard?.writeText(j.jeton).then(() => toast("Jeton copié."), () => toast("Copie impossible : sélectionnez le jeton.", true));
    } catch (err) { toast(err.message, true); }
  };
  $$("[data-jeton]", box).forEach(b => armer(b, "Confirmer", async () => {
    try { await api("DELETE", "/api/compte/jetons/" + encodeURIComponent(b.dataset.jeton)); toast("Jeton retiré."); maj(); }
    catch (err) { toast(err.message, true); }
  }));
}

/** Comptes du serveur (administrateur) : créer, donner un rôle dans l'univers ouvert, désactiver. */
export async function panneauComptes(box, moi) {
  const [l, etat, pjs] = await Promise.all([api("GET", "/api/comptes"), api("GET", "/api/acces"), api("GET", "/api/entites?type=pj")]);
  const choixPJ = c => `<select data-a="pj" aria-label="PJ de ${esc(c.nom)}"><option value="0">— son PJ —</option>${pjs.map(p => `<option value="${p.id}" ${p.id === c.pj ? "selected" : ""}>${esc(p.nom)}</option>`).join("")}</select>`;
  const choixRole = (v, attrs = "") => `<select ${attrs}>${Object.entries(ROLES).map(([k, n]) => `<option value="${k}" ${k === (v || "") ? "selected" : ""}>${n}</option>`).join("")}</select>`;
  box.innerHTML = `<h3>Comptes du serveur</h3>
    <p class="hint">Le rôle vaut pour l'univers où vous êtes (changez d'univers pour régler les autres) : un <b>MJ</b> le lit et le modifie, un <b>auditeur</b> le lit en entier (secrets compris) sans rien modifier, un <b>joueur</b> ne lit que les fiches que le MJ partage, sans leurs champs réservés au MJ. Un <b>administrateur</b> gère le serveur et est MJ de tous les univers.</p>
    <div style="overflow-x:auto"><table class="tab"><thead><tr><th>Compte</th><th>Rôle dans cet univers</th><th>Admin</th><th>Actif</th><th></th></tr></thead><tbody>
      ${l.map(c => `<tr data-nom="${esc(c.nom)}">
        <td><b>${esc(c.nom)}</b>${c.nom === moi ? ` <span class="hint">(vous)</span>` : ""}${c.jetons.length ? ` <span class="hint">· ${c.jetons.length} jeton(s)</span>` : ""}</td>
        <td>${c.admin ? `<span class="hint">MJ (administrateur)</span>` : choixRole(c.role, `data-a="role" aria-label="Rôle de ${esc(c.nom)}"`)}${!c.admin && c.role === "joueur" ? " " + choixPJ(c) : ""}</td>
        <td><input type="checkbox" data-a="admin" aria-label="Administrateur" ${c.admin ? "checked" : ""}></td>
        <td><input type="checkbox" data-a="actif" aria-label="Actif" ${c.inactif ? "" : "checked"}></td>
        <td style="text-align:right;white-space:nowrap"><button class="ghost petit" data-a="mdp">Mot de passe…</button>${c.nom === moi ? "" : `<button class="ghost petit" data-a="suppr">Supprimer</button>`}</td>
      </tr>`).join("")}
    </tbody></table></div>
    <label class="t-coche" style="margin-top:10px"><input type="checkbox" id="cpts-edition" ${etat.edition_joueurs ? "checked" : ""}>
      <span>Édition par les joueurs<small>${esc(aideEditionJoueurs())}</small></span></label>
    <h3 style="margin-top:14px">Nouveau compte</h3>
    <form id="cpts-nouveau" class="colonne" style="gap:10px">
      <div class="org-form">
        <label class="champ"><span>Nom d'utilisateur <small>a-z, 0-9, - et _</small></span><input id="cpts-nom" autocomplete="off" autocapitalize="none" spellcheck="false" required></label>
        <label class="champ"><span>Rôle dans cet univers</span>${choixRole("mj", `id="cpts-role"`)}</label>
      </div>
      <div class="org-form">
        <label class="champ"><span>Mot de passe</span><input type="password" id="cpts-mdp" autocomplete="new-password" placeholder="${MDP_MIN} caractères au moins" required></label>
        <label class="champ"><span>Confirmation</span><input type="password" id="cpts-mdp2" autocomplete="new-password" required></label>
      </div>
      <label class="t-coche"><input type="checkbox" id="cpts-admin"><span>Administrateur du serveur</span></label>
      <div><button class="primary" type="submit">Créer le compte</button></div>
    </form>
    <p class="hint">Transmettez le mot de passe à la personne, qui le changera dans « Mon compte ».</p>`;
  const maj = () => panneauComptes(box, moi);
  const modifier = async (nom, corps, message) => {
    try { await api("PATCH", "/api/comptes/" + encodeURIComponent(nom), corps); toast(message); }
    catch (err) { toast(err.message, true); }
    maj();
  };
  $$("tr[data-nom]", box).forEach(tr => {
    const nom = tr.dataset.nom;
    const role = $('[data-a="role"]', tr);
    if (role) role.onchange = () => modifier(nom, { role: role.value }, `${nom} : ${ROLES[role.value].toLowerCase()}.`);
    const pj = $('[data-a="pj"]', tr);
    if (pj) pj.onchange = () => modifier(nom, { pj: +pj.value }, +pj.value ? `${nom} joue ${pj.selectedOptions[0].textContent}.` : `${nom} n'a plus de PJ.`);
    $('[data-a="admin"]', tr).onchange = e => modifier(nom, { admin: e.target.checked }, e.target.checked ? `${nom} est administrateur.` : `${nom} n'est plus administrateur.`);
    $('[data-a="actif"]', tr).onchange = e => modifier(nom, { inactif: !e.target.checked }, e.target.checked ? `${nom} est réactivé.` : `${nom} est désactivé et déconnecté.`);
    $('[data-a="mdp"]', tr).onclick = () => saisirMdp(box, nom, maj);
    const suppr = $('[data-a="suppr"]', tr);
    if (suppr) armer(suppr, "Confirmer", async () => {
      try { await api("DELETE", "/api/comptes/" + encodeURIComponent(nom)); toast(`Compte ${nom} supprimé.`); }
      catch (err) { toast(err.message, true); }
      maj();
    });
  });
  $("#cpts-edition", box).onchange = async e => {
    try { await api("PUT", "/api/joueurs/edition", { active: e.target.checked }); window.cofAcces.edition_joueurs = e.target.checked; toast(e.target.checked ? "Les joueurs peuvent modifier leur fiche de PJ." : "Les joueurs ne modifient plus rien."); }
    catch (err) { toast(err.message, true); }
    maj();
  };
  $("#cpts-nouveau", box).onsubmit = async e => {
    e.preventDefault();
    const mdp = nouveauMdp($("#cpts-mdp", box), $("#cpts-mdp2", box));
    if (!mdp) return;
    try {
      await api("POST", "/api/comptes", { nom: $("#cpts-nom", box).value, mdp, admin: $("#cpts-admin", box).checked, role: $("#cpts-role", box).value });
      toast("Compte créé."); maj();
    } catch (err) { toast(err.message, true); }
  };
}

/** Boîte de saisie d'un nouveau mot de passe pour un compte (par l'administrateur). */
function saisirMdp(box, nom, maj) {
  const voile = document.createElement("div");
  voile.className = "voile";
  voile.innerHTML = `<div class="boite" role="dialog" aria-label="Mot de passe de ${esc(nom)}"><div class="boite-corps"><h3>Mot de passe de ${esc(nom)}</h3>
    <form class="colonne" style="gap:10px">
      <label class="champ"><span>Nouveau mot de passe</span><input type="password" id="adm-mdp" autocomplete="new-password" placeholder="${MDP_MIN} caractères au moins" required></label>
      <label class="champ"><span>Confirmation</span><input type="password" id="adm-mdp2" autocomplete="new-password" required></label>
      <p class="hint">Les appareils connectés à ce compte seront déconnectés.</p>
      <div class="ligne"><button class="primary" type="submit">Enregistrer</button><button type="button" id="adm-annuler">Annuler</button></div>
    </form></div></div>`;
  document.body.append(voile);
  const fermer = () => voile.remove();
  voile.addEventListener("click", e => { if (e.target === voile) fermer(); });
  $("#adm-annuler", voile).onclick = fermer;
  $("#adm-mdp", voile).focus();
  $("form", voile).onsubmit = async e => {
    e.preventDefault();
    const mdp = nouveauMdp($("#adm-mdp", voile), $("#adm-mdp2", voile));
    if (!mdp) return;
    try { await api("PATCH", "/api/comptes/" + encodeURIComponent(nom), { mdp }); toast(`Mot de passe de ${nom} changé.`); fermer(); maj(); }
    catch (err) { toast(err.message, true); }
  };
}
