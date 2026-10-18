// Monstres du bestiaire de l'univers : lecture et bloc de profil.
import { api, esc } from "./ui.js";

let liste = null, lue = 0;
/** Le bestiaire, relu au plus toutes les cinq secondes (plusieurs sélecteurs le demandent à la fois). */
export function listerMonstres() {
  if (!liste || Date.now() - lue > 5000) {
    lue = Date.now();
    liste = api("GET", "/api/bestiaire").catch(e => { liste = null; throw e; });
  }
  return liste;
}
export const lireMonstre = id => api("GET", "/api/bestiaire/" + id);

export const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
export const nc = n => (n === 0.5 ? "½" : String(n ?? 0).replace(".", ","));
export const libelleMonstre = m => m.nom + (m.nc != null ? ` · NC ${nc(m.nc)}` : "");

/** Bloc de profil d'un monstre. */
export function blocMonstre(m) {
  const car = m.carac || {};
  const att = (m.attaques || []).map(a => `<li><b>${esc(a.nom)}</b> ${signe(a.bonus || 0)}${a.dm ? `, DM ${esc(a.dm)}` : ""}${a.notes ? ` <span class="hint">${esc(a.notes)}</span>` : ""}</li>`).join("");
  return `<div class="bloc-combat">
    <div class="ligne" style="gap:6px 14px">
      <span><b>NC</b> ${nc(m.nc)}</span><span><b>DEF</b> ${m.defense}</span><span><b>PV</b> ${m.pv}</span><span><b>Init</b> ${m.init}</span>
      ${m.rd ? `<span><b>RD</b> ${m.rd}</span>` : ""}<span class="hint">${esc(m.categorie || "")} · ${esc(m.taille || "")}${m.source === "officiel" ? " · officiel" : ""}</span>
    </div>
    <div class="carac">${["FOR", "DEX", "CON", "INT", "SAG", "CHA"].map(k => `<span><small>${k}</small>${signe(car[k] || 0)}${(m.superieures || []).includes(k) ? "*" : ""}</span>`).join("")}</div>
    ${att ? `<ul class="attaques">${att}</ul>` : ""}
    ${m.capacites ? `<p class="hint" style="white-space:pre-line">${esc(m.capacites)}</p>` : ""}
  </div>`;
}
