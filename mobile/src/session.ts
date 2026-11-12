import { documentDirectory, readAsStringAsync, writeAsStringAsync } from 'expo-file-system/legacy';

let accessToken = '';

type Listener = () => void;

const listeners = new Set<Listener>();

const sessionFile = documentDirectory == null ? null : `${documentDirectory}session.json`;

export type StoredProfile = {
  name: string;
  email: string;
  remaining?: number;
  provider: 'google' | 'apple' | null;
};

type StoredSession = StoredProfile & { token: string };

export function setAccessToken(token: string) {
  accessToken = token;
}

export function clearAccessToken() {
  accessToken = '';
  void writeSession(null);
}

export function saveCurrentSession(profile: StoredProfile) {
  if (accessToken === '') {
    return;
  }
  void writeSession({ token: accessToken, ...profile });
}

export async function loadSession(): Promise<StoredProfile | null> {
  if (sessionFile == null) {
    return null;
  }
  try {
    const raw = await readAsStringAsync(sessionFile);
    if (raw.trim() === '') {
      return null;
    }
    const parsed = JSON.parse(raw) as Partial<StoredSession>;
    if (typeof parsed.token !== 'string' || parsed.token === '') {
      return null;
    }
    accessToken = parsed.token;
    const provider = parsed.provider === 'google' || parsed.provider === 'apple' ? parsed.provider : null;
    return {
      name: typeof parsed.name === 'string' ? parsed.name : '',
      email: typeof parsed.email === 'string' ? parsed.email : '',
      remaining: typeof parsed.remaining === 'number' ? parsed.remaining : 0,
      provider,
    };
  } catch {
    accessToken = '';
    return null;
  }
}

export function authHeaders(): Record<string, string> {
  if (accessToken === '') {
    return {};
  }
  return { Authorization: `Bearer ${accessToken}` };
}

export function subscribeUnauthorized(listener: Listener) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function notifyUnauthorized() {
  clearAccessToken();
  listeners.forEach((listener) => listener());
}

async function writeSession(session: StoredSession | null) {
  if (sessionFile == null) {
    return;
  }
  try {
    await writeAsStringAsync(sessionFile, session == null ? '' : JSON.stringify(session));
  } catch {
    // Keep the in-memory token even if disk write fails.
  }
}
