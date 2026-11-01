// Second facteur d'un administrateur : l'enrôlement, ce qu'il en a dans « Mon compte », et la
// confirmation que demandent les actions sensibles. Aucun secret ne se revoit après son affichage.
import { $, esc, api, toast, dateHeure, dialogue } from "../ui.js";

const CHAMP_MDP = `<label class="champ"><span>Votre mot de passe</span><input type="password" id="sf-mdp" autocomplete="current-password" required></label>`;

/** Les champs d'un second facteur : l'application, ou un code de secours. */
function champsFacteur(racine) {
  let secours = false;
  const zone = $("#sf-facteur", racine);
  const dessiner = () => {
    zone.innerHTML = `<label class="champ"><span>${secours ? "Code de secours" : "Code de votre application"}</span>
        <input id="sf-code" ${secours ? `autocomplete="off" autocapitalize="characters" spellcheck="false" placeholder="XXXX-XXXX-XXXX-XXXX"` : `inputmode="numeric" autocomplete="one-time-code" maxlength="7" placeholder="123456"`} required></label>
      <p class="hint" style="margin:6px 0 0"><button type="button" class="ghost petit" id="sf-autre">${secours ? "Utiliser mon application" : "Utiliser un code de secours"}</button></p>`;
    $("#sf-autre", zone).onclick = () => { secours = !secours; dessiner(); $("#sf-code", zone).focus(); };
  };
  dessiner();
  return () => ({ facteur: secours ? "secours" : "totp", code: $("#sf-code", zone).value });
}

/** Une action qui demande le mot de passe et un second facteur. `action(confirmation)` lève une erreur
 * pour que le dialogue reste ouvert. */
export function confirmerAction({ titre, texte, bouton, action }) {
  const { voile, fermer } = dialogue(titre, `
    <form class="colonne" style="gap:10px">
      <p class="hint" style="margin:0">${esc(texte)}</p>
      ${CHAMP_MDP}
      <div id="sf-facteur"></div>
      <div class="ligne"><button class="primary" type="submit">${esc(bouton)}</button><button type="button" class="ghost" id="sf-annuler">Annuler</button></div>
    </form>`);
  const facteur = champsFacteur(voile);
  $("#sf-annuler", voile).onclick = fermer;
  $("#sf-mdp", voile).focus();
  $("form", voile).onsubmit = async e => {
    e.preventDefault();
    try { await action({ mdp: $("#sf-mdp", voile).value, ...facteur() }); fermer(); }
    catch (err) { toast(err.message, true); }
  };
}

/** Les codes de secours, montrés cette seule fois : on ne passe à la suite qu'après les avoir rangés. */
function montrerSecours(box, codes, suite) {
  box.innerHTML = `<h3>Vos codes de secours</h3>
    <p class="alerte">Ils ne s'afficheront plus. Rangez-les hors de cet appareil : chacun ouvre une seule connexion si vous perdez votre application.</p>
    <ul class="sf-codes">${codes.map(c => `<li>${esc(c)}</li>`).join("")}</ul>
    <div class="ligne"><button type="button" id="sf-copier">Copier</button></div>
    <label class="t-coche" style="margin-top:10px"><input type="checkbox" id="sf-range"><span>J'ai rangé ces codes</span></label>
    <div style="margin-top:10px"><button class="primary" id="sf-suite" disabled>Continuer</button></div>`;
  $("#sf-copier", box).onclick = () => navigator.clipboard?.writeText(codes.join("\n")).then(() => toast("Codes copiés."), () => toast("Copie impossible : sélectionnez les codes.", true));
  $("#sf-range", box).onchange = e => { $("#sf-suite", box).disabled = !e.target.checked; };
  $("#sf-suite", box).onclick = suite;
}

/** Le secret montré : son QR code, fait par le serveur, et le même en texte ; puis un code pour l'activer. */
function montrerSecret(box, d, suite) {
  box.innerHTML = `<h3>Enregistrer votre application</h3>
    <p class="hint">Scannez ce QR code avec votre application d'authentification, ou saisissez-y le secret. Rien n'est actif tant que vous n'avez pas donné un code.</p>
    <img class="sf-qr" src="${esc(d.qr)}" alt="QR code du secret, repris en texte ci-dessous">
    <p class="hint">Secret : <span class="sf-secret">${esc(d.secret.replace(/(.{4})/g, "$1 ").trim())}</span></p>
    <form class="colonne" style="gap:10px">
      <label class="champ"><span>Code affiché par l'application</span><input id="sf-code" inputmode="numeric" autocomplete="one-time-code" maxlength="7" placeholder="123456" required></label>
      <div><button class="primary" type="submit">Activer</button></div>
    </form>`;
  $("#sf-code", box).focus();
  $("form", box).onsubmit = async e => {
    e.preventDefault();
    try {
      const r = await api("POST", "/api/compte/second-facteur/totp/activer", { code: $("#sf-code", box).value });
      toast("Second facteur activé.");
      if (r.secours) montrerSecours(box, r.secours, suite);
      else suite();
    } catch (err) { toast(err.message, true); $("#sf-code", box).select(); }
  };
}

/** Sans aucun facteur : le mot de passe seul n'ouvre que cet écran. */
export function enrolement(box, suite) {
  box.innerHTML = `<h3>Enregistrez un second facteur</h3>
    <p class="alerte">Un administrateur du serveur doit prouver un second facteur à chaque connexion. Tant qu'il n'y en a aucun, rien d'autre n'est accessible.</p>
    <p class="hint">Il vous faut une application d'authentification sur un autre appareil.</p>
    <form class="colonne" style="gap:10px">
      ${CHAMP_MDP}
      <div class="ligne"><button class="primary" type="submit">Continuer</button><button type="button" class="ghost" id="sf-sortir">Se déconnecter</button></div>
    </form>`;
  $("#sf-mdp", box).focus();
  $("#sf-sortir", box).onclick = async () => { await api("POST", "/api/acces/deconnexion").catch(() => {}); location.replace("/"); };
  $("form", box).onsubmit = async e => {
    e.preventDefault();
    try {
      const d = await api("POST", "/api/compte/second-facteur/totp/debut", { confirmation: { mdp: $("#sf-mdp", box).value } });
      montrerSecret(box, d, suite);
    } catch (err) { toast(err.message, true); $("#sf-mdp", box).select(); }
  };
}

/** Mon compte : ses facteurs, et de quoi remplacer l'application ou régénérer les codes de secours. */
export async function panneauSecondFacteur(box) {
  const { second_facteur: f } = await api("GET", "/api/compte");
  const maj = () => panneauSecondFacteur(box);
  box.innerHTML = `<h3>Second facteur</h3>
    <table class="tab"><tbody>
      <tr><td>Application d'authentification</td><td class="num">${f.totp ? `active depuis le ${esc(dateHeure(f.totp_active))}` : "aucune"}</td></tr>
      <tr><td>Codes de secours</td><td class="num">${f.secours_restants} restant${f.secours_restants > 1 ? "s" : ""}</td></tr>
    </tbody></table>
    ${f.secours_restants <= 2 ? `<p class="alerte">Il vous reste peu de codes de secours : régénérez-les.</p>` : ""}
    <div class="ligne" style="margin-top:10px"><button id="sf-remplacer">Changer d'application…</button><button id="sf-regenerer">Régénérer les codes…</button></div>
    <p class="hint">Vous avez perdu l'accès à votre application ? Un code de secours peut servir à confirmer le changement.</p>`;
  $("#sf-remplacer", box).onclick = () => confirmerAction({
    titre: "Changer d'application",
    texte: "L'application actuelle reste valable jusqu'à l'activation de la nouvelle.",
    bouton: "Continuer",
    action: async confirmation => {
      const d = await api("POST", "/api/compte/second-facteur/totp/debut", { confirmation });
      montrerSecret(box, d, maj);
    },
  });
  $("#sf-regenerer", box).onclick = () => confirmerAction({
    titre: "Régénérer les codes de secours",
    texte: "Les anciens codes ne vaudront plus rien.",
    bouton: "Régénérer",
    action: async confirmation => {
      const r = await api("POST", "/api/compte/second-facteur/secours", { confirmation });
      montrerSecours(box, r.secours, maj);
    },
  });
}
