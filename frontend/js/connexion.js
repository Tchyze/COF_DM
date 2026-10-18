// Mode de travail de l'application de bureau, en bas à gauche de la fenêtre : client seul (local)
// ou reliée à un serveur (connectée, ou hors connexion : les changements partiront plus tard). Un
// clic ouvre le panneau : se connecter, changer de serveur, repasser en client seul, copier un
// univers du serveur ou y envoyer celui-ci. Relié à un serveur, un bouton sous Fichier ouvre la
// fenêtre de ses univers, pour passer de l'un à l'autre. Réservé à l'ordinateur du MJ (routes
// /api/univers).
import { $, esc, api, toast, dialogue, choisirUniversOnglet } from "./ui.js";

const hote = url => { try { return new URL(url).host; } catch { return url || ""; } };
const heure = s => new Date(s * 1000).toLocaleTimeString("fr-FR", { hour: "2-digit", minute: "2-digit" });
const ROLES = { mj: "MJ", auditeur: "auditeur", joueur: "joueur" };

let dernier = null; // le dernier état lu, pour le panneau

/** L'univers a changé : tout est à relire. */
function recharger() {
  choisirUniversOnglet("");
  location.hash = "#/";
  location.reload();
}

/** Ce que dit la zone : [voyant, titre, détail]. */
function resume(d) {
  if (d.mode !== "serveur") {
    if (d.pause) return ["pause", "Local", `Copie de ${hote(d.url)} en pause`];
    return ["local", "Local", d.reseau ? "Client seul · réseau local ouvert" : "Client seul"];
  }
  const s = d.serveur || {};
  if (s.etat === "refuse") return ["erreur", "Reconnexion nécessaire", hote(s.url)];
  if (s.etat === "hors_ligne" || d.synchro?.etat === "hors_ligne") return ["hors-ligne", "Hors connexion", "Synchro au retour du serveur"];
  if (d.pause) return ["pause", "Connecté · univers en pause", `Copie de ${hote(d.url)}`];
  if (!d.relie) return ["ok", "Connecté", `${hote(s.url)} · univers local`];
  if (d.conflits) return ["erreur", "Connecté", `${d.conflits} conflit${d.conflits > 1 ? "s" : ""} à régler`];
  if (d.synchro?.etat === "erreur") return ["erreur", "Connecté", "Erreur de synchronisation"];
  if (d.synchro?.etat === "a_jour") return ["ok", "Connecté", `${hote(s.url)} · à jour ${heure(d.synchro.derniere)}`];
  return ["ok", "Connecté", hote(s.url)];
}

function dessiner(d) {
  const zone = $("#etat-lien");
  const [voyant, titre, detail] = resume(d);
  zone.dataset.voyant = voyant;
  $("b", zone).textContent = titre;
  $("small", zone).textContent = detail;
  zone.title = d.mode === "serveur" ? `Relié à ${d.serveur?.url} (compte ${d.serveur?.compte}) — cliquer pour changer` : "Travail en local — cliquer pour se connecter à un serveur";
  zone.hidden = false;
  const bouton = $("#univers-serveur");
  bouton.hidden = d.mode !== "serveur";
  if (d.mode === "serveur") {
    $("#serveur-nom").textContent = hote(d.serveur?.url);
    bouton.title = `Les univers de ${d.serveur?.url} : passer de l'un à l'autre`;
  }
}

async function lire() {
  dernier = await api("GET", "/api/univers/serveur");
  dessiner(dernier);
  return dernier;
}

/** La zone d'état, tenue à jour (toutes les 20 secondes, et après chaque action). */
export async function initEtatLien() {
  const zone = $("#etat-lien");
  try { await lire(); } catch { return; }
  zone.onclick = () => ouvrirPanneau();
  $("#univers-serveur").onclick = () => explorerUnivers().catch(e => toast(e.message, true));
  setInterval(() => lire().catch(() => {}), 20000);
  window.addEventListener("cof-synchro", () => lire().catch(() => {}));
}

const changer = () => window.dispatchEvent(new Event("cof-synchro"));

/** Le panneau du mode de travail, ancré au-dessus de la zone. */
export async function ouvrirPanneau() {
  if ($(".panneau-lien")) return;
  let d;
  try { d = await lire(); } catch (e) { toast(e.message, true); return; }
  const serveur = d.mode === "serveur", s = d.serveur || {};
  const panneau = document.createElement("div");
  panneau.className = "menu menu-fichier panneau-lien";
  panneau.setAttribute("role", "dialog");
  panneau.setAttribute("aria-label", "Mode de travail");
  const mode = (m, ic, titre, texte) => `<button type="button" role="radio" data-mode="${m}" aria-checked="${(m === "serveur") === serveur}">
    <span class="ic">${ic}</span><span class="u"><b>${titre}</b><small>${texte}</small></span></button>`;
  let etat = "";
  if (serveur) {
    const qui = `<b>${esc(hote(s.url))}</b> (compte <b>${esc(s.compte)}</b>)`;
    const lignes = {
      connecte: `Connecté à ${qui}.`,
      hors_ligne: `<b>Hors connexion</b> : ${qui} ne répond pas. Travaillez normalement, vos changements partiront à son retour.`,
      refuse: `${qui} ne reconnaît plus ce poste (son jeton a été retiré) : reconnectez-vous.`,
    };
    etat = `<p class="hint">${lignes[s.etat] || `Relié à ${qui}.`}</p>`;
    if (d.pause) etat += `<p class="hint">Cet univers est une copie de <code>${esc(hote(d.url))}</code> : en pause tant que l'application est reliée à un autre serveur.</p>`;
    else if (!d.relie) etat += `<p class="hint">L'univers ouvert est local : il n'est pas sur le serveur.</p>`;
  } else {
    etat = `<p class="hint">Vos univers restent sur cet ordinateur. Vous pouvez ouvrir l'application au réseau local (tablette, page des joueurs) dans Paramètres.</p>`;
    if (d.pause) etat += `<p class="hint">Cet univers est une copie de <code>${esc(hote(d.url))}</code>, en pause : ses changements partiront quand vous vous reconnecterez à ce serveur.</p>`;
  }
  const action = (a, ic, texte) => `<button type="button" data-a="${a}"><span class="ic">${ic}</span>${texte}</button>`;
  panneau.innerHTML = `<h4>Mode de travail</h4>
    <div role="radiogroup" aria-label="Mode de travail">
      ${mode("local", "💻", "Client seul", "Tout reste sur cet ordinateur")}
      ${mode("serveur", "🌐", "Serveur", "Univers synchronisés, travail hors connexion possible")}
    </div>
    <hr>${etat}
    ${serveur ? [
      s.etat === "refuse" ? action("reconnecter", "🔑", "Se reconnecter…") : "",
      d.actif ? action("synchroniser", "⇅", "Synchroniser maintenant") : "",
      action("copier", "🌍", "Univers du serveur…"),
      !d.copie ? action("envoyer", "⇪", "Envoyer cet univers sur le serveur…") : "",
      d.relie ? action("details", "⚖", d.conflits ? `Conflits (${d.conflits}) et détails…` : "Synchronisation et conflits…") : "",
      action("changer", "🔁", "Changer de serveur…"),
    ].join("") : [
      action("connecter", "🌐", "Se connecter à un serveur…"),
      d.relie ? action("details", "⚖", "Synchronisation et conflits…") : "",
    ].join("")}`;
  const r = $("#etat-lien").getBoundingClientRect();
  panneau.style.left = Math.max(8, r.left) + "px";
  panneau.style.bottom = (innerHeight - r.top + 6) + "px";
  panneau.style.width = "min(340px, calc(100vw - 16px))";
  document.body.append(panneau);
  const fermer = () => { panneau.remove(); document.removeEventListener("mousedown", dehors, true); document.removeEventListener("keydown", echap, true); };
  const dehors = e => { if (!panneau.contains(e.target) && !$("#etat-lien").contains(e.target)) fermer(); };
  const echap = e => { if (e.key === "Escape") fermer(); };
  document.addEventListener("mousedown", dehors, true);
  document.addEventListener("keydown", echap, true);
  (panneau.querySelector("[aria-checked=true]") || panneau.querySelector("button")).focus();

  panneau.onclick = async e => {
    const b = e.target.closest("button");
    if (!b) return;
    if (b.dataset.mode === "serveur") {
      fermer();
      if (!serveur) dialogueConnexion();
      return;
    }
    if (b.dataset.mode === "local") {
      if (!serveur) { fermer(); return; }
      if (!b.dataset.arme) { // une confirmation : la synchronisation s'arrête
        b.dataset.arme = "1";
        $("small", b).textContent = `Cliquez encore pour quitter ${hote(s.url)} : les copies passent en pause.`;
        return;
      }
      fermer();
      try {
        const r = await api("POST", "/api/univers/serveur/local");
        toast(r.avertissement || "Vous travaillez en local. Les copies du serveur sont en pause.", !!r.avertissement);
      } catch (err) { toast(err.message, true); }
      changer();
      return;
    }
    fermer();
    const actions = {
      connecter: () => dialogueConnexion(),
      reconnecter: () => dialogueConnexion({ url: s.url, compte: s.compte, reconnexion: true }),
      changer: () => dialogueConnexion({ changer: s.url }),
      copier: () => explorerUnivers(),
      envoyer: () => dialogueEnvoyer(),
      details: () => { location.hash = "#/synchro"; },
      synchroniser: async () => {
        const r = await api("POST", "/api/univers/serveur/synchroniser");
        toast(r.etat === "a_jour" ? "Synchronisé." : r.erreur || "Échange impossible.", r.etat !== "a_jour");
        window.dispatchEvent(new Event("cof-change"));
        changer();
      },
    };
    Promise.resolve(actions[b.dataset.a]?.()).catch(err => toast(err.message, true));
  };
}

/** Se connecter à un serveur (ou en changer, ou s'y reconnecter), puis choisir quoi faire. */
export function dialogueConnexion({ url = "", compte = "", changer: ancien = "", reconnexion = false } = {}) {
  const titre = ancien ? "Changer de serveur" : reconnexion ? "Se reconnecter au serveur" : "Se connecter à un serveur";
  const { voile, fermer } = dialogue(titre, `
    <p class="hint">${ancien
      ? `L'application quitte <code>${esc(hote(ancien))}</code> : ses copies restent sur cet ordinateur, en pause, et reprendront si vous vous y reconnectez.`
      : "L'application se relie à un serveur COF DM : vous y copiez ses univers, ils se synchronisent tout seuls et vous pouvez y travailler hors connexion. Tant qu'elle est reliée, la tablette et la page des joueurs passent par le serveur."}</p>
    <form id="s-form" class="colonne" style="gap:10px">
      <label class="champ"><span>Adresse du serveur</span><input id="s-url" placeholder="https://jdr.example.org" autocomplete="url" spellcheck="false" required value="${esc(url)}"></label>
      <div class="ligne ligne-connexion">
        <label class="champ"><span>Nom d'utilisateur</span><input id="s-nom" autocomplete="username" autocapitalize="none" spellcheck="false" required value="${esc(compte)}"></label>
        <label class="champ"><span>Mot de passe</span><input type="password" id="s-mdp" autocomplete="current-password" required></label>
      </div>
      <p class="hint">🔒 Le mot de passe ne sert qu'à cette connexion et n'est pas gardé : le serveur donne à ce poste son propre jeton, retiré quand vous quittez le serveur (et visible dans « Mon compte » sur le serveur).</p>
      <div class="ligne"><button class="primary" type="submit">Se connecter</button><button class="ghost" type="button" id="s-annuler">Annuler</button></div>
    </form>
    <div id="s-suite" hidden></div>`);
  $("#s-annuler", voile).onclick = fermer;
  (url ? $(compte ? "#s-mdp" : "#s-nom", voile) : $("#s-url", voile)).focus();
  $("#s-form", voile).onsubmit = async e => {
    e.preventDefault();
    const bouton = e.submitter || $("#s-form button[type=submit]", voile);
    bouton.disabled = true;
    bouton.textContent = "Connexion…";
    let r;
    try { r = await api("POST", "/api/univers/serveur/connexion", { url: $("#s-url", voile).value, nom: $("#s-nom", voile).value, mdp: $("#s-mdp", voile).value }); }
    catch (err) { toast(err.message, true); bouton.disabled = false; bouton.textContent = "Se connecter"; return; }
    if (r.avertissement) toast(r.avertissement, true);
    changer();
    $("#s-form", voile).hidden = true;
    const suite = $("#s-suite", voile);
    suite.hidden = false;
    if (reconnexion || r.actif) { // la copie ouverte reprend : rien d'autre à choisir
      toast(`Connecté à ${hote(r.url)}.`);
      fermer();
      return;
    }
    suite.insertAdjacentHTML("beforeend", `<p>✓ Connecté à <code>${esc(hote(r.url))}</code>. Ouvrez un de ses univers :</p><div class="colonne" style="gap:12px"></div>`);
    listeUnivers(suite.lastElementChild, r, fermer);
  };
}

/** Les univers du serveur, pour passer de l'un à l'autre : la copie de cet ordinateur s'ouvre ; s'il
 * n'y en a pas encore, elle est tirée d'abord. Hors connexion, seules les copies d'ici sont listées. */
function listeUnivers(boite, r, fermer) {
  const envoi = !dernier?.copie;
  const etat = u => u.ouvert ? ["✓", "Ouvert en ce moment", ""]
    : u.ici ? ["🌍", `Sur cet ordinateur · ${esc(ROLES[u.role] || u.role)}`, "Ouvrir"]
    : u.role === "mj" ? ["☁", "Sur le serveur : copié ici à l'ouverture", "Copier et ouvrir"]
    : ["☁", `${esc(ROLES[u.role] || u.role)} : la copie est réservée au MJ`, ""];
  const ligne = u => {
    const [ic, x, act] = etat(u);
    return `<button type="button" data-id="${esc(u.id)}" data-nom="${esc(u.nom.toLowerCase())}" ${act ? "" : "disabled"} ${u.ouvert ? 'aria-current="true"' : ""}>
      <span class="ic">${ic}</span><span class="u"><b>${esc(u.nom)}</b><span class="x">${x}</span></span>${act ? `<small>${act}</small>` : ""}</button>`;
  };
  boite.innerHTML = `
    ${r.hors_ligne ? `<p class="alerte">Hors connexion : seules les copies de cet ordinateur sont listées. Vos changements partiront au retour du serveur.</p>` : ""}
    ${r.univers.length > 6 ? `<input type="search" id="s-filtre" placeholder="Filtrer les univers" aria-label="Filtrer les univers">` : ""}
    ${r.univers.length ? `<div class="menu liste-univers">${r.univers.map(ligne).join("")}</div>`
      : `<p class="vide">${r.hors_ligne ? "Aucune copie de ce serveur sur cet ordinateur." : "Ce compte n'a de rôle dans aucun univers de ce serveur."}</p>`}
    <div class="ligne">${envoi && !r.hors_ligne ? `<button type="button" id="s-envoyer">⇪ Envoyer l'univers ouvert…</button>` : ""}<button class="ghost" type="button" id="s-fermer">Fermer</button></div>`;
  $("#s-fermer", boite).onclick = fermer;
  const env = $("#s-envoyer", boite);
  if (env) env.onclick = () => { fermer(); dialogueEnvoyer(); };
  const filtre = $("#s-filtre", boite);
  if (filtre) {
    filtre.oninput = () => boite.querySelectorAll(".liste-univers button").forEach(b => { b.hidden = !b.dataset.nom.includes(filtre.value.trim().toLowerCase()); });
    filtre.focus();
  }
  const liste = $(".liste-univers", boite);
  if (liste) liste.onclick = async ev => {
    const b = ev.target.closest("button[data-id]");
    if (!b || b.disabled) return;
    liste.querySelectorAll("button").forEach(x => { x.disabled = true; });
    const avant = $(".x", b).textContent;
    $(".x", b).textContent = b.querySelector("small")?.textContent === "Ouvrir" ? "Ouverture…" : "Copie en cours…";
    try { await api("POST", "/api/univers/serveur/ouvrir", { univers: b.dataset.id }); recharger(); }
    catch (err) { toast(err.message, true); $(".x", b).textContent = avant; listeUnivers(boite, r, fermer); }
  };
}

/** La fenêtre des univers du serveur (bouton sous Fichier, ou panneau du mode de travail). */
export async function explorerUnivers() {
  const r = await api("GET", "/api/univers/serveur/univers");
  changer(); // la liste vient de dire si le serveur répond
  const { voile, fermer } = dialogue(`Univers de ${hote(r.url)}`, `<div id="s-suite" class="colonne" style="gap:12px"></div>`);
  listeUnivers($("#s-suite", voile), r, fermer);
}

/** Publier l'univers ouvert sur le serveur (compte administrateur), puis le garder synchronisé. */
async function dialogueEnvoyer() {
  const etat = await api("GET", "/api/univers");
  const { voile, fermer } = dialogue("Envoyer sur le serveur", `
    <p class="hint">Publie « ${esc(etat.courant?.nom)} » sur <code>${esc(hote(dernier?.serveur?.url))}</code> (compte administrateur), puis le garde synchronisé ici. L'univers tel qu'il est maintenant reste dans ses sauvegardes.</p>
    <label class="champ"><span>Nom de l'univers sur le serveur</span><input id="s-nom-univers" maxlength="80" value="${esc(etat.courant?.nom || "")}"></label>
    <div class="ligne"><button class="primary" id="s-envoyer">Envoyer</button><button class="ghost" id="s-annuler">Annuler</button></div>`);
  $("#s-annuler", voile).onclick = fermer;
  $("#s-envoyer", voile).onclick = async ev => {
    ev.target.disabled = true;
    ev.target.textContent = "Envoi…";
    try { await api("POST", "/api/univers/serveur/envoyer", { nom: $("#s-nom-univers", voile).value.trim() }); recharger(); }
    catch (err) { toast(err.message, true); ev.target.disabled = false; ev.target.textContent = "Envoyer"; }
  };
  $("#s-nom-univers", voile).focus();
}
