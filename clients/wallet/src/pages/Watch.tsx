import { t } from '@yamale/chain';
import { Link, useNavigate } from 'react-router-dom';

import { AddressForm } from '../App.tsx';

/**
 * The landing screen, deliberately the harmless one.
 *
 * Most people arriving at a wallet want to know whether money arrived, not to
 * create a key. Watching needs nothing from them and risks nothing.
 */
export function WatchPage() {
  const navigate = useNavigate();

  return (
    <>
      <h1>{t('watch.title')}</h1>
      <p className="lede">{t('watch.lede')}</p>

      <section className="card">
        <AddressForm onSubmit={(address) => navigate(`/a/${address}`)} />
      </section>

      <p className="small muted">
        {/* Link, not a bare anchor: the app is served under /wallet, and a raw
            href skips the router's basename and lands on the site's origin
            instead. Every other link here goes through the router, which is why
            this was the only one that broke. */}
        {t('watch.needAccount')} <Link to="/create">{t('watch.createOne')}</Link> — {t('watch.phraseNote')}
      </p>
    </>
  );
}
