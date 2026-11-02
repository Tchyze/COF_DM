// Installer l'application sur une tablette : une icône sur l'écran d'accueil, qui l'ouvre en plein écran
// dans le navigateur. Le navigateur ne le propose qu'en HTTPS : sans lui, aucun bouton ne s'affiche. Pas de
// service worker : rien ne reste sur l'appareil, et l'application a besoin du serveur.
import { toast } from "./ui.js";

let invite = null;
const boutons = new Set();

const installee = () => matchMedia("(display-mode: fullscreen), (display-mode: standalone)").matches;
// Safari n'a pas d'invitation : on ne peut qu'expliquer le chemin
const safariIOS = () => /iPad|iPhone/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);

function maj() {
  for (const b of boutons) {
    if (b.isConnected) b.hidden = installee() || !(invite || safariIOS());
    else boutons.delete(b);
  }
}

window.addEventListener("beforeinstallprompt", e => {
  e.preventDefault(); // le bouton plutôt que le bandeau du navigateur
  invite = e;
  maj();
});
window.addEventListener("appinstalled", () => { invite = null; maj(); });

/** Ajoute à cet élément (avant `avant`, s'il est donné) le bouton d'installation, caché tant que ce n'est pas possible. */
export function boutonInstaller(hote, avant = null) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "ghost petit installer";
  b.textContent = "⤓ Installer sur cet appareil";
  b.hidden = true;
  b.onclick = async () => {
    if (!invite) {
      toast("Dans Safari : bouton Partager, puis « Sur l'écran d'accueil ».");
      return;
    }
    const e = invite;
    invite = null; // une invitation ne sert qu'une fois
    await e.prompt();
    maj();
  };
  hote.insertBefore(b, avant);
  boutons.add(b);
  maj();
  return b;
}
