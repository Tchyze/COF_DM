// Chronologie : toutes les dates en jeu, du plus ancien au plus récent.
import { $, esc, api } from "../ui.js";
import { vignette, badgeType } from "../schema.js";

export async function mount(el, params) {
  const campagnes = await api("GET", "/api/entites?type=campagne");
  let camp = +(params.get("campagne") || 0);
  el.innerHTML = `
    <div class="entete"><h1>Chronologie</h1><span class="sp"></span>
      <select id="camp" style="width:auto"><option value="0">Tout le monde</option>${campagnes.map(c => `<option value="${c.id}" ${c.id === camp ? "selected" : ""}>${esc(c.nom)}</option>`).join("")}</select>
      <a href="#/nouveau/evenement"><button class="primary">+ Événement</button></a>
    </div>
    <div id="liste"></div>`;
  const charger = async () => {
    const l = await api("GET", "/api/chronologie" + (camp ? "?campagne=" + camp : ""));
    if (!l.length) {
      $("#liste", el).innerHTML = `<p class="vide">Aucune date en jeu. Datez des événements, des séances ou le début d'une campagne pour les voir ici.</p>`;
      return;
    }
    let an = null, html = "";
    for (const m of l) {
      if (m.date.an !== an) { an = m.date.an; html += `<div class="an">${an}</div>`; }
      html += `<a class="moment" href="#/f/${m.fiche.id}" data-type="${m.fiche.type}">
        <span class="date">${esc(m.date_texte.replace(/ \d+( .*)?$/, ""))}</span>${vignette(m.fiche, 30)}
        <span style="flex:1;min-width:0"><b>${esc(m.fiche.nom)}</b> <span class="hint">${esc(m.champ.toLowerCase())}</span></span>${badgeType(m.fiche.type)}</a>`;
    }
    $("#liste", el).innerHTML = `<div class="chrono">${html}</div>`;
  };
  $("#camp", el).onchange = e => { camp = +e.target.value; charger(); };
  await charger();
}
