// Organigramme d'une faction : dirigeants, membres et sous-factions, chacun sous celui à qui il
// répond (supérieur réglé ici, sinon « Responsable » de sa fiche). Les PNJ sans faction qui
// répondent à un membre y figurent comme rattachés, les infiltrés aussi (responsable secret).
// Glisser une carte sur une autre change son supérieur ; ✎ modifie rang, supérieur et secret.
import { $, $$, esc, api, toast, armer } from "../ui.js";
import { vignette } from "../schema.js";
import { selecteur } from "../composants.js";

export async function organigramme(box, faction, apresChangement) {
  const arbre = await api("GET", `/api/entites/${faction.id}/organigramme`);
  const postes = [];
  const parcourir = (l, niveau) => l.forEach(p => { postes.push(p); parcourir(p.enfants, niveau + 1); });
  parcourir(arbre, 0);
  const nb = postes.filter(p => !p.rattache).length, rattaches = postes.length - nb;

  const carte = p => `
    <div class="poste${p.statut === "mort" ? " mort" : ""}${p.secret ? " secret" : ""}${p.dirigeant ? " chef" : ""}${p.rattache ? " rattache" : ""}" data-id="${p.fiche.id}" data-rel="${p.relation || ""}"
      ${p.relation ? `draggable="true"` : ""} title="${p.relation ? "Glisser sur un autre membre pour changer de supérieur" : p.rattache ? "Pas membre : placé ici par le champ « Responsable » de sa fiche" : ""}">
      ${vignette(p.fiche, 36)}
      <a class="nom" href="#/f/${p.fiche.id}">${esc(p.fiche.nom)}</a>
      ${p.rang ? `<span class="rang">${p.dirigeant ? "♛ " : ""}${esc(p.rang)}</span>` : p.rattache ? `<span class="rang">hors faction</span>` : ""}
      ${p.repond ? `<span class="rang">répond à <a href="#/f/${p.repond.id}">${esc(p.repond.nom)}</a></span>` : ""}
      ${p.statut && p.statut !== "vivant" ? `<span class="badge statut-${esc(p.statut)}">${esc(p.statut)}</span>` : ""}
      ${p.secret ? `<span class="cadenas" title="Appartenance secrète (MJ)">🔒</span>` : ""}
      ${p.relation ? `<button class="ghost petit modif" data-rel="${p.relation}" aria-label="Modifier le poste">✎</button>` : ""}
    </div>`;
  // Ceux qui ont eux-mêmes des subordonnés se rangent côte à côte, chacun avec son équipe. Les
  // autres, s'ils sont nombreux, forment un seul lot en grille : ni une ligne sans fin, ni une pile
  // sans fin, et aucun trait entre eux qui ferait croire que l'un commande l'autre.
  const HAUTEUR = 3; // rangées d'un lot, à peu près
  const branche = (l, niveau = 0) => {
    if (!l.length) return "";
    const chefs = l.filter(p => p.enfants.length), seuls = l.filter(p => !p.enfants.length);
    const items = chefs.map(p => `<li>${carte(p)}${branche(p.enfants, niveau + 1)}</li>`);
    if (seuls.length <= (chefs.length ? 2 : 4)) items.push(...seuls.map(p => `<li>${carte(p)}</li>`));
    else {
      // sous le chef, on peut s'étaler ; plus bas, deux colonnes au plus pour ne pas écarter les branches
      const colonnes = Math.min(Math.ceil(seuls.length / HAUTEUR), niveau <= 1 ? 5 : 2);
      items.push(`<li><div class="lot" style="--colonnes:${colonnes}">${seuls.map(p => `<div class="case">${carte(p)}</div>`).join("")}</div></li>`);
    }
    return `<ul>${items.join("")}</ul>`;
  };

  box.innerHTML = `
    <h3>Organigramme <span class="hint" style="text-transform:none;letter-spacing:0">${nb ? `${nb} membre${nb > 1 ? "s" : ""}` : ""}${rattaches ? ` · ${rattaches} rattaché${rattaches > 1 ? "s" : ""}` : ""}</span><span class="sp"></span>
      <button class="petit" id="org-ajout">＋ Membre</button></h3>
    <form id="org-form" class="org-form" hidden>
      <div class="champ"><span class="aide">Membre (PNJ, PJ ou faction)</span><div id="org-qui"></div></div>
      <label class="champ"><span>Rang</span><input id="org-rang" placeholder="lieutenant, espion, recrue…" autocomplete="off"></label>
      <label class="champ"><span>Supérieur</span><select id="org-sup"><option value="">${arbre.length ? "— son responsable, sinon le dirigeant —" : "— aucun —"}</option>${postes.map(p => `<option value="${p.fiche.id}">${esc(p.fiche.nom)}</option>`).join("")}</select></label>
      <label class="champ" style="flex-direction:row;align-items:center;gap:8px;align-self:end;padding-bottom:8px"><input type="checkbox" id="org-secret"><span>Appartenance secrète</span></label>
      <div class="ligne pleine"><button class="primary petit" type="submit">Ajouter</button><button class="ghost petit" type="button" id="org-annuler">Annuler</button></div>
    </form>
    ${postes.length ? `<div class="organi" id="organi">${branche(arbre)}</div>
      <div class="org-racine" id="org-racine">Déposer ici : plus de supérieur réglé dans la faction (retour au responsable de sa fiche, sinon au dirigeant)</div>
      <p class="hint">Chacun se place sous le supérieur réglé ici, sinon sous le « Responsable » de sa fiche, sinon sous le dirigeant.</p>`
      : `<p class="vide">Aucun membre. Indiquez un dirigeant dans la fiche, puis ajoutez les membres avec leur rang et leur supérieur.</p>`}`;

  const zone = $("#organi", box);
  if (zone) requestAnimationFrame(() => { zone.scrollLeft = (zone.scrollWidth - zone.clientWidth) / 2; });

  // ajout d'un membre
  const form = $("#org-form", box);
  let qui = null;
  $("#org-ajout", box).onclick = () => {
    form.hidden = !form.hidden;
    if (!form.hidden && !qui) qui = selecteur($("#org-qui", box), { cibles: ["pnj", "pj", "faction"], multiple: false });
    if (!form.hidden) $("#org-qui input", box)?.focus();
  };
  $("#org-annuler", box).onclick = () => { form.hidden = true; };
  form.onsubmit = async e => {
    e.preventDefault();
    const membre = qui?.valeur();
    if (!membre) { toast("Choisissez le membre.", true); return; }
    const sup = +$("#org-sup", box).value || 0;
    try {
      await api("POST", "/api/relations", {
        a: membre, b: faction.id, type: "membre", rang: $("#org-rang", box).value,
        secrete: $("#org-secret", box).checked, ...(sup ? { superieur: sup } : {}),
      });
      apresChangement();
    } catch (err) { toast(err.message, true); }
  };

  // modification d'un poste
  box.addEventListener("click", e => {
    const b = e.target.closest("button.modif");
    if (!b) return;
    const p = postes.find(x => x.relation === +b.dataset.rel);
    const voile = document.createElement("div");
    voile.className = "voile";
    voile.innerHTML = `<div class="boite panneau org-edit" role="dialog" aria-label="Modifier le poste">
      <h3>${esc(p.fiche.nom)}</h3>
      <label class="champ"><span>Rang</span><input id="ed-rang" value="${esc(p.rang === "dirigeant" && p.dirigeant ? "" : p.rang)}"></label>
      <label class="champ"><span>Supérieur dans cette faction</span><select id="ed-sup"><option value="0">— son responsable, sinon le dirigeant —</option>
        ${postes.filter(x => x.fiche.id !== p.fiche.id).map(x => `<option value="${x.fiche.id}" ${x.fiche.id === p.superieur ? "selected" : ""}>${esc(x.fiche.nom)}</option>`).join("")}</select></label>
      <label class="champ" style="flex-direction:row;gap:8px;align-items:center"><input type="checkbox" id="ed-secret" ${p.secret ? "checked" : ""}><span>Appartenance secrète (connue du MJ seulement)</span></label>
      <div class="ligne"><button class="primary petit" id="ed-ok">Enregistrer</button><button class="ghost petit" id="ed-annuler">Annuler</button><span style="flex:1"></span><button class="danger petit" id="ed-retirer">Retirer de la faction</button></div>
    </div>`;
    document.body.append(voile);
    const ed = voile;
    const fermer = () => { voile.remove(); document.removeEventListener("keydown", echap); };
    const echap = ev => { if (ev.key === "Escape") fermer(); };
    document.addEventListener("keydown", echap);
    voile.onclick = ev => { if (ev.target === voile) fermer(); };
    $("#ed-rang", ed).focus();
    $("#ed-annuler", ed).onclick = fermer;
    $("#ed-ok", ed).onclick = async () => {
      try {
        await api("PATCH", "/api/relations/" + p.relation, { rang: $("#ed-rang", ed).value, superieur: +$("#ed-sup", ed).value, secrete: $("#ed-secret", ed).checked });
        fermer();
        apresChangement();
      } catch (err) { toast(err.message, true); }
    };
    armer($("#ed-retirer", ed), "Confirmer", async () => {
      try { await api("DELETE", "/api/relations/" + p.relation); fermer(); apresChangement(); }
      catch (err) { toast(err.message, true); }
    });
  });

  // glisser-déposer : changer de supérieur
  let tire = null;
  box.addEventListener("dragstart", e => {
    const c = e.target.closest(".poste[data-rel]");
    if (!c || !c.dataset.rel) return;
    tire = c;
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", c.dataset.id);
    box.classList.add("glisse");
  });
  box.addEventListener("dragend", () => { tire = null; box.classList.remove("glisse"); $$(".cible", box).forEach(x => x.classList.remove("cible")); });
  box.addEventListener("dragover", e => {
    const c = e.target.closest(".poste, #org-racine");
    if (!tire || !c || (c.id !== "org-racine" && tire.closest(".case, li").contains(c))) return; // pas sur soi ni sous soi
    e.preventDefault();
    $$(".cible", box).forEach(x => x.classList.remove("cible"));
    c.classList.add("cible");
  });
  box.addEventListener("drop", async e => {
    const c = e.target.closest(".poste, #org-racine");
    if (!tire || !c) return;
    e.preventDefault();
    const sup = c.id === "org-racine" ? 0 : +c.dataset.id;
    try {
      await api("PATCH", "/api/relations/" + tire.dataset.rel, { superieur: sup });
      apresChangement();
    } catch (err) { toast(err.message, true); }
  });
}
