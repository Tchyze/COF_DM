// Capacités des voies : leurs effets en clair (bonus, action en combat, utilisations), pour la
// bulle de la feuille et l'arbre des voies de la page des joueurs.

/** Un nom sans accents ni majuscules, pour rapprocher deux écritures d'une voie. */
export const cle = s => (s || "").toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[\s'’,-]+/g, " ").trim();

/**
 * La voie de la bibliothèque que désigne une voie de feuille, comme le serveur : même ref quand
 * les deux en ont une, sinon même nom (undefined : voie libre).
 */
export const voieLiee = (ref, v) => (ref?.voies || []).find(x => (v.ref && x.ref ? x.ref === v.ref : cle(x.nom) === cle(v.nom)));

const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);
const CIBLES = { pv: "PV max", pm: "PM max", def: "DEF", init: "Initiative", att: "Attaques", att_c: "Attaque au contact", att_d: "Attaque à distance", att_m: "Attaque magique", rd: "RD" };
const GENRES = { sort: "Sort", attaque: "Attaque spéciale", soin: "Soin", etat: "État", autre: "Action" };
const JETS = { C: "au contact", D: "à distance", M: "magique" };

/** « par combat », « par jour », « par nuit de repos »… ; repos : ceux des règles de la feuille. */
export function libellePar(par, repos = []) {
  if (par === "combat") return "par combat";
  if (par === "jour") return "par jour";
  if (par === "main") return "à remettre à zéro à la main";
  const c = par.slice(6);
  return `par ${(repos.find(x => x.cle === c)?.libelle || c).toLowerCase()}`;
}

/** Les effets d'une capacité, une ligne par effet. */
export function lignesEffets(c, repos = []) {
  const lignes = [];
  for (const b of c.bonus || []) lignes.push(`${CIBLES[b.cible] || b.cible} ${signe(b.valeur)}${b.par_rang ? " × rang" : ""}${b.niveau ? " + niveau" : ""}${b.carac ? ` + mod. ${b.carac}` : ""}`);
  if (c.action) {
    const a = c.action;
    lignes.push([GENRES[a.genre] || a.genre, a.cout_pm && `${a.cout_pm} PM`, a.attaque && `attaque ${JETS[a.attaque] || a.attaque}${a.bonus ? ` ${signe(a.bonus)}` : ""}`,
      a.degats && `DM ${a.degats}`, a.soin && `soigne ${a.soin}`, a.etat && `état : ${a.etat}`].filter(Boolean).join(" · "));
  }
  if (c.utilisations) lignes.push(`${c.utilisations.fois} fois ${libellePar(c.utilisations.par, repos)}`);
  if (c.limitee) lignes.push("Action limitée");
  return lignes;
}
