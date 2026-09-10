/**
 * Le registre du Ladoum, lu sur la chaîne.
 *
 * # Ce que ce démonstrateur est
 *
 * Il est réel : chaque animal affiché ici est une inscription sur la chaîne
 * Yamale, lue en direct, et rien n'est simulé côté client.
 *
 * # Ce qui a changé, et pourquoi cela compte
 *
 * Ce fichier roulait sur x/land — le module de registre foncier. La
 * correspondance tenait bien : une parcelle et un animal ont la même forme,
 * un bien de grande valeur identifié de manière unique, inscrit par une
 * autorité habilitée, cédé sous supervision.
 *
 * Ce que x/land ne fournissait pas, et que son en-tête disait déjà : LA
 * FILIATION COMME RELATION DE PREMIER ORDRE. L'ascendance vivait dans le texte
 * libre d'un acte, le graphe généalogique était reconstruit dans ce navigateur,
 * et la cohérence d'une lignée était donc vérifiée par la page qui l'affichait.
 * Une vérification qui vit dans une page est une convention d'affichage : elle
 * ne refuse rien, et quiconque envoie une transaction directement à la chaîne
 * ne la rencontre jamais.
 *
 * x/herd tient cette ligne dans le consensus. Un père qui n'est pas mâle, une
 * mère qui a mis bas quinze jours plus tôt, un animal déclaré son propre
 * ancêtre — la chaîne les refuse, au niveau du bloc, pour tout le monde. Ce
 * fichier n'a donc plus à faire semblant.
 *
 * Trois conséquences visibles ici :
 *
 *   - les champs du certificat (nom, sexe, robe, type de portée, date de
 *     naissance) sont des champs typés du module, non plus des paires
 *     `clé=valeur` glissées dans la référence d'un acte ;
 *   - la généalogie est une requête, `Pedigree`, bornée côté chaîne, qui dit
 *     elle-même si la borne a interrompu la remontée ;
 *   - le niveau de preuve est le champ `level` du registre, celui-là même sur
 *     lequel la chaîne s'appuie pour refuser une certification sans ADN.
 */

import {
  herdQuery, herdOrNull, head, blockTime, abci,
} from './chain.js';
import { JURISDICTION, PEDIGREE_DEPTH } from './registrar.js';

/* ========================================================================= */
/*  Le niveau de preuve                                                      */
/* ========================================================================= */

/**
 * Le niveau de preuve du lien entre la fiche et l'animal.
 *
 * C'est la seule note qui compte pour un acheteur, et c'est celle que le
 * certificat papier actuel ne porte pas. Un certificat parfait décrivant un
 * autre animal reste un certificat parfait : ce qu'on gradue ici, c'est ce qui
 * rattache le papier à la bête.
 *
 * La différence avec la version précédente n'est pas cosmétique. Ce niveau
 * était déduit ici, de la présence de deux actes ; il est maintenant lu du
 * champ `level`, que la chaîne écrit elle-même et sur lequel elle s'appuie
 * pour refuser. Une fiche annonçant « filiation vérifiée » signifie donc que
 * le consensus l'a acceptée, et non que cette page a su compter jusqu'à deux.
 */
export function evidenceLevel(animal) {
  if (!animal) return null;
  const dna = Boolean(animal.dna_hash);
  const chip = Boolean(animal.tag);

  if (animal.level === 'LEVEL_CERTIFIED') {
    return {
      level: 'verifiee',
      label: 'Filiation vérifiée',
      why: 'Niveau 3 : puce posée par un agent habilité, profil ADN au dossier, '
        + 'et ascendance acceptée par la chaîne — qui refuse une lignée impossible.',
      chain: true,
    };
  }
  if (animal.level === 'LEVEL_INDIVIDUAL') {
    return {
      level: chip ? 'pucee' : 'individuelle',
      label: chip ? 'Identifié, filiation déclarée' : 'Identifié sans puce',
      why: chip
        ? 'Niveau 2 : l’animal est identifiable par sa puce. Ses ascendants sont '
          + 'déclarés et non testés en laboratoire.'
        : 'Niveau 2 sans transpondeur : la fiche existe, rien ne la rattache '
          + 'physiquement à l’animal au quotidien.',
      chain: true,
    };
  }
  if (animal.level === 'LEVEL_HERD') {
    return {
      level: 'troupeau',
      label: 'Troupeau seulement',
      why: 'Niveau 1 : le troupeau est inscrit, l’animal ne l’est pas '
        + 'individuellement. C’est le niveau qui peut couvrir un cheptel national.',
      chain: true,
    };
  }
  return {
    level: 'declaree',
    label: 'Déclaration seule',
    why: 'Aucun niveau porté par le registre. C’est le niveau du certificat '
      + 'papier actuel, qui porte « non pucé ».',
    chain: false,
    dna,
  };
}

/* ========================================================================= */
/*  Le certificat                                                            */
/* ========================================================================= */

const SEX_FR = {
  SEX_MALE: 'Mâle',
  SEX_FEMALE: 'Femelle',
  SEX_UNSPECIFIED: '—',
};

const SPECIES_FR = {
  SPECIES_CATTLE: 'Bovin',
  SPECIES_SHEEP: 'Ovin',
  SPECIES_GOAT: 'Caprin',
  SPECIES_HORSE: 'Équin',
  SPECIES_CAMEL: 'Camelin',
  SPECIES_UNSPECIFIED: '—',
};

export const sexLabel = (s) => SEX_FR[s] ?? '—';
export const speciesLabel = (s) => SPECIES_FR[s] ?? '—';

/**
 * Les données du certificat.
 *
 * Elles viennent maintenant du registre lui-même. Auparavant, elles étaient
 * encodées en `clé=valeur` dans la référence textuelle d'un acte de naissance
 * et relues ici — ce qui donnait l'illusion que la chaîne validait ces champs
 * alors qu'elle ne faisait que stocker une chaîne de caractères. Le module les
 * type ; un sexe est un enum, et c'est ce même enum qui fait refuser un père
 * qui n'est pas mâle.
 */
export function certificate(animal) {
  if (!animal) return null;
  return {
    idn: animal.national_id,
    puce: animal.tag || '—',
    nom: animal.name || '—',
    sexe: sexLabel(animal.sex),
    robe: animal.coat || '—',
    portee: animal.birth_type || '—',
    naissance: animal.born_on || '—',
    proprietaire: animal.holder,
    office: animal.authority,
    adn: animal.dna_hash || '',
    photo: animal.photo_hash || '',
    inscrit_au_bloc: animal.registered_at,
    statut: animal.status,
  };
}

/* ========================================================================= */
/*  Les lectures                                                             */
/* ========================================================================= */

/** Un ascendant déclaré, ou null quand la filiation est inconnue. */
export const hasParent = (id) => id !== undefined && id !== null && id !== '0';

/** Charge un animal par son IDN — le numéro imprimé sur le certificat. */
export async function animalByIdn(idn) {
  const res = await herdOrNull('AnimalByNationalID', String(idn).trim());
  return res?.animal || null;
}

/** Charge un animal par le code lu au lecteur de puce. */
export async function animalByTag(tag) {
  const res = await herdOrNull('AnimalByTag', String(tag).trim());
  return res?.animal || null;
}

/** Charge un animal par son numéro d'inscription. */
export async function animalById(id) {
  const res = await herdOrNull('Animal', String(id));
  return res?.animal || null;
}

/** Charge un troupeau par sa référence de dossier papier. */
export async function herdByRef(reference) {
  const res = await herdOrNull('HerdByRef', String(reference).trim());
  return res?.herd || null;
}

/**
 * Ce qu'un lecteur de terrain rend, quel qu'il soit.
 *
 * Un agent en brousse ne sait pas — et n'a pas à savoir — si ce qu'il tient
 * est un IDN, un code de transpondeur ou un numéro d'inscription. Les trois
 * sont essayés, dans l'ordre où ils sont probables, et l'appelant reçoit ce
 * qui a répondu.
 */
export async function lookup(text) {
  const q = String(text || '').trim();
  if (!q) return null;
  if (q.includes('/')) return animalByIdn(q);
  // Un transpondeur ISO 11784 fait quinze chiffres ; un numéro d'inscription
  // en fait beaucoup moins. Les deux sont essayés de toute façon, car un
  // registre qui refuse la saisie plutôt que de chercher est un registre que
  // l'agent contourne.
  return (await animalByTag(q)) || (await animalById(q)) || (await animalByIdn(q));
}

/* ========================================================================= */
/*  La généalogie                                                            */
/* ========================================================================= */

/**
 * Remonte la lignée, en une requête.
 *
 * C'est l'endpoint que le certificat papier ne peut pas avoir. Une ligne
 * « Classico / Atomique / Palmarin » s'imprime mais ne se remonte pas ; ici
 * chaque ascendant est une fiche, donc un acheteur voit ce que chacun est
 * réellement.
 *
 * `truncated` est répercuté et jamais avalé. Quand c'est la borne qui a
 * arrêté la remontée, une page qui n'afficherait que les nœuds dirait « plus
 * d'ascendants » là où la vérité est « nous avons cessé de chercher » : ce
 * sont deux faits opposés sur une lignée.
 */
export async function pedigree(animal, depth = PEDIGREE_DEPTH) {
  if (!animal) return null;
  const res = await herdQuery('Pedigree', { animalId: animal.id, depth });
  const nodes = res.nodes || [];
  return {
    nodes,
    truncated: Boolean(res.truncated),
    height: res.height,
    tree: asTree(nodes),
    // Tous les animaux renvoyés, par numéro. Voir `known` ci-dessous : une
    // vraie lignée close est un treillis, pas un arbre, et la chaîne ne renvoie
    // chaque ascendant qu'une fois.
    known: new Set(nodes.map((n) => n.animal?.id).filter(Boolean)),
  };
}

/**
 * Le tableau plat des nœuds, remonté en arbre par leur chemin.
 *
 * Le chemin est `sire`, `dam`, `sire.dam`, `sire.sire.dam` — la chaîne le
 * construit en descendant, donc reconstruire l'arbre est purement
 * présentationnel et n'invente aucune relation. C'est la limite exacte de ce
 * qu'une page a le droit de faire d'une lignée : la mettre en forme.
 */
export function asTree(nodes) {
  const byPath = new Map();
  let root = null;
  for (const n of nodes) {
    const node = { animal: n.animal, generation: n.generation, path: n.path, sire: null, dam: null };
    byPath.set(n.path, node);
    if (n.path === '') root = node;
  }
  for (const node of byPath.values()) {
    if (node.path === '') continue;
    const cut = node.path.lastIndexOf('.');
    const parentPath = cut === -1 ? '' : node.path.slice(0, cut);
    const side = cut === -1 ? node.path : node.path.slice(cut + 1);
    const parent = byPath.get(parentPath);
    if (parent && (side === 'sire' || side === 'dam')) parent[side] = node;
  }
  return root;
}

/**
 * Combien d'ascendants sont réellement inscrits, sur le nombre cité.
 *
 * Un ascendant cité mais absent du registre est une information et non une
 * erreur : c'est le cas normal des lignées antérieures au dispositif. Ce qui
 * serait malhonnête serait de ne pas le compter.
 *
 * `known` est indispensable, et son absence produisait un mensonge. Une lignée
 * de stud fermé est un TREILLIS et non un arbre : le même bélier apparaît sur
 * plusieurs branches. La chaîne ne renvoie chaque ascendant qu'une fois — la
 * deuxième occurrence est un nœud absent du tableau, pas un animal absent du
 * registre. Sans `known`, la page affichait « cité comme n° 1, non inscrit au
 * registre » pour un fondateur parfaitement inscrit, deux lignes plus haut. Sur
 * la page dont tout le métier est d'être crue, c'est la pire erreur possible.
 */
export function pedigreeCoverage(root, known = null) {
  let cited = 0;
  let held = 0;
  const walk = (n) => {
    if (!n || !n.animal) return;
    for (const side of ['sire', 'dam']) {
      const declared = side === 'sire' ? n.animal.sire_id : n.animal.dam_id;
      if (!hasParent(declared)) continue;
      cited += 1;
      if (n[side]?.animal || known?.has(declared)) held += 1;
      walk(n[side]);
    }
  };
  walk(root);
  return { cited, held };
}

/**
 * Ce qu'il faut afficher pour un ascendant cité mais sans nœud propre.
 *
 * Trois cas, et les confondre trompe l'acheteur :
 *
 *   'ailleurs'  déjà présenté sur une autre branche — le treillis ;
 *   'absent'    cité au registre, mais l'animal n'y est pas inscrit ;
 *   'coupe'     la remontée s'est arrêtée à la borne, on a cessé de chercher.
 */
export function missingParent(id, known, truncated) {
  if (known?.has(id)) return 'ailleurs';
  return truncated ? 'coupe' : 'absent';
}

/* ========================================================================= */
/*  Le troupeau, la santé, la provenance                                     */
/* ========================================================================= */

/** Les animaux individuellement inscrits d'un troupeau. */
export async function animalsOf(herdId) {
  const res = await herdOrNull('Animals', String(herdId));
  return res?.animals || [];
}

/**
 * Ce pour quoi un troupeau a été traité.
 *
 * C'est la couverture vaccinale, et non un décompte de doses. La différence
 * est celle qui intéresse un bailleur : mille doses livrées ne disent rien,
 * mille troupeaux traités disent tout.
 */
export async function healthOf(herdId) {
  const res = await herdOrNull('HealthEvents', String(herdId));
  return res?.health_events || [];
}

/**
 * La provenance d'un animal, refus compris.
 *
 * Une vente à laquelle quelqu'un s'est opposé fait partie de ce qu'un acheteur
 * doit voir. Un animal qu'on a tenté de vendre et qu'on a été empêché de
 * vendre mérite d'être signalé, même si l'opposition a ensuite été écartée —
 * et le registre conserve les deux, l'opposition et la décision qui l'a
 * tranchée, avec le nom de l'office qui l'a prise.
 */
export async function provenance(animalId) {
  const res = await herdOrNull('TransfersByAnimal', String(animalId));
  return res?.transfers || [];
}

/** Une cession par son numéro. */
export async function transfer(id) {
  const res = await herdOrNull('Transfer', String(id));
  return res?.transfer || null;
}

/* ========================================================================= */
/*  Les offices et le registre                                               */
/* ========================================================================= */

/**
 * Le registre de la juridiction sénégalaise.
 *
 * Les offices sont admis par la gouvernance de la chaîne, jamais entre eux —
 * c'est ce qui empêche une bergerie de se déclarer registre. Et x/herd exige
 * qu'un office soit un compte de groupe, donc chacune de ses décisions requiert
 * déjà plusieurs signatures à l'intérieur de l'office.
 */
export async function offices(jurisdiction = JURISDICTION) {
  const res = await herdQuery('Authorities');
  return (res.authorities || []).filter((a) => a.jurisdiction === jurisdiction);
}

/** Les paramètres en vigueur : quorum, fenêtre de contestation, intervalle. */
export async function params() {
  const res = await herdQuery('Params');
  return { ...res.params, height: res.height };
}

/**
 * Les animaux inscrits, balayés par identifiant.
 *
 * x/herd ne publie volontairement aucune requête « tous les animaux » : une
 * liste publique de qui détient quel bétail de valeur, et où, est une liste de
 * cibles. Ce balayage borné est donc une commodité de démonstration et non une
 * fonctionnalité du registre ; il est filtré sur les offices admis, de sorte
 * qu'une fiche inscrite par une adresse qui n'est pas un office de la
 * juridiction n'apparaît pas.
 */
export async function roll({ from = 1, limit = 40, jurisdiction = JURISDICTION } = {}) {
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
  return { height: h.height, time: h.time, catchingUp: h.catchingUp };
}

export { blockTime, abci, herdQuery, herdOrNull };
