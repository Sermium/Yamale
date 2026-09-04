/**
 * Le registre du Ladoum, lu sur la chaîne.
 *
 * # Ce que ce démonstrateur est, et ce qu'il n'est pas
 *
 * Il est réel : chaque animal affiché ici est une inscription sur la chaîne
 * Yamale, lue en direct, et rien n'est simulé côté client.
 *
 * Il roule sur x/land — le module de registre foncier — et pas sur un module
 * dédié, ce qui mérite d'être dit plutôt que caché. Un parcelle et un animal
 * ont la même forme : un bien de grande valeur, identifié de manière unique,
 * inscrit par une autorité habilitée, portant des actes, et cédé sous
 * supervision. Le module fournit déjà toutes ces propriétés, y compris le refus
 * d'inscrire deux fois la même identité physique — le mécanisme qui interdit la
 * double vente d'un terrain interdit ici la double inscription d'un animal.
 *
 * Ce que x/land ne fournit pas, et qu'un module x/ladoum fournirait en
 * production : la filiation comme relation de premier ordre. Ici elle est portée
 * par des actes, et le graphe généalogique est reconstruit côté client. Cela
 * suffit à démontrer la mécanique ; cela ne suffirait pas à interdire une
 * déclaration de filiation incohérente au niveau du consensus, ce qui est
 * précisément l'intérêt d'un module dédié.
 *
 * # La correspondance
 *
 *   parcelle          -> animal
 *   cadastral_ref     -> IDN, tel que la filière l'écrit déjà : 221/682/021029/L
 *   geometry_hash     -> empreinte d'identité physique : puce + ADN + biométrie
 *   holder            -> propriétaire
 *   authority         -> l'office d'enregistrement (OS Ladoum)
 *   actes (deeds)     -> certificat de naissance, filiation, rapport ADN, concours
 *   transfert         -> cession à quatre parties
 */

import {
  landQuery, landOrNull, head, blockTime, abci,
} from './chain.js';

/** Les types d'actes que ce registre utilise. */
export const DEED = {
  BIRTH: 'certificat-naissance',
  SIRE: 'filiation-pere',
  DAM: 'filiation-mere',
  DNA: 'adn',
  SHOW: 'concours',
  CHIP: 'puce',
};

/**
 * Le niveau de preuve du lien entre la fiche et l'animal.
 *
 * C'est la seule note qui compte pour un acheteur, et c'est celle que le
 * certificat papier actuel ne porte pas. Un certificat parfait décrivant un
 * autre animal reste un certificat parfait : ce qu'on gradue ici, c'est ce qui
 * rattache le papier à la bête.
 */
export function evidenceLevel(animal) {
  const kinds = new Set((animal.deeds || []).map((d) => d.kind));
  const chip = kinds.has(DEED.CHIP);
  const dna = kinds.has(DEED.DNA);
  if (chip && dna) {
    return {
      level: 'verifiee',
      label: 'Filiation vérifiée',
      why: 'Puce électronique posée par un agent habilité et filiation confirmée en laboratoire.',
    };
  }
  if (dna) {
    return {
      level: 'adn',
      label: 'ADN seul',
      why: 'La filiation est testée mais rien ne rattache physiquement l’animal à sa fiche au quotidien.',
    };
  }
  if (chip) {
    return {
      level: 'pucee',
      label: 'Identifié, filiation déclarée',
      why: 'L’animal est identifiable ; ses ascendants sont déclarés par l’éleveur et non testés.',
    };
  }
  return {
    level: 'declaree',
    label: 'Déclaration seule',
    why: 'Aucun marqueur physique. C’est le niveau du certificat papier actuel, qui porte « non pucé ».',
  };
}

/** Un acte d'un type donné, ou null. */
export const deedOf = (animal, kind) =>
  (animal.deeds || []).find((d) => d.kind === kind) || null;

/**
 * Les données du certificat, telles que la filière les écrit.
 *
 * Les champs du certificat papier sont portés par l'acte de naissance : son
 * `reference` transporte les valeurs déclaratives — nom, sexe, robe, type de
 * portée — que la chaîne n'a pas vocation à typer. Ce qu'elle type, elle,
 * c'est l'identité, l'ascendance et la propriété.
 */
export function certificate(animal) {
  const birth = deedOf(animal, DEED.BIRTH);
  const fields = parseFields(birth?.reference || '');
  return {
    idn: animal.cadastral_ref,
    nom: fields.nom || '—',
    sexe: fields.sexe || '—',
    robe: fields.robe || '—',
    portee: fields.portee || '—',
    naissance: birth?.issued_on || '—',
    bergerie: fields.bergerie || '—',
    proprietaire: animal.holder,
    empreinte: animal.geometry_hash,
    inscrit_au_bloc: animal.registered_at,
  };
}

/**
 * `cle=valeur;cle=valeur` — le format du champ `reference` d'un acte.
 *
 * Volontairement pauvre. Un acte porte une référence textuelle courte, et
 * inventer ici un encodage riche donnerait l'illusion que la chaîne valide ces
 * champs alors qu'elle ne fait que les stocker. En production ils sont typés
 * par le module.
 */
export function parseFields(reference) {
  const out = {};
  for (const pair of String(reference).split(';')) {
    const at = pair.indexOf('=');
    if (at < 1) continue;
    out[pair.slice(0, at).trim()] = pair.slice(at + 1).trim();
  }
  return out;
}

/** L'IDN d'un ascendant, tel que l'acte de filiation le référence. */
export const parentRef = (animal, kind) => {
  const deed = deedOf(animal, kind);
  if (!deed) return null;
  const f = parseFields(deed.reference);
  return f.idn || null;
};

/** Charge un animal par son IDN. */
export async function animalByIdn(idn) {
  const res = await landOrNull('ParcelByRef', idn.trim());
  return res?.parcel || null;
}

/** Charge un animal par son numéro d'inscription. */
export async function animalById(id) {
  const res = await landOrNull('Parcel', id);
  return res?.parcel || null;
}

/**
 * Remonte la généalogie, en largeur, jusqu'à `depth` générations.
 *
 * Borné, et pas seulement pour la performance : une généalogie déclarée peut
 * contenir un cycle — un animal déclaré son propre ancêtre, par erreur de
 * saisie ou par fraude. Une remontée non bornée boucle indéfiniment ; celle-ci
 * s'arrête et le signale, ce qui est une information utile plutôt qu'un plantage.
 */
export async function pedigree(animal, depth = 3) {
  const seen = new Set([animal.cadastral_ref]);
  const node = async (a, level) => {
    if (!a || level > depth) return null;
    const out = { animal: a, level, sire: null, dam: null, cycle: false };
    for (const [key, kind] of [['sire', DEED.SIRE], ['dam', DEED.DAM]]) {
      const ref = parentRef(a, kind);
      if (!ref) continue;
      if (seen.has(ref)) { out.cycle = true; continue; }
      seen.add(ref);
      const parent = await animalByIdn(ref);
      // Un ascendant cité mais absent du registre est une information, pas une
      // erreur : c'est le cas normal des lignées antérieures au dispositif.
      out[key] = parent
        ? await node(parent, level + 1)
        : { animal: null, level: level + 1, missing: ref, sire: null, dam: null };
    }
    return out;
  };
  return node(animal, 0);
}

/** Combien d'ascendants sont réellement inscrits, sur le nombre cité. */
export function pedigreeCoverage(root) {
  let cited = 0; let held = 0;
  const walk = (n) => {
    if (!n) return;
    for (const k of ['sire', 'dam']) {
      const child = n[k];
      if (!child) continue;
      cited += 1;
      if (child.animal) held += 1;
      walk(child);
    }
  };
  walk(root);
  return { cited, held };
}

/**
 * Le registre de la juridiction sénégalaise.
 *
 * Les offices sont admis par la gouvernance de la chaîne, jamais entre eux —
 * c'est ce qui empêche une bergerie de se déclarer registre.
 */
export async function offices(jurisdiction = 'SN') {
  const res = await landQuery('Authorities');
  return (res.authorities || []).filter((a) => a.jurisdiction === jurisdiction);
}

/** Les animaux inscrits, balayés par identifiant. */
export async function roll({ from = 1, limit = 40, jurisdiction = 'SN' } = {}) {
  const admitted = new Set((await offices(jurisdiction)).map((o) => o.address));
  const found = [];
  for (let id = from; id < from + limit; id += 1) {
    const a = await animalById(String(id));
    if (!a) continue;
    if (!admitted.has(a.authority)) continue;
    found.push(a);
  }
  return found;
}

/** L'état de la chaîne, pour dater ce que la page affiche. */
export async function chainHead() {
  const h = await head();
  return { height: h.height, time: h.time };
}

export { blockTime, abci, landQuery, landOrNull };
