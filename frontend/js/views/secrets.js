// Qui sait quoi : chaque secret face aux PJ (et aux PNJ ou factions qui savent quelque chose).
import { $, $$, esc, api, toast } from "../ui.js";
import { vignette, valeurCourte } from "../schema.js";

export async function mount(el, params) {
  const campagnes = await api("GET", "/api/entites?type=campagne");
  let camp = +(params.get("campagne") || (campagnes.length === 1 ? campagnes[0].id : 0));
  let filtre = "";
  el.innerHTML = `
    <div class="entete"><h1>Qui sait quoi</h1><span class="sp"></span>
      <select id="camp" style="width:auto"><option value="0">Toutes les campagnes</option>${campagnes.map(c => `<option value="${c.id}" ${c.id === camp ? "selected" : ""}>${esc(c.nom)}</option>`).join("")}</select>
      <select id="filtre" style="width:auto"><option value="">Tous les secrets</option><option value="caché">encore cachés</option><option value="partiel">en partie connus des PJ</option><option value="révélé">révélés</option></select>
      <a href="#/nouveau/secret${camp ? "?campagne=" + camp : ""}"><button class="primary">+ Secret</button></a>
    </div>
    <p class="hint" style="margin-bottom:12px">Cliquez une case pour indiquer qu'un PJ sait (ou ne sait plus) un secret. Le détail (depuis quelle séance, comment) se règle sur la fiche du secret.</p>
    <div id="tableau"></div>`;
  const charger = async () => {
    const d = await api("GET", "/api/secrets" + (camp ? "?campagne=" + camp : ""));
    const sait = new Map(d.connaissances.map(c => [c.secret + ":" + c.qui, c]));
    let secrets = d.secrets;
    if (filtre) {
      secrets = secrets.filter(sec => {
        const n = d.pj.filter(p => sait.has(sec.id + ":" + p.id)).length;
        return filtre === "caché" ? n === 0 : filtre === "révélé" ? n > 0 && n === d.pj.length : n > 0 && n < d.pj.length;
      });
    }
    if (!d.secrets.length) {
      $("#tableau", el).innerHTML = `<div class="panneau"><p class="vide">Aucun secret. Créez une fiche « Secret » : la vérité, ce qui la révèle, qui la sait.</p></div>`;
      return;
    }
    const autres = sec => d.pnj.filter(p => sait.has(sec.id + ":" + p.id));
    $("#tableau", el).innerHTML = `<div class="panneau" style="overflow-x:auto"><table class="tab matrice">
      <thead><tr><th>Secret</th><th>Statut</th>${d.pj.map(p => `<th class="pj" title="${esc(p.nom)}">${vignette(p, 28)}<span>${esc(p.nom)}</span></th>`).join("")}<th>PNJ, factions qui savent</th></tr></thead>
      <tbody>${secrets.map(sec => `<tr>
        <td><a class="lien-fiche" href="#/f/${sec.id}" data-id="${sec.id}">🔒 ${esc(sec.nom)}</a>${sec.carte?.importance ? ` <span class="badge">${esc(sec.carte.importance)}</span>` : ""}</td>
        <td>${valeurCourte("secret", "statut", sec.carte?.statut) || `<span class="hint">—</span>`}</td>
        ${d.pj.map(p => { const c = sait.get(sec.id + ":" + p.id); return `<td class="case${c ? " sait" : ""}"><button data-s="${sec.id}" data-p="${p.id}" ${c ? `data-rel="${c.relation}"` : ""} aria-label="${esc(p.nom)} ${c ? "sait" : "ne sait pas"}">${c ? "✓" : ""}</button></td>`; }).join("")}
        <td><div class="ligne" style="gap:4px 10px">${autres(sec).map(x => `<a class="lien-fiche" href="#/f/${x.id}" data-id="${x.id}">${vignette(x, 18)}${esc(x.nom)}</a>`).join("") || `<span class="hint">—</span>`}</div></td>
      </tr>`).join("") || `<tr><td colspan="${d.pj.length + 3}" class="vide">Aucun secret ne correspond.</td></tr>`}</tbody>
    </table>${d.pj.length ? "" : `<p class="hint">Aucun PJ${camp ? " dans cette campagne" : ""} : créez les fiches PJ pour suivre ce qu'ils savent.</p>`}</div>`;
  };
  $("#camp", el).onchange = e => { camp = +e.target.value; charger(); };
  $("#filtre", el).onchange = e => { filtre = e.target.value; charger(); };
  el.addEventListener("click", async e => {
    const b = e.target.closest("td.case button");
    if (!b) return;
    try {
      if (b.dataset.rel) await api("DELETE", "/api/relations/" + b.dataset.rel);
      else await api("POST", `/api/secrets/${b.dataset.s}/reveler`, { qui: [+b.dataset.p] });
      window.dispatchEvent(new Event("cof-change"));
      charger();
    } catch (err) { toast(err.message, true); }
  });
  await charger();
}
