import { Link } from 'react-router-dom';

export function NotFoundPage() {
  return (
    <div className="state">
      <div className="state__title">There is nothing at this address</div>
      <p className="small" style={{ maxWidth: '44ch', margin: '0.25rem auto 1rem' }}>
        The page you were looking for does not exist. Search above for an account, a transaction or a block
        number.
      </p>
      <Link to="/">Back to recent activity</Link>
    </div>
  );
}
