import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { chainHead, findByInscription, normaliseInscription, t } from '@yamale/chain';

import { useSession } from '../account/session.tsx';
import { AppBar, Loading, Problem, Section, useLoad } from '../ui.tsx';
import { HeldRow, loadHoldings } from '../owner/HeldList.tsx';
import { useRole } from './role.ts';

export function Atelier() {
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [missing, setMissing] = useState(false);
  const [findFailed, setFindFailed] = useState<{ reason: string; detail: string } | null>(null);
  const business = role.state === 'role' ? role.jeweler.business : null;
  // An offer past the chain's clock is no offer: the list waits for that clock, not the phone's.
  const [head] = useLoad(() => chainHead(), []);

  if (role.state === 'loading') return <><AppBar title={t('traceability.atelierTitle')} eyebrow={t('traceability.title')} /><main className="y-screen"><Loading /></main></>;
  if (role.state !== 'role') {
    return (
      <>
        <AppBar title={t('traceability.atelierTitle')} eyebrow={t('traceability.title')} />
        <main className="y-screen">
          {role.state === 'failed' ? <Problem outcome={role.outcome} /> : (
            <>
              <p className="note">{t('traceability.notAJeweler')}</p>
              <Link className="btn" to="/apply">{t('traceability.applyLink')}</Link>
            </>
          )}
        </main>
      </>
    );
  }
  const j = role.jeweler;

  async function find(e: FormEvent) {
    e.preventDefault();
    const n = normaliseInscription(code);
    if (!n) return;
    setFindFailed(null);
    const out = await findByInscription(n);
    if (!out.ok) { setFindFailed(out); return; }
    const mine = out.value.find((x) => x.jewelerId === j.id);
    if (mine) navigate(`/atelier/jewel/${mine.id}`); else setMissing(true);
  }

  return (
    <>
      <AppBar title={j.name} eyebrow={t('traceability.atelierTitle')} />
      <main className="y-screen">
        <div className="actions">
          <span className={`y-chip ${role.active ? 'y-chip--ok' : 'y-chip--bad'}`}>
            {role.active ? t('traceability.jewelerActive') : t('traceability.jewelerSuspended')}
          </span>
          {role.business && <span className="y-chip y-chip--brass">{t('traceability.youAreBusiness')}</span>}
        </div>
        {!role.active && <p className="banner banner--warn">{t('traceability.suspendedNote')}</p>}
        {role.business && !role.staff && <p className="hint">{t('traceability.businessAddYourself')}</p>}

        <div className="actions">
          {role.staff && role.active && <Link className="btn btn--primary" to="/atelier/register">{t('traceability.registerTitle')}</Link>}
          {role.active && <Link className="btn" to="/atelier/catalogue">{t('traceability.catalogueLink')}</Link>}
          {role.staff && role.active && <Link className="btn" to="/atelier/redeem">{t('traceability.redeemLink')}</Link>}
          {role.business && <Link className="btn" to="/atelier/look">{t('traceability.lookLink')}</Link>}
          {role.business && <Link className="btn" to="/atelier/staff">{t('traceability.staffLink')}</Link>}
        </div>

        <Section title={t('traceability.findJewel')}>
          <form className="stack" onSubmit={find}>
            <input className="bigfield inscription" value={code} onChange={(e) => { setCode(e.target.value); setMissing(false); setFindFailed(null); }}
                   aria-label={t('traceability.codeLabel')} autoCapitalize="characters" spellCheck={false} />
            <button className="btn">{t('traceability.verifyButton')}</button>
            {missing && <p className="hint">{t('traceability.notOurs')}</p>}
            {findFailed && <Problem outcome={findFailed} />}
          </form>
        </Section>

        <Section title={t('traceability.stock')}>
          {!head && <Loading />}
          {head && !head.ok && <Problem outcome={head} />}
          {head?.ok && business && <Stock business={business} jewelerId={j.id} now={head.value.time} />}
        </Section>
      </main>
    </>
  );
}

/** The shop's stock: what its business account holds of its own jewels, one row per design. */
function Stock({ business, jewelerId, now }: { business: string; jewelerId: string; now: number }) {
  // A business account can hold jewels of other jewelers; a jeweler's stock is its own.
  const [stock] = useLoad(() => loadHoldings(business, now, (h) => h.jewelerId === jewelerId), [business, jewelerId]);
  if (!stock) return <Loading />;
  if (!stock.ok) return <Problem outcome={stock} />;
  if (stock.value.length === 0) return <p className="hint">{t('traceability.noStock')}</p>;
  return <ul className="y-cards">{stock.value.map((g) => <HeldRow key={g.key} group={g} to={(id) => `/atelier/jewel/${id}`} />)}</ul>;
}
