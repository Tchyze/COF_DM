// Administration d'un serveur (admin.html) : les univers (créer, renommer, archiver, supprimer), les
// comptes et leur place dans chaque univers, la version du serveur, son propre compte.
// L'administrateur ne voit rien du contenu des univers : cette page ne lit que /api/admin/…, la
// version du serveur et son compte.
import { $, $$, esc, api, toast, armer, dateHeure, dialogue, taille } from "./ui.js";
import { panneauCompte } from "./views/comptes.js";
import { panneauVersionServeur } from "./majour.js";

const MDP_MIN = 10;

// la place d'un compte dans un univers, telle que l'administrateur la règle
const PLACES = {
  "": ["Aucun accès", ""],
  mj: ["MJ", "lit et modifie l'univers, y joint les joueurs"],
  auditeur: ["Auditeur", "lit tout, sans rien modifier"],
  joueur: ["Joueur possible", "le MJ peut le joindre à l'univers"],
};

/** La place d'un compte dans un univers : son rôle, ou « joueur » s'il y est admis. */
function place(c, id) {
  const r = c.roles[id] || "";
  if (r === "joueur" || (!r && c.admis.includes(id))) return "joueur";
  return r;
}

/** Ce qu'il faut envoyer pour mettre un compte à cette place dans l'univers. */
function corpsPlace(id, p) {
  if (p === "joueur") return { roles: { [id]: "" }, admis: { [id]: true } };
  return { roles: { [id]: p }, admis: { [id]: false } };
}

function etatCompte(role) {
  if (!role) return `<span class="badge">joueur · en attente du MJ</span>`;
  if (role === "joueur") return `<span class="badge">joueur · joint par le MJ</span>`;
  return `<span class="badge">${esc(PLACES[role]?.[0] || role)}</span>`;
}

// ---------------------------------------------------------------- univers

async function vueUnivers(el) {
  const l = await api("GET", "/api/admin/univers");
  const servis = l.filter(u => !u.archive), archives = l.filter(u => u.archive);
  const carte = u => `<article class="panneau adm-univers" data-id="${esc(u.id)}">
      <div class="ligne adm-tete"><h3>${u.archive ? "🗄" : "🌍"} ${esc(u.nom)}</h3>${u.archive ? `<span class="badge">archivé</span>` : ""}</div>
      <p class="hint">${taille(u.taille)}${u.modifie ? ` · modifié le ${esc(dateHeure(u.modifie))}` : ""}</p>
      ${u.comptes.length ? `<ul class="adm-comptes">${u.comptes.map(c => `<li><b>${esc(c.nom)}</b> ${etatCompte(c.role)}</li>`).join("")}</ul>`
        : `<p class="vide">Aucun compte lié.</p>`}
      <div class="ligne">
        ${u.archive ? "" : `<button data-a="comptes">Comptes…</button>`}
        <button class="ghost" data-a="renommer">Renommer…</button>
        <button class="ghost" data-a="archive">${u.archive ? "Désarchiver" : "Archiver"}</button>
        <button class="ghost danger" data-a="suppr">Supprimer…</button>
      </div>
    </article>`;
  el.innerHTML = `<div class="entete"><h1>Univers</h1></div>
    <p class="hint">Vous créez les univers et y liez les comptes ; leur contenu (fiches, règles, bestiaire) est l'affaire de leurs MJ, qui l'écrivent ou l'importent. Vous n'en voyez rien.</p>
    <form id="adm-nouveau" class="panneau"><div class="adm-creer"><label class="champ"><span>Nouvel univers</span><input id="adm-nom" maxlength="80" autocomplete="off" placeholder="Nom de l'univers" required></label><button class="primary" type="submit">Créer</button></div></form>
    <div class="colonne" style="gap:12px">${servis.map(carte).join("")}</div>
    ${archives.length ? `<h2 class="param-groupe">Archivés</h2><p class="hint">Un univers archivé n'est plus accessible à personne ; ses comptes y retrouvent leur place quand il est désarchivé.</p>
      <div class="colonne" style="gap:12px">${archives.map(carte).join("")}</div>` : ""}`;
  const maj = () => vueUnivers(el);
  $("#adm-nouveau", el).onsubmit = async e => {
    e.preventDefault();
    try { const u = await api("POST", "/api/admin/univers", { nom: $("#adm-nom", el).value.trim() }); toast(`Univers « ${u.nom} » créé : liez-y son MJ.`); await maj(); dialogueComptes(u, maj); }
    catch (err) { toast(err.message, true); }
  };
  $$(".adm-univers", el).forEach(art => {
    const u = l.find(x => x.id === art.dataset.id);
    const b = a => $(`[data-a="${a}"]`, art);
    if (b("comptes")) b("comptes").onclick = () => dialogueComptes(u, maj);
    b("renommer").onclick = () => dialogueRenommer(u, maj);
    armer(b("archive"), u.archive ? "Confirmer" : "Confirmer l'archivage", async () => {
      try { await api("PATCH", "/api/admin/univers/" + encodeURIComponent(u.id), { archive: !u.archive }); toast(u.archive ? `« ${u.nom} » est de nouveau servi.` : `« ${u.nom} » est archivé.`); }
      catch (err) { toast(err.message, true); }
      maj();
    });
    b("suppr").onclick = () => dialogueSupprimer(u, maj);
  });
}

function dialogueRenommer(u, maj) {
  const { voile, fermer } = dialogue("Renommer l'univers", `
    <label class="champ"><span>Nouveau nom</span><input id="r-nom" maxlength="80" autocomplete="off" value="${esc(u.nom)}"></label>
    <p class="hint">Les comptes et les postes reliés suivent : rien d'autre ne change.</p>
    <div class="ligne"><button class="primary" id="r-ok">Renommer</button><button class="ghost" id="r-annuler">Annuler</button></div>`);
  const champ = $("#r-nom", voile);
  $("#r-annuler", voile).onclick = fermer;
  $("#r-ok", voile).onclick = async () => {
    try { await api("PATCH", "/api/admin/univers/" + encodeURIComponent(u.id), { nom: champ.value.trim() }); toast("Univers renommé."); fermer(); maj(); }
    catch (err) { toast(err.message, true); }
  };
  champ.onkeydown = e => { if (e.key === "Enter") $("#r-ok", voile).click(); };
  champ.select();
}

function dialogueSupprimer(u, maj) {
  const { voile, fermer } = dialogue("Supprimer l'univers", `
    <p class="alerte">« ${esc(u.nom)} » sera supprimé du serveur, et les comptes perdront leur place dans cet univers. Une sauvegarde est faite avant, gardée avec ses autres sauvegardes sur le serveur.</p>
    <label class="champ"><span>Retapez le nom de l'univers pour confirmer</span><input id="s-nom" autocomplete="off" spellcheck="false"></label>
    <div class="ligne"><button class="danger" id="s-ok" disabled>Supprimer définitivement</button><button class="ghost" id="s-annuler">Annuler</button></div>`);
  const champ = $("#s-nom", voile), ok = $("#s-ok", voile);
  champ.oninput = () => { ok.disabled = champ.value.trim() !== u.nom; };
  $("#s-annuler", voile).onclick = fermer;
  ok.onclick = async () => {
    try { const r = await api("DELETE", "/api/admin/univers/" + encodeURIComponent(u.id), { nom: champ.value.trim() }); toast(`« ${u.nom} » supprimé (sauvegarde : ${r.sauvegarde}).`); fermer(); maj(); }
    catch (err) { toast(err.message, true); }
  };
  champ.focus();
}

/** La place de chaque compte dans un univers : MJ, auditeur, joueur possible, ou aucun accès. */
async function dialogueComptes(u, maj) {
  const comptes = (await api("GET", "/api/admin/comptes")).filter(c => !c.admin);
  const choix = c => {
    const p = place(c, u.id);
    return `<select data-nom="${esc(c.nom)}" aria-label="Place de ${esc(c.nom)}">${Object.entries(PLACES).map(([k, [n]]) => `<option value="${k}" ${k === p ? "selected" : ""}>${n}</option>`).join("")}</select>`;
  };
  const { voile, fermer } = dialogue(`Comptes de « ${u.nom} »`, `
    <p class="hint">${Object.entries(PLACES).filter(([k]) => k).map(([, [n, d]]) => `<b>${n}</b> : ${d}`).join(" ; ")}. Le MJ voit les joueurs possibles et choisit qui il joint, avec quel PJ.</p>
    ${comptes.length ? `<div style="overflow-x:auto"><table class="tab adm-places"><tbody>${comptes.map(c => `<tr><td><b>${esc(c.nom)}</b>${c.inactif ? ` <span class="hint">(désactivé)</span>` : ""}${c.roles[u.id] === "joueur" ? ` <span class="hint">· joint par le MJ</span>` : ""}</td><td style="text-align:right">${choix(c)}</td></tr>`).join("")}</tbody></table></div>`
      : `<p class="vide">Aucun compte : créez-en dans « Comptes ».</p>`}
    <div class="ligne"><button class="primary" id="c-fermer">Fermer</button></div>`);
  const fin = () => { fermer(); maj(); };
  $("#c-fermer", voile).onclick = fin;
  voile.addEventListener("click", e => { if (e.target === voile) maj(); });
  $$("select[data-nom]", voile).forEach(sel => sel.onchange = async () => {
    const nom = sel.dataset.nom;
    try { await api("PATCH", "/api/admin/comptes/" + encodeURIComponent(nom), corpsPlace(u.id, sel.value)); toast(`${nom} : ${PLACES[sel.value][0].toLowerCase()}.`); }
    catch (err) { toast(err.message, true); }
  });
}

// ---------------------------------------------------------------- comptes

async function vueComptes(el, moi) {
  const [l, univers] = await Promise.all([api("GET", "/api/admin/comptes"), api("GET", "/api/admin/univers")]);
  const nomDe = id => univers.find(u => u.id === id)?.nom || id;
  const places = c => {
    if (c.admin) return `<span class="hint">Administrateur du serveur</span>`;
    const ids = [...new Set([...Object.keys(c.roles), ...c.admis])];
    return ids.length ? ids.map(id => `<span class="badge">${esc(nomDe(id))} · ${esc(PLACES[place(c, id)]?.[0].toLowerCase() || "")}</span>`).join(" ")
      : `<span class="hint">aucun univers</span>`;
  };
  el.innerHTML = `<div class="entete"><h1>Comptes</h1></div>
    <p class="hint">Un compte <b>administrateur</b> gère le serveur, sans rôle dans les univers ni jeton d'API. Les autres reçoivent leur place univers par univers (« Univers » → Comptes…).</p>
    <div class="panneau" style="overflow-x:auto"><table class="tab adm-table"><thead><tr><th>Compte</th><th>Univers</th><th>Actif</th><th></th></tr></thead><tbody>
      ${l.map(c => `<tr data-nom="${esc(c.nom)}">
        <td><b>${esc(c.nom)}</b>${c.nom === moi ? ` <span class="hint">(vous)</span>` : ""}${c.jetons ? ` <span class="hint">· ${c.jetons} jeton(s)</span>` : ""}</td>
        <td><div class="ligne" style="gap:4px">${places(c)}</div></td>
        <td><label class="adm-actif"><input type="checkbox" data-a="actif" aria-label="${esc(c.nom)} actif" ${c.inactif ? "" : "checked"}><span>actif</span></label></td>
        <td style="text-align:right;white-space:nowrap"><button class="ghost petit" data-a="mdp">Mot de passe…</button>${c.nom === moi ? "" : `<button class="ghost petit" data-a="suppr">Supprimer</button>`}</td>
      </tr>`).join("")}
    </tbody></table></div>
    <form id="adm-compte" class="panneau colonne" style="gap:10px">
      <h3>Nouveau compte</h3>
      <label class="champ"><span>Nom d'utilisateur <small>a-z, 0-9, - et _</small></span><input id="n-nom" autocomplete="off" autocapitalize="none" spellcheck="false" required></label>
      <div class="org-form">
        <label class="champ"><span>Mot de passe</span><input type="password" id="n-mdp" autocomplete="new-password" placeholder="${MDP_MIN} caractères au moins" required></label>
        <label class="champ"><span>Confirmation</span><input type="password" id="n-mdp2" autocomplete="new-password" required></label>
      </div>
      <label class="t-coche"><input type="checkbox" id="n-admin"><span>Administrateur du serveur<small>Il gère univers et comptes, sans voir le contenu des univers.</small></span></label>
      <div><button class="primary" type="submit">Créer le compte</button></div>
      <p class="hint">Transmettez le mot de passe à la personne, qui le changera dans « Mon compte ».</p>
    </form>`;
  const maj = () => vueComptes(el, moi);
  $$("tr[data-nom]", el).forEach(tr => {
    const nom = tr.dataset.nom;
    $('[data-a="actif"]', tr).onchange = async e => {
      try { await api("PATCH", "/api/admin/comptes/" + encodeURIComponent(nom), { inactif: !e.target.checked }); toast(e.target.checked ? `${nom} est réactivé.` : `${nom} est désactivé et déconnecté.`); }
      catch (err) { toast(err.message, true); }
      maj();
    };
    $('[data-a="mdp"]', tr).onclick = () => saisirMdp(nom);
    const suppr = $('[data-a="suppr"]', tr);
    if (suppr) armer(suppr, "Confirmer", async () => {
      try { await api("DELETE", "/api/admin/comptes/" + encodeURIComponent(nom)); toast(`Compte ${nom} supprimé.`); }
      catch (err) { toast(err.message, true); }
      maj();
    });
  });
  $("#adm-compte", el).onsubmit = async e => {
    e.preventDefault();
    const mdp = nouveauMdp($("#n-mdp", el), $("#n-mdp2", el));
    if (!mdp) return;
    try { await api("POST", "/api/admin/comptes", { nom: $("#n-nom", el).value, mdp, admin: $("#n-admin", el).checked }); toast("Compte créé."); maj(); }
    catch (err) { toast(err.message, true); }
  };
}

function nouveauMdp(a, b) {
  if (a.value.length < MDP_MIN) { toast(`Le mot de passe doit faire au moins ${MDP_MIN} caractères.`, true); return null; }
  if (a.value !== b.value) { toast("Les deux mots de passe ne correspondent pas.", true); return null; }
  return a.value;
}

function saisirMdp(nom) {
  const { voile, fermer } = dialogue(`Mot de passe de ${nom}`, `
    <form class="colonne" style="gap:10px">
      <label class="champ"><span>Nouveau mot de passe</span><input type="password" id="m-mdp" autocomplete="new-password" placeholder="${MDP_MIN} caractères au moins" required></label>
      <label class="champ"><span>Confirmation</span><input type="password" id="m-mdp2" autocomplete="new-password" required></label>
      <p class="hint">Les appareils connectés à ce compte seront déconnectés.</p>
      <div class="ligne"><button class="primary" type="submit">Enregistrer</button><button type="button" class="ghost" id="m-annuler">Annuler</button></div>
    </form>`);
  $("#m-annuler", voile).onclick = fermer;
  $("#m-mdp", voile).focus();
  $("form", voile).onsubmit = async e => {
    e.preventDefault();
    const mdp = nouveauMdp($("#m-mdp", voile), $("#m-mdp2", voile));
    if (!mdp) return;
    try { await api("PATCH", "/api/admin/comptes/" + encodeURIComponent(nom), { mdp }); toast(`Mot de passe de ${nom} changé.`); fermer(); }
    catch (err) { toast(err.message, true); }
  };
}

// ---------------------------------------------------------------- page

async function route(moi) {
  const vue = (location.hash.replace(/^#\/?/, "") || "univers").split("/")[0];
  $$("#adm-nav a").forEach(a => a.classList.toggle("on", a.dataset.nav === vue));
  const el = $("#vue");
  el.innerHTML = `<p class="hint">Chargement…</p>`;
  try {
    if (vue === "comptes") await vueComptes(el, moi);
    else if (vue === "serveur" || vue === "compte") {
      el.innerHTML = `<div class="panneau" id="adm-panneau"></div>`;
      await (vue === "serveur" ? panneauVersionServeur($("#adm-panneau")) : panneauCompte($("#adm-panneau"), { jetons: false }));
    } else await vueUnivers(el);
  } catch (e) { el.innerHTML = `<div class="erreur">${esc(e.message)}</div>`; }
}

(async () => {
  const acces = await api("GET", "/api/acces").catch(() => null);
  // la connexion se fait sur la page de l'application, qui renvoie ici un administrateur
  if (!acces?.serveur || !acces.connecte || !acces.compte?.admin) { location.replace("/"); return; }
  $("#adm-nav").hidden = false;
  const sortir = $("#adm-sortir");
  sortir.hidden = false;
  sortir.title = `Fermer la session de ${acces.compte.nom}`;
  sortir.onclick = async () => { await api("POST", "/api/acces/deconnexion").catch(() => {}); location.replace("/"); };
  window.addEventListener("hashchange", () => route(acces.compte.nom));
  route(acces.compte.nom);
})();
