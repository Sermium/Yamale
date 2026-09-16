/**
 * The explorer's own catalogue, in all five tier-1 languages.
 *
 * Why not `sdk/src/catalogues.ts`, where the rest of the product's strings
 * live? Because that file is the shared glossary for six surfaces, and these
 * keys are about one page's furniture — a liveness strip and a feed. Adding
 * forty explorer-specific keys to the shared catalogue makes every other
 * surface carry them, and the shared file is where five other live branches are
 * also editing. `register()` merges, so a catalogue registered here layers onto
 * the shared one without touching it.
 *
 * The `xp.` prefix keeps that layering honest: nothing here can shadow an
 * `exp.` key the shared catalogue already defines, so a term translated once in
 * the glossary stays translated once.
 *
 * All five locales carry every key, and `strings.test.ts` fails if one does
 * not. The failure mode is asymmetric and invisible — `t()` falls back to
 * English, so a missing French key ships as an English sentence in the middle of
 * a French page, which looks like a rendering bug to the reader and like nothing
 * at all to us.
 *
 * NOT translated, deliberately: the decoded event sentences themselves
 * ("Alice's country was recorded as KE"). Those are composed in `feed.ts` and
 * in the SDK's decoder from sixty-odd per-message templates, and the SDK has
 * always emitted them in English. Translating half of them would produce a row
 * whose label is French and whose sentence is English, which is worse than
 * either. Doing it properly means moving every decoder onto the catalogue, and
 * that is a change to the shared SDK rather than to this page.
 */

import { register } from '../../sdk/src/i18n.ts';

export const en = {
  // --- liveness -------------------------------------------------------------
  'xp.health.live': 'Live',
  'xp.health.slow': 'Late',
  'xp.health.stopped': 'Stopped',
  'xp.health.catchingUp': 'Catching up',
  'xp.health.unreachable': 'No answer',
  'xp.health.unknown': 'Checking',
  'xp.health.lastBlock': 'last block {age}',
  'xp.health.noBlockTime': 'no timestamp',

  'xp.status.height': 'Block',
  'xp.status.blockTime': 'Block time',
  'xp.status.validators': 'Validators',
  'xp.status.bonded': 'Stake bonded',
  'xp.status.seconds': '{value}s',
  'xp.status.median': 'median of {count}',
  'xp.status.ofSupply': '{percent} of supply',
  'xp.status.fragile': 'losing one halts the chain',
  'xp.status.spare': '{count} can be lost',
  'xp.status.concentrated': 'stake is concentrated',

  'xp.stopped.title': 'This chain has stopped producing blocks.',
  'xp.stopped.body':
    'Everything below is the state at block {height}, read by asking the node for that height explicitly — the only thing a halted node will answer. It is accurate and it is not current.',
  'xp.stopped.bodyPlain':
    'The last block was {age}. Everything below is historical, and nothing new can settle until blocks resume.',
  'xp.unreachable.title': 'The explorer cannot reach the chain.',
  'xp.unreachable.body':
    'This may be the node or it may be this connection — the two look the same from here. The page retries on its own and recovers when the connection does.',
  'xp.catchingUp.title': 'This node is still replaying the chain.',
  'xp.catchingUp.body':
    'It answers from block {height}, which is behind the tip. Figures below are real and incomplete.',

  // --- the feed -------------------------------------------------------------
  'xp.feed.title': 'What happened',
  'xp.feed.outcome': 'Done',
  'xp.feed.step': 'In progress',
  'xp.feed.routine': 'Housekeeping',
  'xp.feed.refused': 'Refused',
  'xp.feed.approved': '{yes} of {total} approved',
  'xp.feed.request': 'request {id}',
  'xp.feed.votedOn': 'voted {option} on {what}',
  'xp.feed.ranFailed': 'approved, but the action itself was refused',
  'xp.feed.whyRefused': 'Why it was refused',
  'xp.feed.rawMessage': 'Raw message',
  'xp.feed.showAll': 'Show housekeeping',
  'xp.feed.showEveryday': 'Hide housekeeping',
  'xp.feed.emptyTitle': 'Nothing has happened yet',
  'xp.feed.emptyHint': 'When money moves on this network, it appears here.',
  'xp.feed.emptyStopped': 'The chain is stopped, so nothing new can appear here.',
  'xp.feed.noAmount': 'no amount',
  'xp.feed.inBlock': 'block {height}',

  // --- identifiers ----------------------------------------------------------
  'xp.id.copy': 'Copy',
  'xp.id.copied': 'Copied',
  'xp.id.reveal': 'Show in full',
  'xp.id.shorten': 'Shorten',

  // --- search ---------------------------------------------------------------
  'xp.search.label': 'Search the chain',
  'xp.search.placeholder': 'Account, user ID, transaction, block, currency or validator',
  'xp.search.looking': 'Looking for {term}',
  'xp.search.notFound': 'Nothing on this chain matched that',
  'xp.search.notFoundHint': 'You can paste any of these, and nothing else is needed:',
  'xp.search.kindAddress': 'an account address',
  'xp.search.kindUserId': 'a Yamale user ID, such as NG-K3M9-7QRT-5',
  'xp.search.kindTx': 'a transaction hash',
  'xp.search.kindHeight': 'a block number',
  'xp.search.kindDenom': 'a currency code, such as NGN',
  'xp.search.kindValidator': "a validator's name",
  'xp.search.noAccountForId': 'No account on this chain holds the user ID {id}.',
  'xp.search.currency': 'Currency',
  'xp.search.supply': 'In circulation',
  'xp.search.viewPrices': 'See its rate and how old the rate is',
  'xp.search.viewValidator': 'See it in the validator set',
  'xp.feed.ledeSimple': 'Money moving on the Yamale network, and decisions that were carried out. Search above for an account, a payment or a currency.',
  'xp.feed.ledeExpert': 'Every message the chain has accepted or refused, newest first, with the raw payload one disclosure away.',
  'xp.feed.blocks': 'Recent blocks',
  'xp.feed.loadingActivity': 'Reading recent activity',
  'xp.feed.loadingBlocks': 'Reading blocks',
  'xp.feed.emptyBlock': 'empty',
  'xp.search.storedAs': 'Stored as',
  'xp.search.decimals': '{count} decimals',
  'xp.search.noneIssued': 'none issued',
  'xp.search.operator': 'Operator',
  'xp.search.validator': 'Validator',
  'xp.search.jailed': 'Jailed',
  'xp.search.signing': 'Signing',
  'xp.search.idNeverRepointed': 'An identifier is issued when an account is placed in a country, and is never repointed once issued.',

  // --- information design ---------------------------------------------------
  // The front page's figures, its folders, and the three different ways of
  // having nothing to show. See Home.tsx: a feed that is empty because the
  // chain is new and a feed that is empty because everything in the window was
  // housekeeping are not the same fact, and the page shipped saying the first
  // one while the second was true.
  'xp.kpi.transactions': 'Transactions',
  'xp.kpi.transactionsHint': 'accepted since genesis, as the node counts them',
  'xp.kpi.outcomes': 'Outcomes',
  'xp.kpi.outcomesHint': 'money moved, or a decision took effect',
  'xp.kpi.routine': 'Routine',
  'xp.kpi.routineHint': 'oracle prices and parameter housekeeping',
  'xp.kpi.parties': 'Accounts involved',
  'xp.kpi.partiesHint': 'distinct parties named in this window',
  'xp.kpi.window': 'of the last {count} read',
  'xp.kpi.unread': 'the node did not answer',
  'xp.fold.everyday': 'Everyday',
  'xp.fold.everything': 'Everything',
  'xp.fold.blocks': 'Blocks',
  'xp.feed.routineOnlyTitle': 'Only housekeeping in this window',
  'xp.feed.routineOnlyBody':
    'The chain accepted {count} messages here and every one of them was routine — oracle price submissions and parameter reads. No money moved. They are under Everything.',

  // --- staking --------------------------------------------------------------
  'xp.stake.lede':
    'Locking tokens with a validator secures the network and earns a share of new issuance and transaction fees.',
  'xp.stake.noticeTitle': 'Your tokens are not available while staked.',
  'xp.stake.noticeBody':
    'Withdrawing takes {delay} from the moment you ask, and they earn nothing during that time. If a validator misbehaves, part of what you staked with them can be lost.',
  'xp.stake.securing': 'in the bonded set',
  'xp.stake.delay': 'Withdrawal delay',
  'xp.stake.afterAsking': 'after you ask to unstake',
  'xp.stake.largest': 'Largest validator',
  'xp.stake.largestHint': '{name} holds this much of the bonded stake',
  'xp.stake.concentration': 'Who holds the chain',
  'xp.stake.concLede':
    'A block needs two thirds of the voting power to commit. One holder above a third can stop that on their own; one holder above two thirds can do it on their own.',
  'xp.stake.overThird': 'over one third — can stall a block alone',
  'xp.stake.overTwoThirds': 'over two thirds — can commit a block alone',
  'xp.stake.cumulative': 'Running total',
  'xp.stake.others': 'Everyone else',
  'xp.stake.ofRewards': 'of your rewards',
  'xp.stake.unnamed': 'unnamed',
  'xp.stake.noneTitle': 'No bonded validators',
  'xp.stake.noneBody': 'The chain reports nobody in the bonded set, so nothing is securing it.',
  'xp.stake.enterAmount': 'Enter an amount.',
  'xp.stake.action': 'Stake',
  'xp.stake.amountFor': 'Amount of YML to stake with {name}',
  'xp.stake.consequence':
    '{amount} stops being available to you. Getting it back takes {delay} from the moment you ask, and it earns nothing during that time. If this validator misbehaves, part of it can be lost.',
};

export const fr: typeof en = {
  'xp.health.live': 'En marche',
  'xp.health.slow': 'En retard',
  'xp.health.stopped': 'Arrêtée',
  'xp.health.catchingUp': 'Rattrapage',
  'xp.health.unreachable': 'Sans réponse',
  'xp.health.unknown': 'Vérification',
  'xp.health.lastBlock': 'dernier bloc {age}',
  'xp.health.noBlockTime': 'sans horodatage',

  'xp.status.height': 'Bloc',
  'xp.status.blockTime': 'Temps de bloc',
  'xp.status.validators': 'Validateurs',
  'xp.status.bonded': 'Mise immobilisée',
  'xp.status.seconds': '{value} s',
  'xp.status.median': 'médiane sur {count}',
  'xp.status.ofSupply': '{percent} de la masse',
  'xp.status.fragile': "la perte d'un seul arrête la chaîne",
  'xp.status.spare': '{count} peuvent être perdus',
  'xp.status.concentrated': 'la mise est concentrée',

  'xp.stopped.title': 'Cette chaîne ne produit plus de blocs.',
  'xp.stopped.body':
    "Tout ce qui suit est l'état au bloc {height}, obtenu en demandant explicitement cette hauteur au nœud — la seule chose qu'un nœud arrêté accepte de répondre. C'est exact, et ce n'est pas actuel.",
  'xp.stopped.bodyPlain':
    "Le dernier bloc date {age}. Tout ce qui suit est historique, et rien de nouveau ne peut être réglé avant la reprise des blocs.",
  'xp.unreachable.title': "L'explorateur ne peut pas joindre la chaîne.",
  'xp.unreachable.body':
    "Cela peut venir du nœud comme de cette connexion — d'ici, les deux se ressemblent. La page réessaie seule et se rétablit dès que la connexion revient.",
  'xp.catchingUp.title': 'Ce nœud rejoue encore la chaîne.',
  'xp.catchingUp.body':
    "Il répond depuis le bloc {height}, en retard sur la pointe. Les chiffres ci-dessous sont réels et incomplets.",

  'xp.feed.title': "Ce qui s'est passé",
  'xp.feed.outcome': 'Fait',
  'xp.feed.step': 'En cours',
  'xp.feed.routine': 'Administration',
  'xp.feed.refused': 'Refusé',
  'xp.feed.approved': '{yes} approbations sur {total}',
  'xp.feed.request': 'demande {id}',
  'xp.feed.votedOn': 'a voté {option} sur {what}',
  'xp.feed.ranFailed': "approuvé, mais l'action elle-même a été refusée",
  'xp.feed.whyRefused': 'Motif du refus',
  'xp.feed.rawMessage': 'Message brut',
  'xp.feed.showAll': "Afficher l'administration",
  'xp.feed.showEveryday': "Masquer l'administration",
  'xp.feed.emptyTitle': "Rien ne s'est encore produit",
  'xp.feed.emptyHint': "Dès que de l'argent circule sur ce réseau, cela apparaît ici.",
  'xp.feed.emptyStopped': "La chaîne est arrêtée : rien de nouveau ne peut apparaître ici.",
  'xp.feed.noAmount': 'aucun montant',
  'xp.feed.inBlock': 'bloc {height}',

  'xp.id.copy': 'Copier',
  'xp.id.copied': 'Copié',
  'xp.id.reveal': 'Afficher en entier',
  'xp.id.shorten': 'Abréger',

  'xp.search.label': 'Rechercher dans la chaîne',
  'xp.search.placeholder': 'Compte, identifiant, transaction, bloc, devise ou validateur',
  'xp.search.looking': 'Recherche de {term}',
  'xp.search.notFound': 'Rien sur cette chaîne ne correspond',
  'xp.search.notFoundHint': "Vous pouvez coller n'importe lequel de ces éléments, et rien d'autre :",
  'xp.search.kindAddress': "une adresse de compte",
  'xp.search.kindUserId': 'un identifiant Yamale, par exemple NG-K3M9-7QRT-5',
  'xp.search.kindTx': "une empreinte de transaction",
  'xp.search.kindHeight': 'un numéro de bloc',
  'xp.search.kindDenom': 'un code de devise, par exemple NGN',
  'xp.search.kindValidator': "le nom d'un validateur",
  'xp.search.noAccountForId': "Aucun compte de cette chaîne ne porte l'identifiant {id}.",
  'xp.search.currency': 'Devise',
  'xp.search.supply': 'En circulation',
  'xp.search.viewPrices': "Voir son cours et l'ancienneté de ce cours",
  'xp.search.viewValidator': "Voir dans l'ensemble des validateurs",
  'xp.feed.ledeSimple': "L'argent qui circule sur le réseau Yamale, et les décisions qui ont été exécutées. Cherchez ci-dessus un compte, un paiement ou une devise.",
  'xp.feed.ledeExpert': "Chaque message que la chaîne a accepté ou refusé, du plus récent au plus ancien, avec la charge brute à un clic.",
  'xp.feed.blocks': 'Blocs récents',
  'xp.feed.loadingActivity': "Lecture de l'activité récente",
  'xp.feed.loadingBlocks': 'Lecture des blocs',
  'xp.feed.emptyBlock': 'vide',
  'xp.search.storedAs': 'Stocké comme',
  'xp.search.decimals': '{count} décimales',
  'xp.search.noneIssued': 'aucune émission',
  'xp.search.operator': 'Opérateur',
  'xp.search.validator': 'Validateur',
  'xp.search.jailed': 'Exclu',
  'xp.search.signing': 'Signe',
  'xp.search.idNeverRepointed': "Un identifiant est délivré lorsqu'un compte est rattaché à un pays, et il n'est jamais réattribué ensuite.",

  // --- information design ---------------------------------------------------
  'xp.kpi.transactions': 'Transactions',
  'xp.kpi.transactionsHint': 'acceptées depuis la genèse, selon le décompte du nœud',
  'xp.kpi.outcomes': 'Effets',
  'xp.kpi.outcomesHint': "de l'argent a circulé, ou une décision a pris effet",
  'xp.kpi.routine': 'Administration',
  'xp.kpi.routineHint': "cours de l'oracle et gestion des paramètres",
  'xp.kpi.parties': 'Comptes concernés',
  'xp.kpi.partiesHint': 'parties distinctes nommées dans cette fenêtre',
  'xp.kpi.window': 'sur les {count} dernières lues',
  'xp.kpi.unread': "le nœud n'a pas répondu",
  'xp.fold.everyday': 'Courant',
  'xp.fold.everything': 'Tout',
  'xp.fold.blocks': 'Blocs',
  'xp.feed.routineOnlyTitle': "Rien que de l'administration dans cette fenêtre",
  'xp.feed.routineOnlyBody':
    "La chaîne a accepté ici {count} messages, et tous relevaient de l'administration — soumissions de cours par l'oracle et lectures de paramètres. Aucun argent n'a circulé. Ils figurent sous Tout.",

  // --- staking --------------------------------------------------------------
  'xp.stake.lede':
    "Bloquer des jetons auprès d'un validateur sécurise le réseau et rapporte une part des nouvelles émissions et des frais de transaction.",
  'xp.stake.noticeTitle': 'Vos jetons ne sont pas disponibles tant qu’ils sont misés.',
  'xp.stake.noticeBody':
    "Le retrait prend {delay} à compter de votre demande, et les jetons ne rapportent rien pendant ce délai. Si un validateur se comporte mal, une partie de ce que vous avez misé chez lui peut être perdue.",
  'xp.stake.securing': "dans l'ensemble actif",
  'xp.stake.delay': 'Délai de retrait',
  'xp.stake.afterAsking': 'à compter de votre demande',
  'xp.stake.largest': 'Plus gros validateur',
  'xp.stake.largestHint': '{name} détient cette part de la mise active',
  'xp.stake.concentration': 'Qui tient la chaîne',
  'xp.stake.concLede':
    "Un bloc exige les deux tiers du pouvoir de vote pour être validé. Un seul détenteur au-dessus du tiers peut l'empêcher à lui seul ; un seul détenteur au-dessus des deux tiers peut le valider à lui seul.",
  'xp.stake.overThird': 'plus du tiers — peut bloquer un bloc à lui seul',
  'xp.stake.overTwoThirds': 'plus des deux tiers — peut valider un bloc à lui seul',
  'xp.stake.cumulative': 'Cumul',
  'xp.stake.others': 'Tous les autres',
  'xp.stake.ofRewards': 'de vos récompenses',
  'xp.stake.unnamed': 'sans nom',
  'xp.stake.noneTitle': 'Aucun validateur actif',
  'xp.stake.noneBody': "La chaîne ne déclare personne dans l'ensemble actif : rien ne la sécurise.",
  'xp.stake.enterAmount': 'Saisissez un montant.',
  'xp.stake.action': 'Miser',
  'xp.stake.amountFor': 'Montant de YML à miser auprès de {name}',
  'xp.stake.consequence':
    "{amount} cessent d'être disponibles pour vous. Les récupérer prend {delay} à compter de votre demande, et ils ne rapportent rien pendant ce délai. Si ce validateur se comporte mal, une partie peut être perdue.",
};

export const ar: typeof en = {
  'xp.health.live': 'تعمل',
  'xp.health.slow': 'متأخّرة',
  'xp.health.stopped': 'متوقّفة',
  'xp.health.catchingUp': 'تلحق بالركب',
  'xp.health.unreachable': 'لا استجابة',
  'xp.health.unknown': 'جارٍ التحقّق',
  'xp.health.lastBlock': 'آخر كتلة {age}',
  'xp.health.noBlockTime': 'بلا طابع زمني',

  'xp.status.height': 'الكتلة',
  'xp.status.blockTime': 'زمن الكتلة',
  'xp.status.validators': 'المدقّقون',
  'xp.status.bonded': 'الحصص المرتبطة',
  'xp.status.seconds': '{value} ثانية',
  'xp.status.median': 'وسيط {count}',
  'xp.status.ofSupply': '{percent} من المعروض',
  'xp.status.fragile': 'فقدان واحد يوقف السلسلة',
  'xp.status.spare': 'يمكن فقدان {count}',
  'xp.status.concentrated': 'الحصص مركّزة',

  'xp.stopped.title': 'توقّفت هذه السلسلة عن إنتاج الكتل.',
  'xp.stopped.body':
    'كل ما يلي هو الحالة عند الكتلة {height}، مقروءة بسؤال العقدة عن هذا الارتفاع تحديدًا — وهو الشيء الوحيد الذي تجيب عنه عقدة متوقّفة. البيانات صحيحة وليست حديثة.',
  'xp.stopped.bodyPlain':
    'آخر كتلة كانت {age}. كل ما يلي تاريخي، ولا يمكن تسوية أي شيء جديد قبل استئناف الكتل.',
  'xp.unreachable.title': 'لا يستطيع المستكشف الوصول إلى السلسلة.',
  'xp.unreachable.body':
    'قد يكون السبب العقدة أو هذا الاتصال — من هنا يتشابه الأمران. تُعيد الصفحة المحاولة تلقائيًا وتتعافى عند عودة الاتصال.',
  'xp.catchingUp.title': 'لا تزال هذه العقدة تعيد تشغيل السلسلة.',
  'xp.catchingUp.body':
    'تجيب من الكتلة {height}، وهي متأخّرة عن الطرف. الأرقام أدناه حقيقية وغير مكتملة.',

  'xp.feed.title': 'ما الذي حدث',
  'xp.feed.outcome': 'تمّ',
  'xp.feed.step': 'قيد التنفيذ',
  'xp.feed.routine': 'إدارة داخلية',
  'xp.feed.refused': 'مرفوض',
  'xp.feed.approved': '{yes} موافقات من {total}',
  'xp.feed.request': 'الطلب {id}',
  'xp.feed.votedOn': 'صوّت {option} على {what}',
  'xp.feed.ranFailed': 'تمّت الموافقة، لكن الإجراء نفسه رُفض',
  'xp.feed.whyRefused': 'سبب الرفض',
  'xp.feed.rawMessage': 'الرسالة الأصلية',
  'xp.feed.showAll': 'إظهار الإدارة الداخلية',
  'xp.feed.showEveryday': 'إخفاء الإدارة الداخلية',
  'xp.feed.emptyTitle': 'لم يحدث أي شيء بعد',
  'xp.feed.emptyHint': 'عندما تتحرّك الأموال على هذه الشبكة ستظهر هنا.',
  'xp.feed.emptyStopped': 'السلسلة متوقّفة، فلا يمكن أن يظهر هنا شيء جديد.',
  'xp.feed.noAmount': 'بلا مبلغ',
  'xp.feed.inBlock': 'الكتلة {height}',

  'xp.id.copy': 'نسخ',
  'xp.id.copied': 'تم النسخ',
  'xp.id.reveal': 'إظهار كاملًا',
  'xp.id.shorten': 'اختصار',

  'xp.search.label': 'ابحث في السلسلة',
  'xp.search.placeholder': 'حساب أو معرّف أو معاملة أو كتلة أو عملة أو مدقّق',
  'xp.search.looking': 'البحث عن {term}',
  'xp.search.notFound': 'لا شيء على هذه السلسلة يطابق ذلك',
  'xp.search.notFoundHint': 'يمكنك لصق أي من هذه، ولا حاجة لغيرها:',
  'xp.search.kindAddress': 'عنوان حساب',
  'xp.search.kindUserId': 'معرّف يمالي، مثل NG-K3M9-7QRT-5',
  'xp.search.kindTx': 'بصمة معاملة',
  'xp.search.kindHeight': 'رقم كتلة',
  'xp.search.kindDenom': 'رمز عملة، مثل NGN',
  'xp.search.kindValidator': 'اسم مدقّق',
  'xp.search.noAccountForId': 'لا يوجد حساب على هذه السلسلة يحمل المعرّف {id}.',
  'xp.search.currency': 'العملة',
  'xp.search.supply': 'المتداول',
  'xp.search.viewPrices': 'اعرض سعرها وعمر هذا السعر',
  'xp.search.viewValidator': 'اعرضه ضمن مجموعة المدقّقين',
  'xp.feed.ledeSimple': 'حركة الأموال على شبكة يمالي، والقرارات التي نُفّذت. ابحث أعلاه عن حساب أو دفعة أو عملة.',
  'xp.feed.ledeExpert': 'كل رسالة قبلتها السلسلة أو رفضتها، من الأحدث إلى الأقدم، والحمولة الأصلية على بعد نقرة واحدة.',
  'xp.feed.blocks': 'الكتل الأخيرة',
  'xp.feed.loadingActivity': 'قراءة النشاط الأخير',
  'xp.feed.loadingBlocks': 'قراءة الكتل',
  'xp.feed.emptyBlock': 'فارغة',
  'xp.search.storedAs': 'مخزّنة بصيغة',
  'xp.search.decimals': '{count} منازل عشرية',
  'xp.search.noneIssued': 'لم يُصدر شيء',
  'xp.search.operator': 'المشغّل',
  'xp.search.validator': 'مدقّق',
  'xp.search.jailed': 'موقوف',
  'xp.search.signing': 'يوقّع',
  'xp.search.idNeverRepointed': 'يُصدر المعرّف عند تسجيل الحساب في بلد، ولا يُعاد توجيهه بعد إصداره أبدًا.',

  // --- information design ---------------------------------------------------
  'xp.kpi.transactions': 'المعاملات',
  'xp.kpi.transactionsHint': 'المقبولة منذ التكوين، بحسب عدّ العقدة',
  'xp.kpi.outcomes': 'النتائج',
  'xp.kpi.outcomesHint': 'تحرّكت أموال، أو نفذ قرار',
  'xp.kpi.routine': 'إدارة داخلية',
  'xp.kpi.routineHint': 'أسعار المُوَجِّه وضبط المعاملات الداخلية',
  'xp.kpi.parties': 'الحسابات المعنيّة',
  'xp.kpi.partiesHint': 'أطراف متمايزة وردت في هذه النافذة',
  'xp.kpi.window': 'من آخر {count} قُرئت',
  'xp.kpi.unread': 'لم تستجب العقدة',
  'xp.fold.everyday': 'اليومي',
  'xp.fold.everything': 'كل شيء',
  'xp.fold.blocks': 'الكتل',
  'xp.feed.routineOnlyTitle': 'لا شيء سوى الإدارة الداخلية في هذه النافذة',
  'xp.feed.routineOnlyBody':
    'قبلت السلسلة هنا {count} رسالة، وكانت كلها إدارية — إرسال أسعار من المُوَجِّه وقراءات معاملات. لم تتحرّك أي أموال. تجدها تحت «كل شيء».',

  // --- staking --------------------------------------------------------------
  'xp.stake.lede':
    'حجز الرموز لدى مُثبِّت يؤمّن الشبكة ويدرّ حصّة من الإصدار الجديد ومن رسوم المعاملات.',
  'xp.stake.noticeTitle': 'رموزك غير متاحة ما دامت محجوزة.',
  'xp.stake.noticeBody':
    'يستغرق السحب {delay} من لحظة الطلب، ولا تدرّ شيئاً خلال تلك المدة. وإذا أساء المُثبِّت التصرّف فقد يضيع جزء ممّا حجزته لديه.',
  'xp.stake.securing': 'ضمن المجموعة النشطة',
  'xp.stake.delay': 'مهلة السحب',
  'xp.stake.afterAsking': 'من لحظة طلب السحب',
  'xp.stake.largest': 'أكبر مُثبِّت',
  'xp.stake.largestHint': 'يملك {name} هذه الحصّة من الحجز النشط',
  'xp.stake.concentration': 'من يمسك السلسلة',
  'xp.stake.concLede':
    'تحتاج الكتلة إلى ثلثي قوة التصويت كي تُعتمد. ومن يملك أكثر من الثلث يستطيع تعطيل ذلك وحده؛ ومن يملك أكثر من الثلثين يستطيع اعتمادها وحده.',
  'xp.stake.overThird': 'أكثر من الثلث — يعطّل كتلة وحده',
  'xp.stake.overTwoThirds': 'أكثر من الثلثين — يعتمد كتلة وحده',
  'xp.stake.cumulative': 'التراكم',
  'xp.stake.others': 'الباقون جميعاً',
  'xp.stake.ofRewards': 'من مكافآتك',
  'xp.stake.unnamed': 'بلا اسم',
  'xp.stake.noneTitle': 'لا مُثبِّتين نشطين',
  'xp.stake.noneBody': 'لا تعلن السلسلة عن أحد في المجموعة النشطة، فلا شيء يؤمّنها.',
  'xp.stake.enterAmount': 'أدخل مبلغاً.',
  'xp.stake.action': 'احجز',
  'xp.stake.amountFor': 'مبلغ YML للحجز لدى {name}',
  'xp.stake.consequence':
    'يتوقّف {amount} عن كونه متاحاً لك. واستعادته تستغرق {delay} من لحظة الطلب، ولا يدرّ شيئاً خلال تلك المدة. وإذا أساء هذا المُثبِّت التصرّف فقد يضيع جزء منه.',
};

export const pt: typeof en = {
  'xp.health.live': 'Ativa',
  'xp.health.slow': 'Atrasada',
  'xp.health.stopped': 'Parada',
  'xp.health.catchingUp': 'A sincronizar',
  'xp.health.unreachable': 'Sem resposta',
  'xp.health.unknown': 'A verificar',
  'xp.health.lastBlock': 'último bloco {age}',
  'xp.health.noBlockTime': 'sem data e hora',

  'xp.status.height': 'Bloco',
  'xp.status.blockTime': 'Tempo de bloco',
  'xp.status.validators': 'Validadores',
  'xp.status.bonded': 'Participação vinculada',
  'xp.status.seconds': '{value} s',
  'xp.status.median': 'mediana de {count}',
  'xp.status.ofSupply': '{percent} da oferta',
  'xp.status.fragile': 'perder um para a cadeia',
  'xp.status.spare': '{count} podem cair',
  'xp.status.concentrated': 'a participação está concentrada',

  'xp.stopped.title': 'Esta cadeia deixou de produzir blocos.',
  'xp.stopped.body':
    'Tudo abaixo é o estado no bloco {height}, lido pedindo ao nó exatamente essa altura — a única coisa que um nó parado responde. É exato e não é atual.',
  'xp.stopped.bodyPlain':
    'O último bloco foi {age}. Tudo abaixo é histórico e nada de novo pode ser liquidado até os blocos voltarem.',
  'xp.unreachable.title': 'O explorador não consegue alcançar a cadeia.',
  'xp.unreachable.body':
    'Pode ser o nó ou pode ser esta ligação — daqui, as duas parecem iguais. A página tenta de novo sozinha e recupera quando a ligação voltar.',
  'xp.catchingUp.title': 'Este nó ainda está a reproduzir a cadeia.',
  'xp.catchingUp.body':
    'Responde a partir do bloco {height}, atrás da ponta. Os números abaixo são reais e incompletos.',

  'xp.feed.title': 'O que aconteceu',
  'xp.feed.outcome': 'Feito',
  'xp.feed.step': 'Em curso',
  'xp.feed.routine': 'Administração',
  'xp.feed.refused': 'Recusado',
  'xp.feed.approved': '{yes} de {total} aprovações',
  'xp.feed.request': 'pedido {id}',
  'xp.feed.votedOn': 'votou {option} em {what}',
  'xp.feed.ranFailed': 'aprovado, mas a própria ação foi recusada',
  'xp.feed.whyRefused': 'Motivo da recusa',
  'xp.feed.rawMessage': 'Mensagem original',
  'xp.feed.showAll': 'Mostrar administração',
  'xp.feed.showEveryday': 'Ocultar administração',
  'xp.feed.emptyTitle': 'Ainda não aconteceu nada',
  'xp.feed.emptyHint': 'Quando o dinheiro se mover nesta rede, aparece aqui.',
  'xp.feed.emptyStopped': 'A cadeia está parada, por isso nada de novo pode aparecer aqui.',
  'xp.feed.noAmount': 'sem montante',
  'xp.feed.inBlock': 'bloco {height}',

  'xp.id.copy': 'Copiar',
  'xp.id.copied': 'Copiado',
  'xp.id.reveal': 'Mostrar por inteiro',
  'xp.id.shorten': 'Abreviar',

  'xp.search.label': 'Pesquisar na cadeia',
  'xp.search.placeholder': 'Conta, identificador, transação, bloco, moeda ou validador',
  'xp.search.looking': 'A procurar {term}',
  'xp.search.notFound': 'Nada nesta cadeia corresponde a isso',
  'xp.search.notFoundHint': 'Pode colar qualquer um destes, e mais nada é necessário:',
  'xp.search.kindAddress': 'um endereço de conta',
  'xp.search.kindUserId': 'um identificador Yamale, como NG-K3M9-7QRT-5',
  'xp.search.kindTx': 'uma impressão de transação',
  'xp.search.kindHeight': 'um número de bloco',
  'xp.search.kindDenom': 'um código de moeda, como NGN',
  'xp.search.kindValidator': 'o nome de um validador',
  'xp.search.noAccountForId': 'Nenhuma conta desta cadeia tem o identificador {id}.',
  'xp.search.currency': 'Moeda',
  'xp.search.supply': 'Em circulação',
  'xp.search.viewPrices': 'Ver a sua cotação e a idade dessa cotação',
  'xp.search.viewValidator': 'Ver no conjunto de validadores',
  'xp.feed.ledeSimple': 'Dinheiro a mover-se na rede Yamale, e decisões que foram executadas. Pesquise acima uma conta, um pagamento ou uma moeda.',
  'xp.feed.ledeExpert': 'Todas as mensagens que a cadeia aceitou ou recusou, da mais recente para a mais antiga, com a carga original a um clique.',
  'xp.feed.blocks': 'Blocos recentes',
  'xp.feed.loadingActivity': 'A ler a atividade recente',
  'xp.feed.loadingBlocks': 'A ler os blocos',
  'xp.feed.emptyBlock': 'vazio',
  'xp.search.storedAs': 'Armazenada como',
  'xp.search.decimals': '{count} casas decimais',
  'xp.search.noneIssued': 'nada emitido',
  'xp.search.operator': 'Operador',
  'xp.search.validator': 'Validador',
  'xp.search.jailed': 'Excluído',
  'xp.search.signing': 'A assinar',
  'xp.search.idNeverRepointed': 'Um identificador é emitido quando uma conta é registada num país, e nunca é reatribuído depois disso.',

  // --- information design ---------------------------------------------------
  'xp.kpi.transactions': 'Transações',
  'xp.kpi.transactionsHint': 'aceites desde a génese, conforme a contagem do nó',
  'xp.kpi.outcomes': 'Efeitos',
  'xp.kpi.outcomesHint': 'houve movimento de dinheiro, ou uma decisão produziu efeito',
  'xp.kpi.routine': 'Administração',
  'xp.kpi.routineHint': 'cotações do oráculo e gestão de parâmetros',
  'xp.kpi.parties': 'Contas envolvidas',
  'xp.kpi.partiesHint': 'partes distintas nomeadas nesta janela',
  'xp.kpi.window': 'das últimas {count} lidas',
  'xp.kpi.unread': 'o nó não respondeu',
  'xp.fold.everyday': 'Corrente',
  'xp.fold.everything': 'Tudo',
  'xp.fold.blocks': 'Blocos',
  'xp.feed.routineOnlyTitle': 'Só administração nesta janela',
  'xp.feed.routineOnlyBody':
    'A cadeia aceitou aqui {count} mensagens e todas eram de administração — envios de cotações pelo oráculo e leituras de parâmetros. Não houve movimento de dinheiro. Estão em Tudo.',

  // --- staking --------------------------------------------------------------
  'xp.stake.lede':
    'Bloquear fichas junto de um validador protege a rede e rende uma parte da nova emissão e das taxas de transação.',
  'xp.stake.noticeTitle': 'As suas fichas não estão disponíveis enquanto estiverem em garantia.',
  'xp.stake.noticeBody':
    'O levantamento demora {delay} a contar do momento em que o pede, e nada rendem durante esse período. Se um validador se comportar mal, parte do que lhe entregou pode perder-se.',
  'xp.stake.securing': 'no conjunto ativo',
  'xp.stake.delay': 'Prazo de levantamento',
  'xp.stake.afterAsking': 'a contar do seu pedido',
  'xp.stake.largest': 'Maior validador',
  'xp.stake.largestHint': '{name} detém esta parte da garantia ativa',
  'xp.stake.concentration': 'Quem segura a cadeia',
  'xp.stake.concLede':
    'Um bloco precisa de dois terços do poder de voto para ser aprovado. Um único detentor acima de um terço pode impedi-lo sozinho; um único detentor acima de dois terços pode aprová-lo sozinho.',
  'xp.stake.overThird': 'acima de um terço — bloqueia um bloco sozinho',
  'xp.stake.overTwoThirds': 'acima de dois terços — aprova um bloco sozinho',
  'xp.stake.cumulative': 'Acumulado',
  'xp.stake.others': 'Todos os restantes',
  'xp.stake.ofRewards': 'das suas recompensas',
  'xp.stake.unnamed': 'sem nome',
  'xp.stake.noneTitle': 'Nenhum validador ativo',
  'xp.stake.noneBody': 'A cadeia não declara ninguém no conjunto ativo, por isso nada a protege.',
  'xp.stake.enterAmount': 'Indique um montante.',
  'xp.stake.action': 'Bloquear',
  'xp.stake.amountFor': 'Montante de YML a bloquear junto de {name}',
  'xp.stake.consequence':
    '{amount} deixam de estar disponíveis para si. Recuperá-los demora {delay} a contar do momento em que o pede, e nada rendem durante esse período. Se este validador se comportar mal, parte pode perder-se.',
};

export const sw: typeof en = {
  'xp.health.live': 'Inaendelea',
  'xp.health.slow': 'Imechelewa',
  'xp.health.stopped': 'Imesimama',
  'xp.health.catchingUp': 'Inajipatanisha',
  'xp.health.unreachable': 'Hakuna jibu',
  'xp.health.unknown': 'Inahakiki',
  'xp.health.lastBlock': 'kizuizi cha mwisho {age}',
  'xp.health.noBlockTime': 'hakuna muda',

  'xp.status.height': 'Kizuizi',
  'xp.status.blockTime': 'Muda wa kizuizi',
  'xp.status.validators': 'Wathibitishaji',
  'xp.status.bonded': 'Hisa zilizofungwa',
  'xp.status.seconds': 'sekunde {value}',
  'xp.status.median': 'wastani wa {count}',
  'xp.status.ofSupply': '{percent} ya jumla',
  'xp.status.fragile': 'kupoteza mmoja husimamisha mnyororo',
  'xp.status.spare': '{count} wanaweza kupotea',
  'xp.status.concentrated': 'hisa zimejikita mahali pamoja',

  'xp.stopped.title': 'Mnyororo huu umeacha kutoa vizuizi.',
  'xp.stopped.body':
    'Kila kitu hapa chini ni hali kwenye kizuizi {height}, kilichosomwa kwa kuomba nodi urefu huo hasa — jambo pekee ambalo nodi iliyosimama hujibu. Ni sahihi, na si cha sasa.',
  'xp.stopped.bodyPlain':
    'Kizuizi cha mwisho kilikuwa {age}. Kila kitu hapa chini ni cha kihistoria, na hakuna kipya kinaweza kukamilika hadi vizuizi vianze tena.',
  'xp.unreachable.title': 'Kichunguzi hakiwezi kufikia mnyororo.',
  'xp.unreachable.body':
    'Inaweza kuwa nodi au inaweza kuwa muunganisho huu — kutoka hapa vinaonekana sawa. Ukurasa hujaribu tena wenyewe na hurejea muunganisho unaporejea.',
  'xp.catchingUp.title': 'Nodi hii bado inarudia mnyororo.',
  'xp.catchingUp.body':
    'Hujibu kutoka kizuizi {height}, nyuma ya ncha. Namba hapa chini ni za kweli na hazikamilika.',

  'xp.feed.title': 'Kilichotokea',
  'xp.feed.outcome': 'Imekamilika',
  'xp.feed.step': 'Inaendelea',
  'xp.feed.routine': 'Utawala',
  'xp.feed.refused': 'Imekataliwa',
  'xp.feed.approved': 'idhini {yes} kati ya {total}',
  'xp.feed.request': 'ombi {id}',
  'xp.feed.votedOn': 'alipiga kura {option} kuhusu {what}',
  'xp.feed.ranFailed': 'iliidhinishwa, lakini hatua yenyewe ilikataliwa',
  'xp.feed.whyRefused': 'Sababu ya kukataliwa',
  'xp.feed.rawMessage': 'Ujumbe halisi',
  'xp.feed.showAll': 'Onyesha utawala',
  'xp.feed.showEveryday': 'Ficha utawala',
  'xp.feed.emptyTitle': 'Hakuna kilichotokea bado',
  'xp.feed.emptyHint': 'Pesa zinapohama kwenye mtandao huu, zitaonekana hapa.',
  'xp.feed.emptyStopped': 'Mnyororo umesimama, hivyo hakuna kipya kinaweza kuonekana hapa.',
  'xp.feed.noAmount': 'hakuna kiasi',
  'xp.feed.inBlock': 'kizuizi {height}',

  'xp.id.copy': 'Nakili',
  'xp.id.copied': 'Imenakiliwa',
  'xp.id.reveal': 'Onyesha yote',
  'xp.id.shorten': 'Fupisha',

  'xp.search.label': 'Tafuta kwenye mnyororo',
  'xp.search.placeholder': 'Akaunti, kitambulisho, muamala, kizuizi, sarafu au mthibitishaji',
  'xp.search.looking': 'Inatafuta {term}',
  'xp.search.notFound': 'Hakuna kitu kwenye mnyororo huu kinacholingana',
  'xp.search.notFoundHint': 'Unaweza kubandika chochote cha haya, na hakuna kingine kinahitajika:',
  'xp.search.kindAddress': 'anwani ya akaunti',
  'xp.search.kindUserId': 'kitambulisho cha Yamale, kama NG-K3M9-7QRT-5',
  'xp.search.kindTx': 'alama ya muamala',
  'xp.search.kindHeight': 'namba ya kizuizi',
  'xp.search.kindDenom': 'kodi ya sarafu, kama NGN',
  'xp.search.kindValidator': 'jina la mthibitishaji',
  'xp.search.noAccountForId': 'Hakuna akaunti kwenye mnyororo huu yenye kitambulisho {id}.',
  'xp.search.currency': 'Sarafu',
  'xp.search.supply': 'Inayozunguka',
  'xp.search.viewPrices': 'Ona bei yake na umri wa bei hiyo',
  'xp.search.viewValidator': 'Ona kwenye kundi la wathibitishaji',
  'xp.feed.ledeSimple': 'Pesa zinazohama kwenye mtandao wa Yamale, na maamuzi yaliyotekelezwa. Tafuta juu akaunti, malipo au sarafu.',
  'xp.feed.ledeExpert': 'Kila ujumbe ambao mnyororo umekubali au kukataa, mpya kwanza, na maandishi halisi kwa mbofyo mmoja.',
  'xp.feed.blocks': 'Vizuizi vya karibuni',
  'xp.feed.loadingActivity': 'Inasoma shughuli za karibuni',
  'xp.feed.loadingBlocks': 'Inasoma vizuizi',
  'xp.feed.emptyBlock': 'tupu',
  'xp.search.storedAs': 'Imehifadhiwa kama',
  'xp.search.decimals': 'desimali {count}',
  'xp.search.noneIssued': 'hakuna iliyotolewa',
  'xp.search.operator': 'Mwendeshaji',
  'xp.search.validator': 'Mthibitishaji',
  'xp.search.jailed': 'Amefungiwa',
  'xp.search.signing': 'Anatia sahihi',
  'xp.search.idNeverRepointed': 'Kitambulisho hutolewa akaunti inapoandikishwa katika nchi, na hakielekezwi tena baada ya kutolewa.',

  // --- information design ---------------------------------------------------
  'xp.kpi.transactions': 'Miamala',
  'xp.kpi.transactionsHint': 'iliyokubaliwa tangu mwanzo, kwa hesabu ya nodi',
  'xp.kpi.outcomes': 'Matokeo',
  'xp.kpi.outcomesHint': 'pesa zilihama, au uamuzi ukatekelezwa',
  'xp.kpi.routine': 'Utawala',
  'xp.kpi.routineHint': 'bei za orakli na usimamizi wa vigezo',
  'xp.kpi.parties': 'Akaunti zilizohusika',
  'xp.kpi.partiesHint': 'pande tofauti zilizotajwa katika dirisha hili',
  'xp.kpi.window': 'kati ya {count} za mwisho zilizosomwa',
  'xp.kpi.unread': 'nodi haikujibu',
  'xp.fold.everyday': 'Ya kawaida',
  'xp.fold.everything': 'Yote',
  'xp.fold.blocks': 'Vizuizi',
  'xp.feed.routineOnlyTitle': 'Utawala tu katika dirisha hili',
  'xp.feed.routineOnlyBody':
    'Mnyororo ulikubali hapa ujumbe {count} na wote ulikuwa wa utawala — mawasilisho ya bei kutoka orakli na usomaji wa vigezo. Hakuna pesa zilizohama. Zipo chini ya Yote.',

  // --- staking --------------------------------------------------------------
  'xp.stake.lede':
    'Kufunga sarafu kwa mthibitishaji hulinda mtandao na huleta sehemu ya utoaji mpya na ada za miamala.',
  'xp.stake.noticeTitle': 'Sarafu zako hazipatikani zikiwa zimefungwa.',
  'xp.stake.noticeBody':
    'Kuzitoa huchukua {delay} tangu unapoomba, na hazileti chochote katika kipindi hicho. Mthibitishaji akikosea, sehemu ya ulichofunga kwake inaweza kupotea.',
  'xp.stake.securing': 'katika kundi linalofanya kazi',
  'xp.stake.delay': 'Muda wa kutoa',
  'xp.stake.afterAsking': 'tangu unapoomba kutoa',
  'xp.stake.largest': 'Mthibitishaji mkubwa zaidi',
  'xp.stake.largestHint': '{name} ana sehemu hii ya kilichofungwa',
  'xp.stake.concentration': 'Nani anashikilia mnyororo',
  'xp.stake.concLede':
    'Kizuizi kinahitaji theluthi mbili za nguvu ya kupiga kura ili kuidhinishwa. Mmiliki mmoja aliye juu ya theluthi moja anaweza kukizuia peke yake; mmiliki mmoja aliye juu ya theluthi mbili anaweza kukiidhinisha peke yake.',
  'xp.stake.overThird': 'zaidi ya theluthi moja — anazuia kizuizi peke yake',
  'xp.stake.overTwoThirds': 'zaidi ya theluthi mbili — anaidhinisha kizuizi peke yake',
  'xp.stake.cumulative': 'Jumla inayoongezeka',
  'xp.stake.others': 'Wengine wote',
  'xp.stake.ofRewards': 'ya zawadi zako',
  'xp.stake.unnamed': 'bila jina',
  'xp.stake.noneTitle': 'Hakuna wathibitishaji hai',
  'xp.stake.noneBody': 'Mnyororo hautaji mtu yeyote katika kundi linalofanya kazi, hivyo hakuna kinachoulinda.',
  'xp.stake.enterAmount': 'Weka kiasi.',
  'xp.stake.action': 'Funga',
  'xp.stake.amountFor': 'Kiasi cha YML cha kufunga kwa {name}',
  'xp.stake.consequence':
    '{amount} zinaacha kupatikana kwako. Kuzirudisha huchukua {delay} tangu unapoomba, na hazileti chochote katika kipindi hicho. Mthibitishaji huyu akikosea, sehemu yake inaweza kupotea.',
};

export const CATALOGUES: Record<string, typeof en> = { en, fr, ar, pt, sw };

/**
 * Layers the explorer's keys onto whatever the shared catalogue registered.
 *
 * Called once, before the first render, alongside the SDK's own `registerAll()`.
 */
export function registerExplorerStrings(): void {
  for (const [locale, catalogue] of Object.entries(CATALOGUES)) {
    register(locale, catalogue);
  }
}
