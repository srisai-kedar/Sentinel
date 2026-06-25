import { useEffect, useRef, useState } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from 'recharts';
import { fetchLimits, fetchStats, statsWebSocketURL, updateLimit, type LimitEntry, type Snapshot } from './api';

const COLORS = ['#22c55e', '#ef4444'];

export default function App() {
  const [stats, setStats] = useState<Snapshot | null>(null);
  const [history, setHistory] = useState<{ t: string; rps: number }[]>([]);
  const [limits, setLimits] = useState<LimitEntry[]>([]);
  const [route, setRoute] = useState('/api/test');
  const [capacity, setCapacity] = useState(10);
  const [refillRate, setRefillRate] = useState(1);
  const [algorithm, setAlgorithm] = useState('token_bucket');
  const [status, setStatus] = useState('');
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    fetchLimits().then(setLimits).catch(console.error);
    fetchStats().then(applySnapshot).catch(console.error);

    const ws = new WebSocket(statsWebSocketURL());
    wsRef.current = ws;
    ws.onmessage = (ev) => applySnapshot(JSON.parse(ev.data));
    ws.onerror = () => setStatus('WebSocket disconnected — polling fallback active');
    return () => ws.close();
  }, []);

  useEffect(() => {
    const id = setInterval(() => {
      fetchStats().then(applySnapshot).catch(() => undefined);
    }, 3000);
    return () => clearInterval(id);
  }, []);

  function applySnapshot(s: Snapshot) {
    setStats(s);
    setHistory((prev) => {
      const next = [...prev, { t: new Date(s.timestamp).toLocaleTimeString(), rps: s.requests_per_sec }];
      return next.slice(-30);
    });
  }

  async function onSave(e: React.FormEvent) {
    e.preventDefault();
    try {
      await updateLimit(route, {
        algorithm,
        capacity,
        refill_rate: refillRate,
        window_sec: 60,
      });
      setStatus(`Updated limits for ${route}`);
      setLimits(await fetchLimits());
    } catch (err) {
      setStatus(String(err));
    }
  }

  const ratio = stats
    ? [
        { name: 'Allowed', value: stats.allowed },
        { name: 'Blocked', value: stats.blocked },
      ]
    : [];

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: '2rem 1.5rem' }}>
      <header style={{ marginBottom: '2rem' }}>
        <p style={{ color: '#737373', margin: 0, fontSize: 14 }}>Sentinel Admin</p>
        <h1 style={{ margin: '0.25rem 0 0', fontSize: 28, fontWeight: 600 }}>Rate Limiter Dashboard</h1>
        {stats && (
          <p style={{ color: '#525252', marginTop: 8, fontSize: 13 }}>
            Instance: {stats.instance_id} · {stats.requests_per_sec.toFixed(1)} req/s
          </p>
        )}
      </header>

      <div className="grid grid-2">
        <div className="card">
          <h2 style={{ marginTop: 0, fontSize: 16, color: '#a3a3a3' }}>Requests / sec</h2>
          <ResponsiveContainer width="100%" height={240}>
            <LineChart data={history}>
              <XAxis dataKey="t" stroke="#525252" fontSize={11} />
              <YAxis stroke="#525252" fontSize={11} />
              <Tooltip contentStyle={{ background: '#171717', border: '1px solid #333' }} />
              <Line type="monotone" dataKey="rps" stroke="#3b82f6" strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>

        <div className="card">
          <h2 style={{ marginTop: 0, fontSize: 16, color: '#a3a3a3' }}>Allowed vs Blocked</h2>
          <ResponsiveContainer width="100%" height={240}>
            <PieChart>
              <Pie data={ratio} dataKey="value" nameKey="name" innerRadius={55} outerRadius={85}>
                {ratio.map((_, i) => (
                  <Cell key={i} fill={COLORS[i % COLORS.length]} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ background: '#171717', border: '1px solid #333' }} />
            </PieChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="grid grid-2" style={{ marginTop: '1rem' }}>
        <div className="card">
          <h2 style={{ marginTop: 0, fontSize: 16, color: '#a3a3a3' }}>Top Offenders</h2>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
            <thead>
              <tr style={{ color: '#737373', textAlign: 'left' }}>
                <th style={{ padding: '8px 4px' }}>Client</th>
                <th>Route</th>
                <th>Blocked</th>
                <th>Total</th>
              </tr>
            </thead>
            <tbody>
              {(stats?.top_offenders ?? []).map((o) => (
                <tr key={`${o.client}-${o.route}`} style={{ borderTop: '1px solid #262626' }}>
                  <td style={{ padding: '8px 4px', fontFamily: 'monospace' }}>{o.client}</td>
                  <td>{o.route}</td>
                  <td style={{ color: '#ef4444' }}>{o.blocked}</td>
                  <td>{o.total}</td>
                </tr>
              ))}
              {!stats?.top_offenders?.length && (
                <tr><td colSpan={4} style={{ padding: 12, color: '#525252' }}>No traffic yet</td></tr>
              )}
            </tbody>
          </table>
        </div>

        <div className="card">
          <h2 style={{ marginTop: 0, fontSize: 16, color: '#a3a3a3' }}>Live Config</h2>
          <form onSubmit={onSave} style={{ display: 'grid', gap: 10 }}>
            <label>
              Route
              <input value={route} onChange={(e) => setRoute(e.target.value)} style={inputStyle} />
            </label>
            <label>
              Algorithm
              <select value={algorithm} onChange={(e) => setAlgorithm(e.target.value)} style={inputStyle}>
                <option value="token_bucket">Token Bucket</option>
                <option value="sliding_window_log">Sliding Window Log</option>
                <option value="sliding_window_counter">Sliding Window Counter</option>
                <option value="leaky_bucket">Leaky Bucket</option>
              </select>
            </label>
            <label>
              Capacity
              <input type="number" value={capacity} onChange={(e) => setCapacity(+e.target.value)} style={inputStyle} />
            </label>
            <label>
              Refill rate (/sec)
              <input type="number" step="0.1" value={refillRate} onChange={(e) => setRefillRate(+e.target.value)} style={inputStyle} />
            </label>
            <button type="submit" style={buttonStyle}>Apply</button>
          </form>
          {status && <p style={{ fontSize: 13, color: '#a3a3a3' }}>{status}</p>}
          <div style={{ marginTop: 16, fontSize: 13, color: '#525252' }}>
            Active limits: {limits.map((l) => l.route).join(', ') || 'default only'}
          </div>
        </div>
      </div>
    </div>
  );
}

const inputStyle: React.CSSProperties = {
  display: 'block',
  width: '100%',
  marginTop: 4,
  padding: '8px 10px',
  background: '#0a0a0a',
  border: '1px solid #333',
  borderRadius: 8,
  color: '#e5e5e5',
};

const buttonStyle: React.CSSProperties = {
  marginTop: 8,
  padding: '10px 14px',
  background: '#2563eb',
  color: '#fff',
  border: 'none',
  borderRadius: 8,
  cursor: 'pointer',
  fontWeight: 600,
};
