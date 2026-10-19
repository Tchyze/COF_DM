// Tableau de bord : le monde en un coup d'œil.
import { esc, api, dateHeure } from "../ui.js";
import { types, vignette, texteDate, valeurCourte, actif as fonctionActive } from "../schema.js";

export async function mount(el) {
  const st = await api("GET", "/api/stats");
  const lignes = (l, meta) => l.length ? `<div class="lignes">${l.map(f => `<a href="#/f/${f.id}">${vignette(f, 30)}<span class="t">${esc(f.nom)}</span><span class="m">${meta(f)}</span></a>`).join("")}</div>` : "";
  const joueur = !!window.cofAcces?.joueur;
  const premier = st.total === 0 && !joueur;
  el.innerHTML = `
    <div class="entete"><h1>Tableau de bord</h1><span class="sp"></span>
      ${joueur || !fonctionActive("ecran_mj") ? "" : `<a href="#/table"><button class="primary">🧭 Écran du MJ</button></a>`}
      <button id="rech">🔍 Rechercher <kbd>/</kbd></button>
    </div>
    ${premier ? `<div class="panneau" style="margin-bottom:16px">
      <h2>Bienvenue dans COF DM</h2>
      <p class="hint" style="font-size:.95rem">Le monde est vide. Créez une première campagne, puis les PNJ, lieux et factions qui la peuplent.
      Dans les textes, écrivez <code>[[Nom d'une fiche]]</code> pour la lier ; reliez les fiches entre elles dans la colonne de droite de chaque fiche.</p>
      <div class="ligne"><a href="#/nouveau/campagne"><button class="primary">📖 Nouvelle campagne</button></a>
      <a href="#/nouveau/pnj"><button>👤 Nouveau PNJ</button></a><a href="#/nouveau/lieu"><button>🏰 Nouveau lieu</button></a></div>
    </div>` : ""}
    ${joueur && st.total === 0 ? `<div class="panneau" style="margin-bottom:16px"><p class="vide">Le MJ n'a encore partagé aucune fiche avec les joueurs.</p></div>` : ""}
    <div class="accueil${joueur ? " vue-joueur" : ""}">
      <section class="panneau large">
        <h3>Le monde</h3>
        <div class="compteurs">${types().map(t => `<a href="#/t/${t.cle}" data-type="${t.cle}">${vignette({ type: t.cle }, 36)}<span><b>${st.compte[t.cle] || 0}</b><span>${esc(t.pluriel)}</span></span></a>`).join("")}</div>
      </section>
      <section class="panneau">
        <h3>Campagnes</h3>
        ${lignes(st.campagnes, f => valeurCourte("campagne", "statut", f.carte?.statut) + (f.carte?.date_actuelle ? " · " + esc(texteDate(f.carte.date_actuelle)) : "")) || `<p class="vide">Aucune campagne.</p>`}
      </section>
      <section class="panneau">
        <h3>Quêtes en cours</h3>
        ${lignes(st.quetes_en_cours, f => esc(f.carte?.categorie || "")) || `<p class="vide">Aucune quête en cours.</p>`}
      </section>
      <section class="panneau">
        <h3>Dernières séances</h3>
        ${lignes(st.seances, f => valeurCourte("seance", "statut", f.carte?.statut) + (f.carte?.date_reelle ? " · " + valeurCourte("seance", "date_reelle", f.carte.date_reelle) : "")) || `<p class="vide">Aucune séance.</p>`}
      </section>
      <section class="panneau">
        <h3>Dernières modifications <span class="sp"></span><a href="#/journal" class="hint" style="text-transform:none;letter-spacing:0">tout le journal</a></h3>
        ${st.journal.length ? `<div class="lignes">${st.journal.map(j => `<div><span class="t">${j.entite ? `<a class="wl" href="#/f/${j.entite}">${esc(j.nom)}</a>` : esc(j.nom || j.detail)} <span class="hint">${esc(j.action)}${j.auteur && j.auteur !== "mj" ? " · " + esc(j.auteur) : ""}</span></span><span class="m">${dateHeure(j.quand)}</span></div>`).join("")}</div>` : `<p class="vide">Rien pour l'instant.</p>`}
        ${st.liens_morts ? `<a href="#/liens" class="alerte" style="text-decoration:none">⛓ ${st.liens_morts} lien${st.liens_morts > 1 ? "s" : ""} [[…]] ne mène${st.liens_morts > 1 ? "nt" : ""} à aucune fiche.</a>` : ""}
      </section>
    </div>`;
  el.querySelector("#rech").onclick = () => window.cofRecherche();
}
