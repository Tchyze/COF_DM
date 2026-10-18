// Schéma des fiches (servi par /api/schema) et aides d'affichage.
import { api, esc, lien } from "./ui.js";

export let schema = null;
const parType = new Map();

export async function chargerSchema() {
  schema = await api("GET", "/api/schema");
  parType.clear();
  for (const t of schema.types) parType.set(t.cle, t);
  return schema;
}

export const typeDe = cle => parType.get(cle);
export const types = () => schema.types;
export const champDe = (type, cle) => typeDe(type)?.champs.find(c => c.cle === cle);
export const relationDe = cle => schema.relations.find(r => r.cle === cle);

/** Date en jeu en toutes lettres selon le calendrier. */
export function texteDate(d) {
  if (!d || typeof d !== "object") return "";
  const cal = schema.calendrier;
  const m = cal.mois[d.mois - 1];
  if (!m) return `${d.jour}/${d.mois}/${d.an}`;
  return `${d.jour} ${m.nom} ${d.an}${cal.ere ? " " + cal.ere : ""}`;
}

/** Vignette ronde : image principale ou icône du type. */
export function vignette(f, taille = 44, carree = false) {
  const t = typeDe(f.type);
  const contenu = f.image ? `<img src="${lien("/medias/" + esc(f.image))}" alt="" loading="lazy">` : esc(t?.icone || "•");
  return `<span class="vignette${carree ? " carree" : ""}" data-type="${esc(f.type)}" style="--t:${taille}px">${contenu}</span>`;
}

/** Lien vers une fiche (avec petite vignette). */
export function lienFiche(f, avecVignette = true) {
  if (!f) return "";
  return `<a class="lien-fiche" href="#/f/${f.id}" data-id="${f.id}">${avecVignette ? vignette(f, 20) : ""}<span>${esc(f.nom)}</span></a>`;
}

export function badgeType(type) {
  const t = typeDe(type);
  return `<span class="badge type" data-type="${esc(type)}">${esc(t?.icone || "")} ${esc(t?.nom || type)}</span>`;
}

/** Valeur courte d'un champ pour une carte ou un tableau de propriétés. refs : id → résumé. */
export function valeurCourte(type, cle, v, refs = {}) {
  const ch = champDe(type, cle);
  if (v == null || v === "" || !ch) return "";
  switch (ch.genre) {
    case "ref": return refs[v] ? lienFiche(refs[v]) : `<span class="vide">fiche n°${esc(v)}</span>`;
    case "refs": return (v || []).map(x => refs[x] ? lienFiche(refs[x]) : "").filter(Boolean).join(", ");
    case "date_jeu": return esc(texteDate(v));
    case "date": return esc(new Date(v + "T12:00:00").toLocaleDateString("fr-FR"));
    case "booleen": return v ? "oui" : "non";
    case "nombre": return esc(String(v).replace(".", ","));
    case "choix": return `<span class="statut-${esc(String(v).replace(/ /g, "."))}">${esc(v)}</span>`;
    case "monstre": return `<a href="#/bestiaire/${esc(v)}">bestiaire n°${esc(v)}</a>`;
    case "adversaires": return esc((v || []).map(a => `${a.qte} × ${a.nom || "créature"}`).join(", "));
    default: return esc(v);
  }
}
