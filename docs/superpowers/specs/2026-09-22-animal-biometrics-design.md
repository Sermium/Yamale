# Identifying the animal itself

A mobile and tablet application that recognises a beast from its own body —
muzzle, iris, face, coat — reads its tattoo or its chip, and joins that to the
record `x/herd` already holds: pedigree, genetics, health, transfers.

Written 2026-09-22. `x/herd` exists and carries most of what this needs; this
document is about the parts that are missing and the two parts of the plan that
do not work as stated.

---

## 1. Two things that do not work, before anything is bought

### 1.1 A phone cannot read an animal transponder

`Animal.tag` is the **ISO 11784/11785** transponder code, and that standard
operates at **134.2 kHz — low frequency**. Every NFC radio in every phone and
tablet is **13.56 MHz — high frequency**. They are different bands with
different antennas and different physics. No software fixes it, and no phone on
the market reads an ISO 11784/11785 implant.

This matters because it is the one requirement that cannot be discovered late:
a field team issued with phones and told to "scan the chip" will find out on the
first farm.

The options, and they are a real choice:

| | |
|---|---|
| **External LF reader over Bluetooth** | The livestock industry's own answer — a stick or panel reader paired to the device. Reads the national tags already in the animals. Adds a second object to carry and charge. **Recommended.** |
| **Fit HF (13.56 MHz) tags instead** | Phone reads them directly, no second device. But the animal is then outside ISO 11784/11785 and invisible to every national system and abattoir reader that exists. Only sensible for a closed programme. |
| **Both** | An LF reader for existing animals, HF for new ones. Two identifier spaces to reconcile, which is a cost that lands later and forever. |

The tattoo is unaffected: that is OCR from the camera, plus manual entry, and
manual entry must always be available because a tattoo in a dusty ear at dusk is
sometimes simply unreadable.

### 1.2 A hash cannot identify an animal

`Animal.photo_hash` holds "the standardised photographic biometrics". A hash
answers **"is this the same file?"** — it cannot answer **"which animal is
this?"**

Identification is a 1:N search: take a fresh capture, compare it against every
enrolled template, return the candidates ranked by similarity. That requires the
templates themselves and a comparison function. Hashes have neither — change one
pixel and the hash is unrelated, which is exactly the property that makes them
useless for matching a photograph taken on a different day in different light.

So **the chain cannot be the matcher**, and no amount of on-chain data changes
that. What the chain can do is hold what it is good at: who enrolled what, when,
and who is answerable for the claim that two captures are the same animal.

---

## 2. What goes on the chain, and what does not

The rule this follows is `x/herd`'s own. That module already refuses a pedigree
at consensus, uses accredited agents, and lets an objection be answered. A
biometric match is the same shape of thing: **a claim by somebody who can be
held to it**, not a fact the chain computed.

**On the chain**

- The animal, its pedigree, its history — as today.
- One **enrolment record per capture**: modality, a commitment to the template,
  the capture time, the agent, the device, a position commitment, and the
  quality score the capture achieved. A *series*, never one field — see §4.
- An **identification claim**: this capture was matched to this animal, by this
  agent, at this score, on this device. Recorded because it is an assertion
  somebody made, and a wrong one should be attributable.
- **Objections**, exactly as transfers already have them.

**Never on the chain**

- The template. A biometric template is not a secret that can be rotated: if it
  leaks, it leaks for the life of the animal, and for a breeding programme it is
  commercially valuable in itself. A public ledger is the worst possible home
  for it.
- The photograph.
- The match score as a *verdict*. The chain records that an agent claimed a
  match at 0.94; it does not conclude anything from 0.94.

**Off the chain, in the biometric service**

- Templates, the index, and the 1:N search.
- Held per jurisdiction, because livestock registries are national and the data
  residency question is asked early by exactly the ministries this is sold to.

---

## 3. Which biometric, for which animal

Published accuracies for animal biometrics are laboratory figures. A restrained
animal, even light, a clean dry muzzle, a cooperative handler. The field has
dust, rain, a wet muzzle, a moving head, and the sun behind the operator. Plan
for the field number, which nobody publishes.

**Muzzle print** is the established one for cattle — the bovine fingerprint,
used for registration since the 1920s, and the best performer in the literature.
It is the primary modality for cattle. It needs a **close, sharp, evenly lit**
capture, which is what drives the camera requirement in §6.

**Iris** works and is stable over life, which muzzle and face are not. It wants
infrared illumination and a close working distance, and **no phone exposes an
infrared camera** — the Face ID emitter on iPhones is not available to
applications. Visible-light iris on a dark-eyed animal is much weaker than the
human-iris literature suggests.

**Face** is usable and less mature, and sensitive to pose. It is the friendliest
to capture, which matters more than it sounds: a modality an operator will
actually use twice a day beats a better one they avoid.

**Coat and body pattern** is excellent for a patterned breed and useless for a
solid-coloured one. For **Ladoum** — the breed this registry was built around —
patterning is varied enough to be worth capturing, and it doubles as the
photograph a certificate wants anyway.

**Retinal vascular pattern** is accurate and needs specialised capture
equipment. Out of scope for a phone.

**The recommendation: muzzle first for cattle, face-and-coat for sheep, and the
tag or tattoo always.** The module's own comment says a registry treating a
removable tag as proof "can be defeated with a pair of pliers". The inverse is
equally true: a registry treating a biometric as proof can be defeated by a bad
capture, and a bad capture is far more common than a thief with pliers. Two
independent markers, and a disagreement between them is a finding rather than an
error.

---

## 4. An animal is not the same shape all its life

A calf's muzzle is not an adult's. A lamb's face changes for a year. Iris is
stable; coat changes with season and condition.

So an enrolment is **a series with dates**, and a match is always against the
template nearest in time — with a re-enrolment prompted when the interval since
the last one exceeds what the modality tolerates. A single `photo_hash` captured
at registration and never revisited would stop matching the animal it belongs to
within a year, and the failure would look like the system rejecting a legitimate
beast.

This is the strongest argument for the series being on-chain: re-enrolment is
the moment an animal could be quietly swapped, so **who re-enrolled it and when
is exactly the thing worth making permanent.**

---

## 5. Offline, and the herd in your hand

Pastoral areas have no connectivity, and the app has to work anyway.

- **A local replica scoped to one herd.** A keeper downloads their own animals —
  records, pedigree, health, the templates for their own beasts — and works
  against that all day. The scope matters: the whole national index on a tablet
  is both impractical and a data-protection problem, whereas your own herd on
  your own tablet is just your own records.
- **Capture queues and syncs.** Everything captured offline is held with its
  timestamp and position and submitted when a connection returns.
- **Offline matching is 1:N within the herd**, which is the common case — "which
  of my animals is this?" — and is tractable on-device for a herd of hundreds.
  A national 1:N search needs the service.
- **The chain is the arbiter of conflicts, not the device.** Two tablets that
  both recorded a treatment offline are reconciled by submitting both; the
  registry holds both records with their agents, and a contradiction is visible
  rather than silently resolved by whoever synced last.

---

## 6. The device, specified so it can be bought

The single most important specification is **close focus**, and it is the one
most phone datasheets hide. A muzzle print needs sharp detail at 10–20 cm. Many
phones cannot focus that close with the main camera, and the "macro" camera
fitted to mid-range phones is frequently a 2 MP fixed-focus sensor that produces
an unusable image. **A fixed-focus macro camera disqualifies a device.**

**Minimum, for a capture device**

| | |
|---|---|
| OS | Android 13+ (Camera2/CameraX gives the manual focus and exposure control this needs; iOS is viable but AVFoundation gives less) |
| Main camera | ≥12 MP **with optical stabilisation** — the animal moves, and OIS matters more than megapixels |
| Close focus | **autofocus** macro or an ultra-wide with AF, usable at ≤8 cm. Verify by taking a picture, not by reading a spec sheet |
| RAM | 6 GB |
| Storage | 128 GB — captures accumulate before sync |
| Battery | ≥5000 mAh, and USB-C PD |
| NFC | 13.56 MHz, for HF ear tags only. **Not** ISO 11784/11785 — see §1.1 |
| GNSS | dual-band if capture position is to be trusted |
| Ingress | IP54 minimum, in a rugged case. This is a field instrument |

**Recommended, and worth the difference**

- A **ring light or dual-LED**, or a clip-on diffuser. A single LED puts a
  specular highlight in the middle of a wet muzzle, which is precisely where the
  detail is.
- **A depth sensor or LiDAR** for body measurement and for liveness — a check
  that the operator is photographing an animal rather than a photograph.
- **An NPU** if matching is to run on-device offline. Not needed if the herd is
  small and the comparison is classical rather than a neural embedding.
- **A Bluetooth LF stick reader** for ISO 11784/11785. Not optional if the
  animals already carry national transponders.

**For the tablet**, the same camera rules apply if it captures. If it is only
the herd's local database and the reading surface, then 8 GB RAM, 128 GB and a
sunlight-readable screen matter more than the camera, and screen brightness
above 600 nits is the difference between usable and useless outdoors.

---

## 7. What to build first

In this order, because each step is worth having before the next exists:

1. **Capture and enrolment, with the quality gate.** An app that takes a muzzle
   capture, scores it, refuses a bad one, and records the enrolment against an
   existing animal. No matching at all. This alone replaces a paper photograph
   with something checkable, and it is where the field lessons are.
2. **The tag and tattoo path.** LF reader over Bluetooth, tattoo OCR, manual
   entry. Identification against the existing register, no biometrics involved.
   This is immediately useful and unblocks the whole workflow.
3. **1:N within one herd, offline.** "Which of my animals is this?" — the
   question keepers actually ask.
4. **The service, and national 1:N.** Only once the templates from steps 1–3
   exist in volume, because a matcher tuned on laboratory captures and deployed
   against field captures will disappoint everybody.
5. **Identification claims and objections on chain**, using `x/herd`'s existing
   attestation and objection machinery rather than a new one.

## 8. What this changes in `x/herd`

- `Animal.photo_hash` becomes a **series of enrolments** rather than one field.
  The existing field stays and keeps meaning what it means — the level-3
  registration photograph — because reusing it for something else would make
  every existing record ambiguous.
- New: `Enrolment` (modality, commitment, captured_at, agent, device, quality,
  position commitment) and `IdentificationClaim` (capture commitment, matched
  animal, score, agent, device).
- Accredited agents already exist. A biometric enrolment agent is the same idea
  with a different scope, and the scope field is already there.

Nothing here needs a new module.
