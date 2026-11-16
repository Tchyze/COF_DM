// Agenda des séances : les dates RÉELLES où la table se retrouve (le calendrier du monde, lui, est
// dans la chronologie). Liste à venir ou vue par mois ; planifier, déplacer, dupliquer, annuler ;
// conflits d'horaires en orange. Tout est rangé dans les fiches « séance ».
import { $, $$, esc, api, toast, lien, dialogue, delai } from "../ui.js";
import { panneauAbonnement } from "../abonnement.js";

const JOURS = ["lun.", "mar.", "mer.", "jeu.", "ven.", "sam.", "dim."];
const lecture = () => !!window.cofAcces?.lecture_seule;

/** « samedi 6 mars 2027 » d'une date AAAA-MM-JJ. */
export const texteJour = (d, court = false) => new Date(d + "T12:00:00").toLocaleDateString("fr-FR",
  court ? { weekday: "short", day: "numeric", month: "short" } : { weekday: "long", day: "numeric", month: "long", year: "numeric" });
const texteMois = d => new Date(d + "T12:00:00").toLocaleDateString("fr-FR", { month: "long", year: "numeric" });
/** L'heure locale à l'univers d'une date RFC 3339 rendue par le serveur (« 20:00+01:00 » → « 20:00 »). */
const heureDe = iso => iso.slice(11, 16);
const jourDe = iso => iso.slice(0, 10);
const aujourdhui = () => { const d = new Date(); d.setMinutes(d.getMinutes() - d.getTimezoneOffset()); return d.toISOString().slice(0, 10); };
const plusJours = (d, n) => { const x = new Date(d + "T12:00:00Z"); x.setUTCDate(x.getUTCDate() + n); return x.toISOString().slice(0, 10); };

/** Les horaires d'une séance : « 20:00 – 00:00 », « toute la journée ». */
export function horaires(s) {
  if (s.jour_entier) return "toute la journée";
  const fin = heureDe(s.fin) + (jourDe(s.fin) !== s.date ? " (lendemain)" : "");
  return `${heureDe(s.debut)} – ${fin}`;
}

const MARQUES = { "présent": ["✓", "présent"], "peut-être": ["?", "peut-être"], "absent": ["✗", "absent"] };

/** Qui est attendu, avec sa réponse (✓ présent, ? peut-être, ✗ absent), et le compte des réponses. */
export function texteParticipants(participants, { resume = true } = {}) {
  if (!participants?.length) return "";
  const noms = participants.map(p => {
    const nom = esc(p.joueur ? `${p.joueur} (${p.nom})` : p.nom);
    const m = MARQUES[p.reponse];
    return m ? `<span class="rep rep-${esc(p.reponse.replace(/[^a-z]/g, ""))}" title="${esc(m[1])}">${nom} ${m[0]}</span>` : `<span class="rep" title="pas encore répondu">${nom}</span>`;
  }).join(", ");
  const n = r => participants.filter(p => p.reponse === r).length, attente = participants.filter(p => !p.reponse).length;
  const bilan = [n("présent") && `${n("présent")} présent${n("présent") > 1 ? "s" : ""}`, n("peut-être") && `${n("peut-être")} peut-être`,
    n("absent") && `${n("absent")} absent${n("absent") > 1 ? "s" : ""}`, attente && `${attente} sans réponse`].filter(Boolean).join(" · ");
  return `<div class="hint">👥 ${noms}${resume && bilan && attente < participants.length ? ` <span class="rep-bilan">— ${bilan}</span>` : ""}</div>`;
}

/** Les conflits d'une séance, en orange : une alerte pour un chevauchement, un message pour moins d'une heure d'écart. */
export function texteConflits(conflits, { court = false } = {}) {
  if (!conflits?.length) return "";
  return `<div class="agenda-conflits">${conflits.map(c => {
    const qui = c.personnes.join(", ");
    // un autre univers du serveur : rien n'en sort, ni la séance, ni l'heure
    if (c.genre === "ailleurs") return `<div class="agenda-conflit">⚠ Conflit avec un autre univers du serveur : ${esc(qui)} ${c.personnes.length > 1 ? "ont" : "a"} déjà quelque chose à ce moment-là.</div>`;
    const quand = `${texteJour(jourDe(c.debut), true)} à ${heureDe(c.debut)}`;
    const autre = `<a href="#/f/${c.seance}">${esc(c.nom)}</a>${c.campagne ? ` (${esc(c.campagne)})` : ""}`;
    return c.genre === "chevauchement"
      ? `<div class="agenda-conflit">⚠ Conflit${court ? "" : ` avec ${autre}, ${esc(quand)}`} : ${esc(qui)} ${c.personnes.length > 1 ? "y sont" : "y est"} aussi attendu${c.personnes.length > 1 ? "s" : ""}.</div>`
      : `<div class="agenda-conflit proche">⏱ Moins d'une heure ${court ? "avec une autre séance" : `avec ${autre}, ${esc(quand)}`} pour ${esc(qui)}.</div>`;
  }).join("")}</div>`;
}

let reglages = null;
const lireReglages = async () => (reglages ||= await api("GET", "/api/agenda/reglages"));

export async function mount(el, params) {
  const [campagnes] = await Promise.all([api("GET", "/api/entites?type=campagne"), lireReglages()]);
  let camp = +(params.get("campagne") || 0);
  let vue = params.get("vue") === "mois" ? "mois" : "liste";
  let passees = false;
  let mois = aujourdhui().slice(0, 7);
  el.innerHTML = `
    <div class="agenda">
      <div class="entete"><h1>Agenda des séances</h1><span class="sp"></span>
        <select id="camp" style="width:auto" aria-label="Campagne"><option value="0">Toutes les campagnes</option>${campagnes.map(c => `<option value="${c.id}" ${c.id === camp ? "selected" : ""}>${esc(c.nom)}</option>`).join("")}</select>
        <div class="agenda-bascule" role="group" aria-label="Affichage">
          <button type="button" data-vue="liste">Liste</button><button type="button" data-vue="mois">Mois</button>
        </div>
        ${lecture() ? "" : `<button class="primary" id="nouvelle">+ Planifier</button>`}
      </div>
      <p class="hint agenda-aide">Les dates réelles des séances (le calendrier du monde est dans la chronologie). Heures dans le fuseau
        <b id="fuseau">${esc(reglages.fuseau)}</b>${lecture() ? "" : ` · <button type="button" class="lien-bouton" id="changer-fuseau">changer</button>`}
        · <a id="ics" download>⬇ Fichier .ics</a> à importer dans un agenda (Google, Apple, Proton, Outlook…).</p>
      <div id="contenu"></div>
      ${window.cofAcces?.serveur ? `<section class="panneau abonnement" id="abonnement"></section>` : ""}
    </div>`;
  if ($("#abonnement", el)) panneauAbonnement($("#abonnement", el));
  const majIcs = () => { $("#ics", el).href = lien("/api/agenda/ics" + (camp ? "?campagne=" + camp : "")); };
  majIcs();
  let tour = 0; // seule la dernière réponse s'affiche, quand deux chargements se croisent
  const charger = async () => {
    const t = ++tour;
    const r = await api("GET", "/api/agenda" + (camp ? "?campagne=" + camp : ""));
    if (t !== tour) return;
    $$("[data-vue]", el).forEach(b => b.classList.toggle("on", b.dataset.vue === vue));
    if (vue === "mois") dessinerMois(r.seances); else dessinerListe(r.seances);
  };
  const dessinerListe = seances => {
    const auj = aujourdhui();
    const avenir = seances.filter(s => jourDe(s.fin) >= auj);
    const vieilles = seances.filter(s => jourDe(s.fin) < auj).reverse();
    const l = passees ? vieilles : avenir;
    let html = "";
    if (!l.length) {
      html = `<p class="vide">${passees ? "Aucune séance passée." : `Aucune séance à venir.${lecture() ? "" : " « + Planifier » pour en ajouter une ; une séance de l'agenda est une fiche « séance » qui a une date réelle."}`}</p>`;
    } else {
      let m = null;
      for (const s of l) {
        if (s.date.slice(0, 7) !== m) { m = s.date.slice(0, 7); html += `<h3 class="agenda-mois">${esc(texteMois(s.date))}</h3>`; }
        html += carte(s);
      }
    }
    html += `<p><button type="button" class="ghost petit" id="basculer-passees">${passees ? "← Séances à venir" : `Séances passées (${vieilles.length})`}</button></p>`;
    $("#contenu", el).innerHTML = html;
    $("#basculer-passees", el).onclick = () => { passees = !passees; charger(); };
    brancher(seances);
  };
  const dessinerMois = seances => {
    const premier = mois + "-01";
    const d0 = new Date(premier + "T12:00:00Z");
    const decalage = (d0.getUTCDay() + 6) % 7; // lundi en premier
    const debut = plusJours(premier, -decalage);
    const parJour = {};
    for (const s of seances) (parJour[s.date] ||= []).push(s);
    let cases = "";
    for (let i = 0; i < 42; i++) {
      const j = plusJours(debut, i);
      if (i === 35 && j.slice(0, 7) !== mois) break;
      const l = parJour[j] || [];
      cases += `<div class="agenda-jour${j.slice(0, 7) !== mois ? " hors" : ""}${j === aujourdhui() ? " auj" : ""}" data-jour="${j}">
        <span class="n">${+j.slice(8)}</span>
        ${l.map(s => `<button type="button" class="agenda-puce${s.statut === "annulée" ? " annulee" : ""}${s.conflits.some(c => c.genre !== "rapproche") ? " conflit" : s.conflits.length ? " proche" : ""}" data-id="${s.id}" title="${esc(s.nom)}${s.campagne ? " · " + esc(s.campagne.nom) : ""}">${s.jour_entier ? "" : `<b>${heureDe(s.debut)}</b> `}${esc(s.nom)}</button>`).join("")}
        ${lecture() ? "" : `<button type="button" class="agenda-ajout ghost petit" data-ajout="${j}" aria-label="Planifier le ${esc(texteJour(j))}">＋</button>`}
      </div>`;
    }
    $("#contenu", el).innerHTML = `
      <div class="agenda-nav"><button type="button" class="ghost" id="m-prec" aria-label="Mois précédent">‹</button>
        <h3>${esc(texteMois(premier))}</h3><button type="button" class="ghost" id="m-suiv" aria-label="Mois suivant">›</button>
        <button type="button" class="ghost petit" id="m-auj">Aujourd'hui</button></div>
      <div class="agenda-grille">${JOURS.map(j => `<div class="agenda-tete">${j}</div>`).join("")}${cases}</div>`;
    const changer = n => { const d = new Date(mois + "-01T12:00:00Z"); d.setUTCMonth(d.getUTCMonth() + n); mois = d.toISOString().slice(0, 7); charger(); };
    $("#m-prec", el).onclick = () => changer(-1);
    $("#m-suiv", el).onclick = () => changer(1);
    $("#m-auj", el).onclick = () => { mois = aujourdhui().slice(0, 7); charger(); };
    $$(".agenda-puce", el).forEach(b => b.onclick = () => {
      const s = seances.find(x => x.id === +b.dataset.id);
      const { voile, fermer } = dialogue(s.nom, carte(s, true), { piege: true, croix: true, classe: "agenda" });
      brancher(seances, voile, fermer);
    });
    $$("[data-ajout]", el).forEach(b => b.onclick = () => planifier({ date: b.dataset.ajout, campagne: camp }, charger, campagnes));
  };
  const carte = (s, seule = false) => `
    <article class="agenda-carte${s.statut === "annulée" ? " annulee" : ""}${s.conflits.some(c => c.genre !== "rapproche") ? " conflit" : s.conflits.length ? " proche" : ""}" data-id="${s.id}">
      <div class="agenda-quand"><b>${esc(texteJour(s.date))}</b><span>${esc(horaires(s))}</span></div>
      <div class="agenda-quoi">
        <div class="ligne"><a class="agenda-nom" href="#/f/${s.id}">${esc(s.nom)}</a>
          ${s.campagne ? `<span class="badge">${esc(s.campagne.nom)}</span>` : ""}
          ${s.nature && s.nature !== "séance" ? `<span class="badge tag">${esc(s.nature)}</span>` : ""}
          ${s.statut === "annulée" ? `<span class="badge archive">annulée</span>` : s.statut === "jouée" ? `<span class="badge">jouée</span>` : ""}</div>
        ${s.lieu || s.visio ? `<div class="hint">${s.lieu ? `📍 ${esc(s.lieu)}` : ""}${s.lieu && s.visio ? " · " : ""}${s.visio ? `🔗 ${/^https?:\/\//.test(s.visio) ? `<a href="${esc(s.visio)}" target="_blank" rel="noopener noreferrer">${esc(s.visio)}</a>` : esc(s.visio)}` : ""}</div>` : ""}
        ${texteParticipants(s.participants)}
        ${s.statut === "annulée" ? "" : texteConflits(s.conflits)}
      </div>
      ${lecture() ? "" : `<div class="agenda-actions">
        <button type="button" class="petit" data-action="modifier">✎ Modifier</button>
        <button type="button" class="petit ghost" data-action="dupliquer">⧉ Dupliquer</button>
        <button type="button" class="petit ghost" data-action="${s.statut === "annulée" ? "retablir" : "annuler"}">${s.statut === "annulée" ? "↺ Rétablir" : "✕ Annuler"}</button>
      </div>`}
      ${seule ? `<p><a href="#/f/${s.id}">Ouvrir la fiche</a></p>` : ""}
    </article>`;
  const brancher = (seances, racine = el, fermer = null) => {
    $$(".agenda-carte [data-action]", racine).forEach(b => b.onclick = async () => {
      const s = seances.find(x => x.id === +b.closest(".agenda-carte").dataset.id);
      fermer?.();
      const a = b.dataset.action;
      if (a === "modifier") return planifier(s, charger, campagnes);
      if (a === "dupliquer") return dupliquer(s, charger);
      try {
        await api("PATCH", "/api/entites/" + s.id, { data: { statut: a === "annuler" ? "annulée" : "prévue" } });
        toast(a === "annuler" ? "Séance annulée : elle reste visible, barrée." : "Séance rétablie.");
        charger();
      } catch (e) { toast(e.message, true); }
    });
  };
  $("#camp", el).onchange = e => { camp = +e.target.value; majIcs(); charger(); };
  $$("[data-vue]", el).forEach(b => b.onclick = () => { vue = b.dataset.vue; charger(); });
  if ($("#nouvelle", el)) $("#nouvelle", el).onclick = () => planifier({ campagne: camp }, charger, campagnes);
  if ($("#changer-fuseau", el)) $("#changer-fuseau", el).onclick = () => changerFuseau(() => { $("#fuseau", el).textContent = reglages.fuseau; charger(); });
  await charger();
}

/** Le numéro qui suit la dernière séance de la campagne. */
async function numeroSuivant(campagne) {
  if (!campagne) return null;
  return (await api("GET", "/api/agenda?campagne=" + campagne)).numero_suivant || null;
}

/** Fenêtre « Planifier » (nouvelle séance) ou « Modifier » (s : une séance de l'agenda). Les conflits s'affichent en direct. */
export async function planifier(s, apres, campagnes) {
  const r = await lireReglages();
  campagnes ||= await api("GET", "/api/entites?type=campagne");
  const nouvelle = !s.id;
  const camp = s.campagne?.id ?? s.campagne ?? 0;
  const { voile, fermer } = dialogue(nouvelle ? "Planifier une séance" : `Modifier « ${s.nom} »`, `
    <form class="formulaire agenda-form" novalidate>
      <div class="erreur" hidden></div>
      <div class="grille2">
        <label class="champ"><span>Campagne</span><select name="campagne"><option value="0">—</option>${campagnes.map(c => `<option value="${c.id}" ${c.id === camp ? "selected" : ""}>${esc(c.nom)}</option>`).join("")}</select></label>
        <label class="champ"><span>Nature</span><select name="nature">${["séance", "séance zéro", "création de personnages", "autre"].map(n => `<option ${n === (s.nature || "séance") ? "selected" : ""}>${esc(n)}</option>`).join("")}</select></label>
        <label class="champ"><span>Date</span><input type="date" name="date" required value="${esc(s.date || "")}"></label>
        <label class="champ"><span>Heure de début</span><input type="time" name="heure" step="300" value="${esc(s.heure || "")}"></label>
        <label class="champ"><span>Durée</span><select name="duree">${r.durees.map(d => `<option ${d === (s.duree || r.duree_defaut) ? "selected" : ""}>${esc(d)}</option>`).join("")}</select></label>
        <label class="champ"><span>Nom</span><input name="nom" value="${esc(s.nom || "")}" placeholder="Séance n" autocomplete="off"></label>
      </div>
      <label class="champ"><span>Lieu (réel) <small>facultatif</small></span><input name="lieu" value="${esc(s.lieu || "")}" autocomplete="off"></label>
      <label class="champ"><span>Lien de visio <small>facultatif</small></span><input name="visio" value="${esc(s.visio || "")}" autocomplete="off" inputmode="url"></label>
      <label class="champ"><span>Pour les joueurs <small>montré dans leur agenda</small></span><textarea name="infos" rows="2">${esc(s.infos || "")}</textarea></label>
      <div class="agenda-verif" aria-live="polite"></div>
      <div class="barre-form"><button type="button" class="ghost" data-fermer>Annuler</button><button type="submit" class="primary">${nouvelle ? "Planifier" : "Enregistrer"}</button></div>
    </form>`, { piege: true, croix: true, classe: "agenda" });
  const f = $("form", voile), champ = n => f.elements[n];
  $("[data-fermer]", voile).onclick = fermer;
  if (nouvelle && !s.heure) champ("heure").value = "";
  // le nom proposé suit le numéro de la campagne
  let numero = s.numero || null;
  const proposerNom = async () => {
    if (!nouvelle) return;
    numero = await numeroSuivant(+champ("campagne").value);
    champ("nom").placeholder = numero ? `Séance ${numero}` : "Séance";
  };
  const verifier = delai(async () => {
    const zone = $(".agenda-verif", voile);
    if (!champ("date").value) { zone.innerHTML = ""; return; }
    const q = new URLSearchParams({ date: champ("date").value, heure: champ("heure").value, duree: champ("duree").value, campagne: champ("campagne").value });
    if (!nouvelle) q.set("seance", s.id);
    try {
      const l = await api("GET", "/api/agenda/conflits?" + q);
      zone.innerHTML = l.length ? texteConflits(l) : `<p class="hint">✓ Aucun conflit d'horaire.</p>`;
    } catch (e) { zone.innerHTML = `<p class="hint">${esc(e.message)}</p>`; }
  }, 250);
  f.addEventListener("input", verifier);
  f.addEventListener("change", e => { if (e.target.name === "campagne") proposerNom(); verifier(); });
  proposerNom();
  verifier();
  f.onsubmit = async e => {
    e.preventDefault();
    const err = $(".erreur", f);
    err.hidden = true;
    if (!champ("date").value) { err.textContent = "Choisissez une date."; err.hidden = false; return; }
    const v = n => champ(n).value.trim() || null;
    const data = { campagne: +champ("campagne").value || null, nature: champ("nature").value === "séance" ? null : champ("nature").value,
      date_reelle: champ("date").value, heure: v("heure"), duree: v("duree"), lieu_reel: v("lieu"), visio: v("visio"), infos_joueurs: v("infos") };
    try {
      if (nouvelle) {
        data.statut = "prévue";
        if (numero) data.numero = numero;
        await api("POST", "/api/entites", { type: "seance", nom: v("nom") || (numero ? `Séance ${numero}` : "Séance"), data });
        toast("Séance planifiée.");
      } else {
        await api("PATCH", "/api/entites/" + s.id, { nom: v("nom") || s.nom, data, modifie: s.modifie });
        toast("Séance enregistrée.");
      }
      fermer();
      window.dispatchEvent(new Event("cof-change"));
      apres?.();
    } catch (ex) { err.textContent = ex.message; err.hidden = false; }
  };
}

/** Dupliquer : la même séance une ou deux semaines plus tard (ou à une autre date), numéro suivant. */
function dupliquer(s, apres) {
  const { voile, fermer } = dialogue(`Dupliquer « ${s.nom} »`, `
    <p class="hint">Même campagne, même heure, même durée, même lieu ; le numéro suit.</p>
    <div class="ligne" style="flex-wrap:wrap;gap:8px">
      <button type="button" class="primary" data-j="7">Dans 1 semaine (${esc(texteJour(plusJours(s.date, 7), true))})</button>
      <button type="button" data-j="14">Dans 2 semaines (${esc(texteJour(plusJours(s.date, 14), true))})</button>
    </div>
    <label class="champ" style="margin-top:12px"><span>Ou à une autre date</span><input type="date" name="autre"></label>
    <div class="barre-form"><button type="button" class="ghost" data-fermer>Annuler</button><button type="button" data-autre>Dupliquer à cette date</button></div>`,
  { piege: true, croix: true, classe: "agenda" });
  const copier = async date => {
    if (!date) return;
    try {
      const numero = await numeroSuivant(s.campagne?.id);
      const data = { campagne: s.campagne?.id || null, nature: s.nature || null, statut: "prévue", date_reelle: date, heure: s.heure || null,
        duree: s.duree || null, lieu_reel: s.lieu || null, visio: s.visio || null, infos_joueurs: s.infos || null };
      if (numero) data.numero = numero;
      const nom = numero ? `Séance ${numero}` : s.nom;
      await api("POST", "/api/entites", { type: "seance", nom, data });
      fermer();
      toast(`« ${nom} » planifiée le ${texteJour(date)}.`);
      window.dispatchEvent(new Event("cof-change"));
      apres?.();
    } catch (e) { toast(e.message, true); }
  };
  $$("[data-j]", voile).forEach(b => b.onclick = () => copier(plusJours(s.date, +b.dataset.j)));
  $("[data-autre]", voile).onclick = () => copier($("[name=autre]", voile).value);
  $("[data-fermer]", voile).onclick = fermer;
}

/** Le fuseau horaire de l'univers : les heures des séances y sont écrites. */
function changerFuseau(apres) {
  let fuseaux = [];
  try { fuseaux = Intl.supportedValuesOf("timeZone"); } catch { /* navigateur ancien : saisie libre */ }
  const { voile, fermer } = dialogue("Fuseau horaire de l'agenda", `
    <p class="hint">Les heures des séances sont écrites dans ce fuseau. Chaque joueur les voit ensuite à son heure dans son propre agenda, heure d'été comprise.</p>
    <label class="champ"><span>Fuseau</span><input name="fuseau" list="agenda-fuseaux" value="${esc(reglages.fuseau)}" autocomplete="off"></label>
    <datalist id="agenda-fuseaux">${fuseaux.map(z => `<option value="${esc(z)}">`).join("")}</datalist>
    <div class="erreur" hidden></div>
    <div class="barre-form"><button type="button" class="ghost" data-fermer>Annuler</button><button type="button" class="primary" data-ok>Enregistrer</button></div>`,
  { piege: true, croix: true });
  $("[data-fermer]", voile).onclick = fermer;
  $("[data-ok]", voile).onclick = async () => {
    try {
      reglages = await api("PUT", "/api/agenda/reglages", { fuseau: $("[name=fuseau]", voile).value.trim() });
      fermer();
      toast("Fuseau horaire enregistré.");
      apres();
    } catch (e) { const x = $(".erreur", voile); x.textContent = e.message; x.hidden = false; }
  };
}
