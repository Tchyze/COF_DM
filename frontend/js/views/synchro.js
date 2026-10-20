// Synchronisation avec un serveur (application de bureau) : où en est l'univers ouvert, et la liste
// « Conflits », où le MJ choisit entre sa valeur et celle du serveur.
import { $, $$, esc, api, toast, dateHeure } from "../ui.js";
import { champDe, lienFiche } from "../schema.js";
import { allerAuxMisesAJour } from "../connexion.js";

const ETATS = {
  a_jour: ["✓", "À jour"],
  version: ["⬆", "Mise à jour de l'application nécessaire"],
  hors_ligne: ["⚠", "Hors ligne"],
  erreur: ["⚠", "Erreur"],
  jamais: ["…", "Pas encore synchronisé"],
};

const REGLAGES = { calendrier: "Calendrier", regles: "Fichier de règles", reference: "Profils et voies", fonctionnalites: "Fonctionnalités" };
const CHAMPS_FICHE = { nom: "Nom", tags: "Tags", archive: "Archivée", partage: "Partagée avec les joueurs", type: "Type" };

/** Ce que désigne un conflit, en clair. */
function libelle(c) {
  const champ = c.champ || "";
  switch (c.objet) {
    case "entite": {
      const k = champ.startsWith("data.") ? champ.slice(5) : "";
      return k ? (champDe(c.type, k)?.nom || k) : (CHAMPS_FICHE[champ] || (champ ? champ : "La fiche"));
    }
    case "feuille": return "Feuille de personnage" + (champ.startsWith("data.") ? " : " + champ.slice(5) : "");
    case "relation": return `Relation n°${c.cle}` + (champ && champ !== "*" ? " : " + champ : "");
    case "media": return `Image n°${c.cle}` + (champ && champ !== "*" ? " : " + champ : "");
    case "document": return `Bestiaire ou combat n°${c.cle}`;
    case "meta": return REGLAGES[c.cle] || c.cle;
  }
  return `${c.objet} ${c.cle} ${champ}`;
}

/** Une valeur JSON, lisible. */
function valeur(v) {
  if (v === null || v === undefined || v === "") return `<span class="hint">(vide)</span>`;
  if (typeof v === "boolean") return v ? "oui" : "non";
  if (typeof v === "string") return `<span class="texte">${esc(v.length > 600 ? v.slice(0, 600) + "…" : v)}</span>`;
  return `<code>${esc(JSON.stringify(v).slice(0, 400))}</code>`;
}

/** Ce qu'il s'est passé, et les deux choix. */
function expliquer(c) {
  switch (c.genre) {
    case "supprime_ici": return ["Vous l'avez supprimée ici, mais elle a été modifiée sur le serveur : elle a été gardée.", "Supprimer quand même", "Garder la fiche"];
    case "supprime_la": return ["Elle a été supprimée sur le serveur, mais vous l'avez modifiée ici : elle a été gardée et renvoyée au serveur.", "Garder la fiche", "Supprimer aussi ici"];
    case "refus":
      if (c.champ === "*") return [`Le serveur l'a refusée : ${esc(c.raison)}. Elle reste sur cet ordinateur, pas sur le serveur (s'il est plus ancien que cette application, renvoyez-la une fois qu'il sera à jour).`, "La renvoyer au serveur", "La retirer d'ici"];
      return [`Le serveur a refusé votre valeur : ${esc(c.raison)}.`, "Réessayer la mienne", "Garder celle du serveur"];
  }
  return ["Ce champ a changé ici et sur le serveur pendant que vous étiez hors ligne. La valeur du serveur est en place.", "Garder la mienne", "Garder celle du serveur"];
}

export async function mount(el) {
  el.innerHTML = `<div class="entete"><h1>Synchronisation</h1></div>
    <div class="colonne" style="gap:16px"><section class="panneau" id="sy-etat"></section><section class="panneau" id="sy-conflits"></section></div>`;
  const etat = async () => {
    const d = await api("GET", "/api/univers/serveur");
    const box = $("#sy-etat", el);
    if (!d.relie) {
      box.innerHTML = `<h3>Serveur</h3><p class="vide">Cet univers n'est relié à aucun serveur. Cliquez sur la zone en bas à gauche de la fenêtre pour connecter l'application à un serveur, y copier un de ses univers ou y envoyer celui-ci.</p>`;
      return d;
    }
    if (d.pause) {
      box.innerHTML = `<h3>Serveur</h3><p><b>⏸ En pause</b></p>
        <p class="hint">Copie « ${esc(d.nom_copie)} » de <code>${esc(d.url)}</code>. ${d.mode === "serveur" ? "L'application est reliée à un autre serveur" : "L'application travaille en local"} : vos changements sont gardés et partiront quand vous la reconnecterez à ce serveur (zone en bas à gauche).</p>
        <div class="ligne"><button class="ghost" id="sy-delier">Détacher : en faire un univers local indépendant</button></div>`;
      armerDelier(box);
      return d;
    }
    const s = d.synchro || {};
    const [ic, mot] = s.ecart === "bureau_recent" && s.etat === "a_jour" ? ["⇣", "Réception seule"] : ETATS[s.etat] || ETATS.jamais;
    const b = s.bilan || {};
    const pluriel = (n, mot) => `${n} ${mot}${n > 1 ? "s" : ""}`;
    box.innerHTML = `<h3>Serveur</h3>
      <p><b>${ic} ${esc(mot)}</b>${s.derniere ? ` — dernier échange le ${esc(dateHeure(s.derniere))}` : ""}</p>
      ${s.erreur || s.message ? `<p class="alerte">${esc(s.message || s.erreur)}</p>` : ""}
      ${b.en_attente ? `<p class="hint">${pluriel(b.en_attente, "fiche ou objet")} changé${b.en_attente > 1 ? "s" : ""} ici attend${b.en_attente > 1 ? "ent" : ""} ${s.ecart === "serveur_recent" ? "la mise à jour de cette application" : "la mise à jour du serveur"} : rien n'est perdu, tout partira ensuite.</p>` : ""}
      ${b.recus_en_attente ? `<p class="hint">${pluriel(b.recus_en_attente, "changement")} du serveur attend${b.recus_en_attente > 1 ? "ent" : ""} une version plus récente de cette application pour s'appliquer ici.</p>` : ""}
      ${s.ecart === "serveur_recent" ? `<div class="ligne"><button class="primary" id="sy-maj">⬆ Mettre à jour l'application…</button></div>` : ""}
      <p class="hint">Copie « ${esc(d.nom_copie)} » de <code>${esc(d.url)}</code>, compte <b>${esc(d.compte)}</b>. Elle échange toute seule toutes les 30 secondes ; hors ligne, vos changements attendent le retour de la connexion.</p>
      ${s.bilan && (s.bilan.envoyes || s.bilan.recus || s.bilan.images) ? `<p class="hint">Dernier échange : ${s.bilan.envoyes} champ(s) envoyé(s), ${s.bilan.recus} reçu(s)${s.bilan.images ? `, ${s.bilan.images} image(s)` : ""}.</p>` : ""}
      <div class="ligne"><button class="primary" id="sy-maintenant">⇅ Synchroniser maintenant</button><button class="ghost" id="sy-delier">Ne plus synchroniser</button></div>`;
    $("#sy-maintenant", box).onclick = async e => {
      e.target.disabled = true;
      try {
        const r = await api("POST", "/api/univers/serveur/synchroniser");
        toast(r.etat === "a_jour" ? "Synchronisé." : r.erreur || "Échange impossible.", r.etat !== "a_jour");
        window.dispatchEvent(new Event("cof-change"));
      } catch (err) { toast(err.message, true); }
      await rafraichir();
    };
    const maj = $("#sy-maj", box);
    if (maj) maj.onclick = () => allerAuxMisesAJour();
    armerDelier(box);
    return d;
  };
  const armerDelier = box => {
    const delier = $("#sy-delier", box);
    delier.onclick = async () => {
      if (!delier.dataset.arme) { delier.dataset.arme = "1"; delier.textContent = "Confirmer : cette copie ne suivra plus le serveur"; return; }
      try { await api("POST", "/api/univers/serveur/delier"); toast("Cette copie ne se synchronise plus."); }
      catch (err) { toast(err.message, true); }
      await rafraichir();
    };
  };
  const conflits = async () => {
    const box = $("#sy-conflits", el);
    const { conflits: l } = await api("GET", "/api/conflits");
    box.innerHTML = `<h3>Conflits${l.length ? ` <span class="n">${l.length}</span>` : ""}</h3>` + (l.length
      ? `<p class="hint">Rien n'est bloqué : le reste se synchronise, et ces valeurs attendent ici votre choix.</p>` +
        l.map(c => {
          const [quoi, garder, laisser] = expliquer(c);
          const fiche = c.nom ? lienFiche({ id: +c.cle, nom: c.nom, type: c.type }) : "";
          const deux = c.genre === "champ" || c.genre === "refus";
          return `<div class="conflit" data-id="${c.id}">
            <div class="ligne" style="gap:8px;flex-wrap:wrap">${fiche}<b>${esc(libelle(c))}</b><span class="hint">${esc(dateHeure(c.quand))}</span></div>
            <p class="hint">${quoi}</p>
            ${deux ? `<div class="conflit-valeurs"><div><small>Ici</small>${valeur(c.local)}</div><div><small>Serveur</small>${valeur(c.serveur)}</div></div>` : ""}
            <div class="ligne"><button class="petit" data-garder="local">${esc(garder)}</button><button class="petit" data-garder="serveur">${esc(laisser)}</button></div>
          </div>`;
        }).join("")
      : `<p class="vide">Aucun conflit.</p>`);
    $$("button[data-garder]", box).forEach(b => b.onclick = async () => {
      const id = b.closest(".conflit").dataset.id;
      try { await api("POST", "/api/conflits/" + id, { garder: b.dataset.garder }); toast("C'est noté : le choix partira au prochain échange."); }
      catch (err) { toast(err.message, true); }
      await rafraichir();
    });
  };
  const rafraichir = async () => {
    await etat();
    await conflits();
    window.dispatchEvent(new Event("cof-synchro"));
  };
  await rafraichir();
  const minuterie = setInterval(() => etat().catch(() => {}), 15000);
  return () => clearInterval(minuterie);
}
