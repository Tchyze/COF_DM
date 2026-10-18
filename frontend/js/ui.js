// Petits outils partagés par toutes les pages.
export const $ = (s, r = document) => r.querySelector(s);
export const $$ = (s, r = document) => [...r.querySelectorAll(s)];
export const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

export function toast(msg, err = false) {
  const el = document.createElement("div");
  el.className = "toast" + (err ? " err" : "");
  el.textContent = msg;
  $("#toast").append(el);
  setTimeout(() => el.remove(), err ? 6000 : 2600);
}

// ---------------------------------------------------------------- univers de l'onglet
// Sur un serveur, chaque onglet garde l'univers où il travaille (sessionStorage) et le désigne à
// chaque requête : deux onglets peuvent tenir deux univers. Un nouvel onglet prend le dernier choisi
// (cookie du navigateur). Une page ouverte avec ?univers=<identifiant> l'adopte.
const CLE_UNIVERS = "cof-univers";
try {
  const demande = new URLSearchParams(location.search).get("univers");
  if (demande) sessionStorage.setItem(CLE_UNIVERS, demande);
} catch { /* stockage indisponible : le choix du navigateur vaut */ }

/** L'univers de cet onglet ("" : celui du navigateur). */
export function universOnglet() {
  try { return sessionStorage.getItem(CLE_UNIVERS) || ""; } catch { return ""; }
}

/** Fixe (id) ou oublie ("") l'univers de cet onglet. */
export function choisirUniversOnglet(id) {
  try { if (id) sessionStorage.setItem(CLE_UNIVERS, id); else sessionStorage.removeItem(CLE_UNIVERS); } catch { /* idem */ }
}

/** Adresse d'une ressource lue sans api() (image, WebSocket, téléchargement), pour l'univers de l'onglet. */
export function lien(chemin) {
  const u = universOnglet();
  return u ? chemin + (chemin.includes("?") ? "&" : "?") + "univers=" + encodeURIComponent(u) : chemin;
}

/** En-têtes qui désignent l'univers de l'onglet, pour un fetch() fait à la main. */
export function enTetesUnivers(h = {}) {
  const u = universOnglet();
  if (u) h["X-COF-Univers"] = u;
  return h;
}

/** Appel à l'API. Une erreur porte le message du serveur, le code (.status) et le corps (.data). */
export async function api(method, path, body) {
  const opt = { method, headers: enTetesUnivers() };
  if (method !== "GET") opt.headers["X-COF-DM"] = "1";
  if (body instanceof FormData || body instanceof Blob) opt.body = body;
  else if (body !== undefined) { opt.headers["Content-Type"] = "application/json"; opt.body = JSON.stringify(body); }
  let r;
  try { r = await fetch(path, opt); }
  catch { throw new Error("COF DM ne répond plus : l'application est-elle encore ouverte ?"); }
  const data = await r.json().catch(() => ({}));
  if (r.status === 401) window.dispatchEvent(new Event("cof-session")); // session finie : retour à la connexion
  if (r.status === 404 && universOnglet() && String(data.detail || "").startsWith("Univers inconnu")) {
    choisirUniversOnglet(""); // l'univers de l'onglet n'est plus servi : retour au choix du navigateur
    location.reload();
  }
  if (!r.ok) {
    const e = new Error(data.detail || `Erreur ${r.status}`);
    e.status = r.status; e.data = data;
    throw e;
  }
  return data;
}

/** Bouton à double clic : le premier arme, le second confirme. */
export function armer(btn, labelConfirm, action) {
  btn.addEventListener("click", () => {
    if (btn.dataset.armed) {
      clearTimeout(+btn.dataset.timer);
      delete btn.dataset.armed;
      btn.classList.remove("armed");
      btn.textContent = btn.dataset.label;
      action();
      return;
    }
    btn.dataset.label = btn.textContent;
    btn.dataset.armed = "1";
    btn.classList.add("armed");
    btn.textContent = labelConfirm;
    btn.dataset.timer = setTimeout(() => {
      delete btn.dataset.armed; btn.classList.remove("armed"); btn.textContent = btn.dataset.label;
    }, 4000);
  });
}

export const dateHeure = s => new Date(s * 1000).toLocaleString("fr-FR", { dateStyle: "short", timeStyle: "short" });
export const dateReelle = s => s ? new Date(s + "T12:00:00").toLocaleDateString("fr-FR", { dateStyle: "long" }) : "";

/** Délai : n'appelle f qu'après une pause de ms. */
export function delai(f, ms = 160) {
  let t = null;
  return (...a) => { clearTimeout(t); t = setTimeout(() => f(...a), ms); };
}

/** Une boîte de dialogue simple : titre et contenu HTML ; un clic à côté ou Échap la ferme. */
export function dialogue(titre, corps) {
  const voile = document.createElement("div");
  voile.className = "voile";
  voile.innerHTML = `<div class="boite" role="dialog" aria-label="${esc(titre)}"><div class="boite-corps"><h3>${esc(titre)}</h3>${corps}</div></div>`;
  document.body.append(voile);
  const fermer = () => voile.remove();
  voile.onclick = e => { if (e.target === voile) fermer(); };
  voile.onkeydown = e => { if (e.key === "Escape") fermer(); };
  return { voile, fermer };
}
