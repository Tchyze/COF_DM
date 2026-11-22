// « Notifications sur cet appareil » (Mon compte, sur un serveur) : inscrire ce téléphone, cette
// tablette ou ce navigateur pour qu'il sonne même écran éteint (Web Push), choisir ce qu'on reçoit,
// voir et retirer ses appareils, envoyer un essai. Le serveur n'envoie qu'un texte neutre ; le
// détail reste dans la cloche.
import { $, $$, esc, api, toast, dateHeure } from "./ui.js";

const CLE_ICI = "cof-appareil";
const lireIci = () => { try { return localStorage.getItem(CLE_ICI) || ""; } catch { return ""; } };
const garderIci = id => { try { id ? localStorage.setItem(CLE_ICI, id) : localStorage.removeItem(CLE_ICI); } catch { /* stockage indisponible */ } };

const LIBELLES = {
  rappel: "Rappel de séance (la veille à 19 h)",
  deplacee: "Séance déplacée ou rétablie",
  annulee: "Séance annulée",
  relance: "Le MJ attend votre réponse",
  reponse: "Réponses des joueurs aux séances",
  message: "Message du MJ",
};
// ce que chaque rôle peut recevoir
const POUR_ROLE = { mj: ["reponse"], joueur: ["rappel", "deplacee", "annulee", "relance", "message"] };

const iOS = () => /iPad|iPhone/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
const installee = () => matchMedia("(display-mode: fullscreen), (display-mode: standalone)").matches || navigator.standalone === true;
const possibleIci = () => window.isSecureContext && "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;

/** « Android · Chrome », « iPhone · Safari »… : de quoi reconnaître l'appareil dans la liste. */
function nomAppareil() {
  const ua = navigator.userAgent;
  const sys = /Android/.test(ua) ? "Android" : /iPhone/.test(ua) ? "iPhone" : iOS() ? "iPad" : /Windows/.test(ua) ? "Windows" : /Mac OS X/.test(ua) ? "Mac" : /Linux/.test(ua) ? "Linux" : "Appareil";
  const nav = /Firefox\//.test(ua) ? "Firefox" : /Edg\//.test(ua) ? "Edge" : /SamsungBrowser/.test(ua) ? "Samsung Internet" : /OPR\//.test(ua) ? "Opera" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "navigateur";
  return `${sys} · ${nav}`;
}

function cleServeur(b64) {
  const p = "=".repeat((4 - b64.length % 4) % 4);
  const brut = atob((b64 + p).replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(brut, c => c.charCodeAt(0));
}

async function abonnementIci() {
  if (!possibleIci()) return null;
  const reg = await navigator.serviceWorker.getRegistration("/");
  return reg ? reg.pushManager.getSubscription() : null;
}

async function inscrire(cle) {
  const permis = await Notification.requestPermission();
  if (permis !== "granted") throw new Error(permis === "denied"
    ? "Les notifications sont bloquées pour ce site : autorisez-les dans les réglages du navigateur, puis recommencez."
    : "Notifications non autorisées.");
  const reg = await navigator.serviceWorker.register("/sw.js", { scope: "/" });
  await navigator.serviceWorker.ready;
  let sub = await reg.pushManager.getSubscription();
  if (!sub) sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: cleServeur(cle) });
  const v = await api("POST", "/api/compte/notifications/appareils", { nom: nomAppareil(), abonnement: sub.toJSON() });
  garderIci(v.ici);
  return v;
}

/** Monte le panneau dans box. role : « mj », « joueur » ou « auditeur » dans cet univers. */
export async function panneauAppareils(box, role = "") {
  let v;
  try { v = await api("GET", "/api/compte/notifications"); } catch { box.innerHTML = ""; return; }
  const peindre = async () => {
    const ici = lireIci(), sub = await abonnementIci().catch(() => null);
    const inscritIci = !!sub && v.appareils.some(a => a.id === ici);
    const genres = (v.genres || []).filter(g => (POUR_ROLE[role] || Object.keys(LIBELLES)).includes(g));
    let etatIci;
    if (!v.possible) etatIci = `<p class="hint">Ce serveur n'envoie pas de notifications sur les appareils : il lui faut son adresse publique en HTTPS (réglage de l'administrateur, <code>COF_DM_ORIGINE</code>). La cloche 🔔 de l'application marche quand même.</p>`;
    else if (iOS() && !installee()) etatIci = `<p class="alerte">Sur iPhone et iPad, installez d'abord l'application sur l'écran d'accueil : dans Safari, touchez <b>Partager</b> puis <b>Sur l'écran d'accueil</b>. Ouvrez-la depuis son icône, revenez ici et activez les notifications.</p>`;
    else if (!possibleIci()) etatIci = `<p class="hint">Ce navigateur ne reçoit pas de notifications de site. Essayez Chrome, Firefox, Edge ou Safari à jour.</p>`;
    else if (Notification.permission === "denied") etatIci = `<p class="alerte">Les notifications sont bloquées pour ce site sur cet appareil : autorisez-les dans les réglages du navigateur (le cadenas à côté de l'adresse), puis rechargez la page.</p>`;
    else if (inscritIci) etatIci = `<p class="ok-pastille">✓ Cet appareil reçoit vos notifications.</p><div class="ligne"><button type="button" id="ntf-essai">Envoyer un essai</button><button type="button" class="ghost" id="ntf-quitter">Ne plus recevoir sur cet appareil</button></div>`;
    else etatIci = `<div class="ligne"><button type="button" class="primary" id="ntf-inscrire">🔔 Recevoir les notifications sur cet appareil</button></div>`;
    box.innerHTML = `<h3 style="margin-top:14px">Notifications sur cet appareil</h3>
      <p class="hint">Votre téléphone sonne même écran éteint. Il n'affiche qu'un texte neutre (« COF DM — Rappel de séance ») : le détail se lit dans la cloche 🔔 de l'application. Rien ne sonne entre ${v.calme?.[0] ?? 22} h et ${v.calme?.[1] ?? 8} h : ce qui arrive la nuit sonne le matin.</p>
      ${etatIci}
      ${v.possible && genres.length ? `<fieldset class="ntf-genres"><legend class="hint">Ce que mes appareils reçoivent</legend>
        ${genres.map(g => `<label class="t-coche"><input type="checkbox" data-genre="${esc(g)}" ${v.coupees.includes(g) ? "" : "checked"}><span>${esc(LIBELLES[g] || g)}</span></label>`).join("")}</fieldset>` : ""}
      ${v.appareils.length ? `<table class="tab ntf-appareils"><tbody>${v.appareils.map(a => `<tr><td>${esc(a.nom)}${a.id === ici && inscritIci ? ` <span class="badge">cet appareil</span>` : ""}<br>
        <small class="hint">inscrit le ${dateHeure(a.cree)}${a.recu ? " · dernier envoi le " + dateHeure(a.recu) : ""}${a.erreur ? " · " + esc(a.erreur) : ""}</small></td>
        <td style="text-align:right"><button type="button" class="ghost petit" data-retirer="${esc(a.id)}">Retirer</button></td></tr>`).join("")}</tbody></table>` : ""}`;
    $("#ntf-inscrire", box)?.addEventListener("click", async e => {
      e.target.disabled = true;
      try { v = await inscrire(v.cle); toast("Cet appareil recevra vos notifications."); }
      catch (err) { toast(err.message, true); }
      peindre();
    });
    $("#ntf-essai", box)?.addEventListener("click", async () => {
      try { await api("POST", "/api/compte/notifications/essai"); toast("Essai envoyé : il arrive dans quelques secondes."); }
      catch (err) { toast(err.message, true); }
    });
    $("#ntf-quitter", box)?.addEventListener("click", async () => {
      try {
        await sub?.unsubscribe();
        v = await api("DELETE", "/api/compte/notifications/appareils/" + encodeURIComponent(ici));
        garderIci("");
        toast("Cet appareil ne recevra plus vos notifications.");
      } catch (err) { toast(err.message, true); }
      peindre();
    });
    $$("[data-retirer]", box).forEach(b => b.onclick = async () => {
      try {
        if (b.dataset.retirer === ici) { await sub?.unsubscribe(); garderIci(""); }
        v = await api("DELETE", "/api/compte/notifications/appareils/" + encodeURIComponent(b.dataset.retirer));
      } catch (err) { toast(err.message, true); }
      peindre();
    });
    $$("[data-genre]", box).forEach(c => c.onchange = async () => {
      const coupees = $$("[data-genre]", box).filter(x => !x.checked).map(x => x.dataset.genre);
      try { v = await api("PUT", "/api/compte/notifications/reglages", { coupees: [...new Set([...v.coupees.filter(g => !genres.includes(g)), ...coupees])] }); }
      catch (err) { toast(err.message, true); peindre(); }
    });
  };
  await peindre();
}
