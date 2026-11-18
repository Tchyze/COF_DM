// Service worker de COF DM : il ne sert qu'aux notifications sur l'appareil (Web Push). Il ne garde
// rien en cache et n'intercepte aucune requête : l'application a toujours besoin du serveur.
// Le message reçu est neutre (« COF DM — Rappel de séance ») ; un toucher ouvre l'application où il
// faut (l'agenda, dans le bon univers).

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", e => e.waitUntil(self.clients.claim()));

self.addEventListener("push", e => {
  let d = {};
  try { d = e.data ? e.data.json() : {}; } catch { /* message illisible : le texte par défaut */ }
  e.waitUntil(self.registration.showNotification(d.titre || "COF DM", {
    body: d.texte || "Nouvelle notification",
    icon: "/icones/app-192.png",
    badge: "/icones/app-192.png",
    tag: d.etiquette || "cofdm",
    renotify: true,
    lang: "fr",
    data: { lien: d.lien || "/" },
  }));
});

self.addEventListener("notificationclick", e => {
  e.notification.close();
  const cible = new URL(e.notification.data?.lien || "/", self.location.origin);
  if (cible.origin !== self.location.origin) return; // jamais ailleurs que sur ce serveur
  e.waitUntil((async () => {
    const fenetres = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const f of fenetres) {
      if (new URL(f.url).pathname === cible.pathname && "focus" in f) {
        try { await f.navigate(cible.href); } catch { /* une page d'une autre origine ne se pilote pas */ }
        return f.focus();
      }
    }
    return self.clients.openWindow(cible.href);
  })());
});
