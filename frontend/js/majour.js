// Mises à jour de l'application de bureau : le panneau de Paramètres (canal, recherche,
// installation) et le badge du pied du menu quand une version attend. Rien ne s'installe sans clic.
import { $, esc, api, toast, dateHeure } from "./ui.js";

const CANAUX = {
  stable: ["Stable", "Les versions publiées, pour jouer."],
  test: ["Test", "En plus, les versions d'essai d'une modification en cours (elles peuvent avoir des défauts)."],
};

/** Le panneau « Mises à jour » de Paramètres. */
export async function panneauMaj(box) {
  const dessiner = m => {
    const d = m.disponible;
    box.innerHTML = `<h3>Mises à jour</h3>
      <p class="hint">Version installée : <b>${esc(m.version)}</b>.${m.verifie ? ` Dernière recherche : ${esc(dateHeure(m.verifie))}.` : ""}</p>
      <div role="radiogroup" aria-label="Canal des mises à jour" class="colonne" style="gap:6px">
        ${Object.entries(CANAUX).map(([c, [nom, texte]]) => `<label class="t-coche"><input type="radio" name="maj-canal" value="${c}" ${m.canal === c ? "checked" : ""}><span>${nom}<small>${texte}</small></span></label>`).join("")}
      </div>
      ${m.erreur ? `<p class="alerte">Recherche impossible : ${esc(m.erreur)}</p>` : ""}
      ${d ? `<div class="maj-dispo">
          <p><b>Version ${esc(d.version)}</b> disponible${d.preversion ? " (version d'essai)" : ""}${d.date ? `, publiée le ${esc(new Date(d.date).toLocaleDateString("fr-FR"))}` : ""}.</p>
          ${d.notes ? `<details><summary>Nouveautés</summary><pre class="maj-notes">${esc(d.notes)}</pre></details>` : ""}
          ${m.automatique && m.installable
            ? `<p class="hint">L'univers ouvert est sauvegardé et fermé, la nouvelle version vérifiée (signature de COF DM) puis installée, et l'application redémarre.</p>
               <div class="ligne"><button class="primary" id="maj-installer">⬆ Installer et redémarrer</button></div>`
            : `<p class="hint">Ce programme ne se met pas à jour seul${m.automatique ? "" : " (version de développement, ou binaire Linux hors AppImage)"} : téléchargez la version depuis <a href="${esc(d.page)}" target="_blank" rel="noopener">sa page</a>.</p>`}
        </div>`
      : `<p class="hint">${m.verifie && !m.erreur ? "✓ L'application est à jour." : ""}</p>`}
      <div class="ligne"><button id="maj-chercher">Rechercher maintenant</button></div>`;
    box.querySelectorAll("input[name=maj-canal]").forEach(r => r.onchange = async () => {
      try { dessiner(await api("PUT", "/api/univers/mise-a-jour/canal", { canal: r.value })); badge(); }
      catch (e) { toast(e.message, true); }
    });
    $("#maj-chercher", box).onclick = async e => {
      e.target.disabled = true;
      e.target.textContent = "Recherche…";
      try { dessiner(await api("POST", "/api/univers/mise-a-jour/chercher")); badge(); }
      catch (err) { toast(err.message, true); e.target.disabled = false; }
    };
    const inst = $("#maj-installer", box);
    if (inst) inst.onclick = async () => {
      inst.disabled = true;
      inst.textContent = "Téléchargement et vérification…";
      try {
        const r = await api("POST", "/api/univers/mise-a-jour/installer");
        inst.textContent = r.relance ? "Redémarrage…" : "Installée : relancez l'application";
        if (r.relance) attendreRetour(r.installee);
      } catch (err) { toast(err.message, true); inst.disabled = false; inst.textContent = "⬆ Installer et redémarrer"; }
    };
  };
  dessiner(await api("GET", "/api/univers/mise-a-jour"));
}

/** Après l'installation : la page revient quand la nouvelle version répond. */
function attendreRetour(version) {
  toast(`Version ${version} installée : redémarrage…`);
  const essai = async () => {
    try {
      const r = await fetch("/api/sante", { cache: "no-store" });
      if (r.ok && (await r.json()).version === version) { location.reload(); return; }
    } catch { /* l'application redémarre */ }
    setTimeout(essai, 1000);
  };
  setTimeout(essai, 1500);
}

/** Le badge du pied du menu : une version attend. */
async function badge() {
  const b = $("#maj-badge");
  let m;
  try { m = await api("GET", "/api/univers/mise-a-jour"); } catch { return; }
  const d = m.disponible;
  b.hidden = !d;
  if (d) {
    b.textContent = `⬆ ${d.version}`;
    b.title = `Version ${d.version} disponible : voir Paramètres → Mises à jour`;
  }
}

export function initBadgeMaj() {
  const b = $("#maj-badge");
  b.onclick = () => {
    location.hash = "#/parametres";
    setTimeout(() => $("#maj")?.scrollIntoView({ behavior: "smooth", block: "start" }), 400);
  };
  setTimeout(badge, 8000); // l'application cherche cinq secondes après son lancement
  setInterval(badge, 30 * 60 * 1000);
}
