// Pages d'outils : liens morts, journal, paramètres (calendrier, API, données).
import { $, $$, esc, api, toast, armer, dateHeure, lien, enTetesUnivers } from "../ui.js";
import { panneauCompte, panneauComptes } from "./comptes.js";
import { panneauBibliotheque } from "./bibliotheque.js";
import { schema, chargerSchema, lienFiche, badgeType, actif as fonctionActive } from "../schema.js";
import { taille, dialogueObsidian } from "../fichier.js";
import { panneauMaj, panneauVersionServeur } from "../majour.js";

export async function liens(el) {
  const morts = await api("GET", "/api/liens-morts");
  el.innerHTML = `
    <div class="entete"><h1>Liens morts</h1></div>
    <div class="panneau">
      <p class="hint">${morts.length ? `Ces <code>[[noms]]</code> ne correspondent à aucune fiche. Créez la fiche, ou corrigez le nom dans les fiches qui les citent.` : "Aucun lien mort : chaque [[nom]] mène à une fiche."}</p>
      ${morts.length ? `<table class="tab"><thead><tr><th>Nom cité</th><th>Dans</th><th></th></tr></thead><tbody>
        ${morts.map(m => `<tr><td><span class="wl mort">${esc(m.cible)}</span></td><td><div class="ligne" style="gap:4px 12px">${m.sources.map(s => lienFiche(s)).join("")}</div></td>
        <td><a href="#/nouveau?nom=${encodeURIComponent(m.cible)}"><button class="petit">Créer la fiche</button></a></td></tr>`).join("")}
      </tbody></table>` : ""}
    </div>`;
}

export async function journal(el) {
  const l = await api("GET", "/api/journal?limite=400");
  el.innerHTML = `
    <div class="entete"><h1>Journal</h1></div>
    <div class="panneau">
      ${l.length ? `<table class="tab"><thead><tr><th>Quand</th><th>Action</th><th>Fiche</th><th>Détail</th><th>Par</th></tr></thead><tbody>
        ${l.map(j => `<tr><td class="num" style="white-space:nowrap">${dateHeure(j.quand)}</td><td>${esc(j.action)}</td>
          <td>${j.entite ? `<a class="wl" href="#/f/${j.entite}">${esc(j.nom)}</a>` : esc(j.nom)}</td><td style="overflow-wrap:anywhere">${esc(j.detail)}</td><td>${esc(j.auteur)}</td></tr>`).join("")}
      </tbody></table>` : `<p class="vide">Rien pour l'instant.</p>`}
    </div>`;
}

/** Tablette et réseau : ouvrir l'application aux appareils du réseau local, mot de passe MJ. */
async function panneauAcces(box) {
  const a = await api("GET", "/api/acces");
  const pret = a.reseau && a.mdp_defini;
  const adresses = a.adresses.map(u => u + "/#/table");
  const etat = (ok, texte) => `<li><span class="${ok ? "ok-pastille" : "ko-pastille"}">●</span> ${texte}</li>`;
  box.innerHTML = `<h3>Tablette et réseau</h3>
    <p class="hint">Pour ouvrir COF DM (et l'écran du MJ) sur une tablette ou un autre appareil du même réseau. Par défaut, l'application ne répond qu'à cet ordinateur.</p>
    ${a.relie ? `<p class="alerte">L'application est reliée à un serveur : la tablette et la page des joueurs passent par lui, et le réseau local reste fermé. Pour l'ouvrir, passez en <b>Client seul</b> (zone en bas à gauche).</p>` : ""}
    <ul class="acces-etat">
      ${etat(a.reseau, a.reseau ? "L'application écoute sur le réseau local." : "L'application n'écoute que sur cet ordinateur.")}
      ${etat(a.mdp_defini, a.mdp_defini ? "Un mot de passe MJ est défini." : "Aucun mot de passe MJ : aucun appareil du réseau ne peut entrer.")}
    </ul>
    <label class="t-coche"><input type="checkbox" id="acc-reseau" ${a.reseau_voulu || a.reseau_force ? "checked" : ""} ${a.reglable && !a.reseau_force && !(a.relie && !a.reseau_voulu) ? "" : "disabled"}>
      <span>Autoriser les appareils du réseau local<small>${a.reseau_force ? "Imposé par l'option <code>-reseau</code> au lancement." : "Prend effet au prochain lancement de l'application."}</small></span></label>
    ${a.reseau_voulu !== a.reseau && !a.reseau_force ? `<p class="alerte">Fermez puis relancez COF DM pour ${a.reseau_voulu ? "ouvrir" : "fermer"} l'accès au réseau.</p>` : ""}
    <div class="org-form">
      <label class="champ"><span>${a.mdp_defini ? "Nouveau mot de passe MJ" : "Mot de passe MJ"}</span><input type="password" id="acc-mdp" autocomplete="new-password" placeholder="8 caractères au moins"></label>
      <label class="champ"><span>Confirmation</span><input type="password" id="acc-mdp2" autocomplete="new-password"></label>
    </div>
    <div class="ligne"><button class="primary" id="acc-enr">${a.mdp_defini ? "Changer le mot de passe" : "Créer le mot de passe"}</button>
      ${a.mdp_defini ? `<button class="danger" id="acc-retirer">Retirer le mot de passe</button>` : ""}</div>
    <p class="hint">Le mot de passe est gardé haché. Le changer ou le retirer déconnecte les appareils. Une session dure 30 jours ; huit essais ratés en cinq minutes bloquent l'appareil quelques minutes.</p>
    ${pret ? `<h3 style="margin-top:6px">Sur la tablette</h3>
      ${adresses.length ? `<p class="hint">Scannez le code ou tapez l'adresse dans le navigateur, puis saisissez le mot de passe MJ. La tablette doit être sur le même réseau que cet ordinateur, et COF DM doit rester ouvert.</p>
        <div class="acces-adresses">${adresses.map(u => `<figure><img src="/api/acces/qr?url=${encodeURIComponent(u)}" alt="QR code de ${esc(u)}" loading="lazy"><code>${esc(u)}</code></figure>`).join("")}</div>
        <p class="hint">Si la page ne s'ouvre pas, le pare-feu de cet ordinateur bloque sans doute le port ${esc(new URL(a.adresses[0]).port)} : autorisez COF DM sur les réseaux privés.</p>`
      : `<p class="alerte">Cet ordinateur n'a pas d'adresse sur un réseau local : est-il connecté au Wi-Fi ou au réseau ?</p>`}` : ""}
    ${!fonctionActive("page_joueurs") ? `<h3 style="margin-top:6px">Page des joueurs</h3><p class="hint">Désactivée dans cet univers : elle s'active dans le panneau « Fonctionnalités ».</p>` : `<h3 style="margin-top:6px">Page des joueurs</h3>
    <p class="hint">Chaque joueur y trouve la feuille de son personnage, l'ordre d'initiative du combat en cours (avec les états, sans PV ni statistiques) et les hauts faits. Elle s'ouvre <b>sans mot de passe</b> aux appareils du réseau local : n'importe qui sur ce réseau lit les feuilles des PJ, jamais vos notes ni le reste de l'univers.</p>
    <label class="t-coche"><input type="checkbox" id="acc-joueurs" ${a.page_joueurs ? "checked" : ""} ${a.relie && !a.page_joueurs ? "disabled" : ""}>
      <span>Ouvrir la page des joueurs au réseau local<small>${a.reseau ? "Prend effet aussitôt." : "Sans effet tant que l'application n'écoute pas sur le réseau."}</small></span></label>
    <label class="t-coche"><input type="checkbox" id="acc-edition" ${a.edition_joueurs ? "checked" : ""}>
      <span>Déverrouiller les feuilles<small>Les joueurs modifient alors leur feuille, sauf vos notes. Réglage de cet univers.</small></span></label>
    ${a.page_joueurs && a.reseau && a.adresses.length ? `<div class="acces-adresses">${a.adresses.map(u => `<figure><img src="/api/acces/qr?url=${encodeURIComponent(u + "/joueur.html")}" alt="QR code de la page des joueurs" loading="lazy"><code>${esc(u)}/joueur.html</code></figure>`).join("")}</div>` : ""}
    <p class="hint"><a href="${lien("/joueur.html")}" target="_blank" rel="noopener">Voir la page des joueurs</a> depuis cet ordinateur.</p>`}`;
  const maj = () => panneauAcces(box);
  if ($("#acc-joueurs", box)) $("#acc-joueurs", box).onchange = async e => {
    try { await api("POST", "/api/acces/joueurs", { active: e.target.checked }); toast(e.target.checked ? "Page des joueurs ouverte." : "Page des joueurs fermée."); }
    catch (err) { toast(err.message, true); }
    maj();
  };
  if ($("#acc-edition", box)) $("#acc-edition", box).onchange = async e => {
    try { await api("PUT", "/api/joueurs/edition", { active: e.target.checked }); toast(e.target.checked ? "Feuilles déverrouillées." : "Feuilles verrouillées."); }
    catch (err) { toast(err.message, true); }
    maj();
  };
  $("#acc-reseau", box).onchange = async e => {
    try { await api("POST", "/api/acces/reseau", { reseau: e.target.checked }); toast(e.target.checked ? "Accès au réseau activé : relancez COF DM." : "Accès au réseau désactivé : relancez COF DM."); }
    catch (err) { toast(err.message, true); }
    maj();
  };
  $("#acc-enr", box).onclick = async () => {
    const mdp = $("#acc-mdp", box).value;
    if (!mdp) { toast("Saisissez un mot de passe.", true); return; }
    if (mdp !== $("#acc-mdp2", box).value) { toast("Les deux mots de passe ne correspondent pas.", true); return; }
    try { await api("POST", "/api/acces/mdp", { nouveau: mdp }); toast("Mot de passe MJ enregistré."); maj(); }
    catch (err) { toast(err.message, true); }
  };
  const retirer = $("#acc-retirer", box);
  if (retirer) armer(retirer, "Confirmer", async () => {
    try { await api("POST", "/api/acces/mdp", { nouveau: "" }); toast("Mot de passe retiré : plus aucun appareil du réseau n'entre."); maj(); }
    catch (err) { toast(err.message, true); }
  });
}

/** Fonctionnalités de l'univers : ce que le MJ active ou coupe. Rien n'est effacé en coupant. */
async function panneauFonctions(box) {
  const l = await api("GET", "/api/fonctionnalites");
  const lecture = !!window.cofAcces?.lecture_seule;
  const groupes = [...new Set(l.map(f => f.groupe))];
  box.innerHTML = `<h3>Fonctionnalités</h3>
    <p class="hint">Activez ce dont vous vous servez dans cet univers${window.cofAcces?.serveur ? " (pour tous ses comptes)" : ""}. PNJ, lieux, factions, objets, lore, campagnes, quêtes, séances, PJ et bestiaire sont toujours là. Couper une fonctionnalité la retire de l'interface sans rien effacer : ses fiches et ses données reviennent quand on la réactive.</p>
    ${groupes.map(g => `<h4 class="fonc-groupe">${esc(g)}</h4>${l.filter(f => f.groupe === g).map(f => `<label class="t-coche"><input type="checkbox" data-fonc="${esc(f.cle)}" ${f.active ? "checked" : ""} ${lecture ? "disabled" : ""}>
      <span>${esc(f.nom)}<small>${esc(f.description)}</small></span></label>`).join("")}`).join("")}`;
  box.onchange = async e => {
    const c = e.target.closest("[data-fonc]");
    if (!c) return;
    try {
      await api("PUT", "/api/fonctionnalites", { [c.dataset.fonc]: c.checked });
      toast(`${l.find(f => f.cle === c.dataset.fonc).nom} : ${c.checked ? "activée" : "désactivée"}.`);
      setTimeout(() => location.reload(), 600); // menu, boutons et écrans se refont avec le nouvel état
    } catch (err) { toast(err.message, true); c.checked = !c.checked; }
  };
}

/** Claude : brancher COF DM sur Claude Desktop et Claude Code de cet ordinateur, sans ligne de commande. */
async function panneauClaude(box) {
  let d;
  try { d = await api("GET", "/api/claude"); }
  catch (e) { if (e.status === 404) { box.remove(); return; } throw e; }
  const etat = c => c.erreur ? `<span class="alerte">${esc(c.erreur)}</span>`
    : !c.present ? `<span class="hint">non installé sur cet ordinateur</span>`
    : !c.branche ? `<span class="hint">pas encore branché</span>`
    : c.a_jour ? `<span class="ok-pastille">●</span> branché` : `<span class="ko-pastille">●</span> branché sur un autre emplacement du programme`;
  const parClient = cl => d.cibles.filter(c => c.client === cl);
  const bloc = (cl, nom, aide) => {
    const l = parClient(cl), present = l.some(c => c.present), branche = l.some(c => c.branche), aJour = l.filter(c => c.present).every(c => c.a_jour);
    return `<div class="claude-client"><h4>${nom}</h4>
      ${l.map(c => `<div class="claude-cible">${etat(c)}<code title="${esc(c.chemin)}">${esc(c.chemin)}</code></div>`).join("")}
      <div class="ligne">${present ? `<button class="${branche && aJour ? "" : "primary"}" data-claude="${cl}" data-actif="1">${branche ? (aJour ? "Réinscrire" : "Mettre à jour") : "Brancher COF DM"}</button>` : ""}
        ${branche ? `<button class="ghost" data-claude="${cl}" data-actif="0">Débrancher</button>` : ""}</div>
      <p class="hint">${aide}</p></div>`;
  };
  box.innerHTML = `<h3>Claude</h3>
    <p class="hint">Claude (l'assistant d'Anthropic) peut lire et tenir à jour l'univers ouvert ici : chercher, résumer, créer des fiches, relier, préparer une séance… Ses écritures sont signées « claude » dans le journal ; il n'efface jamais rien définitivement. COF DM doit être ouvert pendant que Claude s'en sert.</p>
    ${bloc("desktop", "Claude Desktop", "Après le branchement, quittez complètement Claude Desktop (icône de la zone de notification → Quitter) puis relancez-le : COF DM apparaît dans ses outils.")}
    ${bloc("code", "Claude Code", "Fermez puis relancez Claude Code. Ou, dans un terminal :")}
    <div class="ligne"><code class="jeton" id="claude-cmd" style="flex:1;min-width:0">${esc(d.commande_code)}</code><button class="petit" id="claude-copier">Copier</button></div>`;
  box.onclick = async e => {
    const b = e.target.closest("[data-claude]");
    if (b) {
      try {
        await api("POST", "/api/claude", { client: b.dataset.claude, actif: b.dataset.actif === "1" });
        toast(b.dataset.actif === "1" ? "COF DM est branché. Relancez Claude pour le voir." : "COF DM est débranché.");
      } catch (err) { toast(err.message, true); }
      panneauClaude(box);
      return;
    }
    if (e.target.closest("#claude-copier")) {
      try { await navigator.clipboard.writeText(d.commande_code); toast("Commande copiée."); }
      catch { toast("Copie impossible : sélectionnez la commande à la main.", true); }
    }
  };
}

export async function parametres(el) {
  if (window.cofAcces?.joueur) { // un joueur ne règle que son compte
    el.innerHTML = `<div class="entete"><h1>Paramètres</h1></div><div class="grille-param"><section class="panneau" id="compte"></section></div>`;
    panneauCompte($("#compte", el)).catch(e => { $("#compte", el).innerHTML = `<div class="erreur">${esc(e.message)}</div>`; });
    return;
  }
  const p = await api("GET", "/api/parametres");
  const surPlace = window.cofAcces?.local !== false; // sur l'ordinateur du MJ (pas une tablette connectée)
  const serveur = !!window.cofAcces?.serveur; // mode serveur : chacun a son compte
  const admin = serveur && !!window.cofAcces.compte?.admin;
  const cal = structuredClone(schema.calendrier);
  el.innerHTML = `
    <div class="entete"><h1>Paramètres</h1></div>
    <div class="grille-param">
      <section class="panneau" id="fonctions"></section>
      <section class="panneau" id="cal">
        <h3>Calendrier du monde</h3>
        <p class="hint">Sert aux dates en jeu (séances, événements, campagnes). Par défaut : douze mois de trente jours, une décade et cinq jours complémentaires.</p>
        <label class="champ"><span>Ère <small>écrite après l'année, ex. « de la Couronne »</small></span><input id="ere" value="${esc(cal.ere)}"></label>
        <div class="champ"><span style="font-size:.78rem;color:var(--muted)">Mois</span><div id="mois" style="display:flex;flex-direction:column;gap:6px"></div>
          <div><button class="petit" id="ajout-mois">＋ Mois</button></div></div>
        <label class="champ"><span>Jours de la semaine <small>séparés par des virgules</small></span><input id="semaine" value="${esc(cal.semaine.join(", "))}"></label>
        <div class="ligne"><button class="primary" id="enr-cal">Enregistrer le calendrier</button><span class="hint" id="jours-an"></span></div>
      </section>
      ${surPlace && !serveur ? `<section class="panneau" id="maj"></section>` : ""}
      ${surPlace && !serveur ? `<section class="panneau" id="claude"></section>` : ""}
      ${surPlace ? `<section class="panneau" id="acces"></section>` : ""}
      ${serveur ? `<section class="panneau" id="compte"></section>` : ""}
      ${admin ? `<section class="panneau" id="comptes"></section>` : ""}
      ${admin ? `<section class="panneau" id="version-serveur"></section>` : ""}
      <section class="panneau"${surPlace ? "" : " hidden"}>
        <h3>API</h3>
        <p class="hint">Tout ce que fait l'interface est possible par l'API, pour un programme ou pour Claude. Documentation : <code>docs/API.md</code> du dépôt.</p>
        <div class="champ"><span style="font-size:.78rem;color:var(--muted)">Adresse</span><div class="jeton">http://${esc(p.adresse.replace("0.0.0.0", "<adresse de cet ordinateur>"))}</div></div>
        <div class="champ"><span style="font-size:.78rem;color:var(--muted)">Jeton (en-tête <code>Authorization: Bearer …</code>)</span><div class="jeton" id="jeton">${esc(p.jeton)}</div></div>
        <div class="ligne"><button id="copier">Copier le jeton</button><button class="danger" id="renouveler">Renouveler</button></div>
        <p class="hint">Renouveler invalide l'ancien jeton. Depuis une autre machine, l'API demande que l'application écoute sur le réseau local (« Tablette et réseau », ou l'option <code>-reseau</code>) ; le jeton y est obligatoire.</p>
      </section>
      <section class="panneau">
        <h3>Univers « ${esc(p.univers.nom)} »</h3>
        <p class="hint">${p.univers.chemin ? `Fichier : <code>${esc(p.univers.chemin)}</code>. Fiches, images et journal y sont rangés ensemble : il se copie tel quel (menu <b>Fichier</b>). ` : ""}Version ${esc(p.version)}.</p>
        <div class="ligne">
          <button id="sauver">💾 Sauvegarder maintenant</button>
          <a href="${lien("/api/export")}" download><button>⬇ Exporter en JSON</button></a>
          ${fonctionActive("obsidian") ? `<button id="obsidian">📝 Exporter vers Obsidian…</button>` : ""}
          <button id="restaurer">⬆ Restaurer un export…</button>
          <input type="file" id="fichier" accept="application/json,.json" hidden>
        </div>
        <p class="hint">L'export vers Obsidian écrit une note Markdown par fiche pour votre vault, avec ou sans les secrets du MJ. L'export JSON contient les fiches et leurs images. Le restaurer remplace toutes les fiches de l'univers par celles de l'export (une sauvegarde est faite avant).</p>
        <div id="restau" class="alerte" hidden></div>
        ${surPlace ? `<h3 style="margin-top:18px">Sauvegardes</h3><div id="sauvegardes" class="colonne" style="gap:8px"></div>` : ""}
        <h3 style="margin-top:18px">Importer des fiches</h3>
        <p class="hint">Ajoute les fiches d'un fichier JSON (une liste de <code>{"type", "nom", "data", "tags"}</code>, par exemple des règles). Une fiche dont le type et le nom existent déjà est ignorée : rien n'est remplacé.</p>
        <div class="ligne"><button id="importer">⬆ Importer des fiches (JSON)…</button><input type="file" id="fichier-imp" accept="application/json,.json" hidden></div>
        <div id="import-res" class="alerte" hidden></div>
      </section>
      <section class="panneau" id="biblio"></section>
    </div>`;

  const monter = (id, f) => f($(id, el)).catch(e => { $(id, el).innerHTML = `<div class="erreur">${esc(e.message)}</div>`; });
  monter("#fonctions", panneauFonctions);
  if (surPlace) monter("#acces", panneauAcces);
  if (surPlace && !serveur) monter("#maj", panneauMaj);
  if (surPlace && !serveur) monter("#claude", panneauClaude);
  // reprendre d'un autre univers touche aux fichiers : ordinateur du MJ, ou administrateur du serveur
  monter("#biblio", box => panneauBibliotheque(box, { peutReprendre: surPlace || admin }));
  if (serveur) monter("#compte", panneauCompte);
  if (admin) monter("#comptes", box => panneauComptes(box, window.cofAcces.compte.nom));
  if (admin) monter("#version-serveur", panneauVersionServeur);

  // import de fiches
  const fimp = $("#fichier-imp", el);
  $("#importer", el).onclick = () => fimp.click();
  fimp.onchange = async () => {
    const f = fimp.files[0];
    fimp.value = "";
    if (!f) return;
    let x;
    try { x = JSON.parse(await f.text()); } catch { toast("Fichier JSON illisible.", true); return; }
    const box = $("#import-res", el);
    box.hidden = false;
    try {
      const r = await api("POST", "/api/import/fiches", x);
      box.innerHTML = `${r.crees} fiche(s) créée(s), ${r.ignores} déjà présente(s).` +
        (r.erreurs.length ? `<ul>${r.erreurs.map(e => `<li>${esc(e)}</li>`).join("")}</ul>` : "");
      window.dispatchEvent(new Event("cof-change"));
    } catch (e) { box.innerHTML = `<span style="color:var(--mort)">${esc(e.message)}</span>`; }
  };

  // calendrier
  const peindreMois = () => {
    $("#mois", el).innerHTML = cal.mois.map((m, i) => `<div class="mois-ed" data-i="${i}">
      <input value="${esc(m.nom)}" aria-label="Nom du mois ${i + 1}"><input type="number" min="1" value="${m.jours}" aria-label="Jours">
      <button class="ghost petit" aria-label="Retirer">✕</button></div>`).join("");
    $("#jours-an", el).textContent = cal.mois.reduce((n, m) => n + (+m.jours || 0), 0) + " jours par an";
  };
  peindreMois();
  $("#mois", el).oninput = e => {
    const d = e.target.closest(".mois-ed");
    const [n, j] = $$("input", d);
    cal.mois[+d.dataset.i] = { nom: n.value, jours: +j.value };
    $("#jours-an", el).textContent = cal.mois.reduce((s, m) => s + (+m.jours || 0), 0) + " jours par an";
  };
  $("#mois", el).onclick = e => {
    const b = e.target.closest("button");
    if (!b) return;
    cal.mois.splice(+b.closest(".mois-ed").dataset.i, 1);
    peindreMois();
  };
  $("#ajout-mois", el).onclick = () => { cal.mois.push({ nom: "", jours: 30 }); peindreMois(); };
  $("#enr-cal", el).onclick = async () => {
    cal.ere = $("#ere", el).value.trim();
    cal.semaine = $("#semaine", el).value.split(",").map(x => x.trim()).filter(Boolean);
    try {
      await api("PUT", "/api/calendrier", cal);
      await chargerSchema();
      toast("Calendrier enregistré.");
    } catch (e) { toast(e.message, true); }
  };

  // API
  $("#copier", el).onclick = async () => {
    try { await navigator.clipboard.writeText($("#jeton", el).textContent); toast("Jeton copié."); }
    catch { toast("Copie impossible : sélectionnez le jeton à la main.", true); }
  };
  armer($("#renouveler", el), "Confirmer", async () => {
    const r = await api("POST", "/api/parametres/jeton");
    $("#jeton", el).textContent = r.jeton;
    toast("Nouveau jeton créé.");
  });

  // données
  const MOTIFS = { auto: "automatique", manuelle: "demandée", "avant-restauration": "avant une restauration" };
  const peindreSauvegardes = async () => {
    const box = $("#sauvegardes", el);
    if (!box) return;
    const u = await api("GET", "/api/univers");
    box.innerHTML = `<p class="hint">Une copie complète de l'univers est faite à son ouverture, toutes les demi-heures et à la fermeture, s'il a changé. Les cinq dernières sont gardées, puis une par jour pendant deux semaines ; celles que vous demandez ne sont jamais effacées. Dossier : <code>${esc(u.dossier_sauvegardes)}</code>.</p>
      ${u.sauvegardes.length ? `<div style="overflow-x:auto"><table class="tab"><thead><tr><th>Quand</th><th>Sauvegarde</th><th>Taille</th><th></th></tr></thead><tbody>
        ${u.sauvegardes.map(s => `<tr><td class="num" style="white-space:nowrap">${dateHeure(s.quand)}</td><td>${esc(MOTIFS[s.motif] || s.motif)}</td><td class="num">${taille(s.taille)}</td>
          <td><button class="petit danger" data-restaurer="${esc(s.fichier)}">Restaurer</button></td></tr>`).join("")}
      </tbody></table></div>` : `<p class="vide">Aucune sauvegarde pour l'instant.</p>`}`;
    $$("button[data-restaurer]", box).forEach(b => armer(b, "Remplacer l'univers", async () => {
      try {
        await api("POST", "/api/univers/restaurer", { fichier: b.dataset.restaurer });
        location.reload();
      } catch (e) { toast(e.message, true); }
    }));
  };
  peindreSauvegardes().catch(e => toast(e.message, true));
  if ($("#obsidian", el)) $("#obsidian", el).onclick = dialogueObsidian;
  $("#sauver", el).onclick = async () => {
    try { const r = await api("POST", "/api/sauvegarde"); toast("Sauvegarde : " + r.fichier); peindreSauvegardes(); }
    catch (e) { toast(e.message, true); }
  };
  const fichier = $("#fichier", el);
  $("#restaurer", el).onclick = () => fichier.click();
  fichier.onchange = async () => {
    const f = fichier.files[0];
    if (!f) return;
    let x;
    try { x = JSON.parse(await f.text()); } catch { toast("Fichier JSON illisible.", true); return; }
    const box = $("#restau", el);
    box.hidden = false;
    box.innerHTML = `<p style="margin:0 0 8px">Remplacer toutes les fiches par les <b>${x.entites?.length ?? 0} fiches</b> de cet export (${esc(x.date || "date inconnue")}) ?</p><button class="danger" id="ok-restau">Remplacer</button>`;
    armer($("#ok-restau", box), "Confirmer le remplacement", async () => {
      try {
        const r = await fetch("/api/restaurer", { method: "POST", headers: enTetesUnivers({ "X-COF-DM": "1", "Content-Type": "application/json" }), body: JSON.stringify(x) });
        const d = await r.json();
        if (!r.ok) throw new Error(d.detail);
        await chargerSchema();
        box.innerHTML = `${d.fiches} fiches et ${d.relations} relations restaurées. État précédent sauvegardé dans <code>${esc(d.sauvegarde)}</code>.`;
        window.dispatchEvent(new Event("cof-change"));
        peindreSauvegardes();
      } catch (e) { box.innerHTML = `<span style="color:var(--mort)">${esc(e.message)}</span>`; }
    });
  };
}
