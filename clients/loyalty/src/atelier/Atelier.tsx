import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { chainHead, findByInscription, jewelsHeldBy, normaliseInscription, t } from '@yamale/chain';

import { useSession } from '../account/session.tsx';
import { AppBar, Loading, Problem, Section, useLoad } from '../ui.tsx';
import { JewelCard } from '../verify/JewelCard.tsx';
import { useRole } from './role.ts';

const NO_STOCK = { ok: true as const, value: [] as string[], height: 0 };

export function Atelier() {
  const session = useSession();
  const [role] = useRole(session.address);
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [missing, setMissing] = useState(false);
  const [findFailed, setFindFailed] = useState<{ reason: string; detail: string } | null>(null);
  const business = role.state === 'role' ? role.jeweler.business : null;
  const [stock] = useLoad(() => (business ? jewelsHeldBy(business) : Promise.resolve(NO_STOCK)), [business]);
  // An offer past the chain's clock is no offer: the cards wait for that clock, not the phone's.
  const [head] = useLoad(() => chainHead(), []);

  if (role.state === 'loading') return <><AppBar title={t('loyalty.atelierTitle')} eyebrow={t('loyalty.title')} /><main className="y-screen"><Loading /></main></>;
  if (role.state !== 'role') {
    return (
      <>
        <AppBar title={t('loyalty.atelierTitle')} eyebrow={t('loyalty.title')} />
        <main className="y-screen">
          {role.state === 'failed' ? <Problem outcome={role.outcome} /> : (
            <>
              <p className="note">{t('loyalty.notAJeweler')}</p>
              <Link className="btn" to="/apply">{t('loyalty.applyLink')}</Link>
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
      <AppBar title={j.name} eyebrow={t('loyalty.atelierTitle')} />
      <main className="y-screen">
        <div className="actions">
          <span className={`y-chip ${role.active ? 'y-chip--ok' : 'y-chip--bad'}`}>
            {role.active ? t('loyalty.jewelerActive') : t('loyalty.jewelerSuspended')}
          </span>
          {role.business && <span className="y-chip y-chip--brass">{t('loyalty.youAreBusiness')}</span>}
        </div>
        {!role.active && <p className="banner banner--warn">{t('loyalty.suspendedNote')}</p>}
        {role.business && !role.staff && <p className="hint">{t('loyalty.businessAddYourself')}</p>}

        <div className="actions">
          {role.staff && role.active && <Link className="btn btn--primary" to="/atelier/register">{t('loyalty.registerTitle')}</Link>}
          {role.staff && role.active && <Link className="btn" to="/atelier/redeem">{t('loyalty.redeemLink')}</Link>}
          {role.business && <Link className="btn" to="/atelier/look">{t('loyalty.lookLink')}</Link>}
          {role.business && <Link className="btn" to="/atelier/staff">{t('loyalty.staffLink')}</Link>}
        </div>

        <Section title={t('loyalty.findJewel')}>
          <form className="stack" onSubmit={find}>
            <input className="bigfield inscription" value={code} onChange={(e) => { setCode(e.target.value); setMissing(false); setFindFailed(null); }}
                   aria-label={t('loyalty.codeLabel')} autoCapitalize="characters" spellCheck={false} />
            <button className="btn">{t('loyalty.verifyButton')}</button>
            {missing && <p className="hint">{t('loyalty.notOurs')}</p>}
            {findFailed && <Problem outcome={findFailed} />}
          </form>
        </Section>

        <Section title={t('loyalty.stock')}>
          {!stock && <Loading />}
          {stock && !stock.ok && <Problem outcome={stock} />}
          {stock?.ok && stock.value.length === 0 && <p className="hint">{t('loyalty.noStock')}</p>}
          {stock?.ok && stock.value.length > 0 && head && !head.ok && <Problem outcome={head} />}
          {stock?.ok && stock.value.length > 0 && !head && <Loading />}
          {stock?.ok && stock.value.length > 0 && head?.ok && (
            <ul className="y-cards">{stock.value.map((id) => <JewelCard key={id} id={id} to={`/atelier/jewel/${id}`} now={head.value.time} onlyJeweler={j.id} />)}</ul>
          )}
        </Section>
      </main>
    </>
  );
}
