// Abonnement à l'agenda des séances (serveur) : une adresse secrète, propre au compte, que Google Agenda,
// Apple Calendrier, Proton Calendar ou Outlook relisent d'eux-mêmes. Montrée sur la page des joueurs
// (onglet Agenda) et dans l'agenda du MJ ; un auditeur ne s'abonne pas.
import { $, esc, api, toast, armer, dateHeure } from "./ui.js";

/** Remplit box avec l'abonnement du compte ; rien (box vide) s'il ne peut pas s'abonner. */
export async function panneauAbonnement(box) {
  let a;
  try { a = await api("GET", "/api/compte/agenda"); } catch { box.innerHTML = ""; return; }
  if (!a.abonnable) { box.innerHTML = ""; return; }
  const maj = r => { a = r; dessiner(); };
  const dessiner = () => {
    if (!a.adresse) {
      box.innerHTML = `<h3>📅 Dans mon agenda, à jour tout seul</h3>
        <p class="hint">Abonnez votre agenda (Google, Apple, Proton, Outlook…) à vos séances : il se met à jour de lui-même quand le MJ change une date.</p>
        <div><button type="button" class="primary" data-creer>Créer mon adresse d'abonnement</button></div>`;
      $("[data-creer]", box).onclick = async () => {
        try { maj(await api("POST", "/api/compte/agenda")); toast("Adresse créée."); } catch (e) { toast(e.message, true); }
      };
      return;
    }
    const google = "https://calendar.google.com/calendar/render?cid=" + encodeURIComponent(a.webcal);
    const outlook = "https://outlook.live.com/calendar/0/addfromweb?url=" + encodeURIComponent(a.adresse) + "&name=" + encodeURIComponent("Séances de jeu");
    box.innerHTML = `<h3>📅 Dans mon agenda, à jour tout seul</h3>
      <p class="hint">Choisissez votre agenda. Il relira l'adresse de lui-même : un changement du MJ y arrive en général dans l'heure (Apple, Outlook), parfois seulement après plusieurs heures (Google).</p>
      <div class="abonnement-boutons">
        <a href="${esc(google)}" target="_blank" rel="noopener noreferrer"><button type="button">Google Agenda</button></a>
        <a href="${esc(a.webcal)}"><button type="button">Apple (iPhone, Mac)</button></a>
        <a href="${esc(outlook)}" target="_blank" rel="noopener noreferrer"><button type="button">Outlook</button></a>
        <button type="button" data-copier>Copier l'adresse</button>
      </div>
      <details class="abonnement-aide"><summary>Proton Calendar, Android, autre agenda</summary>
        <p><b>Proton Calendar</b> : « Autres agendas » → « Ajouter un agenda » → « Depuis une URL », puis collez l'adresse.</p>
        <p><b>Android</b> : l'application Google Agenda ne s'abonne pas elle-même. Utilisez le bouton « Google Agenda » (il ouvre le site), connecté au même compte Google : l'agenda apparaît ensuite sur le téléphone.</p>
        <p><b>Autre agenda</b> : cherchez « s'abonner à un agenda » ou « ajouter depuis une URL », et collez l'adresse.</p>
      </details>
      <label class="champ"><span>Votre adresse <small>gardez-la pour vous : qui l'a voit vos séances</small></span><input readonly value="${esc(a.adresse)}" data-adresse></label>
      <p class="hint">Créée le ${esc(dateHeure(a.cree))}${a.utilise ? ` · lue pour la dernière fois le ${esc(dateHeure(a.utilise))}` : " · pas encore lue par un agenda"}.</p>
      <div class="ligne" style="flex-wrap:wrap;gap:8px">
        <button type="button" class="ghost petit" data-changer title="Les agendas abonnés à l'ancienne adresse ne se mettent plus à jour">Changer d'adresse</button>
        <button type="button" class="ghost petit" data-retirer>Supprimer l'adresse</button>
      </div>`;
    $("[data-adresse]", box).onfocus = e => e.target.select();
    $("[data-copier]", box).onclick = () => navigator.clipboard?.writeText(a.adresse)
      .then(() => toast("Adresse copiée."), () => { $("[data-adresse]", box).select(); toast("Copie impossible : l'adresse est sélectionnée, copiez-la.", true); });
    armer($("[data-changer]", box), "Confirmer : l'ancienne ne marchera plus", async () => {
      try { maj(await api("POST", "/api/compte/agenda")); toast("Nouvelle adresse : abonnez-y de nouveau vos agendas."); } catch (e) { toast(e.message, true); }
    });
    armer($("[data-retirer]", box), "Confirmer la suppression", async () => {
      try { maj(await api("DELETE", "/api/compte/agenda")); toast("Adresse supprimée : vos agendas ne se mettront plus à jour."); } catch (e) { toast(e.message, true); }
    });
  };
  dessiner();
}
