import { Call, Clipboard } from '@wailsio/runtime';

export interface Settings {
  Project: string;
  Zone: string;
  Network: string;
  Domain: string;
  CloudflareZone: string;
  TCPPort: number;
  UDPPort: number;
  Hours: number;
  GoogleClientID: string;
  CloudflareClientID: string;
}

export type Phase = 'idle' | 'starting' | 'running' | 'stopping' | 'recoverable';

export interface RelayState {
  Config: Settings;
  Phase: Phase;
  Problem: string;
  Deadline: string;
  ActiveProfile: string;
  Profiles: { ID: string; Domain: string; Phase: Phase }[];
  GoogleReady: boolean;
  CloudflareReady: boolean;
}

type Provider = 'google' | 'cloudflare';

export interface RelayAPI {
  getState(): Promise<RelayState>;
  saveConfig(settings: Settings): Promise<void>;
  logIn(provider: Provider): Promise<void>;
  addProfile(): Promise<void>;
  selectProfile(id: string): Promise<void>;
  start(replaceDNS: boolean): Promise<void>;
  extend(): Promise<void>;
  stop(): Promise<void>;
  copy(text: string): Promise<void>;
}

declare global {
  interface Window { __relayTest?: RelayAPI }
}

const prefix = 'github.com/sya-ri/minecraft-proxy/internal/relay.Desktop.';
const real: RelayAPI = {
  getState: async () => await Call.ByName(prefix + 'GetState') as RelayState,
  saveConfig: async (settings) => { await Call.ByName(prefix + 'SaveConfig', settings); },
  logIn: async (provider) => { await Call.ByName(prefix + 'LogIn', provider); },
  addProfile: async () => { await Call.ByName(prefix + 'AddProfile'); },
  selectProfile: async (id) => { await Call.ByName(prefix + 'SelectProfile', id); },
  extend: async () => { await Call.ByName(prefix + 'Extend'); },
  start: async (replaceDNS) => { await Call.ByName(prefix + 'Start', replaceDNS); },
  stop: async () => { await Call.ByName(prefix + 'Stop'); },
  copy: async (text) => { await Clipboard.SetText(text); },
};

export const relay = (import.meta.env.DEV && window.__relayTest) || real;
