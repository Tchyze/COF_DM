// Panneaux propres à certains types de fiches : profil de combat d'un PNJ, difficulté et lancement
// d'une rencontre, combats d'une séance et leur bilan, secrets (qui sait quoi), envois d'une aide de jeu,
// pot commun d'une campagne.
import { $, $$, esc, api, toast, armer, dateHeure } from "../ui.js";
import { lienFiche, vignette, badgeType, actif as fonctionActive } from "../schema.js";
import { selecteur, selecteurMonstre, saisiePieces, editeurButin, texteButin, textePieces } from "../composants.js";
import { lireMonstre, blocMonstre, nc } from "../monstres.js";
import { lancerCombat } from "./combat.js";
import { recapHTML } from "../recap.js";

const change = () => window.dispatchEvent(new Event("cof-change"));
const lecture = () => !!window.cofAcces?.lecture_seule;

// ---------------------------------------------------------------- PNJ : profil de combat

export async function combatPNJ(box, f, apres) {
  box.innerHTML = `<h3>Combat</h3><div class="contenu"></div>`;
  const zone = $(".contenu", box);
  const id = f.data.profil_combat;
  const lier = async v => {
    try { await api("PATCH", "/api/entites/" + f.id, { data: { profil_combat: v } }); change(); apres(); }
    catch (err) { toast(err.message, true); }
  };
  if (!id) {
    if (lecture()) { zone.innerHTML = `<p class="vide">Pas de profil de combat.</p>`; return; }
    zone.innerHTML = `<p class="vide">Pas de profil de combat : choisissez un monstre du bestiaire qui lui sert de profil.</p>
      <div class="ligne"><div id="choix" style="flex:1;min-width:200px"></div><a href="#/bestiaire/nouveau"><button class="petit ghost">Créer un monstre</button></a></div>`;
    const choix = selecteurMonstre($("#choix", zone), { valeur: null, auChangement: () => { if (choix.valeur()) lier(choix.valeur()); } });
    return;
  }
  const delier = lecture() ? "" : `<button class="petit ghost" id="delier">Retirer le lien</button>`;
  try {
    const m = await lireMonstre(id);
    zone.innerHTML = `<div class="ligne"><b>${esc(m.nom)}</b><span class="sp" style="flex:1"></span>
      <a href="#/bestiaire/${id}"><button class="petit">Ouvrir dans le bestiaire</button></a>${delier}</div>${blocMonstre(m)}`;
  } catch (e) {
    zone.innerHTML = `<p class="alerte">Ce profil n'est plus dans le bestiaire (${esc(e.message)})</p><div>${delier}</div>`;
  }
  if ($("#delier", zone)) armer($("#delier", zone), "Confirmer", () => lier(null));
}

// ---------------------------------------------------------------- rencontre

const virgule = n => String(Math.round(n * 10) / 10).replace(".", ",");
const TON = { ok: "var(--ok)", warn: "var(--warn)", danger: "var(--mort)" };

export async function rencontre(box, f) {
  box.innerHTML = `<h3>Créatures et difficulté</h3><div class="contenu"></div>`;
  const zone = $(".contenu", box);
  let condition = "";
  const peindre = async () => {
    let r;
    try { r = await api("GET", `/api/entites/${f.id}/difficulte${condition ? "?condition=" + encodeURIComponent(condition) : ""}`); }
    catch (e) { zone.innerHTML = `<p class="alerte">${esc(e.message)}</p>`; return; }
    const lignes = r.creatures.map(c => `<li><b>${c.qte} ×</b> ${c.pnj ? lienFiche(c.pnj) : c.introuvable ? esc(c.nom || "monstre n°" + c.monstre) : `<a href="#/bestiaire/${c.monstre}">${esc(c.nom)}</a>`}
        ${c.introuvable ? `<span style="color:var(--warn)">⚠ retiré du bestiaire</span>` : `<span class="hint">NC ${nc(c.nc)} · DEF ${c.defense} · PV ${c.pv}</span>`}
        ${c.pnj ? `<span class="hint">PNJ</span>` : ""}${c.camp === "allie" ? ` <span class="badge">allié</span>` : ""}</li>`).join("")
      + r.pnj_sans_profil.map(p => `<li>${lienFiche(p)} <span class="hint">PNJ sans profil de combat : il ne compte pas dans la difficulté</span></li>`).join("");
    const e = r.evaluation;
    const groupe = r.nb_pj ? `${r.nb_pj} PJ de niveau moyen ${virgule(r.niveau_pj)}` : "aucun PJ en jeu dans la campagne";
    let evaluation;
    if (!e) evaluation = `<p class="hint">La difficulté se calcule avec un fichier de règles : importez-le dans <a href="#/parametres/biblio">Paramètres → Bestiaire et règles</a>.</p>`;
    else if (e.cle === "vide") evaluation = "";
    else {
      const couleur = TON[e.ton] || "var(--gold)";
      evaluation = `<div class="alerte" style="border-color:${couleur}"><b style="color:${couleur}">${esc(e.verdict)}</b>
        <br><span class="hint">Rencontre de niveau ${nc(e.niveau)} · ${e.points} points · ${esc(groupe)}</span>
        ${e.alertes.map(a => `<br><span style="color:var(--warn)">⚠ ${esc(a)}</span>`).join("")}
        <details><summary class="hint">Détail du calcul</summary><ul class="attaques">${e.detail.map(l => `<li>${esc(l)}</li>`).join("")}</ul></details></div>
        ${r.conditions.length ? `<div class="ligne"><label class="hint" for="cond">Conditions</label><select id="cond"><option value="">normales</option>${r.conditions.map(c => `<option value="${esc(c.cle)}" ${c.cle === condition ? "selected" : ""}>${esc(c.libelle)}</option>`).join("")}</select></div>` : ""}`;
    }
    zone.innerHTML = `${lignes ? `<ul class="attaques">${lignes}</ul>` : `<p class="vide">Aucune créature : ajoutez-en dans « Modifier » (monstres du bestiaire) ou des PNJ présents qui ont un profil de combat.</p>`}${evaluation}
      ${lecture() || !fonctionActive("combat") ? "" : `<div class="ligne"><button class="primary" id="lancer">⚔ Lancer le combat</button><span class="hint">avec les PJ en jeu de la campagne</span></div>`}`;
    if ($("#cond", zone)) $("#cond", zone).onchange = ev => { condition = ev.target.value; peindre(); };
    if ($("#lancer", zone)) armer($("#lancer", zone), "Lancer ?", async () => {
      try { const c = await lancerCombat({ rencontre: f.id }); location.hash = "#/combat/" + c.id; }
      catch (err) { toast(err.message, true); }
    });
  };
  await peindre();
}

// ---------------------------------------------------------------- séance : combats joués

/** Les combats rattachés à la séance, leur bilan, et les PNJ du monde qui y sont tombés. */
export async function combatsSeance(box, f, apres) {
  box.innerHTML = `<h3>Combats de la séance</h3><div class="contenu"></div>`;
  const zone = $(".contenu", box);
  let tous = [];
  try { tous = await api("GET", "/api/combats"); }
  catch (e) { zone.innerHTML = `<p class="alerte">${esc(e.message)}</p>`; return; }
  const ici = tous.filter(c => c.seance === f.id), ailleurs = tous.filter(c => c.seance !== f.id);
  const modifiable = !lecture();
  zone.innerHTML = `<div id="liste-combats" class="colonne" style="gap:12px">${ici.length ? "" : `<p class="vide">Aucun combat rattaché. Un combat lancé se rattache de lui-même à la séance prévue de sa campagne.</p>`}</div>
    ${modifiable && ailleurs.length ? `<div class="ligne"><label class="hint" for="rattacher">Rattacher un combat</label><select id="rattacher" style="flex:1;min-width:220px"><option value="">Choisir…</option>${ailleurs.map(c => `<option value="${c.id}">${esc(c.nom)}${c.termine ? "" : " (en cours)"}</option>`).join("")}</select></div>` : ""}`;
  const rattacher = async (id, seance) => {
    try { await api("PATCH", "/api/combats/" + id, { seance }); apres(); }
    catch (e) { toast(e.message, true); }
  };
  if ($("#rattacher", zone)) $("#rattacher", zone).onchange = e => { if (e.target.value) rattacher(+e.target.value, f.id); };
  for (const c of ici) {
    const d = document.createElement("div");
    d.className = "combat-seance";
    $("#liste-combats", zone).append(d);
    try {
      const st = (await api("GET", "/api/combats/" + c.id)).etat;
      // PNJ du monde tombés : leur fiche dit s'ils sont déjà notés morts
      const tombes = Object.values(st.cbts).filter(x => x.pv <= 0 && x.ref?.kind === "pnj");
      const fiches = {};
      await Promise.all(tombes.map(async x => { try { fiches[x.ref.id] = await api("GET", "/api/entites/" + x.ref.id); } catch { /* fiche supprimée */ } }));
      const issue = st.recap?.issue;
      d.innerHTML = `<div class="ligne"><b>⚔ ${esc(st.nom)}</b>
          <span class="badge ${issue === "victoire" ? "statut-réussie" : issue === "defaite" ? "statut-échouée" : ""}">${esc(issue || (st.termine ? "terminé" : "en cours"))}</span>
          <span class="hint">round ${st.round}</span>
          <span style="flex:1"></span><a href="#/combat/${c.id}"><button class="petit">${st.termine ? "Revoir" : "Ouvrir"}</button></a>
          ${modifiable ? `<button class="ghost petit" data-detacher="${c.id}">Détacher</button>` : ""}</div>
        ${st.recap ? `<details><summary class="hint">Récapitulatif</summary><div class="combat">${recapHTML(st.recap, { titre: false })}</div></details>` : ""}
        ${tombes.length ? `<div class="rel-groupe"><small>PNJ tombés au combat</small>${tombes.map(x => {
          const fi = fiches[x.ref.id], statut = fi?.data?.statut || "";
          return `<div class="rel">${fi ? lienFiche(fi) : `<span>${esc(x.nom)}</span>`}
            <span class="note" style="flex:0 0 auto;padding:0;margin:0">${esc(statut || "statut inconnu")}</span>
            ${fi && modifiable && statut !== "mort" ? `<button class="petit danger" data-mort="${fi.id}" style="opacity:1">Marquer mort</button>` : ""}</div>`;
        }).join("")}</div>` : ""}`;
      $$("[data-mort]", d).forEach(btn => armer(btn, "Confirmer", async () => {
        try {
          await api("PATCH", "/api/entites/" + btn.dataset.mort, { data: { statut: "mort" } });
          toast("PNJ marqué mort.");
          change(); apres();
        } catch (err) { toast(err.message, true); }
      }));
    } catch (e) {
      d.innerHTML = `<div class="ligne"><span class="alerte">${esc(c.nom)} : ${esc(e.message)}</span>${modifiable ? `<button class="ghost petit" data-detacher="${c.id}">Détacher</button>` : ""}</div>`;
    }
    $$("[data-detacher]", d).forEach(b => armer(b, "Détacher ?", () => rattacher(+b.dataset.detacher, 0)));
  }
}

// ---------------------------------------------------------------- secrets

/** Sur un secret : qui le sait, depuis quand ; révéler aux PJ. */
export async function quiSait(box, f, apres) {
  const savent = f.relations.filter(r => r.type === "sait" && r.sens === "entrante");
  const groupes = { pj: [], pnj: [], faction: [], autre: [] };
  for (const r of savent) (groupes[r.autre.type] || groupes.autre).push(r);
  const ligne = r => `<div class="rel">${lienFiche(r.autre)}<button class="ghost petit" data-suppr="${r.id}" aria-label="Il ne le sait plus">✕</button>
    ${r.contexte || r.note ? `<span class="note">${r.contexte ? `depuis ${esc(r.contexte.nom)}` : ""}${r.contexte && r.note ? " — " : ""}${esc(r.note || "")}</span>` : ""}</div>`;
  const campagne = f.data.campagne;
  const [pjs, seances, aides] = await Promise.all([
    api("GET", "/api/entites?type=pj" + (campagne ? "&campagne=" + campagne : "")),
    api("GET", "/api/entites?type=seance" + (campagne ? "&campagne=" + campagne : "")),
    fonctionActive("aides") ? api("GET", "/api/aides") : [],
  ]);
  const indices = aides.filter(a => a.indice_de?.id === f.id);
  const optSeances = `<option value="">— séance —</option>${seances.map(s => `<option value="${s.id}">${esc(s.nom)}</option>`).join("")}`;
  box.innerHTML = `<h3>Qui le sait <span class="sp"></span><button class="petit" id="qs-ajout">＋ Quelqu'un</button><button class="petit primary" id="qs-reveler">Révéler aux PJ…</button></h3>
    <form id="qs-form" class="org-form" hidden>
      <div class="champ"><span class="aide">PNJ, PJ ou faction</span><div id="qs-qui"></div></div>
      <label class="champ"><span>Depuis</span><select id="qs-seance">${optSeances}</select></label>
      <label class="champ"><span>Comment / note</span><input id="qs-note" placeholder="l'a deviné, l'a lu dans le registre…"></label>
      <div class="ligne pleine"><button class="primary petit" type="submit">Ajouter</button><button class="ghost petit" type="button" data-fermer>Annuler</button></div>
    </form>
    <form id="qs-rev" class="org-form" hidden>
      <div class="champ pleine"><span class="aide">PJ qui l'apprennent</span><div class="ligne">${pjs.length ? pjs.map(p => `<label class="ligne" style="gap:6px"><input type="checkbox" value="${p.id}" ${savent.some(r => r.autre.id === p.id) ? "checked disabled" : "checked"}>${esc(p.nom)}</label>`).join("") : `<span class="vide">Aucun PJ${campagne ? " dans cette campagne" : ""}.</span>`}</div></div>
      <label class="champ"><span>Lors de</span><select id="rev-seance">${optSeances}</select></label>
      <label class="champ"><span>Comment</span><input id="rev-note" placeholder="confession de Morvan…"></label>
      <div class="ligne pleine"><button class="primary petit" type="submit">Révéler</button><button class="ghost petit" type="button" data-fermer>Annuler</button></div>
    </form>
    ${savent.length ? `
      ${groupes.pj.length ? `<div class="rel-groupe"><small>PJ (${groupes.pj.length}/${pjs.length})</small>${groupes.pj.map(ligne).join("")}</div>` : `<p class="hint">Aucun PJ ne le sait.</p>`}
      ${groupes.pnj.length ? `<div class="rel-groupe"><small>PNJ</small>${groupes.pnj.map(ligne).join("")}</div>` : ""}
      ${groupes.faction.length ? `<div class="rel-groupe"><small>Factions</small>${groupes.faction.map(ligne).join("")}</div>` : ""}
      ${groupes.autre.length ? `<div class="rel-groupe"><small>Autres</small>${groupes.autre.map(ligne).join("")}</div>` : ""}`
      : `<p class="vide">Personne ne le sait encore (à part vous).</p>`}
    ${fonctionActive("aides") ? `<div class="rel-groupe"><small>Indices (aides de jeu) <a href="#/nouveau/aide?indice_de=${f.id}${campagne ? "&campagne=" + campagne : ""}" class="hint">＋ aide de jeu</a></small>
      ${indices.length ? indices.map(a => `<div class="rel">${lienFiche(a.fiche)}<span class="note">${a.envois.length ? "reçue par " + a.envois.map(e => esc(e.pj.nom)).join(", ") : "pas encore envoyée"}</span></div>`).join("")
        : `<p class="hint">Aucune aide de jeu n'en est l'indice.</p>`}</div>` : ""}`;
  let qui = null;
  $("#qs-ajout", box).onclick = () => {
    $("#qs-form", box).hidden = false; $("#qs-rev", box).hidden = true;
    if (!qui) qui = selecteur($("#qs-qui", box), { cibles: ["pnj", "pj", "faction"], multiple: true, valeur: [] });
  };
  $("#qs-reveler", box).onclick = () => { $("#qs-rev", box).hidden = false; $("#qs-form", box).hidden = true; };
  $$("[data-fermer]", box).forEach(b => b.onclick = () => { b.closest("form").hidden = true; });
  const reveler = async (ids, seance, note) => {
    if (!ids.length) { toast("Choisissez qui l'apprend.", true); return; }
    try {
      const r = await api("POST", `/api/secrets/${f.id}/reveler`, { qui: ids, seance: +seance || 0, note });
      toast(r.ajoutes ? `${r.ajoutes} personne${r.ajoutes > 1 ? "s" : ""} de plus le sai${r.ajoutes > 1 ? "ven" : ""}t.` : "Rien de nouveau.");
      change(); apres();
    } catch (e) { toast(e.message, true); }
  };
  $("#qs-form", box).onsubmit = e => { e.preventDefault(); reveler(qui?.valeur() || [], $("#qs-seance", box).value, $("#qs-note", box).value); };
  $("#qs-rev", box).onsubmit = e => {
    e.preventDefault();
    reveler($$("#qs-rev input[type=checkbox]:checked:not(:disabled)", box).map(x => +x.value), $("#rev-seance", box).value, $("#rev-note", box).value);
  };
  box.addEventListener("click", async e => {
    const b = e.target.closest("button[data-suppr]");
    if (!b) return;
    try { await api("DELETE", "/api/relations/" + b.dataset.suppr); change(); apres(); }
    catch (err) { toast(err.message, true); }
  });
}

/** Sur un PNJ, un PJ ou une faction : les secrets qu'il sait et ceux qui le concernent. */
export function secretsDe(box, f) {
  const sait = f.relations.filter(r => r.type === "sait" && r.sens === "sortante");
  const concernent = f.references.filter(r => r.source.type === "secret" && r.genre === "champ");
  if (!sait.length && !concernent.length) {
    box.innerHTML = `<h3>Secrets <span class="sp"></span><a href="#/nouveau/secret?concerne=${f.id}"><button class="petit">＋ Secret</button></a></h3><p class="vide">Ne sait rien de particulier.</p>`;
    return;
  }
  box.innerHTML = `<h3>Secrets <span class="sp"></span><a href="#/nouveau/secret?concerne=${f.id}"><button class="petit">＋ Secret</button></a></h3>
    ${sait.length ? `<div class="rel-groupe"><small>Ce qu'il sait</small>${sait.map(r => `<div class="rel">${lienFiche(r.autre)}${r.autre.carte?.statut ? `<span class="note" style="flex:0 0 auto;padding:0;margin:0">${esc(r.autre.carte.statut)}</span>` : ""}${r.contexte ? `<span class="note">depuis ${esc(r.contexte.nom)}</span>` : ""}</div>`).join("")}</div>` : ""}
    ${concernent.length ? `<div class="rel-groupe"><small>Le concernent</small>${concernent.map(r => `<div class="rel">${lienFiche(r.source)}${r.source.carte?.statut ? `<span class="note" style="flex:0 0 auto;padding:0;margin:0">${esc(r.source.carte.statut)}</span>` : ""}</div>`).join("")}</div>` : ""}`;
}

// ---------------------------------------------------------------- aides de jeu

/** Sur une aide de jeu : à qui elle a été envoyée ; l'envoyer à des PJ ou à tous. */
export async function envois(box, f, apres) {
  const recus = f.relations.filter(r => r.type === "recu" && r.sens === "entrante");
  const campagne = f.data.campagne;
  const [pjs, seances] = await Promise.all([
    api("GET", "/api/entites?type=pj" + (campagne ? "&campagne=" + campagne : "")),
    api("GET", "/api/entites?type=seance" + (campagne ? "&campagne=" + campagne : "")),
  ]);
  const deja = new Set(recus.map(r => r.autre.id));
  const restants = pjs.filter(p => !deja.has(p.id));
  const prevue = f.data.prevue;
  box.innerHTML = `<h3>Envoyée aux joueurs <span class="sp"></span>${lecture() || f.archive ? "" : `<button class="petit primary" id="ae-ouvrir">✉ Envoyer…</button>`}</h3>
    <form id="ae-form" class="org-form" hidden>
      <div class="champ pleine"><span class="aide">PJ qui la reçoivent</span><div class="ligne">${pjs.length ? pjs.map(p => `<label class="ligne" style="gap:6px"><input type="checkbox" value="${p.id}" ${deja.has(p.id) ? "checked disabled" : ""}>${esc(p.nom)}</label>`).join("")
        : `<span class="vide">Aucun PJ${campagne ? " dans cette campagne" : ""}.</span>`}</div>
        ${restants.length > 1 ? `<button type="button" class="ghost petit" id="ae-tous">Tous</button>` : ""}</div>
      <label class="champ"><span>Lors de</span><select id="ae-seance"><option value="">— séance —</option>${seances.map(s => `<option value="${s.id}" ${s.id === prevue ? "selected" : ""}>${esc(s.nom)}</option>`).join("")}</select></label>
      <div class="ligne pleine"><button class="primary petit" type="submit">Envoyer</button><button class="ghost petit" type="button" data-fermer>Annuler</button></div>
    </form>
    ${recus.length ? recus.map(r => `<div class="rel">${lienFiche(r.autre)}${r.contexte ? `<span class="note">${esc(r.contexte.nom)}</span>` : ""}${lecture() ? "" : `<button class="ghost petit" data-retirer="${r.id}" title="Retirer l'aide de ses documents" aria-label="Retirer à ${esc(r.autre.nom)}">✕</button>`}</div>`).join("")
      : `<p class="vide">Pas encore envoyée. Elle arrivera en direct dans l'onglet « Documents » de la page des joueurs : nom, forme, texte remis et images, jamais vos notes ni le secret dont c'est l'indice.</p>`}`;
  const form = $("#ae-form", box);
  const ouvrir = $("#ae-ouvrir", box);
  if (ouvrir) ouvrir.onclick = () => { form.hidden = !form.hidden; };
  $$("[data-fermer]", box).forEach(b => b.onclick = () => { form.hidden = true; });
  const tous = $("#ae-tous", box);
  if (tous) tous.onclick = () => $$("#ae-form input[type=checkbox]:not(:disabled)", box).forEach(x => { x.checked = true; });
  form.onsubmit = async e => {
    e.preventDefault();
    const qui = $$("#ae-form input[type=checkbox]:checked:not(:disabled)", box).map(x => +x.value);
    if (!qui.length) { toast("Cochez les PJ qui la reçoivent.", true); return; }
    try {
      const r = await api("POST", `/api/aides/${f.id}/envoyer`, { qui, seance: +$("#ae-seance", box).value || 0 });
      toast(`Envoyée à ${r.envoyes.length} PJ.${r.statut_secret ? ` Le secret passe à « ${r.statut_secret} ».` : ""}`);
      change(); apres();
    } catch (err) { toast(err.message, true); }
  };
  $$("[data-retirer]", box).forEach(b => armer(b, "Retirer ?", async () => {
    try { await api("DELETE", "/api/relations/" + b.dataset.retirer); change(); apres(); }
    catch (err) { toast(err.message, true); }
  }));
}

// ---------------------------------------------------------------- campagne : pot commun

const GENRES_POT = { entree: "Entrée", retrait: "Retrait", partage: "Partage" };

/** Sur une campagne : son pot commun, les propositions de partage à valider, partager, ajouter,
 *  retirer, et l'historique (un partage s'annule tant que le PJ a encore ce qu'il a reçu). */
export async function potCommun(box, f, apres) {
  box.id = "pot";
  box.innerHTML = `<h3>Pot commun</h3><div class="contenu"><p class="hint">…</p></div>`;
  const zone = $(".contenu", box);
  let p, pjs;
  try {
    [p, pjs] = await Promise.all([api("GET", `/api/campagnes/${f.id}/pot`), api("GET", `/api/entites?type=pj&campagne=${f.id}`)]);
  } catch (e) { zone.innerHTML = `<p class="alerte">${esc(e.message)}</p>`; return; }
  const nom = id => p.noms[id] || `fiche n°${id}`;
  const lienDe = id => p.noms[id] ? `<a href="#/f/${id}">${esc(p.noms[id])}</a>` : esc(nom(id));
  const objetsPris = x => (x.objets || []).map(o => ({ ...o, nom: p.objets.find(l => l.id === o.ligne)?.nom || "objet parti du pot" }));
  const ecrire = !lecture(), feuilles = fonctionActive("feuilles");
  const vide = !textePieces(p.pieces) && !p.objets.length;
  $("h3", box).innerHTML = `Pot commun <span class="sp"></span><span class="hint">${vide ? "vide" : esc(textePieces(p.pieces) || "pas de pièce")}${p.objets.length ? ` · ${p.objets.length} objet${p.objets.length > 1 ? "s" : ""}` : ""}</span>`;

  const propositions = p.propositions.map(x => `<div class="pot-prop"><div><b>${lienDe(x.pj)}</b> : ${esc(texteButin({ pieces: x.pieces, objets: objetsPris(x) }) || "rien")}
      ${x.note ? `<span class="hint">« ${esc(x.note)} »</span>` : ""}<br><span class="hint">proposé par ${esc(x.par)} le ${dateHeure(x.le)}</span></div>
      ${ecrire ? `<div class="ligne"><button class="petit primary" data-valider="${x.id}"${feuilles ? "" : " disabled title=\"Les feuilles de personnage sont désactivées\""}>Valider</button><button class="petit ghost" data-refuser="${x.id}">Refuser</button></div>` : ""}</div>`).join("");
  const historique = [...p.historique].reverse().map(m => `<li><span class="hint">${dateHeure(m.le)}</span> <b>${GENRES_POT[m.genre] || esc(m.genre)}</b>
      ${m.genre === "partage" ? `→ ${lienDe(m.pj)}` : ""}${m.rencontre ? ` · ${lienDe(m.rencontre)}` : m.libelle ? ` · ${esc(m.libelle)}` : ""} : ${esc(texteButin(m) || "rien")}
      ${m.annule ? `<span class="badge">annulé</span>` : m.genre === "partage" && ecrire ? `<button class="petit ghost" data-annuler="${m.id}" title="Reprendre au PJ ce qu'il a reçu et le remettre dans le pot">Annuler</button>` : ""}</li>`).join("");

  zone.innerHTML = `${vide ? `<p class="vide">Le pot est vide. Le butin d'une rencontre y arrive à la fin du combat ; vous pouvez aussi y ajouter une récompense.</p>`
      : `<div class="pot-pieces">${esc(textePieces(p.pieces) || "Aucune pièce")}</div>
        ${p.objets.length ? `<ul class="pot-objets">${p.objets.map(o => `<li>${o.fiche ? lienDe(o.fiche) : esc(o.nom)}${o.quantite > 1 ? ` <span class="hint">× ${o.quantite}</span>` : ""}</li>`).join("")}</ul>` : ""}`}
    ${propositions ? `<div class="rel-groupe"><small>Propositions de partage à valider</small>${propositions}</div>` : ""}
    ${ecrire && !vide ? `<details class="pot-volet" id="pot-partage"><summary>Partager ou retirer</summary><form class="colonne" style="gap:8px">
        <div data-pieces></div>
        ${p.objets.length ? `<div class="pot-choix">${p.objets.map(o => `<label class="ligne"><input type="number" min="0" max="${o.quantite}" value="0" data-ligne="${o.id}" aria-label="Quantité de ${esc(o.nom)}"><span>${esc(o.nom)} <span class="hint">sur ${o.quantite}</span></span></label>`).join("")}</div>` : ""}
        <div class="ligne"><select data-pj aria-label="PJ qui reçoit"><option value="">— PJ qui reçoit —</option>${pjs.map(x => `<option value="${x.id}">${esc(x.nom)}</option>`).join("")}</select>
          <input data-note placeholder="Note (facultatif)" aria-label="Note" style="flex:1;min-width:140px"></div>
        <div class="ligne"><button type="button" class="primary petit" data-donner${feuilles ? "" : " disabled"}>Donner au PJ</button><button type="button" class="ghost petit" data-retirer>Retirer du pot</button>
          ${feuilles ? `<span class="hint">les pièces vont dans sa bourse, les objets dans son inventaire</span>` : `<span class="hint">Les feuilles de personnage sont désactivées : rien ne peut être donné.</span>`}</div>
      </form></details>` : ""}
    ${ecrire ? `<details class="pot-volet" id="pot-ajout"><summary>Ajouter au pot</summary><form class="colonne" style="gap:8px">
        <div data-butin></div><input data-libelle placeholder="D'où cela vient : récompense, trésor…" aria-label="Origine">
        <div><button type="submit" class="primary petit">Ajouter</button></div></form></details>` : ""}
    ${historique ? `<details class="pot-volet"><summary>Historique</summary><ul class="pot-histo">${historique}</ul></details>` : ""}`;

  const action = async (chemin, corps, message) => {
    try { await api("POST", `/api/campagnes/${f.id}/pot/${chemin}`, corps); toast(message); change(); apres(); }
    catch (err) { toast(err.message, true); }
  };
  $$("[data-valider]", zone).forEach(b => b.onclick = () => action(`propositions/${b.dataset.valider}/valider`, {}, "Partage validé : le PJ a reçu sa part."));
  $$("[data-refuser]", zone).forEach(b => armer(b, "Refuser ?", () => action(`propositions/${b.dataset.refuser}/refuser`, {}, "Proposition refusée.")));
  $$("[data-annuler]", zone).forEach(b => armer(b, "Annuler ?", () => action(`partages/${b.dataset.annuler}/annuler`, {}, "Partage annulé : tout est revenu au pot.")));

  const partage = $("#pot-partage form", zone);
  if (partage) {
    const pieces = saisiePieces($("[data-pieces]", partage), {});
    const choix = () => ({ pieces: pieces.lire(), objets: $$("[data-ligne]", partage).map(i => ({ ligne: +i.dataset.ligne, quantite: Math.trunc(+i.value || 0) })).filter(o => o.quantite > 0) });
    $("[data-donner]", partage).onclick = () => {
      const pj = +$("[data-pj]", partage).value;
      if (!pj) { toast("Choisissez le PJ qui reçoit.", true); return; }
      action("partager", { pj, ...choix(), note: $("[data-note]", partage).value.trim() }, "Partagé : le PJ a reçu sa part.");
    };
    armer($("[data-retirer]", partage), "Retirer ?", () => action("retirer", { ...choix(), libelle: $("[data-note]", partage).value.trim() }, "Retiré du pot."));
  }
  const ajout = $("#pot-ajout form", zone);
  if (ajout) {
    const ed = editeurButin($("[data-butin]", ajout), null, {});
    ajout.onsubmit = e => {
      e.preventDefault();
      const b = ed.valeur();
      if (!b) { toast("Rien à ajouter : des pièces ou un objet.", true); return; }
      action("ajouter", { ...b, libelle: $("[data-libelle]", ajout).value.trim() }, "Ajouté au pot.");
    };
  }
}
