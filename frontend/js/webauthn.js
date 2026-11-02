// Clés de sécurité : ce que le navigateur fait avec le défi du serveur. L'API échange du base64url, le
// navigateur des octets. Ni le serveur ni la page ne voient jamais la clé privée.

const enOctets = s => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
const enTexte = b => btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
const liste = l => l?.map(x => ({ ...x, id: enOctets(x.id) }));

/** Ce navigateur sait parler à une clé ici : WebAuthn n'existe qu'en HTTPS, ou sur localhost. */
export const clesPossibles = () => !!window.PublicKeyCredential && window.isSecureContext;

/** Un message pour ce que le navigateur a refusé. */
export function messageCle(err) {
  if (err?.name === "SecurityError" && err.domaine) {
    return `Vos clés sont liées à « ${err.domaine} », et cette page est ouverte sur « ${location.hostname} » : ouvrez-la à l'adresse du serveur.`;
  }
  switch (err?.name) {
    case "NotAllowedError": return "Opération annulée, ou délai dépassé : recommencez et touchez votre clé.";
    case "InvalidStateError": return "Cette clé est déjà enregistrée sur ce compte.";
    case "SecurityError": return "Cette adresse n'est pas celle que le serveur connaît : vérifiez son option -origine.";
    case "NotSupportedError": return "Cette clé ne convient pas : elle ne connaît aucun des algorithmes demandés.";
    default: return err?.message || "Échec avec la clé de sécurité.";
  }
}

function commun(c) {
  return { id: c.id, rawId: enTexte(c.rawId), type: c.type, authenticatorAttachment: c.authenticatorAttachment || undefined,
    clientExtensionResults: c.getClientExtensionResults() };
}

/** navigator.credentials.create, pour ces options du serveur. */
export async function creerCle(o) {
  const publicKey = { ...o, challenge: enOctets(o.challenge), user: { ...o.user, id: enOctets(o.user.id) }, excludeCredentials: liste(o.excludeCredentials) };
  const c = await navigator.credentials.create({ publicKey }).catch(e => { e.domaine = o.rp?.id; throw e; });
  return { ...commun(c), response: { clientDataJSON: enTexte(c.response.clientDataJSON), attestationObject: enTexte(c.response.attestationObject),
    transports: c.response.getTransports?.() || [] } };
}

/** navigator.credentials.get, pour ces options du serveur. */
export async function prouverCle(o) {
  const publicKey = { ...o, challenge: enOctets(o.challenge), allowCredentials: liste(o.allowCredentials) };
  const c = await navigator.credentials.get({ publicKey }).catch(e => { e.domaine = o.rpId; throw e; });
  return { ...commun(c), response: { clientDataJSON: enTexte(c.response.clientDataJSON), authenticatorData: enTexte(c.response.authenticatorData),
    signature: enTexte(c.response.signature), userHandle: c.response.userHandle ? enTexte(c.response.userHandle) : undefined } };
}
