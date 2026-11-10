let accessToken = '';

type Listener = () => void;

const listeners = new Set<Listener>();

export function setAccessToken(token: string) {
  accessToken = token;
}

export function clearAccessToken() {
  accessToken = '';
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
