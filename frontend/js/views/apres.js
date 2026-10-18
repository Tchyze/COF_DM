// Après la séance (#/apres/<id>) : le MJ reporte en une fois ce qui s'est passé (résumé, PJ
// présents, date en jeu, lieu du groupe, étapes et statut des quêtes, secrets appris, PNJ tombés,
// rencontres jouées) et prépare la séance suivante. Tout part dans une seule requête, contrôlée
// avant la première écriture.
import { $, $$, esc, api, toast } from "../ui.js";
import { selecteur, saisieDate } from "../composants.js";
import { typeDe } from "../schema.js";

const STATUTS_QUETE = () => typeDe("quete").champs.find(c => c.cle === "statut").options;

export async function mount(el, id) {
  el.innerHTML = `<p class="hint">Chargement…</p>`;
  const [seance, p] = await Promise.all([api("GET", `/api/entites/${id}`), api("GET", `/api/entites/${id}/preparation`)]);
  const camp = p.campagne ? await api("GET", `/api/entites/${p.campagne.id}`).catch(() => null) : null;
  const quetes = await Promise.all(p.quetes.map(q => api("GET", `/api/entites/${q.fiche.id}`).catch(() => null)));
  // PNJ du monde tombés dans les combats de la séance, pas encore notés morts
  const tombes = new Map();
  try {
    for (const c of await api("GET", `/api/combats?seance=${id}`)) {
      const st = (await api("GET", `/api/combats/${c.id}`)).etat;
      for (const x of Object.values(st.cbts)) {
        if (x.pv <= 0 && x.ref?.kind === "pnj" && !tombes.has(x.ref.id)) {
          const f = await api("GET", `/api/entites/${x.ref.id}`).catch(() => null);
          if (f && f.data.statut !== "mort") tombes.set(f.id, { fiche: f, combat: st.nom });
        }
      }
    }
  } catch { /* sans combats */ }

  const presents = new Set(seance.data.presents || []);
  const pjCoche = x => presents.size ? presents.has(x.fiche.id) : x.statut !== "absent";
  const fiche = f => `<a class="wl" href="#/f/${f.id}">${esc(f.nom)}</a>`;
  const pjParNom = Object.fromEntries(p.pj.map(x => [x.fiche.nom, x.fiche.id]));
  const section = (titre, corps, vide) => `<section class="panneau"><h3>${titre}</h3>${corps || `<p class="vide">${vide}</p>`}</section>`;

  el.innerHTML = `
    <div class="entete"><h1>Après ${esc(seance.nom)}</h1><span class="sp"></span>
      <a href="#/f/${id}"><button>Voir la séance</button></a></div>
    <p class="hint">Reportez ce qui s'est passé : tout est enregistré d'un coup, la séance passe « jouée », et rien n'est changé si une valeur est refusée.</p>
    <div class="prep">
      <section class="panneau large"><h3>Résumé</h3>
        <textarea id="ap-resume" placeholder="Ce qui s'est passé, en quelques lignes (les joueurs le voient si la séance est partagée).">${esc(seance.data.resume || "")}</textarea>
      </section>
      ${section("Présents", p.pj.map(x => `<label class="prep-item"><input type="checkbox" data-present="${x.fiche.id}"${pjCoche(x) ? " checked" : ""}><span>${fiche(x.fiche)}${x.statut === "absent" ? ` <span class="hint">(absent)</span>` : ""}</span></label>`).join(""), "Aucun PJ dans cette campagne.")}
      <section class="panneau"><h3>Où et quand</h3>
        ${p.campagne ? `<label class="champ"><span>Date en jeu à la fin de la séance</span><div id="ap-date"></div></label>
        <label class="champ"><span>Où est le groupe</span><div id="ap-lieu"></div></label>` : `<p class="vide">La séance n'a pas de campagne.</p>`}
      </section>
      ${section("Quêtes", quetes.filter(Boolean).map(q => `<div class="ap-quete" data-quete="${q.id}">
          <div class="ligne">${fiche(q)}<span class="sp" style="flex:1"></span>
            <select data-statut aria-label="Statut de ${esc(q.nom)}">${STATUTS_QUETE().map(s => `<option${s === q.data.statut ? " selected" : ""}>${esc(s)}</option>`).join("")}</select></div>
          ${(q.data.etapes || []).map((e, i) => `<label class="prep-item"><input type="checkbox" data-etape="${i}"${e.fait ? " checked" : ""}><span>${esc(e.texte)}</span></label>`).join("")}
        </div>`).join(""), "Aucune quête en cours.")}
      ${section("Secrets appris", p.secrets.map(s => `<div class="ap-secret" data-secret="${s.fiche.id}">
          <div>🔒 ${fiche(s.fiche)} <span class="badge">${esc([s.importance, s.statut].filter(Boolean).join(" · "))}</span></div>
          <div class="ligne">${s.ignorent.map(n => `<label class="prep-item ap-qui"><input type="checkbox" data-qui="${pjParNom[n]}"><span>${esc(n)}</span></label>`).join("") || `<span class="hint">Tous les PJ présents le savent.</span>`}</div>
        </div>`).join(""), "Aucun secret en attente.")}
      ${section("PNJ tombés", [...tombes.values()].map(x => `<label class="prep-item"><input type="checkbox" data-mort="${x.fiche.id}"><span>${fiche(x.fiche)} <span class="hint">à terre dans « ${esc(x.combat)} » : le noter mort</span></span></label>`).join(""), "Aucun PNJ du monde à terre dans les combats de la séance.")}
      ${section("Rencontres", p.rencontres.map(r => `<div class="ligne ap-renc">⚔ ${fiche(r.fiche)}<span class="sp" style="flex:1"></span>
          <select data-renc="${r.fiche.id}" aria-label="Rencontre ${esc(r.fiche.nom)}"><option value="">pas jouée</option><option>jouée</option><option>évitée</option></select></div>`).join(""), "Aucune rencontre préparée.")}
    </div>
    <div class="ligne prep-bas">
      <label class="prep-item"><input type="checkbox" id="ap-suivante" checked><span>Créer la séance suivante (prévue)</span></label>
      <span class="sp" style="flex:1"></span>
      <button class="primary" id="ap-valider">Clore la séance</button>
    </div>`;

  let date = null, lieu = null;
  if (p.campagne) {
    date = saisieDate($("#ap-date", el), seance.data.date_jeu || camp?.data.date_actuelle || null);
    const refs = p.lieu_groupe ? { [p.lieu_groupe.id]: p.lieu_groupe } : {};
    lieu = selecteur($("#ap-lieu", el), { cibles: ["lieu"], valeur: p.lieu_groupe?.id || null, refs });
  }

  $("#ap-valider", el).onclick = async () => {
    const b = $("#ap-valider", el);
    const corps = {
      resume: $("#ap-resume", el).value,
      presents: $$("[data-present]:checked", el).map(i => +i.dataset.present),
      quetes: $$("[data-quete]", el).map(d => ({ id: +d.dataset.quete, statut: $("[data-statut]", d).value, etapes: $$("[data-etape]", d).map(i => i.checked) })),
      secrets: $$("[data-secret]", el).map(d => ({ id: +d.dataset.secret, qui: $$("[data-qui]:checked", d).map(i => +i.dataset.qui) })).filter(s => s.qui.length),
      morts: $$("[data-mort]:checked", el).map(i => +i.dataset.mort),
      rencontres: $$("[data-renc]", el).filter(s => s.value).map(s => ({ id: +s.dataset.renc, statut: s.value })),
      suivante: $("#ap-suivante", el).checked,
    };
    if (date) corps.date_jeu = date.valeur();
    if (lieu && lieu.valeur()) corps.lieu_groupe = lieu.valeur();
    b.disabled = true;
    try {
      const r = await api("POST", `/api/entites/${id}/apres-seance`, corps);
      window.dispatchEvent(new Event("cof-change"));
      toast(`Séance close${r.revelations ? ` · ${r.revelations} secret${r.revelations > 1 ? "s" : ""} appris` : ""}${r.suivante ? " · séance suivante créée" : ""}.`);
      location.hash = r.suivante ? `#/preparer/${r.suivante}` : `#/f/${id}`;
    } catch (e) { toast(e.message, true); b.disabled = false; }
  };
}
