// Lanceur de dés, hors combat : le panneau du MJ (sur toutes les pages de l'application) et le
// rendu d'un jet, partagé avec la page des joueurs. Les jets publics se voient de toute la table ;
// les jets cachés du MJ ne quittent pas son écran.
import { $, esc, api, toast } from "./ui.js";

const heure = s => new Date(s * 1000).toLocaleTimeString("fr-FR", { hour: "2-digit", minute: "2-digit" });

/** Un jet, en une ligne. moi : le jet est celui de ce joueur. critique : naturel à partir duquel marquer. */
export function ligneJet(j, { moi = false, critique = 0 } = {}) {
  let nat = "";
  if (j.nat) {
    const haut = critique ? j.nat >= critique : j.nat === j.faces;
    nat = `<span class="jet-nat${haut ? " haut" : j.nat === 1 ? " bas" : ""}" title="Le dé qui compte">nat. ${j.nat}</span>`;
  }
  return `<li class="jet${j.mj ? " mj" : ""}${j.cache ? " cache" : ""}${moi ? " moi" : ""}" data-jet="${j.id}">
      <span class="jet-total num">${j.total}</span>
      <span class="jet-corps">
        <span class="jet-tete"><b>${esc(j.qui)}</b>${j.libelle ? ` · ${esc(j.libelle)}` : ""}${j.cache ? ` <span class="tag" title="Vu du MJ seulement">🔒 caché</span>` : ""}</span>
        <span class="jet-detail">${esc(j.detail)} ${nat}</span>
      </span>
      <span class="jet-heure">${heure(j.quand)}</span>
    </li>`;
}

export const DES_RAPIDES = [4, 6, 8, 10, 12, 20, 100];

/** Panneau de dés du MJ. lecture : l'auditeur voit les jets sans lancer. */
export function initDes({ lecture = false } = {}) {
  const bouton = document.createElement("button");
  bouton.id = "des-bouton";
  bouton.className = "des-bouton";
  bouton.title = "Dés (D)";
  bouton.setAttribute("aria-label", "Dés");
  bouton.innerHTML = `🎲<span class="n" hidden></span>`;
  const panneau = document.createElement("section");
  panneau.className = "des-panneau";
  panneau.hidden = true;
  panneau.setAttribute("aria-label", "Dés");
  panneau.innerHTML = `
    <div class="des-tete"><h2>Dés</h2><button class="ghost petit" data-fermer aria-label="Fermer">✕</button></div>
    ${lecture ? "" : `<form class="des-form" autocomplete="off">
      <div class="des-saisie">
        <input name="formule" placeholder="1d20+3, 2d6, 2d20kh1…" aria-label="Formule" spellcheck="false" required>
        <button class="primary" type="submit">Lancer</button>
      </div>
      <input name="libelle" placeholder="Pour quoi ? (facultatif)" aria-label="Libellé" maxlength="60">
      <div class="des-rapides">${DES_RAPIDES.map(f => `<button type="button" class="petit" data-de="${f}">d${f}</button>`).join("")}</div>
      <label class="des-cache"><input type="checkbox" name="cache"> Jet caché <span class="hint">(les joueurs ne le voient pas)</span></label>
    </form>`}
    <ol class="des-liste" aria-live="polite"><li class="vide">Aucun jet pour l'instant.</li></ol>
    <p class="hint">kh1 : garder le meilleur dé ; kl1 : le pire. Les jets publics s'affichent sur la page des joueurs.</p>`;
  document.body.append(bouton, panneau);

  const liste = $(".des-liste", panneau), badge = $(".n", bouton);
  const jets = [];
  let dernier = 0, nonLus = 0, enCours = false;

  const peindre = () => {
    liste.innerHTML = jets.length ? jets.slice().reverse().map(j => ligneJet(j)).join("") : `<li class="vide">Aucun jet pour l'instant.</li>`;
  };
  const ouvrir = oui => {
    panneau.hidden = !oui;
    bouton.classList.toggle("on", oui);
    if (oui) {
      nonLus = 0; badge.hidden = true;
      $("[name=formule]", panneau)?.focus();
    }
  };
  bouton.onclick = () => ouvrir(panneau.hidden);
  $("[data-fermer]", panneau).onclick = () => ouvrir(false);
  document.addEventListener("keydown", e => {
    if (e.target.closest("input,textarea,select,[contenteditable]")) {
      if (e.key === "Escape" && panneau.contains(e.target)) ouvrir(false);
      return;
    }
    if (e.key.toLowerCase() === "d" && !e.ctrlKey && !e.metaKey && !e.altKey && !$(".voile")) { e.preventDefault(); ouvrir(panneau.hidden); }
    else if (e.key === "Escape" && !panneau.hidden) ouvrir(false);
  });

  // les jets de toute la table, relus en boucle tant que la page est visible
  const relire = async () => {
    if (enCours || document.visibilityState !== "visible") return;
    enCours = true;
    try {
      const d = await api("GET", "/api/des?depuis=" + dernier);
      if (d.dernier < dernier) { jets.length = 0; } // l'application a redémarré : nouvelle partie
      const premiere = dernier === 0;
      dernier = d.dernier;
      if (!d.jets.length) return;
      jets.push(...d.jets);
      jets.splice(0, Math.max(0, jets.length - 100));
      peindre();
      const autres = d.jets.filter(j => !j.mj);
      if (!premiere && autres.length && panneau.hidden) {
        nonLus += autres.length; badge.textContent = nonLus; badge.hidden = false;
        const j = autres[autres.length - 1];
        toast(`🎲 ${j.qui}${j.libelle ? " (" + j.libelle + ")" : ""} : ${j.total}`);
      }
    } catch (e) { if (e.status === 404) arreter(); /* 404 : dés coupés depuis un autre appareil ; sinon hors ligne, on réessaie */ } finally { enCours = false; }
  };
  const boucle = setInterval(relire, 3000);
  const arreter = () => { // plus de boucle, plus de bouton : la page se refera au prochain chargement
    clearInterval(boucle);
    document.removeEventListener("visibilitychange", relire);
    bouton.remove(); panneau.remove();
  };
  relire();
  document.addEventListener("visibilitychange", relire);

  if (lecture) return;
  const form = $(".des-form", panneau);
  const lancer = async formule => {
    try {
      const j = await api("POST", "/api/des", { formule, libelle: form.libelle.value, cache: form.cache.checked });
      if (j.id === dernier + 1) { jets.push(j); dernier = j.id; peindre(); }
      else relire(); // d'autres jets sont passés entre-temps
    } catch (e) { toast(e.message, true); }
  };
  form.onsubmit = e => { e.preventDefault(); lancer(form.formule.value); };
  $(".des-rapides", panneau).onclick = e => {
    const b = e.target.closest("[data-de]");
    if (b) lancer("1d" + b.dataset.de);
  };
}
