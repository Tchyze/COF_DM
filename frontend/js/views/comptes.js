// Mode serveur : « Mon compte » (mot de passe, jetons d'API) et, pour le MJ, les joueurs de son
// univers. Les comptes du serveur se gèrent dans l'administration (admin.js).
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
    Conditions : un compte joint à l'univers avec son PJ (Paramètres → Joueurs de l'univers), la fiche du PJ partagée. Vaut pour tout l'univers${acces.relie ? " ; envoyé au serveur à la prochaine synchronisation" : ""}.`;
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
    <p class="hint">Pour un programme ou pour Claude (en-tête <code>Authorization: Bearer …</code>). Un jeton a les droits de ce compte, sauf changer le mot de passe et gérer les jetons.</p>
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

/** Joueurs de l'univers (MJ, sur un serveur) : les comptes que l'administrateur y a admis ; le MJ
 * joint ceux de sa table et donne à chacun son PJ. L'édition par les joueurs se règle ici aussi. */
export async function panneauJoueurs(box) {
  const [l, etat, pjs] = await Promise.all([api("GET", "/api/joueurs/comptes"), api("GET", "/api/acces"), api("GET", "/api/entites?type=pj")]);
  const choixPJ = c => `<select data-a="pj" aria-label="PJ de ${esc(c.nom)}" ${c.joint ? "" : "disabled"}><option value="0">— son PJ —</option>${pjs.map(p => `<option value="${p.id}" ${p.id === c.pj ? "selected" : ""}>${esc(p.nom)}</option>`).join("")}</select>`;
  box.innerHTML = `<h3>Joueurs de l'univers</h3>
    <p class="hint">Les comptes que l'administrateur du serveur a admis comme joueurs de cet univers. Joignez ceux de votre table et donnez à chacun son PJ : il n'a alors que sa page de joueur, avec ce PJ (sa fiche doit être partagée). Les autres groupes du serveur ne voient rien de cet univers.</p>
    ${l.length ? `<div style="overflow-x:auto"><table class="tab"><thead><tr><th>Compte</th><th>Joint</th><th>PJ</th></tr></thead><tbody>
      ${l.map(c => `<tr data-nom="${esc(c.nom)}">
        <td><b>${esc(c.nom)}</b>${c.inactif ? ` <span class="hint">(désactivé)</span>` : ""}${c.autre_role ? ` <span class="hint">(déjà MJ ou auditeur ici)</span>` : ""}</td>
        <td><input type="checkbox" data-a="joint" aria-label="${esc(c.nom)} joint à l'univers" ${c.joint ? "checked" : ""} ${c.autre_role ? "disabled" : ""}></td>
        <td>${choixPJ(c)}</td>
      </tr>`).join("")}
    </tbody></table></div>` : `<p class="vide">Aucun compte admis comme joueur : demandez-le à l'administrateur du serveur.</p>`}
    <label class="t-coche" style="margin-top:10px"><input type="checkbox" id="cpts-edition" ${etat.edition_joueurs ? "checked" : ""}>
      <span>Édition par les joueurs<small>${esc(aideEditionJoueurs())}</small></span></label>`;
  const maj = () => panneauJoueurs(box);
  const modifier = async (nom, corps, message) => {
    try { await api("PATCH", "/api/joueurs/comptes/" + encodeURIComponent(nom), corps); toast(message); }
    catch (err) { toast(err.message, true); }
    maj();
  };
  $$("tr[data-nom]", box).forEach(tr => {
    const nom = tr.dataset.nom;
    $('[data-a="joint"]', tr).onchange = e => modifier(nom, { joint: e.target.checked }, e.target.checked ? `${nom} rejoint l'univers.` : `${nom} quitte l'univers.`);
    const pj = $('[data-a="pj"]', tr);
    pj.onchange = () => modifier(nom, { pj: +pj.value }, +pj.value ? `${nom} joue ${pj.selectedOptions[0].textContent}.` : `${nom} n'a plus de PJ.`);
  });
  $("#cpts-edition", box).onchange = async e => {
    try { await api("PUT", "/api/joueurs/edition", { active: e.target.checked }); window.cofAcces.edition_joueurs = e.target.checked; toast(e.target.checked ? "Les joueurs peuvent modifier leur fiche de PJ." : "Les joueurs ne modifient plus rien."); }
    catch (err) { toast(err.message, true); }
    maj();
  };
}
