import { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import {
  IconAlertCircle, IconArrowRight, IconCheck, IconClock, IconClockPlus, IconCopy, IconCube,
  IconNetwork, IconPlayerPlay, IconPlayerStop, IconPlus, IconSettings, IconWorld, IconX,
} from '@tabler/icons-react';
import { relay, type Phase, type RelayState, type Settings } from './runtime';
import './style.css';

const phases: Record<Phase, string> = {
  idle: 'Stopped', starting: 'Starting', running: 'Online', stopping: 'Stopping', recoverable: 'Reconnect needed',
};

function deadlineOf(value: string) {
  const deadline = new Date(value);
  if (!Number.isFinite(deadline.getTime()) || deadline.getUTCFullYear() < 2000) return '—';
  return new Intl.DateTimeFormat('en', { dateStyle: 'short', timeStyle: 'short' }).format(deadline);
}

function App() {
  const [state, setState] = useState<RelayState>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [replaceDNS, setReplaceDNS] = useState(false);
  const [copied, setCopied] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const refresh = async () => setState(await relay.getState());

  useEffect(() => {
    let mounted = true;
    const poll = async () => {
      try {
        const value = await relay.getState();
        if (mounted) setState(value);
      } catch (failure) {
        if (mounted) setError(failure instanceof Error ? failure.message : String(failure));
      }
    };
    void poll();
    const timer = window.setInterval(poll, 1500);
    return () => { mounted = false; window.clearInterval(timer); };
  }, []);

  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError('');
    try {
      await action();
      await refresh();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setBusy(false);
    }
  }

  const phase = state?.Phase ?? 'idle';
  const controlsLocked = busy || phase === 'starting' || phase === 'stopping';
  const canStop = phase === 'running' || phase === 'recoverable';
  const canStart = Boolean(state?.Config.Domain && state.GoogleReady && state.CloudflareReady) && phase !== 'running';
  const problem = error || state?.Problem;
  const config = state?.Config;

  return (
    <div className="window">
      <header className="topbar">
        <h1><span className="brand-icon"><IconCube aria-hidden="true" /></span>Minecraft Proxy</h1>
        <div className="profile-controls">
          <select aria-label="Relay profile" value={state?.ActiveProfile || ''} disabled={!state || busy}
            onChange={(event) => {
              const profile = event.target.value;
              setReplaceDNS(false);
              void perform(() => relay.selectProfile(profile));
            }}>
            {!state && <option value="">—</option>}
            {state?.Profiles.map((profile) => (
              <option key={profile.ID} value={profile.ID}>
                {profile.Domain || 'New relay'}{profile.Phase !== 'idle' && ' · ' + phases[profile.Phase]}
              </option>
            ))}
          </select>
          <button className="icon-button" type="button" aria-label="Add relay profile" title="Add relay profile" disabled={!state || busy}
            onClick={() => void perform(async () => {
              await relay.addProfile();
              setReplaceDNS(false);
              setSettingsOpen(true);
            })}>
            <IconPlus aria-hidden="true" />
          </button>
          <button className="icon-button" type="button" aria-label="Settings" title="Settings" disabled={!state || controlsLocked} onClick={() => setSettingsOpen(true)}>
            <IconSettings aria-hidden="true" />
          </button>
        </div>
      </header>
      <main aria-label="Relay">
        <section className="relay-panel" aria-label="Connection">
          <div className="connection-mark"><IconWorld aria-hidden="true" /></div>
          <div className="status" data-phase={phase} role="status">
            <span className="status-dot" aria-hidden="true" />{phases[phase]}
          </div>
          <div className="address">
            <h2>{config?.Domain || '—'}</h2>
            {config?.Domain && (
              <button className="icon-button copy-button" type="button" aria-label="Copy address" title="Copy address"
                onClick={() => void perform(async () => {
                  await relay.copy(config.Domain);
                  setCopied(true);
                  window.setTimeout(() => setCopied(false), 1200);
                })}>
                {copied ? <IconCheck aria-hidden="true" /> : <IconCopy aria-hidden="true" />}
              </button>
            )}
          </div>
          <dl className="metrics">
            <div><dt><IconNetwork aria-hidden="true" />TCP</dt><dd>25565</dd></div>
            <div><dt><IconNetwork aria-hidden="true" />UDP</dt><dd>{config?.UDPPort || '—'}</dd></div>
            <div><dt><IconClock aria-hidden="true" />Expires</dt><dd>{deadlineOf(state?.Deadline ?? '')}</dd></div>
          </dl>
        </section>
        {problem && !settingsOpen && <p className="error" role="alert"><IconAlertCircle aria-hidden="true" />{problem}</p>}
        <div className="actions">
          {canStart && phase === 'idle' && (
            <label className="dns-option"><input type="checkbox" checked={replaceDNS} disabled={controlsLocked} onChange={(event) => setReplaceDNS(event.target.checked)} />Replace existing DNS records</label>
          )}
          <div className="action-buttons">
            <button className="button primary" type="button" disabled={!canStart || controlsLocked}
              onClick={() => void perform(async () => { await relay.start(replaceDNS); setReplaceDNS(false); })}>
              <IconPlayerPlay aria-hidden="true" />{phase === 'recoverable' ? 'Reconnect' : 'Start'}
            </button>
            {phase === 'running' && (
              <button className="button secondary" type="button" disabled={controlsLocked}
                onClick={() => void perform(() => relay.extend())}>
                <IconClockPlus aria-hidden="true" />Extend 4 h
              </button>
            )}
            <button className="button secondary" type="button" disabled={!canStop || controlsLocked}
              onClick={() => void perform(() => relay.stop())}>
              <IconPlayerStop aria-hidden="true" />Stop
            </button>
          </div>
        </div>
      </main>
      {settingsOpen && state && <SettingsDialog key={state.ActiveProfile} state={state} error={error} busy={busy} close={() => setSettingsOpen(false)} perform={perform} />}
    </div>
  );
}

function SettingsDialog({ state, close, error, busy, perform }: {
  state: RelayState; close: () => void; error: string; busy: boolean;
  perform: (action: () => Promise<void>) => Promise<void>;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [draft, setDraft] = useState(state.Config);
  const locked = state.Phase !== 'idle';
  useEffect(() => { dialog.current?.showModal(); }, []);

  function update<K extends keyof Settings>(key: K, value: Settings[K]) {
    setDraft((previous) => ({ ...previous, [key]: value }));
  }

  const login = (provider: 'google' | 'cloudflare') => void perform(async () => {
    if (!locked) await relay.saveConfig(draft);
    await relay.logIn(provider);
  });

  return (
    <dialog ref={dialog} onCancel={(event) => { if (busy) event.preventDefault(); }} onClose={close} aria-labelledby="settings-title">
      <form onSubmit={(event) => {
        event.preventDefault();
        void perform(async () => { await relay.saveConfig(draft); close(); });
      }}>
        <div className="dialog-bar">
          <h2 id="settings-title">Settings</h2>
          <button type="button" className="icon-button" aria-label="Close" disabled={busy} onClick={close}><IconX aria-hidden="true" /></button>
        </div>
        <fieldset disabled={locked || busy}>
          <legend>Address</legend>
          <label>Domain<input required placeholder="mc.example.com" value={draft.Domain} onChange={(e) => update('Domain', e.target.value)} /></label>
          <label>Cloudflare zone<input required placeholder="example.com" value={draft.CloudflareZone} onChange={(e) => update('CloudflareZone', e.target.value)} /></label>
          <div className="form-row">
            <label>Local TCP port<input type="number" min="1" max="65535" value={draft.TCPPort} onChange={(e) => update('TCPPort', Number(e.target.value))} required /></label>
            <label>UDP port<input type="number" min="1" max="65535" value={draft.UDPPort || ''} onChange={(e) => update('UDPPort', Number(e.target.value))} /></label>
            <label>Duration (hours)<input type="number" min="1" max="12" value={draft.Hours} onChange={(e) => update('Hours', Number(e.target.value))} required /></label>
          </div>
        </fieldset>
        <fieldset disabled={locked || busy}>
          <legend>Google Cloud</legend>
          <label>Project ID<input required value={draft.Project} onChange={(e) => update('Project', e.target.value)} /></label>
          <div className="form-row">
            <label>Tokyo zone<select value={draft.Zone} onChange={(e) => update('Zone', e.target.value)}>
              <option value="asia-northeast1-a">asia-northeast1-a</option>
              <option value="asia-northeast1-b">asia-northeast1-b</option>
              <option value="asia-northeast1-c">asia-northeast1-c</option>
            </select></label>
            <label>VPC network<input required value={draft.Network} onChange={(e) => update('Network', e.target.value)} /></label>
          </div>
          <label>Google OAuth client ID<input value={draft.GoogleClientID} onChange={(e) => update('GoogleClientID', e.target.value)} /></label>
          <label>Cloudflare OAuth client ID<input value={draft.CloudflareClientID} onChange={(e) => update('CloudflareClientID', e.target.value)} /></label>
        </fieldset>
        <div className="sign-in">
          <button className="button secondary" type="button" disabled={busy || !draft.GoogleClientID} onClick={() => login('google')}>
            {state.GoogleReady ? <IconCheck aria-hidden="true" /> : <IconArrowRight aria-hidden="true" />}Sign in to Google
          </button>
          <button className="button secondary" type="button" disabled={busy || !draft.CloudflareClientID} onClick={() => login('cloudflare')}>
            {state.CloudflareReady ? <IconCheck aria-hidden="true" /> : <IconArrowRight aria-hidden="true" />}Sign in to Cloudflare
          </button>
        </div>
        {error && <p className="error" role="alert"><IconAlertCircle aria-hidden="true" />{error}</p>}
        <p className="dialog-message" role="status">{busy ? 'Working…' : ''}</p>
        <button type="submit" className="button primary save" disabled={locked || busy}><IconCheck aria-hidden="true" />Save</button>
      </form>
    </dialog>
  );
}

const root = document.getElementById('root');
if (root) createRoot(root).render(<App />);
