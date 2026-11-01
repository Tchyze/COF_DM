// Second facteur d'un administrateur : l'enrôlement, ce qu'il en a dans « Mon compte », et la
// confirmation que demandent les actions sensibles. Aucun secret ne se revoit après son affichage.
import { $, $$, esc, api, toast, dateHeure, dialogue } from "../ui.js";
import { clesPossibles, creerCle, prouverCle, messageCle } from "../webauthn.js";

const CHAMP_MDP = `<label class="champ"><span>Votre mot de passe</span><input type="password" id="sf-mdp" autocomplete="current-password" required></label>`;
const NOMS = { cle: "Utiliser ma clé", totp: "Utiliser mon application", secours: "Utiliser un code de secours" };

/** Ce que ce compte peut donner ici. */
function modesDe(f) {
  const l = [];
  if (f.cles.length && f.cles_disponibles && clesPossibles()) l.push("cle");
  if (f.totp) l.push("totp");
  if (f.secours_restants > 0) l.push("secours");
  return l;
}

/** Les champs d'un second facteur. Renvoie de quoi lire ce qui a été donné ; pour une clé, c'est à ce
 * moment-là que le navigateur la demande. */
function champsFacteur(racine, modes) {
  let mode = modes[0];
  const zone = $("#sf-facteur", racine);
  const dessiner = () => {
    const champ = mode === "cle" ? `<p class="hint" style="margin:0">Votre clé vous sera demandée en validant : touchez-la quand elle clignote.</p>`
      : mode === "secours" ? `<label class="champ"><span>Code de secours</span><input id="sf-code" autocomplete="off" autocapitalize="characters" spellcheck="false" placeholder="XXXX-XXXX-XXXX-XXXX" required></label>`
      : `<label class="champ"><span>Code de votre application</span><input id="sf-code" inputmode="numeric" autocomplete="one-time-code" maxlength="7" placeholder="123456" required></label>`;
    zone.innerHTML = `${champ}<p class="hint" style="margin:6px 0 0">${modes.filter(m => m !== mode).map(m => `<button type="button" class="ghost petit" data-mode="${m}">${NOMS[m]}</button>`).join(" ")}</p>`;
    $$("[data-mode]", zone).forEach(b => b.onclick = () => { mode = b.dataset.mode; dessiner(); $("#sf-code", zone)?.focus(); });
  };
  dessiner();
  return async () => {
    if (mode !== "cle") return { facteur: mode, code: $("#sf-code", zone).value };
    const o = await api("POST", "/api/compte/second-facteur/defi");
    return { facteur: "cle", reponse: await prouverCle(o) };
  };
}

/** Une action qui demande le mot de passe et un second facteur. `action(confirmation, voile)` lève une
 * erreur pour que le dialogue reste ouvert. `champs` : du HTML en plus, avant le mot de passe. */
export async function confirmerAction({ titre, texte, bouton, action, champs = "" }) {
  const { second_facteur: f } = await api("GET", "/api/compte");
  const { voile, fermer } = dialogue(titre, `
    <form class="colonne" style="gap:10px">
      <p class="hint" style="margin:0">${esc(texte)}</p>
      ${champs}
      ${CHAMP_MDP}
      <div id="sf-facteur"></div>
      <div class="ligne"><button class="primary" type="submit">${esc(bouton)}</button><button type="button" class="ghost" id="sf-annuler">Annuler</button></div>
    </form>`);
  const facteur = champsFacteur(voile, modesDe(f));
  $("#sf-annuler", voile).onclick = fermer;
  ($("input", voile)).focus();
  $("form", voile).onsubmit = async e => {
    e.preventDefault();
    try { await action({ mdp: $("#sf-mdp", voile).value, ...(await facteur()) }, voile); fermer(); }
    catch (err) { toast(messageCle(err), true); }
  };
}

/** Les codes de secours, montrés cette seule fois : on ne passe à la suite qu'après les avoir rangés. */
function montrerSecours(box, codes, suite) {
  box.innerHTML = `<h3>Vos codes de secours</h3>
    <p class="alerte">Ils ne s'afficheront plus. Rangez-les hors de cet appareil : chacun ouvre une seule connexion si vous perdez votre application ou votre clé.</p>
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

/** Enregistrer une clé : le défi, la clé qu'on touche, l'activation. */
async function enregistrerCle(box, nom, confirmation, suite) {
  const o = await api("POST", "/api/compte/second-facteur/cles/debut", { nom, confirmation });
  const reponse = await creerCle(o);
  const r = await api("POST", "/api/compte/second-facteur/cles/activer", { reponse });
  toast("Clé de sécurité enregistrée.");
  if (r.secours) montrerSecours(box, r.secours, suite);
  else suite();
}

const CHAMP_NOM_CLE = `<label class="champ"><span>Nom de la clé <small>pour la reconnaître</small></span><input id="sf-nom" maxlength="60" autocomplete="off" placeholder="Clé du trousseau"></label>`;

/** Sans aucun facteur : le mot de passe seul n'ouvre que cet écran. */
export async function enrolement(box, suite) {
  const { second_facteur: f } = await api("GET", "/api/compte");
  const avecCle = f.cles_disponibles && clesPossibles();
  box.innerHTML = `<h3>Enregistrez un second facteur</h3>
    <p class="alerte">Un administrateur du serveur doit prouver un second facteur à chaque connexion. Tant qu'il n'y en a aucun, rien d'autre n'est accessible.</p>
    <form class="colonne" style="gap:10px">
      ${avecCle ? `<label class="t-coche"><input type="radio" name="sf-type" value="cle" checked><span>Clé de sécurité<small>Une YubiKey ou une autre clé FIDO2. Prévoyez-en une deuxième, de secours.</small></span></label>
        <label class="t-coche"><input type="radio" name="sf-type" value="totp"><span>Application d'authentification<small>Sur un autre appareil.</small></span></label>
        <div id="sf-zone-nom">${CHAMP_NOM_CLE}</div>`
      : `<p class="hint" style="margin:0">Il vous faut une application d'authentification sur un autre appareil.${f.cles_disponibles ? " Une clé de sécurité demande une page en HTTPS." : ""}</p>`}
      ${CHAMP_MDP}
      <div class="ligne"><button class="primary" type="submit">Continuer</button><button type="button" class="ghost" id="sf-sortir">Se déconnecter</button></div>
    </form>`;
  const type = () => $('input[name="sf-type"]:checked', box)?.value || "totp";
  $$('input[name="sf-type"]', box).forEach(x => x.onchange = () => { $("#sf-zone-nom", box).hidden = type() !== "cle"; });
  $("#sf-mdp", box).focus();
  $("#sf-sortir", box).onclick = async () => { await api("POST", "/api/acces/deconnexion").catch(() => {}); location.replace("/"); };
  $("form", box).onsubmit = async e => {
    e.preventDefault();
    const confirmation = { mdp: $("#sf-mdp", box).value };
    try {
      if (type() === "cle") await enregistrerCle(box, $("#sf-nom", box).value || "Clé de sécurité", confirmation, suite);
      else montrerSecret(box, await api("POST", "/api/compte/second-facteur/totp/debut", { confirmation }), suite);
    } catch (err) { toast(messageCle(err), true); }
  };
}

function renommerCle(k, maj) {
  const { voile, fermer } = dialogue("Renommer la clé", `
    <form class="colonne" style="gap:10px">
      <label class="champ"><span>Nom</span><input id="r-nom" maxlength="60" value="${esc(k.nom)}" required></label>
      <div class="ligne"><button class="primary" type="submit">Renommer</button><button type="button" class="ghost" id="r-annuler">Annuler</button></div>
    </form>`);
  $("#r-annuler", voile).onclick = fermer;
  $("#r-nom", voile).select();
  $("form", voile).onsubmit = async e => {
    e.preventDefault();
    try { await api("PATCH", "/api/compte/second-facteur/cles/" + encodeURIComponent(k.id), { nom: $("#r-nom", voile).value }); fermer(); maj(); }
    catch (err) { toast(err.message, true); }
  };
}

/** Mon compte : ses facteurs, et de quoi les changer. */
export async function panneauSecondFacteur(box) {
  const { second_facteur: f } = await api("GET", "/api/compte");
  const maj = () => panneauSecondFacteur(box);
  const dernier = (f.totp ? 1 : 0) + f.cles.length === 1;
  box.innerHTML = `<h3>Second facteur</h3>
    <table class="tab sf-table"><tbody>
      <tr><td>Application d'authentification</td><td class="num">${f.totp ? `active depuis le ${esc(dateHeure(f.totp_active))}` : "aucune"}</td>
        <td style="text-align:right">${f.totp && !dernier ? `<button class="ghost petit" id="sf-retirer-totp">Retirer</button>` : ""}</td></tr>
      ${f.cles.map(k => `<tr><td>🔑 ${esc(k.nom)}<br><small class="hint">${k.utilise ? "utilisée le " + esc(dateHeure(k.utilise)) : "jamais utilisée"}</small></td><td class="num">ajoutée le ${esc(dateHeure(k.cree))}</td>
        <td style="text-align:right;white-space:nowrap"><button class="ghost petit" data-renommer="${esc(k.id)}">Renommer</button>${dernier ? "" : `<button class="ghost petit" data-retirer="${esc(k.id)}">Retirer</button>`}</td></tr>`).join("")}
      <tr><td>Codes de secours</td><td class="num">${f.secours_restants} restant${f.secours_restants > 1 ? "s" : ""}</td><td></td></tr>
    </tbody></table>
    ${f.secours_restants <= 2 ? `<p class="alerte">Il vous reste peu de codes de secours : régénérez-les.</p>` : ""}
    ${f.cles.length === 1 ? `<p class="alerte">Vous n'avez qu'une clé : enregistrez-en une deuxième, de secours, à ranger ailleurs.</p>` : ""}
    <div class="ligne" style="margin-top:10px">
      ${f.cles_disponibles && clesPossibles() ? `<button id="sf-ajouter-cle">Ajouter une clé…</button>` : ""}
      <button id="sf-remplacer">${f.totp ? "Changer d'application…" : "Ajouter une application…"}</button>
      <button id="sf-regenerer">Régénérer les codes…</button>
    </div>
    ${f.cles_disponibles ? (clesPossibles() ? "" : `<p class="hint">Les clés de sécurité demandent une page en HTTPS.</p>`) : `<p class="hint">Les clés de sécurité ne sont pas configurées sur ce serveur (option <code>-origine</code>).</p>`}
    <p class="hint">Vous avez perdu votre application ou votre clé ? Un autre facteur, ou un code de secours, peut confirmer le changement.</p>`;
  const ajouter = $("#sf-ajouter-cle", box);
  if (ajouter) ajouter.onclick = () => confirmerAction({
    titre: "Ajouter une clé de sécurité",
    texte: "Après validation, touchez la nouvelle clé quand elle clignote.",
    bouton: "Continuer",
    champs: CHAMP_NOM_CLE,
    action: async (confirmation, voile) => { await enregistrerCle(box, $("#sf-nom", voile).value || "Clé de sécurité", confirmation, maj); },
  });
  $("#sf-remplacer", box).onclick = () => confirmerAction({
    titre: f.totp ? "Changer d'application" : "Ajouter une application",
    texte: f.totp ? "L'application actuelle reste valable jusqu'à l'activation de la nouvelle." : "Elle s'ajoute à vos clés de sécurité.",
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
  const retirerTotp = $("#sf-retirer-totp", box);
  if (retirerTotp) retirerTotp.onclick = () => confirmerAction({
    titre: "Retirer l'application",
    texte: "Vos autres sessions seront fermées.",
    bouton: "Retirer",
    action: async confirmation => { await api("DELETE", "/api/compte/second-facteur/totp", { confirmation }); toast("Application retirée."); maj(); },
  });
  $$("[data-renommer]", box).forEach(b => b.onclick = () => renommerCle(f.cles.find(k => k.id === b.dataset.renommer), maj));
  $$("[data-retirer]", box).forEach(b => b.onclick = () => confirmerAction({
    titre: "Retirer cette clé",
    texte: `« ${f.cles.find(k => k.id === b.dataset.retirer).nom} » ne pourra plus servir. Vos autres sessions seront fermées.`,
    bouton: "Retirer",
    action: async confirmation => { await api("DELETE", "/api/compte/second-facteur/cles/" + encodeURIComponent(b.dataset.retirer), { confirmation }); toast("Clé retirée."); maj(); },
  }));
}
