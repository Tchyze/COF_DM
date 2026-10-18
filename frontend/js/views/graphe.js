// Graphe des liens entre fiches (relations, champs, mentions), disposé par simulation de forces.
// Les types de fiches se filtrent dans la légende ; les fiches sans lien et les mentions sont
// cachées par défaut. Les liens sont élastiques : la disposition se pose seule, et une fiche tirée
// entraîne ses voisines avant que tout se remette en place. Les noms gardent leur taille à
// l'écran ; ceux qui se chevaucheraient ne s'affichent qu'au survol ou en s'approchant.
import { $, $$, esc, api, lien } from "../ui.js";
import { types, typeDe } from "../schema.js";
import { selecteur } from "../composants.js";

const PREFS = "cof-graphe-2";
const lirePrefs = () => {
  const defaut = { caches: [], isoles: false, mentions: false };
  try {
    const ancien = JSON.parse(localStorage.getItem("cof-graphe") || "{}"); // garde les types cachés
    return { ...defaut, caches: ancien.caches || [], ...JSON.parse(localStorage.getItem(PREFS) || "{}") };
  } catch { return defaut; }
};

export async function mount(el, centre) {
  centre = +centre || 0;
  const prefs = lirePrefs();
  const garder = () => { try { localStorage.setItem(PREFS, JSON.stringify(prefs)); } catch { /* stockage indisponible */ } };
  el.innerHTML = `
    <div class="entete"><h1>Graphe</h1><span class="sp"></span>
      <div id="choix" style="width:260px"></div>
      <select id="prof" aria-label="Profondeur" style="width:auto">
        <option value="1">1 niveau</option><option value="2" selected>2 niveaux</option><option value="3">3 niveaux</option>
      </select>
      <label class="ligne" style="gap:6px;font-size:.85rem;color:var(--muted)"><input type="checkbox" id="mentions"> mentions</label>
      <label class="ligne" style="gap:6px;font-size:.85rem;color:var(--muted)" title="Fiches qui n'ont aucun lien avec les autres"><input type="checkbox" id="isoles"> fiches sans lien</label>
      <button class="petit ghost" id="recadrer" title="Tout faire tenir dans la vue">⤢ Recadrer</button>
    </div>
    <div class="graphe-zone" id="zone"><svg id="svg"></svg></div>
    <div class="legende-graphe" id="legende"></div>
    <p class="hint" style="margin-top:8px">Clic : ouvrir la fiche · double clic : centrer le graphe sur elle · survol : ses liens et son nom · glisser un point pour l'écarter, le fond pour se déplacer · molette : zoom (les noms cachés apparaissent en s'approchant) · légende : afficher ou cacher un type de fiche.</p>`;
  let infoCentre = null;
  if (centre) infoCentre = await api("GET", "/api/entites/" + centre).catch(() => null);
  const sel = selecteur($("#choix", el), {
    cibles: [], multiple: false, valeur: centre || null, refs: infoCentre ? { [centre]: infoCentre } : {},
    auChangement: () => { const v = sel.valeur(); location.hash = v ? "#/graphe/" + v : "#/graphe"; },
  });
  const prof = $("#prof", el), mentions = $("#mentions", el), isoles = $("#isoles", el);
  mentions.checked = prefs.mentions;
  isoles.checked = prefs.isoles;
  let g = null, vue = null;

  const charger = async () => {
    g = await api("GET", `/api/graphe?profondeur=${prof.value}${centre ? "&id=" + centre : ""}`);
    dessiner();
  };
  // les fiches et les liens retenus par les filtres
  function dessiner() {
    const caches = new Set(prefs.caches);
    let noeuds = g.noeuds.filter(n => !caches.has(n.type) || n.id === centre);
    const ici = new Set(noeuds.map(n => n.id));
    const aretes = fusionner(g.aretes.filter(a => ici.has(a.a) && ici.has(a.b) && (prefs.mentions || a.genre !== "mention")));
    if (!prefs.isoles) {
      const lies = new Set(aretes.flatMap(a => [a.a, a.b]));
      noeuds = noeuds.filter(n => lies.has(n.id) || n.id === centre);
    }
    // légende : chaque type présent, avec son nombre de fiches ; un clic le cache ou le montre
    const compte = {};
    g.noeuds.forEach(n => { compte[n.type] = (compte[n.type] || 0) + 1; });
    $("#legende", el).innerHTML = types().filter(t => compte[t.cle]).map(t =>
      `<button type="button" class="badge type" data-type="${esc(t.cle)}" aria-pressed="${!caches.has(t.cle)}" title="${caches.has(t.cle) ? "Afficher" : "Cacher"} : ${esc(t.pluriel || t.nom)}">${esc(t.icone || "")} ${esc(t.nom)} <span class="n">${compte[t.cle]}</span></button>`).join("");
    vue?.arreter();
    vue = disposer($("#svg", el), noeuds.map(n => ({ ...n })), aretes, centre);
  }
  $("#legende", el).onclick = e => {
    const b = e.target.closest("[data-type]");
    if (!b) return;
    const t = b.dataset.type, caches = new Set(prefs.caches);
    if (caches.has(t)) caches.delete(t); else caches.add(t);
    prefs.caches = [...caches]; garder(); dessiner();
  };
  prof.onchange = charger;
  mentions.onchange = () => { prefs.mentions = mentions.checked; garder(); dessiner(); };
  isoles.onchange = () => { prefs.isoles = isoles.checked; garder(); dessiner(); };
  $("#recadrer", el).onclick = () => vue?.recadrer();
  const auRedimensionnement = () => vue?.recadrer();
  window.addEventListener("resize", auRedimensionnement);
  await charger();
  return () => { vue?.arreter(); window.removeEventListener("resize", auRedimensionnement); };
}

const NS = "http://www.w3.org/2000/svg";
const raccourci = (s, n = 26) => s.length > n ? s.slice(0, n - 1) + "…" : s;
const FORCE = { relation: 3, champ: 2, mention: 1 };

// fusionner : un seul trait par paire de fiches, du genre le plus fort, avec tous ses libellés
function fusionner(aretes) {
  const parPaire = new Map();
  for (const a of aretes) {
    const k = a.a < a.b ? a.a + "-" + a.b : a.b + "-" + a.a;
    const x = parPaire.get(k);
    if (!x) { parPaire.set(k, { ...a, libelles: [a.libelle] }); continue; }
    if (!x.libelles.includes(a.libelle)) x.libelles.push(a.libelle);
    if ((FORCE[a.genre] || 0) > (FORCE[x.genre] || 0)) Object.assign(x, { genre: a.genre, libelle: a.libelle });
  }
  return [...parPaire.values()];
}

// disposition par forces, calculée en une fois ; renvoie de quoi recadrer la vue
function disposer(svg, noeuds, aretes, centre) {
  svg.innerHTML = "";
  svg.classList.remove("survol");
  const boite = svg.getBoundingClientRect();
  const W = boite.width || 900, H = boite.height || 600;
  if (!noeuds.length) {
    svg.innerHTML = `<text x="${W / 2}" y="${H / 2}" text-anchor="middle" fill="currentColor" opacity=".6">Aucune fiche à afficher avec ces filtres.</text>`;
    return { recadrer() {}, arreter() {} };
  }
  const parId = new Map(), degre = new Map(), voisins = new Map();
  aretes.forEach(a => {
    degre.set(a.a, (degre.get(a.a) || 0) + 1); degre.set(a.b, (degre.get(a.b) || 0) + 1);
    if (!voisins.has(a.a)) voisins.set(a.a, new Set());
    if (!voisins.has(a.b)) voisins.set(a.b, new Set());
    voisins.get(a.a).add(a.b); voisins.get(a.b).add(a.a);
  });
  // largeur des noms (12 px), pour écarter les fiches assez pour qu'ils tiennent
  const mesure = document.createElementNS(NS, "text");
  mesure.setAttribute("class", "mesure");
  svg.append(mesure);
  noeuds.forEach((n, i) => {
    const ang = i * 2.399963, r = 50 * Math.sqrt(i); // spirale de Vogel : départ bien réparti
    mesure.textContent = raccourci(n.nom);
    const largeur = mesure.getComputedTextLength() || n.nom.length * 6.5;
    Object.assign(n, { x: r * Math.cos(ang), y: r * Math.sin(ang), vx: 0, vy: 0, largeur,
      r: n.id === centre ? 22 : 9 + Math.min(11, (degre.get(n.id) || 0) * 1.3) });
    if (n.id === centre) { n.x = 0; n.y = 0; }
    parId.set(n.id, n);
  });
  mesure.remove();
  const liens = aretes.map(a => ({ ...a, s: parId.get(a.a), t: parId.get(a.b) })).filter(l => l.s && l.t);

  // Forces, comme un ressort : répulsion entre fiches, liens élastiques, légère gravité vers le
  // centre, et collision (le cercle et le nom d'une fiche gardent leur place). La simulation tourne
  // en continu tant qu'elle n'est pas posée ; tirer une fiche la relance, et ses voisines suivent.
  const L = 130, k = L;
  const encombre = n => Math.max(n.r + 8, n.largeur / 2 + 6); // rayon occupé, nom compris
  const fixe = n => n.fx !== undefined;
  const forces = alpha => {
    for (let i = 0; i < noeuds.length; i++) {
      const a = noeuds[i];
      for (let j = i + 1; j < noeuds.length; j++) {
        const b = noeuds[j];
        let dx = b.x - a.x, dy = b.y - a.y;
        const d2 = dx * dx + dy * dy || 0.01;
        if (d2 > 360000) continue;
        const d = Math.sqrt(d2);
        dx /= d; dy /= d;
        let f = (k * k) / d2 * alpha;
        const min = encombre(a) + encombre(b); // collision : écarte franchement ce qui se recouvre
        if (d < min) f += (min - d) * 0.5;
        a.vx -= dx * f; a.vy -= dy * f; b.vx += dx * f; b.vy += dy * f;
      }
    }
    for (const l of liens) {
      const dx = l.t.x - l.s.x, dy = l.t.y - l.s.y;
      const d = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const f = (d - (l.genre === "mention" ? L * 1.3 : L)) * 0.06 * alpha;
      l.s.vx += dx / d * f; l.s.vy += dy / d * f; l.t.vx -= dx / d * f; l.t.vy -= dy / d * f;
    }
    for (const n of noeuds) {
      if (fixe(n)) { n.x = n.fx; n.y = n.fy; n.vx = n.vy = 0; continue; }
      if (n.id === centre) { n.x = n.y = n.vx = n.vy = 0; continue; }
      n.vx -= n.x * 0.01 * alpha; n.vy -= n.y * 0.01 * alpha;
      n.vx *= 0.6; n.vy *= 0.6; // frottement : le mouvement s'amortit, comme un élastique qui se pose
      n.x += Math.max(-40, Math.min(40, n.vx)); n.y += Math.max(-40, Math.min(40, n.vy));
    }
  };
  // départ : la disposition est presque posée avant le premier affichage
  for (let tour = 0; tour < 300; tour++) forces(Math.max(0.15, 1 - tour / 250));

  // priorité des noms quand ils se chevauchent : le centre, ses voisins, puis les plus liées
  const proches = voisins.get(centre) || new Set();
  const priorite = n => (n.id === centre ? 1e6 : 0) + (proches.has(n.id) ? 1e4 : 0) + (degre.get(n.id) || 0) * 10 + n.r;
  const ordre = [...noeuds].sort((a, b) => priorite(b) - priorite(a));

  const racine = document.createElementNS(NS, "g");
  svg.append(racine);
  const gl = document.createElementNS(NS, "g"), gn = document.createElementNS(NS, "g");
  racine.append(gl, gn);
  const traits = liens.map(l => {
    const line = document.createElementNS(NS, "line");
    line.setAttribute("class", "arete " + l.genre);
    const titre = document.createElementNS(NS, "title");
    titre.textContent = `${l.s.nom} — ${l.libelles.join(", ")} — ${l.t.nom}`;
    line.append(titre);
    gl.append(line);
    return line;
  });
  const couleur = type => getComputedStyle(document.documentElement).getPropertyValue("--t-" + type).trim() || "#999";
  const noms = [];
  const groupes = noeuds.map(n => {
    const g = document.createElementNS(NS, "g");
    g.setAttribute("class", "noeud" + (n.id === centre ? " centre" : ""));
    const c = couleur(n.type);
    const titre = document.createElementNS(NS, "title");
    titre.textContent = `${n.nom} (${typeDe(n.type)?.nom || n.type})`;
    g.append(titre);
    let corps;
    if (n.image) {
      const clip = document.createElementNS(NS, "clipPath");
      clip.id = "clip" + n.id;
      const cc = document.createElementNS(NS, "circle");
      cc.setAttribute("r", n.r);
      clip.append(cc);
      g.append(clip);
      corps = document.createElementNS(NS, "image");
      corps.setAttribute("href", lien("/medias/" + n.image));
      corps.setAttribute("x", -n.r); corps.setAttribute("y", -n.r);
      corps.setAttribute("width", 2 * n.r); corps.setAttribute("height", 2 * n.r);
      corps.setAttribute("clip-path", `url(#clip${n.id})`);
      corps.setAttribute("preserveAspectRatio", "xMidYMid slice");
    }
    const cercle = document.createElementNS(NS, "circle");
    cercle.setAttribute("r", n.r);
    cercle.setAttribute("fill", n.image ? "none" : c + "40");
    cercle.setAttribute("stroke", c);
    cercle.setAttribute("stroke-width", n.id === centre ? 3 : 2);
    if (corps) g.append(corps);
    g.append(cercle);
    if (!n.image) {
      const ic = document.createElementNS(NS, "text");
      ic.setAttribute("class", "icone");
      ic.setAttribute("text-anchor", "middle"); ic.setAttribute("dy", "0.35em");
      ic.setAttribute("style", `font-size:${n.r}px`);
      ic.textContent = typeDe(n.type)?.icone || "";
      g.append(ic);
    }
    const t = document.createElementNS(NS, "text");
    t.setAttribute("class", "nom");
    t.setAttribute("text-anchor", "middle");
    t.textContent = raccourci(n.nom);
    g.append(t);
    noms.push(t);
    g.setAttribute("transform", `translate(${n.x},${n.y})`);
    gn.append(g);
    return g;
  });
  const peindreLiens = () => liens.forEach((l, i) => {
    traits[i].setAttribute("x1", l.s.x); traits[i].setAttribute("y1", l.s.y);
    traits[i].setAttribute("x2", l.t.x); traits[i].setAttribute("y2", l.t.y);
  });
  peindreLiens();

  // vue : zoom et déplacement. Les noms gardent leur taille à l'écran ; à chaque changement de vue,
  // on n'affiche que ceux qui ne se chevauchent pas, par ordre de priorité.
  let zoom = 1, ox = W / 2, oy = H / 2, trame = 0;
  const index = new Map(noeuds.map((n, i) => [n, i]));
  const placerNoms = () => {
    const pris = [];
    const libre = b => pris.every(p => b.r <= p.l || b.l >= p.r || b.b <= p.t || b.t >= p.b);
    // les cercles occupent aussi la place
    noeuds.forEach(n => pris.push({ l: n.x * zoom + ox - n.r * zoom, r: n.x * zoom + ox + n.r * zoom, t: n.y * zoom + oy - n.r * zoom, b: n.y * zoom + oy + n.r * zoom }));
    for (const n of ordre) {
      const i = index.get(n), t = noms[i];
      t.setAttribute("transform", `scale(${1 / zoom})`);
      t.setAttribute("y", n.r * zoom + 13);
      const sx = n.x * zoom + ox, sy = n.y * zoom + oy + n.r * zoom + 2;
      const b = { l: sx - n.largeur / 2 - 2, r: sx + n.largeur / 2 + 2, t: sy, b: sy + 15 };
      const ok = n.id === centre || libre(b);
      groupes[i].classList.toggle("nom-cache", !ok);
      if (ok) pris.push(b);
    }
  };
  const appliquer = () => {
    racine.setAttribute("transform", `translate(${ox},${oy}) scale(${zoom})`);
    cancelAnimationFrame(trame);
    trame = requestAnimationFrame(placerNoms);
  };
  const recadrer = () => {
    const b = svg.getBoundingClientRect(), w = b.width || W, h = b.height || H;
    const x0 = Math.min(...noeuds.map(n => n.x - Math.max(n.r, n.largeur / 2))) - 20, x1 = Math.max(...noeuds.map(n => n.x + Math.max(n.r, n.largeur / 2))) + 20;
    const y0 = Math.min(...noeuds.map(n => n.y - n.r)) - 20, y1 = Math.max(...noeuds.map(n => n.y + n.r)) + 40;
    zoom = Math.max(0.15, Math.min(1.5, Math.min(w / (x1 - x0), h / (y1 - y0))));
    ox = w / 2 - zoom * (x0 + x1) / 2; oy = h / 2 - zoom * (y0 + y1) / 2;
    appliquer();
  };
  recadrer();
  svg.onwheel = e => {
    e.preventDefault();
    const r = svg.getBoundingClientRect();
    const mx = e.clientX - r.left, my = e.clientY - r.top;
    const z = Math.max(0.15, Math.min(4, zoom * (e.deltaY < 0 ? 1.15 : 0.87)));
    ox = mx - (mx - ox) * z / zoom; oy = my - (my - oy) * z / zoom; zoom = z;
    appliquer();
  };

  // survol : la fiche, ses voisins et ses liens ressortent, avec leurs noms
  const indexe = new Map(groupes.map((g, i) => [g, noeuds[i]]));
  svg.onpointerover = e => {
    const g = e.target.closest?.(".noeud");
    if (!g || prise) return;
    const n = indexe.get(g), v = voisins.get(n.id) || new Set();
    svg.classList.add("survol");
    groupes.forEach((x, i) => x.classList.toggle("voisin", noeuds[i] === n || v.has(noeuds[i].id)));
    traits.forEach((x, i) => x.classList.toggle("voisin", liens[i].s === n || liens[i].t === n));
  };
  svg.onpointerout = e => {
    if (e.target.closest?.(".noeud") && !e.relatedTarget?.closest?.(".noeud")) svg.classList.remove("survol");
  };

  // animation : la simulation avance tant que son énergie (alpha) n'est pas retombée
  let alpha = 0.3, anim = 0, image = 0;
  const peindreFiches = () => noeuds.forEach((n, i) => groupes[i].setAttribute("transform", `translate(${n.x},${n.y})`));
  const boucle = () => {
    forces(alpha);
    peindreFiches(); peindreLiens();
    if (++image % 4 === 0) placerNoms(); // les noms suivent, sans recalcul à chaque image
    alpha = Math.max(prise?.n ? 0.3 : 0, alpha * 0.985);
    if (alpha > 0.01 || prise?.n) anim = requestAnimationFrame(boucle);
    else { anim = 0; placerNoms(); }
  };
  const relancer = (a = 0.5) => { alpha = Math.max(alpha, a); if (!anim) anim = requestAnimationFrame(boucle); };
  relancer(0.3);

  // glisser une fiche : elle suit le pointeur, ses voisines suivent au bout de leur lien ; lâchée,
  // tout se remet en place. Glisser le fond déplace la vue.
  let prise = null, deplace = false;
  const point = e => { const r = svg.getBoundingClientRect(); return { x: (e.clientX - r.left - ox) / zoom, y: (e.clientY - r.top - oy) / zoom }; };
  svg.onpointerdown = e => {
    const g = e.target.closest(".noeud");
    deplace = false;
    prise = g ? { n: indexe.get(g), g } : { fond: true, x: e.clientX - ox, y: e.clientY - oy };
    svg.setPointerCapture(e.pointerId);
  };
  svg.onpointermove = e => {
    if (!prise) return;
    if (prise.fond) { deplace = true; ox = e.clientX - prise.x; oy = e.clientY - prise.y; appliquer(); return; }
    const p = point(e);
    if (!deplace && Math.hypot(p.x - prise.n.x, p.y - prise.n.y) < 3 / zoom) return; // un clic n'est pas un glisser
    deplace = true;
    prise.n.fx = p.x; prise.n.fy = p.y;
    relancer(0.3);
  };
  svg.onpointerup = () => {
    if (prise?.n && deplace) { delete prise.n.fx; delete prise.n.fy; relancer(0.3); } // l'élastique la ramène
    if (prise?.n && !deplace) {
      const n = prise.n;
      if (svg._dernier === n.id && Date.now() - svg._quand < 350) { svg._dernier = null; location.hash = "#/graphe/" + n.id; }
      else { svg._dernier = n.id; svg._quand = Date.now(); setTimeout(() => { if (svg._dernier === n.id && Date.now() - svg._quand >= 340) location.hash = "#/f/" + n.id; }, 360); }
    }
    prise = null;
  };
  return { recadrer, arreter: () => cancelAnimationFrame(anim) };
}
