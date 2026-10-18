// Récapitulatif d'un combat terminé : bandeau, moments forts, bilan par PJ.
import { esc } from "./ui.js";

const ISSUE = { victoire: "Victoire !", defaite: "Défaite…" };
const ICONE = { gros: "⚔", grace: "☠", frole: "♥", crit: "✦", chance: "☘", nemesis: "☗", mur: "⛨", maudit: "⚀", coriace: "◆" };
const pctPv = p => p.max ? Math.round(p.pv / p.max * 100) : 0;
const etiquette = (p, i, l) => i === l.length - 1 ? "Fin" : "R" + p.r;

export function dateCourte(s) {
  if (!s) return "";
  try { return new Date(s * 1000).toLocaleDateString("fr-FR", { day: "numeric", month: "long", year: "numeric" }); } catch { return ""; }
}

// courbe des PV du groupe (début de chaque round, puis fin) : petite ligne, sans axe superflu
const courbeUtile = c => c?.length > 1 && c.some(p => pctPv(p) !== pctPv(c[0]));

function courbeHTML(c) {
  if (!courbeUtile(c)) return "";
  const W = 600, H = 70, px = 14, py = 10;
  const x = i => px + i * (W - 2 * px) / (c.length - 1), y = v => py + (100 - v) * (H - 2 * py) / 100;
  const pts = c.map((p, i) => [x(i), y(pctPv(p))]);
  const desc = c.map((p, i, l) => `${etiquette(p, i, l)} ${pctPv(p)} %`).join(", ");
  const bas = c.reduce((m, p, i) => pctPv(p) < pctPv(c[m]) ? i : m, 0);
  return `<figure class="recap-courbe" aria-label="PV du groupe : ${desc}">
    <figcaption>PV du groupe</figcaption>
    <svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-hidden="true">
      <line x1="${px}" x2="${W - px}" y1="${y(100)}" y2="${y(100)}" class="g"/><line x1="${px}" x2="${W - px}" y1="${y(0)}" y2="${y(0)}" class="g"/>
      <polyline points="${pts.map(p => p.join(",")).join(" ")}" class="l" vector-effect="non-scaling-stroke"/>
    </svg>
    <div class="recap-courbe-x">${c.map((p, i, l) => `<span class="${i === bas ? "bas" : ""}">${etiquette(p, i, l)}<b class="num">${pctPv(p)}%</b></span>`).join("")}</div>
  </figure>`;
}

export function recapHTML(r, { titre = true } = {}) {
  if (!r) return `<p class="empty">Pas de récapitulatif pour ce combat.</p>`;
  const infos = [
    `${r.rounds} round${r.rounds > 1 ? "s" : ""}`,
    r.duree_min ? `${r.duree_min} min` : "",
    r.ennemis ? `${r.vaincus}/${r.ennemis} adversaire${r.ennemis > 1 ? "s" : ""} vaincu${r.vaincus > 1 ? "s" : ""}` : "",
    r.tombes?.length ? `À terre : ${r.tombes.map(esc).join(", ")}` : "",
  ].filter(Boolean);
  const pct = b => b.tentatives ? Math.round(b.touches / b.tentatives * 100) + " %" : "—";
  return `<div class="recap ${r.issue || "neutre"}">
    ${titre ? `<div class="recap-tete"><div class="recap-issue">${ISSUE[r.issue] || "Combat terminé"}</div>
      <div class="recap-nom">${esc(r.nom)}${r.date ? ` · ${dateCourte(r.date)}` : ""}</div>
      <div class="recap-infos">${infos.map(i => `<span>${i}</span>`).join("")}</div></div>` : ""}
    ${r.heros ? `<div class="recap-heros"><span aria-hidden="true">★</span> Héros du combat : <b>${esc(r.heros.nom)}</b>${r.heros.texte ? ` <span class="hint">— ${esc(r.heros.texte)}</span>` : ""}</div>` : ""}
    ${r.moments?.length ? `<div class="recap-moments">${r.moments.map(m => `
      <div class="moment ${esc(m.camp)}"><span class="moment-ic" aria-hidden="true">${ICONE[m.cle] || "★"}</span>
        <div><div class="moment-t">${esc(m.titre)}</div><div class="moment-qui">${esc(m.qui)}</div><div class="moment-txt">${esc(m.texte)}</div></div></div>`).join("")}</div>` : ""}
    ${r.pjs?.length ? `<table class="recap-pjs"><thead><tr><th>PJ</th><th title="Dégâts infligés">Infligés</th><th title="Dégâts encaissés">Encaissés</th><th title="Attaques réussies / tentées">Précision</th><th title="Coups critiques">Crit.</th><th title="Adversaires mis à terre">Abattus</th></tr></thead>
      <tbody>${r.pjs.map(b => `<tr${b.tombe ? ' class="tombe"' : ""}>
        <td><b>${esc(b.nom)}</b><span class="titre" title="${esc(b.titre_desc)}">${esc(b.titre)}</span></td>
        <td class="num">${b.infliges}</td><td class="num">${b.subis}</td>
        <td class="num">${b.touches}/${b.tentatives}<span class="hint"> ${pct(b)}</span></td>
        <td class="num">${b.critiques}</td><td class="num">${b.abattus}</td></tr>`).join("")}</tbody></table>` : ""}
    ${courbeHTML(r.courbe)}
  </div>`;
}

// note Markdown pour Obsidian : un résumé lisible, pas le détail tour par tour
export function recapMarkdown(r) {
  if (!r) return "";
  const d = r.date ? new Date(r.date * 1000) : new Date();
  const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  const issue = { victoire: "Victoire", defaite: "Défaite" }[r.issue] || "Combat terminé";
  const cell = t => String(t ?? "").replace(/\|/g, "\\|");
  const infos = [dateCourte(r.date), `${r.rounds} round${r.rounds > 1 ? "s" : ""}`, r.duree_min ? `${r.duree_min} min` : "",
    r.ennemis ? `${r.vaincus}/${r.ennemis} adversaires vaincus` : "", r.tombes?.length ? `à terre : ${r.tombes.join(", ")}` : ""].filter(Boolean);
  const L = [
    "---", "type: combat", `date: ${iso}`, `issue: ${r.issue || "terminé"}`, `rounds: ${r.rounds}`,
    ...(r.heros ? [`heros: "${r.heros.nom.replace(/"/g, "'")}"`] : []), "---", "",
    `# ${r.nom} — ${issue}`, "", `*${infos.join(" · ")}*`, "",
  ];
  if (r.heros) L.push(`**Héros du combat : ${r.heros.nom}**${r.heros.texte ? ` — ${r.heros.texte}` : ""}`, "");
  if (r.moments?.length) {
    L.push("## Moments forts");
    r.moments.forEach(m => L.push(`- **${m.titre}** — ${m.qui} : ${m.texte}`));
    L.push("");
  }
  if (r.pjs?.length) {
    L.push("## Bilan", "", "| PJ | Titre | Infligés | Encaissés | Précision | Crit. | Abattus |", "|---|---|--:|--:|--:|--:|--:|");
    r.pjs.forEach(b => L.push(`| ${cell(b.nom)}${b.tombe ? " (à terre)" : ""} | ${cell(b.titre)} | ${b.infliges} | ${b.subis} | ${b.touches}/${b.tentatives} | ${b.critiques} | ${b.abattus} |`));
    L.push("");
  }
  if (courbeUtile(r.courbe)) L.push(`PV du groupe : ${r.courbe.map((p, i, l) => `${etiquette(p, i, l)} ${pctPv(p)} %`).join(" → ")}`, "");
  return L.join("\n");
}
