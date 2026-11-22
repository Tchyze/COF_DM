// La cloche : les notifications du MJ (écran du MJ) ou du joueur (page des joueurs). Un bouton 🔔 avec
// le nombre de non lues ; un clic ouvre la liste. Cliquer une notification la marque lue et mène où
// elle renvoie (l'agenda) ; « Tout marquer comme lu » vide le compteur.
import { esc, dialogue, toast } from "./ui.js";

const ICONES = { rappel: "⏰", deplacee: "📅", annulee: "✖", relance: "❓", reponse: "✉", message: "📣" };

function quand(s) {
  const d = new Date(s * 1000), maintenant = new Date();
  const heure = d.toLocaleTimeString("fr-FR", { hour: "2-digit", minute: "2-digit" });
  if (d.toDateString() === maintenant.toDateString()) return "aujourd'hui à " + heure;
  const hier = new Date(maintenant); hier.setDate(hier.getDate() - 1);
  if (d.toDateString() === hier.toDateString()) return "hier à " + heure;
  return d.toLocaleDateString("fr-FR", { day: "numeric", month: "long" }) + " à " + heure;
}

/** Pose la cloche. lire() → {non_lues, notifications} ; marquer(ids) → idem (ids vide : toutes) ;
 * placer(bouton) l'insère dans la page ; badge(n) : à appeler aussi ailleurs (menu replié…). */
export function initCloche({ lire, marquer, placer, badge = () => {} }) {
  const bouton = document.createElement("button");
  bouton.type = "button";
  bouton.className = "ghost petit cloche";
  bouton.setAttribute("aria-haspopup", "dialog");
  bouton.innerHTML = `<span aria-hidden="true">🔔</span><span class="cloche-texte">Notifications</span><span class="cloche-nombre" hidden></span>`;
  placer(bouton);
  let donnees = { non_lues: 0, notifications: [] }, ouverte = null;

  function peindreBouton() {
    const n = donnees.non_lues || 0, nb = bouton.querySelector(".cloche-nombre");
    nb.hidden = !n;
    nb.textContent = n > 99 ? "99+" : String(n);
    bouton.classList.toggle("a-lire", n > 0);
    bouton.setAttribute("aria-label", n ? `Notifications : ${n} non lue${n > 1 ? "s" : ""}` : "Notifications");
    badge(n);
  }

  function corps() {
    const l = donnees.notifications || [];
    if (!l.length) return `<p class="empty">Aucune notification pour l'instant.</p>`;
    return `<ul class="cloche-liste">${l.map(n => `<li><button type="button" class="cloche-item${n.lue ? "" : " non-lue"}" data-id="${n.id}">
        <span class="cloche-icone" aria-hidden="true">${ICONES[n.genre] || "🔔"}</span>
        <span class="cloche-ligne"><b>${esc(n.titre)}</b>${n.texte ? `<span>${esc(n.texte)}</span>` : ""}<small>${esc(quand(n.cree))}</small></span>
      </button></li>`).join("")}</ul>`;
  }

  function peindreListe() {
    if (!ouverte) return;
    const b = ouverte.voile.querySelector(".boite-corps");
    b.querySelector(".cloche-zone").innerHTML = corps();
    b.querySelector("#cloche-tout").disabled = !donnees.non_lues;
    b.querySelectorAll("[data-id]").forEach(x => {
      x.onclick = async () => {
        const n = donnees.notifications.find(y => y.id === +x.dataset.id);
        if (n && !n.lue) await envoyerLues([n.id]);
        if (n?.lien) { fermer(); location.hash = n.lien; }
      };
    });
  }

  async function envoyerLues(ids) {
    try { donnees = await marquer(ids); peindreBouton(); peindreListe(); }
    catch (e) { toast(e.message, true); }
  }

  function fermer() { ouverte?.fermer(); ouverte = null; }

  bouton.onclick = async () => {
    ouverte = dialogue("Notifications", `<div class="cloche-zone"></div>
      <div class="row cloche-actions"><button type="button" class="ghost petit" id="cloche-tout">Tout marquer comme lu</button><button type="button" class="petit" id="cloche-fermer">Fermer</button></div>`,
    { classe: "cloche-boite", rendre: bouton });
    const f = ouverte.fermer;
    ouverte.fermer = () => { f(); ouverte = null; };
    ouverte.voile.querySelector("#cloche-tout").onclick = () => envoyerLues([]);
    ouverte.voile.querySelector("#cloche-fermer").onclick = fermer;
    ouverte.voile.addEventListener("click", e => { if (e.target === ouverte?.voile) ouverte = null; });
    ouverte.voile.addEventListener("keydown", e => { if (e.key === "Escape") ouverte = null; });
    peindreListe();
    await rafraichir();
  };

  async function rafraichir() {
    try { donnees = await lire(); }
    catch { donnees = { non_lues: 0, notifications: donnees.notifications || [] }; } // hors ligne : pas de compteur
    peindreBouton();
    peindreListe();
  }

  return { rafraichir, bouton, cacher: v => { bouton.hidden = v; if (v) badge(0); } };
}
