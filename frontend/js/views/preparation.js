// Préparation d'une séance (#/preparer/<id>) : ce que la campagne a en cours, rassemblé pour le MJ
// (séance d'avant, quêtes et étapes restantes, secrets à semer, PNJ probables, rencontres prêtes).
// Le MJ coche ce qu'il garde, ajoute ses notes, et l'ensemble s'ajoute au champ « Préparation » de
// la séance, en Markdown, avec des [[liens]] vers les fiches.
import { $, $$, esc, api, toast } from "../ui.js";
import { vignette } from "../schema.js";

const lecture = () => !!window.cofAcces?.lecture_seule;
const lien = nom => `[[${String(nom).replace(/[[\]|]/g, " ").trim()}]]`;
const court = (t, n = 220) => { t = String(t || "").replace(/\s+/g, " ").trim(); return t.length > n ? t.slice(0, n - 1) + "…" : t; };

export async function mount(el, id) {
  el.innerHTML = `<p class="hint">Chargement…</p>`;
  const [p, seance] = await Promise.all([api("GET", `/api/entites/${id}/preparation`), api("GET", `/api/entites/${id}`)]);
  const t = (cle, texte, coche = true) => `<label class="prep-item"><input type="checkbox" data-cle="${esc(cle)}"${coche ? " checked" : ""}${lecture() ? " disabled" : ""}><span>${texte}</span></label>`;
  const fiche = f => `<a class="wl" href="#/f/${f.id}">${esc(f.nom)}</a>`;
  const section = (titre, corps, vide) => `<section class="panneau"><h3>${titre}</h3>${corps || `<p class="vide">${vide}</p>`}</section>`;
  // ce qui sera écrit pour chaque élément coché
  const md = {};

  const ensemble = [];
  if (p.precedente) {
    md.precedente = `**La dernière fois** (${lien(p.precedente.fiche.nom)}) : ${court(p.precedente.resume, 400) || "pas de résumé."}`;
    ensemble.push(t("precedente", `${fiche(p.precedente.fiche)} : ${esc(court(p.precedente.resume, 400)) || `<span class="hint">pas encore de résumé</span>`}`));
  }
  if (p.lieu_groupe || p.date) {
    md.situation = `**Où en est le groupe** : ${[p.lieu_groupe ? lien(p.lieu_groupe.nom) : "", p.date].filter(Boolean).join(", ")}`;
    ensemble.push(t("situation", `Le groupe est ${p.lieu_groupe ? "à " + fiche(p.lieu_groupe) : "quelque part"}${p.date ? `, le ${esc(p.date)}` : ""}.`));
  }
  if (p.pj.length) {
    md.pj = `**PJ** : ${p.pj.map(x => lien(x.fiche.nom) + (x.statut === "absent" ? " (absent)" : "") + (x.vie ? ` ${x.vie.pv}/${x.vie.pv_max} PV` : "")).join(", ")}`;
    ensemble.push(t("pj", `PJ : ${p.pj.map(x => `${fiche(x.fiche)}${x.statut === "absent" ? ` <span class="hint">(absent)</span>` : ""}${x.vie ? ` <span class="hint">${x.vie.pv}/${x.vie.pv_max} PV${x.vie.pm_max ? ` · ${x.vie.pm}/${x.vie.pm_max} PM` : ""}</span>` : ""}`).join(", ")}`));
  }
  if (p.fils_rouges) {
    md.fils = `**Fils rouges** : ${court(p.fils_rouges, 400)}`;
    ensemble.push(t("fils", `<span class="hint">Fils rouges :</span> ${esc(court(p.fils_rouges, 400))}`, false));
  }

  p.quetes.forEach((q, i) => {
    md["q" + i] = `- ${lien(q.fiche.nom)}${q.objectif ? " — " + court(q.objectif, 160) : ""}${q.etapes_restantes.length ? `\n  - reste : ${q.etapes_restantes.join(" ; ")}` : ""}`;
  });
  p.secrets.forEach((s, i) => {
    md["s" + i] = `- ${lien(s.fiche.nom)} (${[s.importance, s.statut].filter(Boolean).join(", ")})${s.indices ? ` — indices : ${court(s.indices, 200)}` : ""}${s.ignorent.length ? `\n  - ne le savent pas : ${s.ignorent.join(", ")}` : ""}`;
  });
  p.pnj.forEach((x, i) => { md["p" + i] = `- ${lien(x.fiche.nom)} — ${x.raison}${x.resume ? " : " + court(x.resume, 120) : ""}`; });
  p.rencontres.forEach((r, i) => { md["r" + i] = `- ${lien(r.fiche.nom)}${r.creatures ? " — " + r.creatures : ""}${r.lieu ? " (" + r.lieu + ")" : ""}`; });

  el.innerHTML = `
    <div class="entete"><h1>Préparer ${esc(seance.nom)}</h1><span class="sp"></span>
      <a href="#/f/${id}"><button>Voir la séance</button></a>
      ${p.campagne ? `<a href="#/f/${p.campagne.id}"><button class="ghost">📖 ${esc(p.campagne.nom)}</button></a>` : ""}</div>
    ${p.campagne ? "" : `<p class="alerte">Cette séance n'est rattachée à aucune campagne : rattachez-la pour que sa préparation rassemble les quêtes, secrets et rencontres de la campagne.</p>`}
    <p class="hint">Cochez ce que vous voulez garder : l'ensemble s'ajoute à la préparation de la séance (réservée au MJ), avec des liens vers les fiches.</p>
    <div class="prep">
      ${section("Où on en est", ensemble.join(""), "Rien à rappeler.")}
      ${section("Quêtes en cours", p.quetes.map((q, i) => t("q" + i, `${fiche(q.fiche)}${q.categorie ? ` <span class="badge">${esc(q.categorie)}</span>` : ""}
          ${q.objectif ? `<br><span class="hint">${esc(court(q.objectif, 200))}</span>` : ""}
          ${q.etapes_restantes.length ? `<br>Reste : ${q.etapes_restantes.map(esc).join(" · ")} <span class="hint">(${q.etapes_faites} faite${q.etapes_faites > 1 ? "s" : ""})</span>` : ""}
          ${[q.commanditaire && "commanditaire : " + q.commanditaire, q.lieu && "lieu : " + q.lieu].filter(Boolean).length ? `<br><span class="hint">${esc([q.commanditaire && "commanditaire : " + q.commanditaire, q.lieu && "lieu : " + q.lieu].filter(Boolean).join(" · "))}</span>` : ""}`)).join("")
        + (p.quetes_a_venir.length ? `<p class="hint">À venir : ${p.quetes_a_venir.map(fiche).join(", ")}</p>` : ""), "Aucune quête en cours dans cette campagne.")}
      ${section("Secrets à semer", p.secrets.map((s, i) => t("s" + i, `🔒 ${fiche(s.fiche)} <span class="badge">${esc([s.importance, s.statut].filter(Boolean).join(" · "))}</span>
          ${s.resume ? `<br><span class="hint">${esc(court(s.resume))}</span>` : ""}
          ${s.indices ? `<br>Indices : ${esc(court(s.indices, 300))}` : `<br><span class="hint">Aucun indice noté : comment les PJ peuvent-ils le découvrir ?</span>`}
          ${s.ignorent.length ? `<br><span class="hint">Ne le savent pas : ${esc(s.ignorent.join(", "))}</span>` : ""}`, s.importance !== "détail")).join(""), "Aucun secret en attente dans cette campagne.")}
      ${section("PNJ à avoir sous la main", p.pnj.map((x, i) => t("p" + i, `<span class="prep-pnj">${vignette(x.fiche, 26)} ${fiche(x.fiche)}${x.secret ? ` <span title="Des secrets du MJ">🔒</span>` : ""}</span> <span class="hint">${esc(x.raison)}${x.resume ? " — " + esc(court(x.resume, 120)) : ""}</span>`)).join(""), "Aucun PNJ lié aux quêtes en cours ni là où est le groupe.")}
      ${section("Rencontres prêtes", p.rencontres.map((r, i) => t("r" + i, `⚔ ${fiche(r.fiche)}${r.creatures ? ` <span class="hint">${esc(r.creatures)}</span>` : ""}${r.lieu || r.quete ? `<br><span class="hint">${esc([r.lieu, r.quete && "quête : " + r.quete].filter(Boolean).join(" · "))}</span>` : ""}`)).join(""), "Aucune rencontre préparée.")}
      <section class="panneau"><h3>Notes</h3>
        <textarea id="prep-notes" placeholder="Ouverture, ambiance, ce que vous voulez absolument placer…"${lecture() ? " disabled" : ""}></textarea>
      </section>
    </div>
    ${lecture() ? "" : `<div class="ligne prep-bas"><button class="primary" id="prep-ecrire">Ajouter à la préparation de la séance</button>
      <button class="ghost" id="prep-copier">Copier en Markdown</button>
      <span class="hint">${seance.data.preparation ? "La préparation existante est gardée : le texte s'ajoute à la suite." : ""}</span></div>`}`;

  const composer = () => {
    const pris = new Set($$("input[data-cle]:checked", el).map(i => i.dataset.cle));
    const bloc = (titre, pre) => {
      const l = Object.keys(md).filter(k => k.startsWith(pre) && /\d$/.test(k) && pris.has(k)).map(k => md[k]);
      return l.length ? `### ${titre}\n${l.join("\n")}` : "";
    };
    const tete = ["precedente", "situation", "pj", "fils"].filter(k => pris.has(k) && md[k]).map(k => md[k]).join("\n\n");
    const notes = $("#prep-notes", el).value.trim();
    const date = new Date().toLocaleDateString("fr-FR", { dateStyle: "long" });
    return [`## Préparation du ${date}`, tete, bloc("Quêtes", "q"), bloc("Secrets à semer", "s"), bloc("PNJ", "p"), bloc("Rencontres", "r"),
      notes ? `### Notes\n${notes}` : ""].filter(Boolean).join("\n\n");
  };
  if (lecture()) return;
  $("#prep-copier", el).onclick = async () => {
    try { await navigator.clipboard.writeText(composer()); toast("Préparation copiée en Markdown."); }
    catch { toast("Copie impossible depuis ce navigateur.", true); }
  };
  $("#prep-ecrire", el).onclick = async () => {
    const b = $("#prep-ecrire", el);
    b.disabled = true;
    try {
      const actuelle = (await api("GET", `/api/entites/${id}`)).data.preparation || "";
      const texte = (actuelle.trim() ? actuelle.trimEnd() + "\n\n" : "") + composer();
      await api("PATCH", `/api/entites/${id}`, { data: { preparation: texte } });
      toast("Préparation ajoutée à la séance.");
      location.hash = `#/f/${id}`;
    } catch (e) { toast(e.message, true); b.disabled = false; }
  };
}
