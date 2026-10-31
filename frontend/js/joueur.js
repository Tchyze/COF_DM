// Page des joueurs : la feuille de son personnage, le combat en cours (l'ordre d'initiative et les
// états, sans aucun chiffre), les documents (aides de jeu envoyées par le MJ), les dés (ses jets et
// ceux de la table), le pot commun de sa campagne (il y propose un partage) et les hauts faits. À la maison, la page est libre quand le MJ l'a
// ouverte et chacun y choisit son PJ ; sur un serveur, on y entre par son compte, avec son PJ : c'est
// alors tout ce qu'un compte joueur a de l'application (son compte, et le choix de l'univers).
import { $, $$, esc, api, toast, lien, universOnglet, choisirUniversOnglet } from "./ui.js";
import { panneauCompte } from "./views/comptes.js";
import { panneauFeuille, CARACS } from "./views/feuille.js";
import { monterNiveau } from "./niveau.js";
import { lignesEffets, libellePar, voieLiee } from "./capacites.js";
import { recapHTML, recapMarkdown, dateCourte } from "./recap.js";
import { ligneJet, DES_RAPIDES } from "./des.js";
import { rendu } from "./md.js";

const CLE = "cof-joueur-pj";
const etat = { serveur: false, edition: false, combat: null, pj: null, pjs: [], vue: "fiche", de: 20, jets: [], dernierJet: 0, crit: 20, docs: [], doc: 0, fonctions: {} };
// les onglets et la fonctionnalité de l'univers dont chacun dépend (Paramètres → Fonctionnalités)
const ONGLETS = [["fiche", "feuilles"], ["voies", "feuilles"], ["combat", "combat"], ["documents", "aides"], ["pot", "butin"], ["des", "des"], ["hauts-faits", "combat"]];
const fonction = cle => etat.fonctions[cle] !== false;
const onglets = () => ONGLETS.filter(([, f]) => fonction(f)).map(([v]) => v).concat(etat.serveur ? ["compte"] : []);
const vue = $("#vue");

const lireChoix = () => { try { return +localStorage.getItem(CLE) || 0; } catch { return 0; } };
const garderChoix = id => { try { id ? localStorage.setItem(CLE, id) : localStorage.removeItem(CLE); } catch { /* stockage indisponible */ } };
const moi = p => !!p && !!etat.pj && p.pj === etat.pj.id;

async function copier(texte) {
  try { await navigator.clipboard.writeText(texte); return true; } catch { return false; }
}

/* ---------------------------------------------------------- choix du personnage */
function vueChoix() {
  vue.className = "combat";
  vue.innerHTML = `<section class="panel pj-choix">
      <h1>Quel est votre personnage ?</h1>
      <p class="hint">Le choix est gardé sur cet appareil. Vous pourrez en changer.</p>
      <div class="cartes-pj">${etat.pjs.map(p => `<button class="carte-pj${etat.pj?.id === p.id ? " sel" : ""}" data-pj="${p.id}">
          <span class="nom">${esc(p.nom)}</span>
          <span class="hint">${esc([p.peuple, p.profil, p.niveau ? "niveau " + p.niveau : ""].filter(Boolean).join(" · "))}</span>
          ${p.joueur ? `<span class="hint">joué par ${esc(p.joueur)}</span>` : ""}
          ${p.statut === "absent" ? `<span class="tag">absent</span>` : ""}
        </button>`).join("") || `<p class="empty">Aucun personnage pour l'instant : le MJ doit d'abord créer les PJ.</p>`}</div>
    </section>`;
  $$("[data-pj]", vue).forEach(b => {
    b.onclick = () => {
      etat.pj = etat.pjs.find(p => p.id === +b.dataset.pj);
      garderChoix(etat.pj.id);
      etat.docs = []; chargerDocs();
      location.hash = "#/";
      route();
      proposerMontee();
    };
  });
}

/* ---------------------------------------------------------- ma fiche */
async function vueFiche() {
  const p = etat.pj;
  vue.className = "";
  vue.innerHTML = `<div class="pj-fiche">
      <div class="pj-tete"><h1>${esc(p.nom)}</h1>
        <span class="hint">${esc([p.peuple, p.profil, p.niveau ? "niveau " + p.niveau : ""].filter(Boolean).join(" · "))}</span>
        <span class="badge">${etat.edition ? "🔓 feuille modifiable" : "🔒 feuille verrouillée par le MJ"}</span></div>
      <section class="panneau" id="p-feuille"></section>
      ${p.recit?.length ? `<section class="panneau" id="p-recit"></section>` : ""}
    </div>`;
  if ($("#p-recit", vue)) peindreRecit($("#p-recit", vue));
  await panneauFeuille($("#p-feuille", vue), { id: p.id, nom: p.nom, data: { profil: p.profil } }, { peutEcrire: etat.edition, joueur: true });
}

// Les champs de la fiche que le joueur tient lui-même (apparence…). Sur un serveur, il les modifie
// quand le MJ a ouvert l'édition ; à la maison, c'est le MJ qui les écrit.
function peindreRecit(box, saisie = false) {
  const p = etat.pj, modifiable = etat.serveur && etat.edition;
  if (saisie && modifiable) {
    box.innerHTML = `<h3>Mon personnage</h3><form class="colonne" id="recit-form" style="gap:10px">
      ${p.recit.map(c => `<label class="champ"><span>${esc(c.nom)}</span><textarea data-cle="${esc(c.cle)}" rows="5" spellcheck="true">${esc(c.texte)}</textarea></label>`).join("")}
      <div class="ligne"><button class="primary" type="submit">Enregistrer</button><button type="button" id="recit-annuler">Annuler</button></div></form>`;
    $("#recit-annuler", box).onclick = () => peindreRecit(box);
    $("#recit-form", box).onsubmit = async e => {
      e.preventDefault();
      const data = {};
      for (const z of $$("textarea[data-cle]", box)) if (z.value !== (p.recit.find(c => c.cle === z.dataset.cle)?.texte ?? "")) data[z.dataset.cle] = z.value;
      if (!Object.keys(data).length) { peindreRecit(box); return; }
      try {
        const maj = await api("PATCH", `/api/joueur/pj/${p.id}`, { data, modifie: p.modifie });
        Object.assign(p, maj);
        toast("Fiche enregistrée.");
        peindreRecit(box);
      } catch (err) { toast(err.message, true); }
    };
    return;
  }
  const remplis = p.recit.filter(c => c.texte);
  box.innerHTML = `<h3>Mon personnage${modifiable ? ` <button class="petit" id="recit-modifier">✎ Modifier</button>` : ""}</h3>
    ${remplis.map(c => `<h4>${esc(c.nom)}</h4><div class="md">${rendu(c.texte)}</div>`).join("") || `<p class="empty">${modifiable ? "Rien d'écrit pour l'instant : à vous de le raconter." : "Rien d'écrit pour l'instant."}</p>`}`;
  const bouton = $("#recit-modifier", box);
  if (bouton) bouton.onclick = () => peindreRecit(box, true);
}
// La feuille a changé ailleurs (combat, MJ) : on la relit, sauf pendant une saisie.
function rafraichirFiche() {
  if (etat.vue === "fiche" && etat.pj && !$("#fp-form", vue) && !$("#recit-form", vue)) vueFiche();
  if (etat.vue === "voies" && etat.pj) vueVoies();
}

// Un niveau accordé par le MJ : l'assistant de montée s'ouvre seul. « Plus tard » le laisse fermé
// jusqu'à la prochaine visite (le bouton « Passer au niveau » reste sur la fiche).
const cleReport = (pj, n) => `cofdm-montee-${pj}-${n}`;
async function proposerMontee() {
  if (!etat.pj || !fonction("feuilles") || document.getElementById("mn")) return;
  const pj = etat.pj;
  let f;
  try { f = await api("GET", `/api/entites/${pj.id}/feuille`); } catch { return; }
  if (!f.existe || f.niveau_accorde !== f.niveau + 1) return;
  try { if (sessionStorage.getItem(cleReport(pj.id, f.niveau_accorde))) return; } catch { /* sans stockage : on propose */ }
  monterNiveau(pj.id, pj.nom, () => relirePJ(), {
    joueur: true,
    plusTard: () => { try { sessionStorage.setItem(cleReport(pj.id, f.niveau_accorde), "1"); } catch { /* rien à retenir */ } },
  });
}

// La fiche du PJ a pu changer (niveau, date en jeu, récit écrit sur un autre appareil).
async function relirePJ() {
  try { etat.pjs = await api("GET", "/api/joueur/pj"); } catch { return; }
  const p = etat.pjs.find(x => x.id === etat.pj?.id);
  if (p) etat.pj = p;
  $("#pj-date").hidden = !etat.pj?.date;
  $("#pj-date").textContent = etat.pj?.date ? "📅 " + etat.pj.date : "";
  rafraichirFiche();
  proposerMontee();
}

/* ---------------------------------------------------------- mon compte (serveur) */
function vueCompte() {
  vue.className = "";
  vue.innerHTML = `<div class="pj-fiche"><section class="panneau" id="p-compte"><p class="hint">Chargement…</p></section>
    <div class="ligne"><button id="pj-sortir">Se déconnecter</button></div></div>`;
  panneauCompte($("#p-compte", vue), { jetons: false }).catch(e => { $("#p-compte", vue).innerHTML = `<p class="alerte">${esc(e.message)}</p>`; });
  $("#pj-sortir", vue).onclick = sortir;
}

async function sortir() {
  await api("POST", "/api/acces/deconnexion").catch(() => {});
  choisirUniversOnglet(""); // le prochain compte part de ses propres univers
  location.href = "/";
}

// Sur un serveur, un compte qui a un rôle dans plusieurs univers passe de l'un à l'autre : sa page
// de joueur là où il est joueur, l'application ailleurs (MJ, auditeur).
async function choixUnivers() {
  const m = await api("GET", "/api/mes-univers").catch(() => null);
  if (!m) return false;
  if (m.courant && !universOnglet()) choisirUniversOnglet(m.courant);
  if (m.univers.length < 2) return true;
  const roles = { mj: "MJ", auditeur: "auditeur", joueur: "joueur" };
  const sel = $("#pj-mondes");
  sel.innerHTML = m.univers.map(u => `<option value="${esc(u.id)}" ${u.id === m.courant ? "selected" : ""}>🌍 ${esc(u.nom)}${u.role === "joueur" ? "" : " — " + (roles[u.role] || esc(u.role))}</option>`).join("");
  sel.hidden = false;
  $("#pj-univers").hidden = true;
  sel.onchange = async () => {
    const u = m.univers.find(x => x.id === sel.value);
    try {
      await api("POST", "/api/mes-univers/courant", { id: sel.value });
      choisirUniversOnglet(sel.value);
      location.href = u?.role === "joueur" ? "/joueur.html" : "/";
    } catch (e) { toast(e.message, true); sel.value = m.courant; }
  };
  return true;
}

/* ---------------------------------------------------------- mes voies */
// Tout l'arbre des voies du PJ, en lecture seule : chaque rang avec son texte et ses effets, ceux
// qu'il a atteints en surbrillance. Lisible au téléphone, sans survol.
async function vueVoies() {
  const p = etat.pj;
  let F, REF;
  try {
    [F, REF] = await Promise.all([api("GET", `/api/entites/${p.id}/feuille`), api("GET", `/api/reference?pj=${p.id}`)]);
  } catch (e) { vue.className = ""; vue.innerHTML = `<p class="alerte">${esc(e.message)}</p>`; return; }
  if (etat.vue !== "voies") return;
  const repos = F.regles?.repos || [];
  const compte = (voie, rang) => (F.comptees || []).find(x => x.voie === voie && x.rang === rang);
  const bloc = v => {
    const lib = voieLiee(REF, v);
    const rangs = lib ? lib.rangs : v.capacites.map(nom => ({ nom }));
    const nommes = rangs.reduce((m, c, i) => (c?.nom ? i + 1 : m), 0);
    return `<section class="panneau pj-voie">
      <h2>${esc(v.nom)} <span class="hint">${esc(v.origine || "")}${v.origine ? " · " : ""}rang ${v.rang}${lib ? "" : " · voie libre"}</span></h2>
      ${lib?.description ? `<p class="hint">${esc(lib.description)}</p>` : ""}
      <ol class="pj-rangs">${rangs.slice(0, Math.max(nommes, v.rang)).map((c, i) => {
        const acquis = i < v.rang, u = lib && compte(lib.nom, i + 1);
        const faites = u && u.par !== "combat" ? F.utilisations?.[u.cle]?.n || 0 : null;
        return `<li class="${acquis ? "acquis" : ""}"><span class="n">${i + 1}</span><div>
          <b>${esc(c?.nom || "—")}</b>${c?.limitee ? " (L)" : ""}${c?.sort ? "*" : ""}${acquis ? ` <span class="badge">acquis</span>` : ""}
          ${c?.texte ? `<div class="bst-texte">${esc(c.texte)}</div>` : ""}
          ${lignesEffets(c || {}, repos).length ? `<ul class="hint">${lignesEffets(c, repos).map(l => `<li>${esc(l)}</li>`).join("")}</ul>` : ""}
          ${acquis && faites !== null ? `<div class="hint">Utilisée ${faites}/${u.fois} ${esc(libellePar(u.par, repos))}</div>` : ""}
        </div></li>`;
      }).join("")}</ol></section>`;
  };
  vue.className = "";
  vue.innerHTML = `<div class="pj-fiche">
      <div class="pj-tete"><h1>${esc(p.nom)}</h1><span class="hint">Mes voies : les rangs atteints sont en surbrillance.</span></div>
      ${F.voies.length ? F.voies.map(bloc).join("") : `<p class="empty">Aucune voie sur la feuille pour l'instant.</p>`}
    </div>`;
}

/* ---------------------------------------------------------- combat */
const CAMPS = { pj: "PJ", allie: "Allié", ennemi: "Adversaire" };
function vueCombat() {
  vue.className = "combat";
  const c = etat.combat;
  if (!c || !c.ordre.length) {
    vue.innerHTML = `<section class="panel suivi"><h1>Combat</h1><p class="empty">Aucun combat en cours. L'ordre d'initiative s'affichera ici dès que le MJ lancera un combat.</p></section>`;
    return;
  }
  const courant = c.ordre.find(p => p.courant), suivant = c.ordre.find(p => p.suivant);
  vue.innerHTML = `<section class="panel suivi">
    <div class="suivi-tete"><h1>${esc(c.nom)}</h1><span class="hint">Round</span> <span class="round-n num">${c.round}</span></div>
    <div class="tour-de ${moi(courant) ? "moi" : ""}">
      <span class="hint">C'est au tour de</span>
      <span class="qui">${courant ? esc(courant.nom) : "—"}</span>
      ${moi(courant) ? `<span class="flag-moi">C'est à vous !</span>` : ""}
    </div>
    ${suivant ? `<div class="ensuite ${moi(suivant) ? "moi" : ""}"><span class="hint">Ensuite</span> <b>${esc(suivant.nom)}</b>${moi(suivant) ? ` <span class="flag-moi petit">préparez-vous</span>` : ""}</div>` : ""}
    <ol class="ordre-joueurs">${c.ordre.map(p => `
      <li class="${p.camp}${p.courant ? " courant" : ""}${p.suivant ? " suivant" : ""}${moi(p) ? " moi" : ""}${p.a_terre ? " terre" : ""}">
        <span class="qui-etats"><span class="nom">${esc(p.nom)}</span>${p.etats.map(e => `<span class="chip">${esc(e)}</span>`).join("")}</span>
        <span class="etiquettes"><span class="tag ${p.camp}">${CAMPS[p.camp] || ""}</span>${p.a_terre ? `<span class="tag">À terre</span>` : ""}${p.courant ? `<span class="flag">Joue</span>` : p.suivant ? `<span class="tag">Ensuite</span>` : ""}</span>
      </li>`).join("")}</ol>
  </section>`;
}

/* ---------------------------------------------------------- documents */
// Les aides de jeu que le MJ a envoyées à ce PJ. Celles pas encore ouvertes sur cet appareil portent
// une pastille (sur l'onglet et sur leur carte).
const CLE_VUS = "cof-joueur-vus";
const lireVus = () => { try { return new Set(JSON.parse(localStorage.getItem(CLE_VUS + "-" + etat.pj?.id) || "[]")); } catch { return new Set(); } };
function marquerVu(id) {
  const vus = lireVus(); vus.add(id);
  try { localStorage.setItem(CLE_VUS + "-" + etat.pj.id, JSON.stringify([...vus])); } catch { /* stockage indisponible */ }
  peindrePastille();
}
const avecPJ = chemin => lien(chemin + (chemin.includes("?") ? "&" : "?") + "pj=" + etat.pj.id);
const imageDoc = (id, f) => avecPJ(`/api/joueur/documents/${id}/images/${encodeURIComponent(f)}`);
const dateDoc = t => new Date(t * 1000).toLocaleDateString("fr-FR", { day: "numeric", month: "long" });

function peindrePastille() {
  const vus = lireVus(), n = etat.docs.filter(d => !vus.has(d.id)).length;
  const b = $("#doc-badge");
  b.hidden = !n; b.textContent = n || "";
}

async function chargerDocs() {
  if (!etat.pj || !fonction("aides")) return;
  try { etat.docs = await api("GET", avecPJ("/api/joueur/documents")); } catch { /* hors ligne : on garde la liste */ }
  peindrePastille();
  if (etat.vue === "documents") vueDocuments();
}

function vueDocuments() {
  vue.className = "combat";
  if (etat.doc) { vueDocument(etat.doc); return; }
  const vus = lireVus();
  vue.innerHTML = `<section class="panel documents">
      <h1>Documents</h1>
      ${etat.docs.length ? `<div class="docs-liste">${etat.docs.map(d => `<a class="doc-carte${vus.has(d.id) ? "" : " nouveau"}" href="#/documents/${d.id}">
          ${d.image ? `<img src="${esc(imageDoc(d.id, d.image))}" alt="" loading="lazy">` : `<span class="doc-icone" aria-hidden="true">✉</span>`}
          <span class="c"><b>${esc(d.nom)}</b><span class="hint">${esc([d.forme, "reçu le " + dateDoc(d.recu)].filter(Boolean).join(" · "))}</span></span>
          ${vus.has(d.id) ? "" : `<span class="flag-moi petit">nouveau</span>`}
        </a>`).join("")}</div>`
        : `<p class="empty">Aucun document pour l'instant. Les lettres, images et indices que le MJ vous remet arriveront ici.</p>`}
    </section>`;
}

async function vueDocument(id) {
  vue.innerHTML = `<section class="panel documents"><p class="hint">Chargement…</p></section>`;
  let d;
  try { d = await api("GET", avecPJ(`/api/joueur/documents/${id}`)); }
  catch (e) { vue.innerHTML = `<section class="panel documents"><p class="empty">${esc(e.status === 404 ? "Ce document n'est plus disponible." : e.message)}</p><div><a href="#/documents"><button>← Documents</button></a></div></section>`; return; }
  if (etat.vue !== "documents" || etat.doc !== id) return;
  marquerVu(id);
  vue.innerHTML = `<article class="panel documents doc">
      <div class="row"><a href="#/documents"><button class="ghost petit">← Documents</button></a></div>
      <h1>${esc(d.nom)}</h1>
      <span class="hint">${esc([d.forme, "reçu le " + dateDoc(d.recu)].filter(Boolean).join(" · "))}</span>
      ${d.contenu ? `<div class="md doc-texte">${rendu(d.contenu)}</div>` : ""}
      ${d.images.map(i => `<figure><a href="${esc(imageDoc(d.id, i.fichier))}" target="_blank" rel="noopener" title="Ouvrir en grand"><img src="${esc(imageDoc(d.id, i.fichier))}" alt="${esc(i.legende || d.nom)}"></a>${i.legende ? `<figcaption>${esc(i.legende)}</figcaption>` : ""}</figure>`).join("")}
    </article>`;
}

/* ---------------------------------------------------------- pot commun */
// Le pot commun de la campagne du PJ : ce que le groupe a gagné et pas encore partagé. Le joueur y
// propose un partage pour son PJ ; le MJ le valide (pièces dans la bourse, objets dans l'inventaire)
// ou le refuse.
const MONNAIES = ["po", "pa", "pc"];
const textePieces = p => MONNAIES.filter(k => p?.[k]).map(k => `${p[k]} ${k}`).join(", ");
const texteObjets = l => (l || []).map(o => (o.quantite > 1 ? `${o.quantite} × ` : "") + o.nom).join(", ");
const ligneButin = (p, l) => [textePieces(p), texteObjets(l)].filter(Boolean).join(", ") || "rien";

async function vuePot() {
  vue.className = "combat";
  if (!vue.querySelector(".pot-joueur")) vue.innerHTML = `<section class="panel pot-joueur"><h1>Pot commun</h1><p class="hint">Chargement…</p></section>`;
  let p;
  try { p = await api("GET", avecPJ("/api/joueur/pot")); }
  catch (e) { if (etat.vue === "pot") vue.innerHTML = `<section class="panel pot-joueur"><h1>Pot commun</h1><p class="empty">${esc(e.message)}</p></section>`; return; }
  if (etat.vue !== "pot") return;
  const vide = !textePieces(p.pieces) && !p.objets.length;
  const miennes = p.propositions.filter(x => x.mienne).length;
  vue.innerHTML = `<section class="panel pot-joueur">
      <h1>Pot commun</h1><span class="hint">${esc(p.campagne)} · ce que le groupe a gagné et pas encore partagé</span>
      ${vide ? `<p class="empty">Le pot est vide. Le butin des combats y arrive quand le MJ le verse.</p>`
        : `<div class="pot-pieces">${esc(textePieces(p.pieces) || "Aucune pièce")}</div>
          ${p.objets.length ? `<ul class="pot-objets">${p.objets.map(o => `<li>${esc(o.nom)}${o.quantite > 1 ? ` <span class="hint">× ${o.quantite}</span>` : ""}</li>`).join("")}</ul>` : ""}`}
      ${p.propositions.length ? `<h2>En attente du MJ</h2><ul class="pot-attente">${p.propositions.map(x => `<li${x.valable ? "" : ` class="perime" title="Le pot n'a plus tout ce qu'elle demande"`}>
          <b>${esc(x.pj_nom || "PJ")}</b> : ${esc(ligneButin(x.pieces, x.objets))}${x.note ? ` <span class="hint">« ${esc(x.note)} »</span>` : ""}
          ${x.mienne ? `<button type="button" class="ghost petit" data-retirer="${x.id}">Retirer</button>` : ""}</li>`).join("")}</ul>` : ""}
      ${vide ? "" : `<h2>Proposer un partage pour ${esc(etat.pj.nom)}</h2>
        <form class="pot-form colonne" autocomplete="off">
          <div class="pieces">${MONNAIES.map(k => `<label><input type="number" min="0" max="${p.pieces[k] || 0}" inputmode="numeric" data-m="${k}" placeholder="0" aria-label="${k}"${p.pieces[k] ? "" : " disabled"}><span>${k}</span></label>`).join("")}</div>
          ${p.objets.length ? `<div class="pot-choix">${p.objets.map(o => `<label class="row"><input type="number" min="0" max="${o.quantite}" value="0" data-ligne="${o.ligne}" aria-label="Quantité de ${esc(o.nom)}"><span>${esc(o.nom)} <span class="hint">sur ${o.quantite}</span></span></label>`).join("")}</div>` : ""}
          <input data-note maxlength="200" placeholder="Un mot pour le groupe et le MJ (facultatif)" aria-label="Note">
          <div class="row"><button class="primary" type="submit">Proposer au MJ</button>${miennes ? `<span class="hint">${miennes} proposition${miennes > 1 ? "s" : ""} en attente</span>` : ""}</div>
          ${p.partage ? "" : `<p class="hint">Les feuilles de personnage sont désactivées : le MJ ne pourra pas encore valider de partage.</p>`}
        </form>`}
      ${p.historique.length ? `<details class="pot-volet"><summary>Ce qui est entré et sorti</summary><ul class="pot-histo">${p.historique.map(m => `<li><span class="hint">${dateDoc(m.le)}</span>
          ${m.genre === "partage" ? `<b>${esc(m.pj_nom || "PJ")}</b> a reçu` : m.genre === "retrait" ? "Sorti du pot" : "Entré dans le pot"}${m.libelle ? ` <span class="hint">(${esc(m.libelle)})</span>` : ""} : ${esc(m.texte)}${m.annule ? ` <span class="hint">— rendu au pot</span>` : ""}</li>`).join("")}</ul></details>` : ""}
    </section>`;
  const form = $(".pot-form", vue);
  if (form) form.onsubmit = async e => {
    e.preventDefault();
    const pieces = Object.fromEntries(MONNAIES.map(k => [k, Math.max(0, Math.trunc(+$(`[data-m=${k}]`, form).value || 0))]));
    const objets = $$("[data-ligne]", form).map(i => ({ ligne: +i.dataset.ligne, quantite: Math.trunc(+i.value || 0) })).filter(o => o.quantite > 0);
    if (!pieces.po && !pieces.pa && !pieces.pc && !objets.length) { toast("Choisissez des pièces ou un objet.", true); return; }
    try {
      await api("POST", lien("/api/joueur/pot/propositions"), { pj: etat.pj.id, pieces, objets, note: $("[data-note]", form).value.trim() });
      toast("Proposition envoyée au MJ.");
      vuePot();
    } catch (err) { toast(err.message, true); }
  };
  $$("[data-retirer]", vue).forEach(b => b.onclick = async () => {
    try { await api("POST", lien(`/api/joueur/pot/propositions/${b.dataset.retirer}/retirer`), { pj: etat.pj.id }); toast("Proposition retirée."); vuePot(); }
    catch (err) { toast(err.message, true); }
  });
}

/* ---------------------------------------------------------- dés */
const signe = n => n > 0 ? "+" + n : n < 0 ? String(n) : "";
let modeDe = "normal"; // dé bonus : garder le meilleur de deux ; dé malus : le pire

async function vueDes() {
  vue.className = "combat";
  vue.innerHTML = `<section class="panel des-joueur">
      <h1>Dés</h1>
      <div class="des-mode" role="group" aria-label="Dé du test">
        ${[["normal", "Normal"], ["bonus", "Dé bonus"], ["malus", "Dé malus"]].map(([k, t]) => `<button type="button" class="petit${modeDe === k ? " on" : ""}" data-mode="${k}" aria-pressed="${modeDe === k}">${t}</button>`).join("")}
      </div>
      <div class="des-tests" id="des-tests"><p class="hint">Chargement de la feuille…</p></div>
      <form class="des-form" autocomplete="off">
        <div class="des-saisie">
          <input name="formule" placeholder="1d20+3, 2d6…" aria-label="Formule" spellcheck="false" required>
          <button class="primary" type="submit">Lancer</button>
        </div>
        <input name="libelle" placeholder="Pour quoi ? (facultatif)" aria-label="Libellé" maxlength="60">
        <div class="des-rapides">${DES_RAPIDES.map(f => `<button type="button" class="petit" data-formule="1d${f}">d${f}</button>`).join("")}</div>
      </form>
      <h2>À la table</h2>
      <ol class="des-liste" id="des-liste"></ol>
    </section>`;
  peindreJets();
  const form = $(".des-form", vue);
  form.onsubmit = e => { e.preventDefault(); lancerJet(form.formule.value, form.libelle.value); };
  vue.onclick = e => {
    const m = e.target.closest("[data-mode]");
    if (m) { modeDe = m.dataset.mode; $$("[data-mode]", vue).forEach(b => { b.classList.toggle("on", b === m); b.setAttribute("aria-pressed", b === m); }); return; }
    const b = e.target.closest("[data-formule]");
    if (b) lancerJet(b.dataset.test ? formuleTest(+b.dataset.test) : b.dataset.formule, b.dataset.libelle || form.libelle.value);
  };
  // les tests tout faits, lus sur la feuille
  let f = null;
  try { f = await api("GET", `/api/entites/${etat.pj.id}/feuille`); } catch { /* sans feuille : formules libres */ }
  if (etat.vue !== "des") return;
  const box = $("#des-tests", vue);
  if (!f || !f.existe) { box.innerHTML = `<p class="hint">Les tests tout faits apparaîtront quand le MJ aura rempli votre feuille.</p>`; return; }
  etat.crit = f.crit || etat.de;
  const caracs = CARACS.filter(k => k in (f.carac || {})).map(k => [k, f.carac[k]]);
  box.innerHTML = `
    <div class="des-groupe"><span class="hint">Tests</span>${caracs.map(([k, v]) => `<button type="button" data-formule="" data-test="${v}" data-libelle="Test de ${esc(k)}">${esc(k)} <b>${signe(v) || "+0"}</b></button>`).join("")}</div>
    ${(f.attaques || []).length ? `<div class="des-groupe"><span class="hint">Attaques</span>${f.attaques.map(a => `
      <button type="button" data-formule="" data-test="${a.bonus || 0}" data-libelle="${esc(a.nom)}">${esc(a.nom)} <b>${signe(a.bonus) || "+0"}</b></button>
      ${a.dm ? `<button type="button" class="ghost" data-formule="${esc(a.dm)}" data-libelle="Dégâts : ${esc(a.nom)}">DM ${esc(a.dm)}</button>` : ""}`).join("")}</div>` : ""}`;
}

/** Le dé du test (avec le dé bonus ou malus choisi) et son modificateur. */
function formuleTest(mod) {
  const de = { normal: `1d${etat.de}`, bonus: `2d${etat.de}kh1`, malus: `2d${etat.de}kl1` }[modeDe];
  return de + signe(mod);
}

async function lancerJet(formule, libelle) {
  try {
    const j = await api("POST", "/api/joueur/des", { formule, libelle, pj: etat.pj.id });
    ajouterJets([j]);
    toast(`🎲 ${j.total}${libelle ? " — " + libelle : ""}`);
    if (modeDe !== "normal") { modeDe = "normal"; $$("[data-mode]", vue).forEach(b => { b.classList.toggle("on", b.dataset.mode === "normal"); b.setAttribute("aria-pressed", b.dataset.mode === "normal"); }); }
  } catch (e) { toast(e.message, true); }
}

function ajouterJets(l) {
  for (const j of l) if (!etat.jets.some(x => x.id === j.id)) etat.jets.push(j);
  etat.jets.sort((a, b) => a.id - b.id);
  etat.jets.splice(0, Math.max(0, etat.jets.length - 100));
  etat.dernierJet = Math.max(etat.dernierJet, ...l.map(j => j.id));
  peindreJets();
}

function peindreJets() {
  const box = $("#des-liste", vue);
  if (!box) return;
  box.innerHTML = etat.jets.length ? etat.jets.slice().reverse().map(j => {
    const mien = !!etat.pj && j.pj === etat.pj.id;
    return ligneJet(j, { moi: mien, critique: mien && j.faces === etat.de ? etat.crit : 0 });
  }).join("") : `<li class="vide">Aucun jet pour l'instant.</li>`;
}

async function relireJets() {
  if (!fonction("des")) return;
  try {
    const d = await api("GET", "/api/joueur/des?depuis=" + etat.dernierJet);
    if (d.dernier < etat.dernierJet) { etat.jets = []; etat.dernierJet = 0; } // l'application a redémarré
    ajouterJets(d.jets);
  } catch { /* hors ligne */ }
}

/* ---------------------------------------------------------- hauts faits */
async function vueHautsFaits() {
  vue.className = "combat";
  vue.innerHTML = `<section class="panel hauts-faits" id="hf"><p class="hint">Chargement…</p></section>`;
  const box = $("#hf", vue);
  let d;
  try { d = await api("GET", "/api/joueur/hauts-faits"); } catch (e) { box.innerHTML = `<p class="empty">${esc(e.message)}</p>`; return; }
  if (etat.vue !== "hauts-faits") return;
  const mien = r => !!etat.pj && r.pj === etat.pj.id;
  const recs = [...(d.records || [])].sort((a, b) => mien(b) - mien(a));
  const titres = r => Object.entries(r.titres || {}).sort((a, b) => b[1] - a[1]).map(([t, n]) => `<span class="tag">${esc(t)}${n > 1 ? " ×" + n : ""}</span>`).join(" ");
  const carte = r => `<article class="record${mien(r) ? " moi" : ""}">
      <h3>${esc(r.nom)}${mien(r) ? ` <span class="flag-moi petit">vous</span>` : ""}</h3>
      <div class="record-stats">
        <div><b class="num">${r.combats}</b><span>combat${r.combats > 1 ? "s" : ""}</span></div>
        <div><b class="num">${r.victoires}</b><span>victoire${r.victoires > 1 ? "s" : ""}</span></div>
        <div><b class="num">${r.abattus}</b><span>abattus</span></div>
        <div><b class="num">${r.critiques}</b><span>critiques</span></div>
        <div><b class="num">${r.infliges}</b><span>DM infligés</span></div>
        <div><b class="num">${r.tentatives ? Math.round(r.touches / r.tentatives * 100) + "%" : "—"}</b><span>précision</span></div>
      </div>
      ${r.meilleur_coup ? `<p class="record-coup">Meilleur coup : <b>${esc(r.meilleur_txt)}</b> <span class="hint">— ${esc(r.meilleur_ou)}</span></p>` : ""}
      ${titres(r) ? `<p class="record-titres">${titres(r)}</p>` : ""}
    </article>`;
  box.innerHTML = `
    <h1>Hauts faits</h1>
    ${d.combats.length ? "" : `<p class="empty">Aucun combat terminé pour l'instant. Le récap de chaque combat apparaîtra ici dès que le MJ le clôturera.</p>`}
    ${recs.length ? `<h2>Records</h2><div class="records">${recs.map(carte).join("")}</div>` : ""}
    ${d.combats.length ? `<h2>Combats</h2><div class="combats-passes">${d.combats.map((c, i) => `
      <details${i === 0 ? " open" : ""}><summary><b>${esc(c.recap.nom)}</b> <span class="tag ${c.recap.issue === "victoire" ? "allie" : c.recap.issue === "defaite" ? "ennemi" : ""}">${{ victoire: "Victoire", defaite: "Défaite" }[c.recap.issue] || "Terminé"}</span> <span class="hint">${dateCourte(c.recap.date)} · ${c.recap.rounds} round${c.recap.rounds > 1 ? "s" : ""}</span></summary>
        ${recapHTML(c.recap, { titre: false })}
        <div class="row"><button type="button" class="ghost" data-copier="${i}">Copier en Markdown</button></div></details>`).join("")}</div>` : ""}`;
  box.onclick = async e => {
    const b = e.target.closest("[data-copier]"); if (!b) return;
    toast(await copier(recapMarkdown(d.combats[+b.dataset.copier].recap)) ? "Récap copié en Markdown." : "Copie impossible depuis ce navigateur.");
  };
}

/* bandeau permanent, hors de la vue du combat : à qui le tour */
function rendreBandeau() {
  const b = $("#bandeau"), c = etat.combat;
  const visible = !!c && c.ordre.length > 0 && etat.vue !== "combat" && !!etat.pj && fonction("combat");
  b.hidden = !visible;
  if (!visible) return;
  const courant = c.ordre.find(p => p.courant), suivant = c.ordre.find(p => p.suivant);
  b.className = moi(courant) ? "moi" : "";
  b.innerHTML = `<a href="#/combat">Round ${c.round} · ${moi(courant) ? "<b>C'est à vous !</b>" : `Au tour de <b>${esc(courant?.nom || "—")}</b>`}${suivant ? ` · ensuite ${moi(suivant) ? "<b>vous</b>" : esc(suivant.nom)}` : ""}</a>`;
}

/* ---------------------------------------------------------- temps réel */
let ws = null, essais = 0, dernier = Date.now(), arrete = false;
function connecter() {
  if (arrete) return;
  const sock = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${lien("/api/joueur/direct")}`);
  ws = sock;
  sock.onopen = () => { essais = 0; dernier = Date.now(); $("#conn").hidden = true; };
  sock.onmessage = e => {
    dernier = Date.now();
    const m = JSON.parse(e.data);
    if (m.type === "jet") { ajouterJets([m.jet]); return; }
    if (m.type === "etat") { relireJets(); chargerDocs(); etat.combat = m.combat; if (etat.edition !== m.edition) { etat.edition = m.edition; rafraichirFiche(); } }
    else if (m.type === "combat") etat.combat = m.combat;
    else if (m.type === "edition") { etat.edition = m.active; toast(m.active ? "Le MJ a déverrouillé les feuilles : vous pouvez modifier la vôtre." : "Le MJ a verrouillé les feuilles."); if (etat.vue === "fiche" && etat.pj) vueFiche(); }
    else if (m.type === "fiche" && etat.pj && m.id === etat.pj.id) relirePJ();
    else if (m.type === "hauts_faits") { if (etat.vue === "hauts-faits") vueHautsFaits(); }
    else if (m.type === "document") { if (etat.pj && m.pj.includes(etat.pj.id)) { toast(`✉ Nouveau document : ${m.nom}`); chargerDocs(); } }
    else if (m.type === "documents") chargerDocs();
    else if (m.type === "pot") { if (etat.vue === "pot") vuePot(); }
    if (etat.vue === "combat") vueCombat();
    rendreBandeau();
  };
  sock.onclose = () => {
    if (ws !== sock || arrete) return;
    $("#conn").hidden = false; $("#conn").textContent = "Reconnexion…";
    setTimeout(connecter, Math.min(3000, 300 * 2 ** essais++));
  };
}
setInterval(() => {
  if (!ws || ws.readyState !== 1) return;
  if (Date.now() - dernier > 45000) { ws.close(); return; }
  ws.send(JSON.stringify({ type: "ping" }));
}, 15000);
document.addEventListener("visibilitychange", () => { if (document.visibilityState === "visible" && (!ws || ws.readyState > 1)) connecter(); });

/* ---------------------------------------------------------- routeur */
async function route() {
  const nom = location.hash.replace(/^#\/?/, "") || "fiche";
  const doc = /^documents\/(\d+)$/.exec(nom);
  etat.doc = doc ? +doc[1] : 0;
  const permis = onglets();
  etat.vue = !etat.pj || nom === "choix" ? "choix" : doc ? "documents" : permis.includes(nom) ? nom : permis[0] || "rien";
  if (etat.vue !== "choix" && etat.vue !== "rien" && !permis.includes(etat.vue)) etat.vue = permis[0] || "rien";
  $$("[data-nav]").forEach(a => a.classList.toggle("on", a.dataset.nav === etat.vue));
  $("#pj-nav").hidden = !etat.pj;
  $("#pj-changer").hidden = etat.serveur || !etat.pj || etat.vue === "choix";
  $("#pj-date").hidden = !etat.pj?.date;
  $("#pj-date").textContent = etat.pj?.date ? "📅 " + etat.pj.date : "";
  rendreBandeau();
  vue.onclick = null;
  window.scrollTo(0, 0);
  if (etat.vue === "choix") vueChoix();
  else if (etat.vue === "rien") { vue.className = "combat"; vue.innerHTML = `<section class="panel pj-choix"><h1>${esc(etat.pj.nom)}</h1><p class="empty">Rien à suivre pour l'instant : le MJ n'a activé ni les feuilles, ni le combat, ni les documents, ni les dés.</p></section>`; }
  else if (etat.vue === "combat") vueCombat();
  else if (etat.vue === "documents") vueDocuments();
  else if (etat.vue === "pot") await vuePot();
  else if (etat.vue === "des") await vueDes();
  else if (etat.vue === "hauts-faits") await vueHautsFaits();
  else if (etat.vue === "compte") vueCompte();
  else if (etat.vue === "voies") await vueVoies();
  else await vueFiche();
}

function fermee(titre, texte, compte = false) {
  arrete = true;
  $("#pj-nav").hidden = true; $("#bandeau").hidden = true;
  vue.className = "combat";
  vue.innerHTML = `<section class="panel pj-choix"><h1>${esc(titre)}</h1><p class="hint">${texte}</p>
    <div class="row"><button class="primary" id="pj-reessayer">Réessayer</button>${compte ? `<button id="pj-sortir">Se déconnecter</button>` : ""}</div></section>`;
  $("#pj-reessayer").onclick = () => location.reload();
  if (compte) $("#pj-sortir").onclick = sortir;
}

async function demarrer() {
  const acces = await api("GET", "/api/acces").catch(() => ({}));
  if (acces.serveur && !acces.connecte) { fermee("Connectez-vous", `La page des joueurs s'ouvre avec votre compte : <a href="/">connectez-vous</a>.`); return; }
  const compte = !!acces.serveur && await choixUnivers(); // sur un serveur, avec un compte
  let e;
  try { e = await api("GET", "/api/joueur/etat"); }
  catch (err) {
    if (err.status === 401) fermee("La page des joueurs est fermée", `Demandez au MJ de l'ouvrir dans Paramètres → Tablette et réseau. Sur un serveur, <a href="/">connectez-vous</a> d'abord avec votre compte.`);
    else if (err.status === 403) fermee("Pas d'accès", "Votre compte n'a pas de rôle dans cet univers.", compte);
    else if (err.status === 404) fermee("Page des joueurs désactivée", "Le MJ ne l'a pas activée dans cet univers.", compte);
    else fermee("COF DM ne répond pas", esc(err.message), compte);
    return;
  }
  Object.assign(etat, { serveur: e.serveur, edition: e.edition, combat: e.combat, de: e.de || 20, fonctions: e.fonctionnalites || {} });
  const permis = onglets();
  $$("[data-nav]").forEach(a => { a.hidden = !permis.includes(a.dataset.nav); });
  $("#pj-univers").textContent = e.univers || "";
  etat.pjs = await api("GET", "/api/joueur/pj").catch(() => []);
  // sur un serveur, le PJ est celui du compte ; à la maison, celui choisi sur cet appareil
  const voulu = e.serveur && e.pj ? e.pj : lireChoix();
  etat.pj = etat.pjs.find(p => p.id === voulu) || null;
  if (e.serveur && !etat.pj && etat.pjs.length === 1) etat.pj = etat.pjs[0];
  if (e.serveur && !etat.pj && !etat.pjs.length) { fermee("Aucun personnage", "Aucun PJ n'est rattaché à votre compte, ou sa fiche n'est pas partagée. Demandez au MJ.", true); return; }
  if (!etat.pj) garderChoix(0);
  $("#pj-changer").onclick = () => { location.hash = "#/choix"; };
  window.addEventListener("hashchange", route);
  connecter();
  await route();
  proposerMontee();
}
demarrer();
