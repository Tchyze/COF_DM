// Montée de niveau d'un PJ (MJ seulement) : PV gagnés, rangs pris dans les voies de la feuille,
// capacités débloquées rappelées. Les PV se lancent côté serveur quand les règles de l'univers en
// donnent la formule (personnage.montee), sinon ils se saisissent ; sans points dans les règles,
// les rangs se prennent librement. Le serveur vérifie tout : ce dialogue ne fait que préparer.
import { $, $$, esc, api, toast, dialogue } from "./ui.js";
import { voieLiee } from "./capacites.js";
import { actif } from "./schema.js";

const signe = n => (n >= 0 ? "+" : "−") + Math.abs(n);

// coût d'un rang et niveau qu'il demande, comme internal/regles (Montee.Cout, Montee.NiveauPour)
export const coutRang = (m, r) => !m?.couts?.length ? 1 : (m.couts[r - 1] ?? m.couts.at(-1));
export const niveauPour = (m, r) => m?.niveau_min?.[r - 1] || 0;
const lancee = m => !!(m && (m.pv?.length || m.pv_fixe));

// formulePv : la formule des PV gagnés, en clair (« 1d8 + 2 (CON) + 1 »).
function formulePv(m, F, niveau) {
  const terme = n => `${n >= 0 ? "+" : "−"} ${Math.abs(n)}`;
  const t = (m.pv || []).map(x => x === "dv" ? `+ 1${F.dv || "d?"}` : x === "con" ? `${terme(F.carac.CON)} (CON)` : `${terme(niveau)} (niveau)`);
  if (m.pv_fixe) t.push(terme(m.pv_fixe));
  return t.join(" ").replace(/^\+ /, "") + (m.pv_min ? `, au moins ${m.pv_min}` : "");
}

/** Une ligne de l'historique en clair : PV gagnés (et le jet), rangs pris, capacités débloquées. */
export function texteMontee(m) {
  const morceaux = [`${signe(m.pv)} PV${m.jet ? ` (${m.jet})` : ""}`];
  for (const r of m.rangs || []) morceaux.push(`${r.voie} rang ${r.de} → ${r.a}${r.capacites?.length ? ` (${r.capacites.join(", ")})` : ""}`);
  if (m.points) morceaux.push(`${m.points} point${m.points > 1 ? "s" : ""} de capacité dépensé${m.points > 1 ? "s" : ""}`);
  return morceaux.join(" ; ");
}

/**
 * Ouvre le dialogue de montée de niveau du PJ `id` (nom : pour le titre). `apres(feuille)` est
 * appelé une fois la montée faite, avec la feuille renvoyée par le serveur.
 */
export async function monterNiveau(id, nom, apres) {
  let F, REF;
  try { [F, REF] = await Promise.all([api("GET", `/api/entites/${id}/feuille`), api("GET", `/api/reference?pj=${id}`).catch(() => ({ voies: [] }))]); }
  catch (e) { toast(e.message, true); return; }
  if (!F.existe) { toast("Ce PJ n'a pas encore de feuille : remplissez-la d'abord.", true); return; }
  const R = F.regles || {}, M = R.montee || null, niveau = F.niveau + 1;
  if (R.niveau_max && niveau > R.niveau_max) { toast(`Niveau ${R.niveau_max} au plus.`, true); return; }
  const parPoints = M?.points > 0, dispo = parPoints ? M.points + (F.points_capacite || 0) : 0;
  const rangMax = v => R.rang_max || Math.max(5, v.capacites.length, v.rang + 1);
  const capacites = v => voieLiee(REF, v)?.rangs || v.capacites.map(nom => ({ nom }));
  const vise = F.voies.map(v => v.rang); // le rang que chaque voie atteindra

  const lignePv = lancee(M)
    ? `<p>PV gagnés : <b>${esc(formulePv(M, F, niveau))}</b>, lancés à la validation.</p>
       ${!F.dv && (M.pv || []).includes("dv") ? `<p class="alerte">La feuille n'a pas de dé de vie : choisissez-le d'abord en la modifiant.</p>` : ""}
       ${actif("des") ? `<label class="mn-case"><input type="checkbox" id="mn-cache"> Jet caché aux joueurs</label>` : `<p class="hint">Dés virtuels coupés : le jet n'apparaîtra que dans l'historique.</p>`}`
    : `<label class="champ"><span>PV gagnés</span><input type="number" id="mn-pv" min="0" max="999" required inputmode="numeric"></label>
       <p class="hint">Les règles de l'univers ne donnent pas la formule des PV : saisissez-les.</p>`;
  const { voile, fermer } = dialogue(`${nom} passe au niveau ${niveau}`, `
    <form class="mn" id="mn">
      ${M?.aide ? `<p class="hint">${esc(M.aide)}</p>` : ""}
      ${lignePv}
      <h4>Rangs de voie${parPoints ? ` <span class="sp"></span><span class="hint" id="mn-points"></span>` : ""}</h4>
      ${parPoints ? `<p class="hint">${M.points} point${M.points > 1 ? "s" : ""} de capacité à ce niveau${F.points_capacite ? `, plus ${F.points_capacite} gardé${F.points_capacite > 1 ? "s" : ""}` : ""}${M.report ? " ; ce qui n'est pas dépensé se garde." : "."}</p>` : `<p class="hint">Les règles ne comptent pas de points : prenez les rangs librement.</p>`}
      ${F.voies.length ? `<div class="mn-voies">${F.voies.map((v, i) => `<div class="mn-voie" data-i="${i}">
          <label><b>${esc(v.nom || "Voie sans nom")}</b> <span class="hint">rang ${v.rang} →</span>
            <select data-voie="${i}" aria-label="${esc(v.nom)} : rang atteint">${Array.from({ length: rangMax(v) - v.rang + 1 }, (_, k) => v.rang + k).map(r => {
              const bloque = [...Array(r - v.rang).keys()].map(k => v.rang + k + 1).find(x => niveauPour(M, x) > niveau);
              return `<option value="${r}"${bloque ? " disabled" : ""}>${r === v.rang ? "inchangé" : `rang ${r}`}${bloque ? ` (niveau ${niveauPour(M, bloque)})` : ""}</option>`;
            }).join("")}</select></label>
          <ul class="mn-caps hint"></ul></div>`).join("")}</div>`
        : `<p class="vide">Aucune voie sur la feuille : ajoutez-en d'abord en la modifiant.</p>`}
      <p class="alerte" id="mn-err" hidden></p>
      <div class="ligne"><span class="sp"></span><button type="button" class="ghost" id="mn-non">Annuler</button><button class="primary" type="submit" id="mn-oui">Passer au niveau ${niveau}</button></div>
    </form>`);
  const form = $("#mn", voile), err = $("#mn-err", voile);
  const cout = i => { let c = 0; for (let r = F.voies[i].rang + 1; r <= vise[i]; r++) c += coutRang(M, r); return c; };
  const depense = () => F.voies.reduce((s, _, i) => s + cout(i), 0);
  function peindre() {
    $$(".mn-voie", voile).forEach(el => {
      const i = +el.dataset.i, v = F.voies[i], caps = capacites(v);
      const nouvelles = [...Array(vise[i] - v.rang).keys()].map(k => v.rang + k);
      $(".mn-caps", el).innerHTML = nouvelles.map(r => {
        const c = caps[r] || {};
        return `<li><b>${esc(c.nom || `Rang ${r + 1}`)}</b>${parPoints ? ` <span>(${coutRang(M, r + 1)} pt)</span>` : ""}${c.texte ? ` : ${esc(c.texte)}` : ""}</li>`;
      }).join("");
    });
    const d = depense(), trop = parPoints && d > dispo;
    if (parPoints) { const p = $("#mn-points", voile); p.textContent = `${d} / ${dispo} point${dispo > 1 ? "s" : ""}`; p.classList.toggle("fp-epuise", trop); }
    $("#mn-oui", voile).disabled = trop;
  }
  $$("select[data-voie]", voile).forEach(s => s.onchange = () => { vise[+s.dataset.voie] = +s.value; peindre(); });
  $("#mn-non", voile).onclick = fermer;
  form.onsubmit = async e => {
    e.preventDefault();
    const corps = { rangs: F.voies.map((v, i) => ({ voie: i, rang: vise[i] })).filter((x, i) => x.rang > F.voies[i].rang), modifie: F.modifie || undefined };
    if (!lancee(M)) corps.pv = +$("#mn-pv", voile).value;
    if ($("#mn-cache", voile)) corps.cache = $("#mn-cache", voile).checked;
    $("#mn-oui", voile).disabled = true;
    try {
      const r = await api("POST", `/api/feuilles/${id}/niveau`, corps);
      fermer();
      toast(`${nom} passe au niveau ${r.niveau} : ${texteMontee(r.montee)}.`);
      apres?.(r);
    } catch (x) {
      err.textContent = x.status === 409 ? "La feuille a été modifiée ailleurs : fermez et recommencez." : x.message;
      err.hidden = false;
      $("#mn-oui", voile).disabled = false;
    }
  };
  peindre();
  ($("#mn-pv", voile) || $("select", voile) || $("#mn-oui", voile))?.focus();
}

/** Annule la dernière montée du PJ `id` ; `apres(feuille)` une fois faite. */
export async function annulerMontee(id, modifie, apres) {
  try {
    const r = await api("POST", `/api/feuilles/${id}/niveau/annuler`, { modifie: modifie || undefined });
    toast(`Montée au niveau ${r.annulee.niveau} annulée : retour au niveau ${r.niveau}.`);
    apres?.(r);
  } catch (e) { toast(e.message, true); }
}
