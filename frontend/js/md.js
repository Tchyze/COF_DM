// Rendu Markdown façon Obsidian : titres, listes (tâches comprises), citations, tableaux, code,
// gras / italique / barré / surligné, liens externes et liens internes [[cible#ancre|texte]].
import { esc } from "./ui.js";

/** Clé de comparaison d'un nom. */
export const cle = s => String(s).trim().toLowerCase();

/**
 * Prépare la résolution des liens [[Nom]] d'une fiche à partir de son champ « mentions » (API).
 * Renvoie une fonction nom → {id, type} | null (lien mort) | undefined (inconnu).
 */
export function resolveur(mentions) {
  const m = new Map(Object.entries(mentions || {}));
  return nom => {
    const x = m.get(cle(nom));
    if (!x) return undefined;
    return x.id ? x : null;
  };
}

function lienInterne(interieur, image, resoudre) {
  let texte = "", cible = interieur, ancre = "";
  let p = cible.indexOf("\\|");
  if (p >= 0) { texte = cible.slice(p + 2); cible = cible.slice(0, p); }
  else if ((p = cible.indexOf("|")) >= 0) { texte = cible.slice(p + 1); cible = cible.slice(0, p); }
  if ((p = cible.indexOf("#")) >= 0) { ancre = cible.slice(p + 1); cible = cible.slice(0, p); }
  cible = cible.trim();
  if (!cible) return "";
  const affiche = texte || cible + (ancre ? ` › ${ancre}` : "");
  const r = resoudre ? resoudre(cible) : undefined;
  if (r && r.id) return `<a class="wl" href="#/f/${r.id}" data-nom="${esc(cible)}">${esc(affiche)}</a>`;
  if (r === null) return `<a class="wl mort" href="#/nouveau?nom=${encodeURIComponent(cible)}" title="Aucune fiche « ${esc(cible)} » : cliquer pour la créer">${esc(affiche)}</a>`;
  return `<a class="wl" href="#/aller/${encodeURIComponent(cible)}" data-nom="${esc(cible)}">${esc(affiche)}</a>`;
}

/** Rendu d'une ligne de texte (sans structure de bloc). */
export function inline(src, resoudre) {
  const gardes = [];
  const garder = html => `\u0000${gardes.push(html) - 1}\u0000`;
  let s = String(src);
  s = s.replace(/`([^`]+)`/g, (_, c) => garder(`<code>${esc(c)}</code>`));
  s = s.replace(/!?\[\[\]\]/g, "");
  s = s.replace(/(!?)\[\[([^\]\n]+?)\]\]/g, (_, img, inter) => garder(lienInterne(inter, !!img, resoudre)));
  s = s.replace(/\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, t, u) => garder(`<a href="${esc(u)}" target="_blank" rel="noopener">${esc(t)}</a>`));
  s = esc(s);
  s = s.replace(/\*\*(.+?)\*\*/g, "<b>$1</b>").replace(/__(.+?)__/g, "<b>$1</b>");
  s = s.replace(/(^|[^*\w])\*(?!\s)(.+?)\*(?!\w)/g, "$1<i>$2</i>").replace(/(^|[^_\w])_(?!\s)(.+?)_(?!\w)/g, "$1<i>$2</i>");
  s = s.replace(/~~(.+?)~~/g, "<s>$1</s>").replace(/==(.+?)==/g, "<mark>$1</mark>");
  return s.replace(/\u0000(\d+)\u0000/g, (_, i) => gardes[+i]);
}

/** Découpe une ligne de tableau en cellules, sans couper sur une barre échappée (\|). */
function cellules(ligne) {
  let l = ligne.trim();
  if (l.startsWith("|")) l = l.slice(1);
  if (l.endsWith("|") && !l.endsWith("\\|")) l = l.slice(0, -1);
  const out = [];
  let cur = "", lien = 0;
  for (let i = 0; i < l.length; i++) {
    if (l.startsWith("[[", i)) lien++;
    if (l.startsWith("]]", i) && lien) lien--;
    if (l[i] === "\\" && l[i + 1] === "|") { cur += "\\|"; i++; continue; }
    if (l[i] === "|" && !lien) { out.push(cur.trim()); cur = ""; continue; }
    cur += l[i];
  }
  out.push(cur.trim());
  return out;
}

const RE_LISTE = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;

/** Rendu complet d'un texte Markdown en HTML. */
export function rendu(src, resoudre) {
  const lignes = String(src || "").replace(/\r\n/g, "\n").split("\n");
  let html = "", i = 0;
  const para = [];
  const fermerPara = () => {
    if (para.length) html += `<p>${para.map(l => inline(l, resoudre)).join("<br>")}</p>`;
    para.length = 0;
  };
  while (i < lignes.length) {
    const l = lignes[i];
    const t = l.trim();
    if (/^(```|~~~)/.test(t)) {
      fermerPara();
      const fin = t.slice(0, 3);
      const code = [];
      i++;
      while (i < lignes.length && !lignes[i].trim().startsWith(fin)) code.push(lignes[i++]);
      i++;
      html += `<pre><code>${esc(code.join("\n"))}</code></pre>`;
      continue;
    }
    if (!t) { fermerPara(); i++; continue; }
    let m;
    if ((m = /^(#{1,6})\s+(.*)$/.exec(t))) {
      fermerPara();
      html += `<h${m[1].length}>${inline(m[2].replace(/\s+#+$/, ""), resoudre)}</h${m[1].length}>`;
      i++; continue;
    }
    if (/^(-{3,}|\*{3,}|_{3,})$/.test(t)) { fermerPara(); html += "<hr>"; i++; continue; }
    if (t.startsWith(">")) {
      fermerPara();
      const bloc = [];
      while (i < lignes.length && lignes[i].trim().startsWith(">")) bloc.push(lignes[i++].trim().replace(/^>\s?/, ""));
      html += `<blockquote>${rendu(bloc.join("\n"), resoudre)}</blockquote>`;
      continue;
    }
    if (t.startsWith("|") && i + 1 < lignes.length && /^\s*\|?\s*:?-{2,}/.test(lignes[i + 1])) {
      fermerPara();
      const tete = cellules(l);
      i += 2;
      let corps = "";
      while (i < lignes.length && lignes[i].trim().startsWith("|")) {
        corps += `<tr>${cellules(lignes[i++]).map(c => `<td>${inline(c, resoudre)}</td>`).join("")}</tr>`;
      }
      html += `<table><thead><tr>${tete.map(c => `<th>${inline(c, resoudre)}</th>`).join("")}</tr></thead><tbody>${corps}</tbody></table>`;
      continue;
    }
    if (RE_LISTE.test(l)) {
      fermerPara();
      const items = [];
      while (i < lignes.length && (m = RE_LISTE.exec(lignes[i]))) {
        items.push({ ind: m[1].replace(/\t/g, "    ").length, ord: /\d/.test(m[2]), texte: m[3] });
        i++;
        // ligne de continuation indentée
        while (i < lignes.length && lignes[i].trim() && /^\s{2,}/.test(lignes[i]) && !RE_LISTE.test(lignes[i])) {
          items[items.length - 1].texte += "\n" + lignes[i++].trim();
        }
      }
      for (let k = 0; k < items.length;) {
        const r = liste(items, k, resoudre);
        html += r.html; k = r.fin;
      }
      continue;
    }
    para.push(t);
    i++;
  }
  fermerPara();
  return html;
}

function liste(items, debut, resoudre) {
  const base = items[debut].ind;
  const tag = items[debut].ord ? "ol" : "ul";
  let html = `<${tag}>`, i = debut;
  while (i < items.length && items[i].ind >= base && !(items[i].ind === base && items[i].ord !== items[debut].ord)) {
    const it = items[i];
    if (it.ind > base) { // sous-liste orpheline : rattachée à l'élément précédent
      const sous = liste(items, i, resoudre);
      html = html.replace(/<\/li>$/, sous.html + "</li>");
      i = sous.fin; continue;
    }
    let texte = it.texte, classe = "";
    const tache = /^\[( |x|X)\]\s*(.*)$/s.exec(texte);
    if (tache) {
      classe = ' class="tache"';
      texte = `<input type="checkbox" disabled${tache[1] !== " " ? " checked" : ""}>` + inline(tache[2], resoudre).replace(/\n/g, "<br>");
    } else texte = texte.split("\n").map(x => inline(x, resoudre)).join("<br>");
    i++;
    let sous = "";
    if (i < items.length && items[i].ind > base) {
      const r = liste(items, i, resoudre);
      sous = r.html; i = r.fin;
    }
    html += `<li${classe}>${texte}${sous}</li>`;
  }
  return { html: html + `</${tag}>`, fin: i };
}

